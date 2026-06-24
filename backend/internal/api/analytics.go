package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"chillcheck/internal/store"

	"github.com/google/uuid"
)

// optionalLocationID reads ?location_id= and returns nil when absent or unparsable,
// so the analytics endpoints default to the whole org.
func optionalLocationID(r *http.Request) *uuid.UUID {
	raw := r.URL.Query().Get("location_id")
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

// handleAnalytics returns compliance metrics for the caller's org over ?from=&to=
// (defaults to the last 7 days, same parsing as the reports endpoint), optionally
// scoped to ?location_id=. Read-only — never billing-gated.
func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	from, to := dateRange(r)
	a, err := s.store.Analytics(r.Context(), orgID(r), from, to, optionalLocationID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load analytics")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleAnalyticsCSV exports the per-unit breakdown as a CSV download.
func (s *Server) handleAnalyticsCSV(w http.ResponseWriter, r *http.Request) {
	from, to := dateRange(r)
	a, err := s.store.Analytics(r.Context(), orgID(r), from, to, optionalLocationID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load analytics")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="analytics-%s-to-%s.csv"`,
			from.Format("2006-01-02"), to.Add(-1).Format("2006-01-02")))
	cw := csv.NewWriter(w)
	_ = cw.WriteAll(analyticsCSVRecords(a))
	cw.Flush()
}

// analyticsCSVRecords renders the per-unit breakdown as CSV rows (header first).
// Units with no readings in the window leave their temp/last-reading cells blank.
func analyticsCSVRecords(a store.Analytics) [][]string {
	rows := [][]string{
		{"Unit", "Location", "Readings", "In-range %", "Deviations", "Avg °F", "Min °F", "Max °F", "Last reading"},
	}
	temp := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'f', 1, 64)
	}
	for _, u := range a.Units {
		last := ""
		if u.LastReadingAt != nil {
			last = u.LastReadingAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		rows = append(rows, []string{
			u.UnitName,
			u.LocationName,
			strconv.Itoa(u.TotalReadings),
			strconv.Itoa(u.InRangePct),
			strconv.Itoa(u.Deviations),
			temp(u.AvgTempF),
			temp(u.MinTempF),
			temp(u.MaxTempF),
			last,
		})
	}
	return rows
}
