package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegration_Analytics verifies the per-unit rollup against a real Postgres:
// in-range counting, the location filter, and strict org isolation. Skipped unless
// TEST_DATABASE_URL is set (see chain_integration_test.go).
func TestIntegration_Analytics(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)

	// Org A: two locations, one unit each.
	userA, orgA, err := st.CreateOrgWithAdmin(ctx, "Org A", "Admin", "a+"+uuid.NewString()+"@t.local", "x")
	if err != nil {
		t.Fatalf("org A: %v", err)
	}
	l1, _ := st.CreateLocation(ctx, orgA.ID, "Downtown", "UTC")
	l2, _ := st.CreateLocation(ctx, orgA.ID, "Airport", "UTC")
	u1, _ := st.CreateUnit(ctx, orgA.ID, l1.ID, "Fridge 1", "fridge", 33, 40, 240)
	u2, _ := st.CreateUnit(ctx, orgA.ID, l2.ID, "Fridge 2", "fridge", 33, 40, 240)

	// u1: 2 in range, 1 over max.
	for _, temp := range []float64{36.0, 38.0, 45.0} {
		if _, err := st.CreateReading(ctx, orgA.ID, u1.ID, userA.ID, temp, ""); err != nil {
			t.Fatalf("reading u1: %v", err)
		}
	}
	// u2: 1 in range.
	if _, err := st.CreateReading(ctx, orgA.ID, u2.ID, userA.ID, 35.0, ""); err != nil {
		t.Fatalf("reading u2: %v", err)
	}

	// Org B: separate org whose data must never leak into Org A's analytics.
	userB, orgB, _ := st.CreateOrgWithAdmin(ctx, "Org B", "Admin", "b+"+uuid.NewString()+"@t.local", "x")
	lB, _ := st.CreateLocation(ctx, orgB.ID, "Other", "UTC")
	uB, _ := st.CreateUnit(ctx, orgB.ID, lB.ID, "Fridge B", "fridge", 33, 40, 240)
	_, _ = st.CreateReading(ctx, orgB.ID, uB.ID, userB.ID, 50.0, "")

	// Whole-org view.
	a, err := st.Analytics(ctx, orgA.ID, from, to, nil)
	if err != nil {
		t.Fatalf("analytics: %v", err)
	}
	if a.TotalReadings != 4 {
		t.Errorf("TotalReadings = %d, want 4 (org B excluded)", a.TotalReadings)
	}
	if a.InRangePct != 75 { // 3 of 4 in range
		t.Errorf("InRangePct = %d, want 75", a.InRangePct)
	}
	if len(a.Units) != 2 {
		t.Fatalf("got %d units, want 2", len(a.Units))
	}
	byID := map[uuid.UUID]UnitStat{}
	for _, u := range a.Units {
		byID[u.UnitID] = u
	}
	if got := byID[u1.ID]; got.TotalReadings != 3 || got.InRangePct != 67 {
		t.Errorf("u1 = %d readings / %d%%, want 3 / 67", got.TotalReadings, got.InRangePct)
	}

	// Location filter: only u1.
	f, err := st.Analytics(ctx, orgA.ID, from, to, &l1.ID)
	if err != nil {
		t.Fatalf("analytics filtered: %v", err)
	}
	if len(f.Units) != 1 || f.Units[0].UnitID != u1.ID {
		t.Errorf("filtered units = %+v, want only u1", f.Units)
	}
	if f.TotalReadings != 3 {
		t.Errorf("filtered TotalReadings = %d, want 3", f.TotalReadings)
	}
}
