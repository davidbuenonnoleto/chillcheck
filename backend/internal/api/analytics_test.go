package api

import (
	"testing"
	"time"

	"chillcheck/internal/store"

	"github.com/google/uuid"
)

func TestAnalyticsCSVRecords(t *testing.T) {
	avg, min, max := 38.5, 36.0, 41.2
	last := time.Date(2026, 6, 20, 14, 30, 0, 0, time.UTC)
	a := store.Analytics{
		Units: []store.UnitStat{
			{
				UnitID:        uuid.New(),
				UnitName:      "Walk-in fridge",
				LocationName:  "Downtown",
				TotalReadings: 100,
				InRangePct:    92,
				Deviations:    3,
				AvgTempF:      &avg,
				MinTempF:      &min,
				MaxTempF:      &max,
				LastReadingAt: &last,
			},
			{
				UnitID:       uuid.New(),
				UnitName:     "New freezer",
				LocationName: "Downtown",
				// no readings -> nil temps, nil last reading
			},
		},
	}

	rec := analyticsCSVRecords(a)

	if len(rec) != 3 {
		t.Fatalf("got %d rows (incl header), want 3", len(rec))
	}
	wantHeader := []string{"Unit", "Location", "Readings", "In-range %", "Deviations", "Avg °F", "Min °F", "Max °F", "Last reading"}
	for i, h := range wantHeader {
		if rec[0][i] != h {
			t.Errorf("header[%d] = %q, want %q", i, rec[0][i], h)
		}
	}

	row := rec[1]
	want := []string{"Walk-in fridge", "Downtown", "100", "92", "3", "38.5", "36.0", "41.2", "2026-06-20T14:30:00Z"}
	for i := range want {
		if row[i] != want[i] {
			t.Errorf("row1[%d] = %q, want %q", i, row[i], want[i])
		}
	}

	// Unit with no readings: temps and last reading render empty, not "0" / "nil".
	empty := rec[2]
	for _, i := range []int{5, 6, 7, 8} {
		if empty[i] != "" {
			t.Errorf("empty row col %d = %q, want \"\"", i, empty[i])
		}
	}
	if empty[2] != "0" {
		t.Errorf("empty row readings = %q, want \"0\"", empty[2])
	}
}
