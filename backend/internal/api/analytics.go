package api

import "net/http"

// handleAnalytics returns per-location compliance metrics for the caller's org
// over ?from=&to= (defaults to the last 7 days, same parsing as the reports
// endpoint). Read-only — never billing-gated.
func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	from, to := dateRange(r)
	a, err := s.store.Analytics(r.Context(), orgID(r), from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load analytics")
		return
	}
	writeJSON(w, http.StatusOK, a)
}
