package api

import "net/http"

// handleIntegrity verifies the org's reading hash chain and reports whether the
// record is intact (tamper-evident).
func (s *Server) handleIntegrity(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.VerifyReadingChain(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not verify records")
		return
	}
	writeJSON(w, http.StatusOK, st)
}
