package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chillcheck/internal/auth"
	"chillcheck/internal/store"
)

// ---------- team / users ----------

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load team")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type createInviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req createInviteReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		writeErr(w, http.StatusBadRequest, "an email is required")
		return
	}
	role := req.Role
	if role != "admin" {
		role = "staff"
	}

	token, err := auth.RandomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	inv, err := s.store.CreateInvite(r.Context(), orgID(r), email, role, auth.HashToken(token), admin.ID, time.Now().Add(7*24*time.Hour))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create invite")
		return
	}

	acceptURL := s.cfg.AppBaseURL + "/accept-invite?token=" + token
	org, _ := s.store.OrgByID(r.Context(), orgID(r))
	body := fmt.Sprintf("%s invited you to join %s on ChillCheck.\n\nSet up your account:\n%s\n\nThis link expires in 7 days.",
		admin.Name, org.Name, acceptURL)
	go func(to string) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = s.mailer.Send(ctx, []string{to}, "You're invited to ChillCheck", body)
	}(email)

	// accept_url is returned so an admin can share the link directly when email
	// isn't configured.
	writeJSON(w, http.StatusCreated, map[string]any{"invite": inv, "accept_url": acceptURL})
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	invites, err := s.store.ListInvites(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load invites")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid invite id")
		return
	}
	if err := s.store.DeleteInvite(r.Context(), orgID(r), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not revoke invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- public: accept invite ----------

func (s *Server) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeErr(w, http.StatusBadRequest, "missing token")
		return
	}
	inv, err := s.store.InviteByTokenHash(r.Context(), auth.HashToken(token))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invite is invalid or has expired")
		return
	}
	org, err := s.store.OrgByID(r.Context(), inv.OrgID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"org_name": org.Name,
		"email":    inv.Email,
		"role":     inv.Role,
	})
}

type acceptInviteReq struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "name and an 8+ character password are required")
		return
	}
	inv, err := s.store.InviteByTokenHash(r.Context(), auth.HashToken(req.Token))
	if err != nil {
		writeErr(w, http.StatusNotFound, "this invite is invalid or has expired")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	user, err := s.store.AcceptInvite(r.Context(), inv.ID, inv.OrgID, inv.Email, req.Name, inv.Role, hash)
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") {
			writeErr(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	tok, err := auth.IssueToken(s.cfg.JWTSecret, user.ID, user.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusCreated, authResp{Token: tok, User: user})
}

// ---------- public: password reset ----------

type forgotReq struct {
	Email string `json:"email"`
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Always respond 200 so we never reveal whether an email is registered.
	if user, _, err := s.store.UserByEmail(r.Context(), email); err == nil {
		if token, terr := auth.RandomToken(); terr == nil {
			if cerr := s.store.CreatePasswordReset(r.Context(), user.ID, auth.HashToken(token), time.Now().Add(time.Hour)); cerr == nil {
				resetURL := s.cfg.AppBaseURL + "/reset-password?token=" + token
				body := fmt.Sprintf("Reset your ChillCheck password:\n%s\n\nThis link expires in 1 hour. If you didn't request this, you can ignore this email.", resetURL)
				go func(to string) {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_ = s.mailer.Send(ctx, []string{to}, "Reset your ChillCheck password", body)
				}(email)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type resetReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	resetID, userID, err := s.store.PasswordResetUserByToken(r.Context(), auth.HashToken(req.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusBadRequest, "this reset link is invalid or has expired")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	if err := s.store.ResetPassword(r.Context(), resetID, userID, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
