package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"chillcheck/internal/auth"
	"chillcheck/internal/store"

	"github.com/google/uuid"
)

// ---------- gateway auth ----------

func (s *Server) requireGateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Gateway-Key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if key == "" {
			writeErr(w, http.StatusUnauthorized, "missing gateway key")
			return
		}
		gw, err := s.store.GatewayByKeyHash(r.Context(), auth.HashGatewayKey(key))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid gateway key")
			return
		}
		// Best-effort liveness update; don't block ingest on it.
		go func(id uuid.UUID) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.store.TouchGateway(ctx, id)
		}(gw.ID)

		ctx := context.WithValue(r.Context(), ctxOrgID, gw.OrgID)
		ctx = context.WithValue(ctx, ctxLocationID, gw.LocationID)
		ctx = context.WithValue(ctx, ctxGatewayID, gw.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func locationIDFromCtx(r *http.Request) uuid.UUID {
	return r.Context().Value(ctxLocationID).(uuid.UUID)
}

// ---------- ingest ----------

type ingestReading struct {
	MAC        string  `json:"mac"`
	TempF      float64 `json:"temp_f"`
	RecordedAt string  `json:"recorded_at"` // RFC3339; empty = now
}

type ingestReq struct {
	Readings []ingestReading `json:"readings"`
}

func (s *Server) handleIngestReadings(w http.ResponseWriter, r *http.Request) {
	var req ingestReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	org := orgID(r)
	now := time.Now()

	accepted, ignored := 0, 0
	for _, rd := range req.Readings {
		mac := normalizeMAC(rd.MAC)
		if mac == "" {
			ignored++
			continue
		}
		unit, err := s.store.UnitByMAC(r.Context(), org, mac)
		if errors.Is(err, store.ErrNotFound) {
			ignored++ // sensor not bound to any unit yet
			continue
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		at := now
		if rd.RecordedAt != "" {
			if parsed, perr := time.Parse(time.RFC3339, rd.RecordedAt); perr == nil {
				at = parsed
			}
		}
		if at.After(now) {
			at = now // never store a future timestamp
		}
		if err := s.store.CreateSensorReading(r.Context(), org, unit.ID, rd.TempF, at); err != nil {
			writeErr(w, http.StatusInternalServerError, "could not store reading")
			return
		}
		accepted++
	}
	writeJSON(w, http.StatusOK, map[string]int{"accepted": accepted, "ignored": ignored})
}

// ---------- gateway administration ----------

type createGatewayReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateGateway(w http.ResponseWriter, r *http.Request) {
	locID, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	if _, err := s.store.GetLocation(r.Context(), orgID(r), locID); err != nil {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	var req createGatewayReq
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a gateway name is required")
		return
	}
	key, hash, prefix, err := auth.GenerateGatewayKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not generate key")
		return
	}
	gw, err := s.store.CreateGateway(r.Context(), orgID(r), locID, req.Name, hash, prefix)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create gateway")
		return
	}
	// The plaintext key is returned exactly once; it is never stored or shown again.
	writeJSON(w, http.StatusCreated, map[string]any{"gateway": gw, "key": key})
}

func (s *Server) handleListGateways(w http.ResponseWriter, r *http.Request) {
	locID, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	gws, err := s.store.ListGateways(r.Context(), orgID(r), locID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load gateways")
		return
	}
	writeJSON(w, http.StatusOK, gws)
}

type setSensorReq struct {
	MAC string `json:"mac"` // empty to unbind
}

func (s *Server) handleSetUnitSensor(w http.ResponseWriter, r *http.Request) {
	unitID, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid unit id")
		return
	}
	var req setSensorReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	mac := normalizeMAC(req.MAC)
	if req.MAC != "" && mac == "" {
		writeErr(w, http.StatusBadRequest, "that doesn't look like a MAC address")
		return
	}
	unit, err := s.store.SetUnitSensorMAC(r.Context(), orgID(r), unitID, mac)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "unit not found")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "idx_units_org_mac") {
			writeErr(w, http.StatusConflict, "that sensor is already bound to another unit")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not bind sensor")
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

// ---------- helpers ----------

var hexOnly = regexp.MustCompile(`[^0-9A-Fa-f]`)

// normalizeMAC accepts "a4:c1:38:aa:bb:cc", "A4-C1-38-AA-BB-CC", or "a4c138aabbcc"
// and returns "A4:C1:38:AA:BB:CC". Returns "" if it isn't 12 hex digits.
func normalizeMAC(s string) string {
	h := strings.ToUpper(hexOnly.ReplaceAllString(s, ""))
	if len(h) != 12 {
		return ""
	}
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = h[i*2 : i*2+2]
	}
	return strings.Join(parts, ":")
}
