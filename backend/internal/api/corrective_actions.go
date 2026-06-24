package api

import (
	"net/http"
)

var validActions = map[string]bool{
	"adjusted_equipment": true,
	"relocated_product":  true,
	"discarded_product":  true,
	"other":              true,
}

var validDispositions = map[string]bool{
	"not_affected": true,
	"relocated":    true,
	"discarded":    true,
}

func (s *Server) handleListCorrectiveActions(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid alert id")
		return
	}
	actions, err := s.store.ListCorrectiveActionsForAlert(r.Context(), orgID(r), alertID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load corrective actions")
		return
	}
	writeJSON(w, http.StatusOK, actions)
}

type createCorrectiveActionReq struct {
	Action      string `json:"action"`
	Disposition string `json:"disposition"`
	Note        string `json:"note"`
}

func (s *Server) handleCreateCorrectiveAction(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid alert id")
		return
	}
	// Confirm the alert exists and belongs to the caller's org.
	if _, err := s.store.AlertByID(r.Context(), orgID(r), alertID); err != nil {
		writeErr(w, http.StatusNotFound, "alert not found")
		return
	}

	var req createCorrectiveActionReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validActions[req.Action] {
		writeErr(w, http.StatusBadRequest, "action must be one of: adjusted_equipment, relocated_product, discarded_product, other")
		return
	}
	disposition := req.Disposition
	if disposition == "" {
		disposition = "not_affected"
	}
	if !validDispositions[disposition] {
		writeErr(w, http.StatusBadRequest, "disposition must be one of: not_affected, relocated, discarded")
		return
	}

	user, err := s.currentUser(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "user not found")
		return
	}
	ca, err := s.store.CreateCorrectiveAction(r.Context(), orgID(r), alertID, req.Action, disposition, req.Note, user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not record corrective action")
		return
	}
	ca.RecordedByName = user.Name
	writeJSON(w, http.StatusCreated, ca)
}

// correctiveActionLabel renders an action/disposition code for the PDF report.
func correctiveActionLabel(code string) string {
	labels := map[string]string{
		"adjusted_equipment": "Adjusted / repaired equipment",
		"relocated_product":  "Relocated product",
		"discarded_product":  "Discarded product",
		"other":              "Other",
		"not_affected":       "Product not affected",
		"relocated":          "Product relocated",
		"discarded":          "Product discarded",
	}
	if l, ok := labels[code]; ok {
		return l
	}
	return code
}
