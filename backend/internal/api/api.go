package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"chillcheck/internal/auth"
	"chillcheck/internal/billing"
	"chillcheck/internal/config"
	"chillcheck/internal/email"
	"chillcheck/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

type Server struct {
	store   *store.Store
	cfg     config.Config
	mailer  email.Mailer
	billing *billing.Client // nil when billing is not configured
}

func NewServer(st *store.Store, cfg config.Config, mailer email.Mailer) *Server {
	s := &Server{store: st, cfg: cfg, mailer: mailer}
	if cfg.BillingEnabled() {
		s.billing = billing.NewClient(cfg.StripeSecretKey, cfg.StripePriceID, cfg.StripeWebhookSecret)
	}
	return s
}

type ctxKey string

const (
	ctxUserID     ctxKey = "user_id"
	ctxOrgID      ctxKey = "org_id"
	ctxLocationID ctxKey = "location_id"
	ctxGatewayID  ctxKey = "gateway_id"
)

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{s.cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Content-Disposition"}, // so the browser can read the CSV download filename
		AllowCredentials: false,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Throttle unauthenticated credential endpoints against brute force.
	authThrottle := newRateLimiter(10, time.Minute).middleware

	r.Route("/api", func(r chi.Router) {
		// public (credential endpoints are rate-limited per client IP)
		r.With(authThrottle).Post("/auth/register", s.handleRegister)
		r.With(authThrottle).Post("/auth/login", s.handleLogin)
		r.With(authThrottle).Post("/auth/forgot", s.handleForgotPassword)
		r.With(authThrottle).Post("/auth/reset", s.handleResetPassword)
		r.Get("/invites/lookup", s.handleGetInvite)
		r.With(authThrottle).Post("/invites/accept", s.handleAcceptInvite)
		r.Post("/webhooks/stripe", s.handleStripeWebhook)

		// authenticated
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/me", s.handleMe)

			r.Get("/locations", s.handleListLocations)
			r.Post("/locations", s.handleCreateLocation)
			r.Get("/locations/{id}", s.handleGetLocation)
			r.Get("/locations/{id}/units", s.handleListUnits)
			r.Post("/locations/{id}/units", s.handleCreateUnit)
			r.Get("/locations/{id}/status", s.handleLocationStatus)
			r.Get("/locations/{id}/alerts", s.handleListAlerts)

			r.Get("/alerts/{id}/corrective-actions", s.handleListCorrectiveActions)
			r.Post("/alerts/{id}/corrective-actions", s.handleCreateCorrectiveAction)

			r.Post("/readings", s.handleCreateReading)
			r.Get("/readings", s.handleListReadings)

			r.Get("/reports/compliance.pdf", s.handleComplianceReport)
			r.Get("/integrity", s.handleIntegrity)
			r.Get("/analytics", s.handleAnalytics)
			r.Get("/analytics/export.csv", s.handleAnalyticsCSV)

			// gateway + sensor administration (manager-facing)
			r.Get("/locations/{id}/gateways", s.handleListGateways)
			r.Post("/locations/{id}/gateways", s.handleCreateGateway)
			r.Put("/units/{id}/sensor", s.handleSetUnitSensor)

			// billing
			r.Get("/billing", s.handleBillingStatus)
			r.Post("/billing/checkout", s.handleBillingCheckout)
			r.Post("/billing/portal", s.handleBillingPortal)

			// team
			r.Get("/users", s.handleListUsers)
			r.Get("/invites", s.handleListInvites)
			r.Post("/invites", s.handleCreateInvite)
			r.Delete("/invites/{id}", s.handleRevokeInvite)
		})

		// sensor ingest, authenticated by a gateway key (not a user token)
		r.Group(func(r chi.Router) {
			r.Use(s.requireGateway)
			r.Post("/ingest/readings", s.handleIngestReadings)
		})
	})

	return r
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" || token == header {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := auth.ParseToken(s.cfg.JWTSecret, token)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		ctx = context.WithValue(ctx, ctxOrgID, claims.OrgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userID(r *http.Request) uuid.UUID { return r.Context().Value(ctxUserID).(uuid.UUID) }
func orgID(r *http.Request) uuid.UUID  { return r.Context().Value(ctxOrgID).(uuid.UUID) }

func (s *Server) currentUser(r *http.Request) (store.User, error) {
	return s.store.UserByID(r.Context(), userID(r))
}

// requireAdmin loads the caller and ensures they're an admin, writing 403 if not.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, err := s.currentUser(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "user not found")
		return store.User{}, false
	}
	if u.Role != "admin" {
		writeErr(w, http.StatusForbidden, "admin access required")
		return store.User{}, false
	}
	return u, true
}

// securityHeaders sets defensive response headers. The API serves only JSON, so
// a locked-down CSP here is defense-in-depth (the SPA document carries its own
// CSP, injected at build time — see frontend/vite.config.ts). These headers
// cost nothing and harden error pages / any non-JSON response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func parseUUID(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}
