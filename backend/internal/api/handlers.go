package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"chillcheck/internal/auth"
	"chillcheck/internal/store"
)

// ---------- auth ----------

type registerReq struct {
	OrgName  string `json:"org_name"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResp struct {
	Token string     `json:"token"`
	User  store.User `json:"user"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.OrgName == "" || req.Name == "" || req.Email == "" || len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "org_name, name, email and an 8+ char password are required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, _, err := s.store.CreateOrgWithAdmin(r.Context(), req.OrgName, req.Name, req.Email, hash)
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") {
			writeErr(w, http.StatusConflict, "that email is already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not create account")
		return
	}
	token, err := auth.IssueToken(s.cfg.JWTSecret, user.ID, user.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusCreated, authResp{Token: token, User: user})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	user, hash, err := s.store.UserByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "incorrect email or password")
		return
	}
	token, err := auth.IssueToken(s.cfg.JWTSecret, user.ID, user.OrgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, authResp{Token: token, User: user})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.store.UserByID(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	org, err := s.store.OrgByID(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "organization": org})
}

// ---------- locations ----------

func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := s.store.ListLocations(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load locations")
		return
	}
	writeJSON(w, http.StatusOK, locs)
}

type createLocationReq struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

func (s *Server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	if !s.requireEntitled(w, r) {
		return
	}
	var req createLocationReq
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a location name is required")
		return
	}
	loc, err := s.store.CreateLocation(r.Context(), orgID(r), req.Name, req.Timezone)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create location")
		return
	}
	go s.syncBillingQuantity(orgID(r)) // keep per-location pricing in sync
	writeJSON(w, http.StatusCreated, loc)
}

func (s *Server) handleGetLocation(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	loc, err := s.store.GetLocation(r.Context(), orgID(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load location")
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

// ---------- units ----------

func (s *Server) handleListUnits(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	units, err := s.store.ListUnits(r.Context(), orgID(r), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load units")
		return
	}
	writeJSON(w, http.StatusOK, units)
}

type createUnitReq struct {
	Name               string  `json:"name"`
	Kind               string  `json:"kind"`
	MinTempF           float64 `json:"min_temp_f"`
	MaxTempF           float64 `json:"max_temp_f"`
	LogIntervalMinutes int     `json:"log_interval_minutes"`
}

func (s *Server) handleCreateUnit(w http.ResponseWriter, r *http.Request) {
	if !s.requireEntitled(w, r) {
		return
	}
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	if _, err := s.store.GetLocation(r.Context(), orgID(r), id); err != nil {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	var req createUnitReq
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "a unit name is required")
		return
	}
	if req.MaxTempF <= req.MinTempF {
		writeErr(w, http.StatusBadRequest, "max temp must be above min temp")
		return
	}
	if req.Kind == "" {
		req.Kind = "fridge"
	}
	unit, err := s.store.CreateUnit(r.Context(), orgID(r), id, req.Name, req.Kind, req.MinTempF, req.MaxTempF, req.LogIntervalMinutes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create unit")
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

// ---------- readings ----------

type createReadingReq struct {
	UnitID string  `json:"unit_id"`
	TempF  float64 `json:"temp_f"`
	Note   string  `json:"note"`
}

func (s *Server) handleCreateReading(w http.ResponseWriter, r *http.Request) {
	var req createReadingReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	unitID, err := parseUUIDString(req.UnitID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid unit id")
		return
	}
	reading, err := s.store.CreateReading(r.Context(), orgID(r), unitID, userID(r), req.TempF, strings.TrimSpace(req.Note))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "unit not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save reading")
		return
	}
	writeJSON(w, http.StatusCreated, reading)
}

func (s *Server) handleListReadings(w http.ResponseWriter, r *http.Request) {
	locID, err := parseUUIDString(r.URL.Query().Get("location_id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "location_id is required")
		return
	}
	from, to := dateRange(r)
	readings, err := s.store.ListReadings(r.Context(), orgID(r), locID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load readings")
		return
	}
	writeJSON(w, http.StatusOK, readings)
}

// ---------- dashboard status ----------

type unitStatus struct {
	store.Unit
	LatestTempF  *float64   `json:"latest_temp_f"`
	LatestAt     *time.Time `json:"latest_at"`
	LatestBy     *string    `json:"latest_by"`
	Status       string     `json:"status"` // ok | out_of_range | overdue | no_data
}

func (s *Server) handleLocationStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	loc, err := s.store.GetLocation(r.Context(), orgID(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load location")
		return
	}
	units, err := s.store.ListUnits(r.Context(), orgID(r), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load units")
		return
	}
	latest, err := s.store.LatestByLocation(r.Context(), orgID(r), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load readings")
		return
	}

	now := time.Now()
	statuses := make([]unitStatus, 0, len(units))
	for _, u := range units {
		us := unitStatus{Unit: u, Status: "no_data"}
		if lr, ok := latest[u.ID]; ok {
			t := lr.TempF
			at := lr.RecordedAt
			us.LatestTempF = &t
			us.LatestAt = &at
			us.LatestBy = lr.By
		}
		us.Status = store.ComputeStatus(u.MinTempF, u.MaxTempF, u.LogIntervalMinutes, us.LatestTempF, us.LatestAt, now)
		statuses = append(statuses, us)
	}
	writeJSON(w, http.StatusOK, map[string]any{"location": loc, "units": statuses})
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid location id")
		return
	}
	if _, err := s.store.GetLocation(r.Context(), orgID(r), id); err != nil {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	alerts, err := s.store.ListRecentAlerts(r.Context(), orgID(r), id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load alerts")
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}
