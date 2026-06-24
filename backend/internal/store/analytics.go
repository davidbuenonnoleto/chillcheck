package store

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

// UnitStat is one unit's compliance picture over the analytics window.
type UnitStat struct {
	UnitID        uuid.UUID  `json:"unit_id"`
	UnitName      string     `json:"unit_name"`
	LocationName  string     `json:"location_name"`
	TotalReadings int        `json:"total_readings"`
	InRangePct    int        `json:"in_range_pct"`
	Deviations    int        `json:"deviations"`
	AvgTempF      *float64   `json:"avg_temp_f"`
	MinTempF      *float64   `json:"min_temp_f"`
	MaxTempF      *float64   `json:"max_temp_f"`
	LastReadingAt *time.Time `json:"last_reading_at"`
}

// Analytics is the org-wide (or single-location) analytics summary over [from,to).
type Analytics struct {
	From                   time.Time     `json:"from"`
	To                     time.Time     `json:"to"`
	TotalReadings          int           `json:"total_readings"`
	InRangePct             int           `json:"in_range_pct"`
	Deviations             int           `json:"deviations"`
	OverdueEvents          int           `json:"overdue_events"`
	UndocumentedDeviations int           `json:"undocumented_deviations"`
	Trend                  []TrendBucket `json:"trend"`
	Units                  []UnitStat    `json:"units"`
}

// pct returns round(100*num/den), capped at 100. An empty denominator (no
// readings) reads as 0% — there's no compliance to claim without data.
func pct(num, den int) int {
	if den <= 0 {
		return 0
	}
	v := int(math.Round(100 * float64(num) / float64(den)))
	if v > 100 {
		v = 100
	}
	return v
}

// TrendBucket is one day of in-range vs. total readings (UTC day).
type TrendBucket struct {
	Date    string `json:"date"`
	InRange int    `json:"in_range"`
	Total   int    `json:"total"`
	Pct     int    `json:"pct"`
}

type dayCount struct{ InRange, Total int }

// buildTrend produces one bucket per UTC calendar day in [from, to), filling
// days with no readings as zeroes so the chart has a continuous x-axis.
func buildTrend(from, to time.Time, counts map[string]dayCount) []TrendBucket {
	out := []TrendBucket{}
	day := from.UTC().Truncate(24 * time.Hour)
	for day.Before(to) {
		key := day.Format("2006-01-02")
		c := counts[key]
		out = append(out, TrendBucket{
			Date:    key,
			InRange: c.InRange,
			Total:   c.Total,
			Pct:     pct(c.InRange, c.Total),
		})
		day = day.Add(24 * time.Hour)
	}
	return out
}

// Analytics computes compliance metrics over [from,to): per-unit % of readings in
// range, deviations (alerts opened), overdue events, undocumented deviations, and a
// daily in-range trend. Pass a non-nil locationID to scope to one location; nil
// covers the whole org. Compliance is "% of readings in range" — no interpolation.
func (s *Store) Analytics(ctx context.Context, orgID uuid.UUID, from, to time.Time, locationID *uuid.UUID) (Analytics, error) {
	out := Analytics{From: from, To: to, Trend: []TrendBucket{}, Units: []UnitStat{}}

	var locArg interface{}
	if locationID != nil {
		locArg = *locationID
	}

	// Per-unit readings rollup (LEFT JOIN so quiet units still appear).
	units := map[uuid.UUID]*UnitStat{}
	order := []uuid.UUID{}
	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.name, l.name,
		        count(r.id) AS total,
		        count(r.id) FILTER (WHERE r.temp_f BETWEEN u.min_temp_f AND u.max_temp_f) AS in_range,
		        avg(r.temp_f), min(r.temp_f), max(r.temp_f), max(r.recorded_at)
		 FROM units u
		 JOIN locations l ON l.id = u.location_id
		 LEFT JOIN readings r ON r.unit_id = u.id AND r.recorded_at >= $2 AND r.recorded_at < $3
		 WHERE u.org_id = $1 AND ($4::uuid IS NULL OR u.location_id = $4)
		 GROUP BY u.id, u.name, l.name
		 ORDER BY l.name, u.name`, orgID, from, to, locArg)
	if err != nil {
		return Analytics{}, err
	}
	totalReadings, totalInRange := 0, 0
	for rows.Next() {
		var st UnitStat
		var inRange int
		if err := rows.Scan(&st.UnitID, &st.UnitName, &st.LocationName,
			&st.TotalReadings, &inRange, &st.AvgTempF, &st.MinTempF, &st.MaxTempF, &st.LastReadingAt); err != nil {
			rows.Close()
			return Analytics{}, err
		}
		st.InRangePct = pct(inRange, st.TotalReadings)
		totalReadings += st.TotalReadings
		totalInRange += inRange
		cp := st
		units[st.UnitID] = &cp
		order = append(order, st.UnitID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Analytics{}, err
	}

	// Deviations per (unit, kind): total alerts opened and how many lack a
	// corrective action. Drives per-unit deviation counts plus org rollups.
	aRows, err := s.pool.Query(ctx,
		`SELECT a.unit_id, a.kind, count(*),
		        count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM corrective_actions ca WHERE ca.alert_id = a.id))
		 FROM alerts a
		 JOIN units u ON u.id = a.unit_id
		 WHERE a.org_id = $1 AND a.opened_at >= $2 AND a.opened_at < $3
		   AND ($4::uuid IS NULL OR u.location_id = $4)
		 GROUP BY a.unit_id, a.kind`, orgID, from, to, locArg)
	if err != nil {
		return Analytics{}, err
	}
	for aRows.Next() {
		var unitID uuid.UUID
		var kind string
		var cnt, undoc int
		if err := aRows.Scan(&unitID, &kind, &cnt, &undoc); err != nil {
			aRows.Close()
			return Analytics{}, err
		}
		if st := units[unitID]; st != nil {
			st.Deviations += cnt
		}
		out.Deviations += cnt
		if kind == "overdue" {
			out.OverdueEvents += cnt
		}
		if kind == "out_of_range" {
			out.UndocumentedDeviations += undoc
		}
	}
	aRows.Close()
	if err := aRows.Err(); err != nil {
		return Analytics{}, err
	}

	// Daily in-range trend across the window.
	tRows, err := s.pool.Query(ctx,
		`SELECT to_char(date_trunc('day', r.recorded_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD'),
		        count(*),
		        count(*) FILTER (WHERE r.temp_f BETWEEN u.min_temp_f AND u.max_temp_f)
		 FROM readings r
		 JOIN units u ON u.id = r.unit_id
		 WHERE r.org_id = $1 AND r.recorded_at >= $2 AND r.recorded_at < $3
		   AND ($4::uuid IS NULL OR u.location_id = $4)
		 GROUP BY 1`, orgID, from, to, locArg)
	if err != nil {
		return Analytics{}, err
	}
	counts := map[string]dayCount{}
	for tRows.Next() {
		var d string
		var total, inRange int
		if err := tRows.Scan(&d, &total, &inRange); err != nil {
			tRows.Close()
			return Analytics{}, err
		}
		counts[d] = dayCount{InRange: inRange, Total: total}
	}
	tRows.Close()
	if err := tRows.Err(); err != nil {
		return Analytics{}, err
	}

	for _, id := range order {
		out.Units = append(out.Units, *units[id])
	}
	out.TotalReadings = totalReadings
	out.InRangePct = pct(totalInRange, totalReadings)
	out.Trend = buildTrend(from, to, counts)
	return out, nil
}
