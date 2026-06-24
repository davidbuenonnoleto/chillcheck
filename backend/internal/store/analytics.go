package store

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

type LocationStat struct {
	LocationID           uuid.UUID `json:"location_id"`
	LocationName         string    `json:"location_name"`
	Units                int       `json:"units"`
	Readings             int       `json:"readings"`
	ExpectedLogs         int       `json:"expected_logs"`
	CompliancePct        int       `json:"compliance_pct"`
	OutOfRange           int       `json:"out_of_range"`
	Deviations           int       `json:"deviations"`
	DocumentedDeviations int       `json:"documented_deviations"`
}

type Analytics struct {
	From                 time.Time      `json:"from"`
	To                   time.Time      `json:"to"`
	Locations            []LocationStat `json:"locations"`
	Readings             int            `json:"readings"`
	ExpectedLogs         int            `json:"expected_logs"`
	CompliancePct        int            `json:"compliance_pct"`
	OutOfRange           int            `json:"out_of_range"`
	Deviations           int            `json:"deviations"`
	DocumentedDeviations int            `json:"documented_deviations"`
	DocumentationPct     int            `json:"documentation_pct"`
}

// pct returns round(100*num/den), capped at 100. An empty denominator (no units
// to log, or no deviations to document) reads as 100% — nothing was missed.
func pct(num, den int) int {
	if den <= 0 {
		return 100
	}
	v := int(math.Round(100 * float64(num) / float64(den)))
	if v > 100 {
		v = 100
	}
	return v
}

// Analytics computes per-location compliance metrics over [from,to): how much
// logging actually happened vs. expected, out-of-range readings, deviations, and
// how many deviations were documented with a corrective action.
func (s *Store) Analytics(ctx context.Context, orgID uuid.UUID, from, to time.Time) (Analytics, error) {
	out := Analytics{From: from, To: to, Locations: []LocationStat{}}

	// Seed from the full location list so quiet locations still appear.
	stats := map[uuid.UUID]*LocationStat{}
	order := []uuid.UUID{}
	rows, err := s.pool.Query(ctx, `SELECT id, name FROM locations WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return Analytics{}, err
	}
	for rows.Next() {
		var st LocationStat
		if err := rows.Scan(&st.LocationID, &st.LocationName); err != nil {
			rows.Close()
			return Analytics{}, err
		}
		cp := st
		stats[st.LocationID] = &cp
		order = append(order, st.LocationID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Analytics{}, err
	}

	// Units + expected log count (window minutes / each unit's interval).
	uRows, err := s.pool.Query(ctx,
		`SELECT location_id, count(*),
		        COALESCE(SUM((EXTRACT(EPOCH FROM ($3::timestamptz - $2::timestamptz)) / 60.0)
		                     / NULLIF(log_interval_minutes, 0)), 0)
		 FROM units WHERE org_id = $1 GROUP BY location_id`, orgID, from, to)
	if err != nil {
		return Analytics{}, err
	}
	for uRows.Next() {
		var locID uuid.UUID
		var units int
		var expected float64
		if err := uRows.Scan(&locID, &units, &expected); err != nil {
			uRows.Close()
			return Analytics{}, err
		}
		if st := stats[locID]; st != nil {
			st.Units = units
			st.ExpectedLogs = int(math.Round(expected))
		}
	}
	uRows.Close()
	if err := uRows.Err(); err != nil {
		return Analytics{}, err
	}

	// Readings + out-of-range in the window.
	rRows, err := s.pool.Query(ctx,
		`SELECT u.location_id, count(*),
		        count(*) FILTER (WHERE r.temp_f < u.min_temp_f OR r.temp_f > u.max_temp_f)
		 FROM readings r JOIN units u ON u.id = r.unit_id
		 WHERE r.org_id = $1 AND r.recorded_at >= $2 AND r.recorded_at < $3
		 GROUP BY u.location_id`, orgID, from, to)
	if err != nil {
		return Analytics{}, err
	}
	for rRows.Next() {
		var locID uuid.UUID
		var readings, oor int
		if err := rRows.Scan(&locID, &readings, &oor); err != nil {
			rRows.Close()
			return Analytics{}, err
		}
		if st := stats[locID]; st != nil {
			st.Readings = readings
			st.OutOfRange = oor
		}
	}
	rRows.Close()
	if err := rRows.Err(); err != nil {
		return Analytics{}, err
	}

	// Deviations (alerts opened in window) + how many are documented.
	aRows, err := s.pool.Query(ctx,
		`SELECT u.location_id, count(*),
		        count(*) FILTER (WHERE EXISTS (SELECT 1 FROM corrective_actions ca WHERE ca.alert_id = a.id))
		 FROM alerts a JOIN units u ON u.id = a.unit_id
		 WHERE a.org_id = $1 AND a.opened_at >= $2 AND a.opened_at < $3
		 GROUP BY u.location_id`, orgID, from, to)
	if err != nil {
		return Analytics{}, err
	}
	for aRows.Next() {
		var locID uuid.UUID
		var dev, doc int
		if err := aRows.Scan(&locID, &dev, &doc); err != nil {
			aRows.Close()
			return Analytics{}, err
		}
		if st := stats[locID]; st != nil {
			st.Deviations = dev
			st.DocumentedDeviations = doc
		}
	}
	aRows.Close()
	if err := aRows.Err(); err != nil {
		return Analytics{}, err
	}

	// Finalize per-location compliance and roll up org totals.
	for _, id := range order {
		st := stats[id]
		st.CompliancePct = pct(st.Readings, st.ExpectedLogs)
		out.Locations = append(out.Locations, *st)
		out.Readings += st.Readings
		out.ExpectedLogs += st.ExpectedLogs
		out.OutOfRange += st.OutOfRange
		out.Deviations += st.Deviations
		out.DocumentedDeviations += st.DocumentedDeviations
	}
	out.CompliancePct = pct(out.Readings, out.ExpectedLogs)
	out.DocumentationPct = pct(out.DocumentedDeviations, out.Deviations)
	return out, nil
}
