package api

import (
	"fmt"
	"net/http"
	"time"

	"chillcheck/internal/store"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

func parseUUIDString(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// dateRange reads ?from=YYYY-MM-DD&to=YYYY-MM-DD (RFC3339 also accepted).
// `to` is made inclusive of the whole day. Defaults to the last 7 days.
func dateRange(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	to := parseDate(q.Get("to"), time.Now())
	if q.Get("to") != "" {
		to = to.Add(24 * time.Hour) // include the whole "to" day
	}
	from := parseDate(q.Get("from"), to.Add(-7*24*time.Hour))
	return from, to
}

func parseDate(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return fallback
}

func (s *Server) handleComplianceReport(w http.ResponseWriter, r *http.Request) {
	locID, err := parseUUIDString(r.URL.Query().Get("location_id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "location_id is required")
		return
	}
	loc, err := s.store.GetLocation(r.Context(), orgID(r), locID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "location not found")
		return
	}
	org, _ := s.store.OrgByID(r.Context(), orgID(r))
	from, to := dateRange(r)

	units, err := s.store.ListUnits(r.Context(), orgID(r), locID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load units")
		return
	}
	unitByID := map[uuid.UUID]store.Unit{}
	for _, u := range units {
		unitByID[u.ID] = u
	}

	readings, err := s.store.ListReadings(r.Context(), orgID(r), locID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load readings")
		return
	}

	// Deviations (alerts) in the window and their documented corrective actions.
	alerts, err := s.store.AlertsForLocationWindow(r.Context(), orgID(r), locID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load deviations")
		return
	}
	actions, err := s.store.CorrectiveActionsForLocationWindow(r.Context(), orgID(r), locID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load corrective actions")
		return
	}
	actionsByAlert := map[uuid.UUID][]store.CorrectiveAction{}
	for _, a := range actions {
		actionsByAlert[a.AlertID] = append(actionsByAlert[a.AlertID], a)
	}

	chain, err := s.store.VerifyReadingChain(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not verify records")
		return
	}

	pdf := buildCompliancePDF(org.Name, loc, from, to, unitByID, readings, alerts, actionsByAlert, chain)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`inline; filename="compliance-%s-%s.pdf"`, sanitize(loc.Name), from.Format("2006-01-02")))
	if err := pdf.Output(w); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not render PDF")
	}
}

func buildCompliancePDF(orgName string, loc store.Location, from, to time.Time, unitByID map[uuid.UUID]store.Unit, readings []store.Reading, alerts []store.Alert, actionsByAlert map[uuid.UUID][]store.CorrectiveAction, chain store.ChainStatus) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 8, "Temperature compliance log", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, orgName+"  -  "+loc.Name, "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Period: %s to %s", from.Format("Jan 2, 2006"), to.Add(-time.Second).Format("Jan 2, 2006")), "", 1, "L", false, 0, "")

	// Tamper-evidence statement
	pdf.SetFont("Helvetica", "B", 9)
	if chain.OK {
		pdf.SetTextColor(30, 120, 60)
		pdf.CellFormat(0, 6, fmt.Sprintf("Record integrity: VERIFIED - %d readings, hash chain intact", chain.Count), "", 1, "L", false, 0, "")
	} else {
		pdf.SetTextColor(190, 40, 40)
		brokenAt := int64(0)
		if chain.BrokenAtSeq != nil {
			brokenAt = *chain.BrokenAtSeq
		}
		pdf.CellFormat(0, 6, fmt.Sprintf("Record integrity: FAILED - chain broken at entry #%d (records may have been altered)", brokenAt), "", 1, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(3)

	// Table header
	widths := []float64{38, 42, 16, 30, 24, 36}
	headers := []string{"Date / time", "Unit", "Temp F", "Safe range", "Status", "Logged by"}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(235, 238, 241)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	if len(readings) == 0 {
		pdf.CellFormat(0, 7, "No readings recorded for this period.", "1", 1, "L", false, 0, "")
	}
	// readings come newest-first; print oldest-first for a log
	for i := len(readings) - 1; i >= 0; i-- {
		rd := readings[i]
		u := unitByID[rd.UnitID]
		safe := fmt.Sprintf("%.0f - %.0f", u.MinTempF, u.MaxTempF)
		status := "OK"
		if rd.TempF < u.MinTempF || rd.TempF > u.MaxTempF {
			status = "Out of range"
		}
		by := "-"
		if rd.RecordedBy != nil {
			by = *rd.RecordedBy
		}
		row := []string{
			rd.RecordedAt.Format("Jan 2 15:04"),
			truncate(rd.UnitName, 24),
			fmt.Sprintf("%.1f", rd.TempF),
			safe,
			status,
			truncate(by, 20),
		}
		for j, c := range row {
			pdf.CellFormat(widths[j], 6.5, c, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	renderDeviations(pdf, alerts, actionsByAlert)

	pdf.Ln(4)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.CellFormat(0, 5, "Generated by ChillCheck on "+time.Now().Format("Jan 2, 2006 15:04 MST"), "", 1, "L", false, 0, "")
	return pdf
}

// renderDeviations prints each deviation in the window with its documented
// corrective actions (or a clear "no corrective action recorded" flag, which is
// itself meaningful to an inspector).
func renderDeviations(pdf *fpdf.Fpdf, alerts []store.Alert, actionsByAlert map[uuid.UUID][]store.CorrectiveAction) {
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 7, "Deviations & corrective actions", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)

	if len(alerts) == 0 {
		pdf.CellFormat(0, 6, "No deviations recorded for this period.", "", 1, "L", false, 0, "")
		return
	}

	kindLabel := map[string]string{"out_of_range": "Out of range", "overdue": "Logging overdue"}
	for _, a := range alerts {
		k := kindLabel[a.Kind]
		if k == "" {
			k = a.Kind
		}
		state := "resolved"
		if a.ResolvedAt == nil {
			state = "open"
		}
		pdf.Ln(1)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.MultiCell(0, 5, fmt.Sprintf("%s  -  %s  -  %s (%s)",
			a.OpenedAt.Format("Jan 2 15:04"), truncate(a.UnitName, 30), k, state), "", "L", false)

		pdf.SetFont("Helvetica", "", 9)
		acts := actionsByAlert[a.ID]
		if len(acts) == 0 {
			pdf.SetTextColor(170, 60, 60)
			pdf.MultiCell(0, 5, "    No corrective action recorded.", "", "L", false)
			pdf.SetTextColor(0, 0, 0)
			continue
		}
		for _, ca := range acts {
			line := fmt.Sprintf("    %s - %s - %s",
				correctiveActionLabel(ca.Action), correctiveActionLabel(ca.Disposition), ca.RecordedByName)
			if ca.Note != "" {
				line += " - \"" + ca.Note + "\""
			}
			line += "  (" + ca.CreatedAt.Format("Jan 2 15:04") + ")"
			pdf.MultiCell(0, 5, line, "", "L", false)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "."
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "location"
	}
	return string(out)
}
