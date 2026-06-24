package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// fixed inputs so hashes are reproducible across runs
var (
	testOrg  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testUnit = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	testUser = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	testTime = time.Date(2026, 6, 23, 15, 4, 5, 0, time.UTC)
)

func TestRound1(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{36.04, 36.0},
		{36.05, 36.1},  // half away from zero (positive)
		{36.06, 36.1},
		{39.95, 40.0},
		{-0.05, -0.1},  // half away from zero (negative)
		{-10.04, -10.0},
		{-10.05, -10.1},
		{-10.06, -10.1},
	}
	for _, c := range cases {
		if got := round1(c.in); got != c.want {
			t.Errorf("round1(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReadingHash_Deterministic(t *testing.T) {
	a := ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "manual", testTime, "")
	b := ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "manual", testTime, "")
	if a != b {
		t.Fatalf("hash not deterministic: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d chars: %q", len(a), a)
	}
}

// TestReadingHash_FieldSensitivity asserts that changing any single chained
// field changes the hash — the property the tamper-evident log depends on.
func TestReadingHash_FieldSensitivity(t *testing.T) {
	base := ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "manual", testTime, "prevhash")

	otherUnit := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	otherUser := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	otherOrg := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	variants := map[string]string{
		"seq":        ReadingHash(2, testOrg, testUnit, &testUser, 36.5, "manual", testTime, "prevhash"),
		"org":        ReadingHash(1, otherOrg, testUnit, &testUser, 36.5, "manual", testTime, "prevhash"),
		"unit":       ReadingHash(1, testOrg, otherUnit, &testUser, 36.5, "manual", testTime, "prevhash"),
		"recordedBy": ReadingHash(1, testOrg, testUnit, &otherUser, 36.5, "manual", testTime, "prevhash"),
		"temp":       ReadingHash(1, testOrg, testUnit, &testUser, 36.6, "manual", testTime, "prevhash"),
		"source":     ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "sensor", testTime, "prevhash"),
		"recordedAt": ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "manual", testTime.Add(time.Second), "prevhash"),
		"prevHash":   ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "manual", testTime, "different"),
	}
	for field, h := range variants {
		if h == base {
			t.Errorf("changing %q did not change the hash — chain is not sensitive to it", field)
		}
	}
}

// A nil recordedBy (sensor reading) must hash differently from any user, and be
// stable.
func TestReadingHash_NilRecordedBy(t *testing.T) {
	withUser := ReadingHash(1, testOrg, testUnit, &testUser, 36.5, "sensor", testTime, "")
	nilUser1 := ReadingHash(1, testOrg, testUnit, nil, 36.5, "sensor", testTime, "")
	nilUser2 := ReadingHash(1, testOrg, testUnit, nil, 36.5, "sensor", testTime, "")
	if nilUser1 != nilUser2 {
		t.Error("nil recordedBy hash not deterministic")
	}
	if withUser == nilUser1 {
		t.Error("nil recordedBy hashed the same as a real user")
	}
}

// The hash must use the rounded temperature, matching what numeric(5,1) stores,
// so a value and its 1-dp rounding can never produce different hashes.
func TestReadingHash_UsesRoundedTemp(t *testing.T) {
	stored := ReadingHash(1, testOrg, testUnit, &testUser, round1(36.04), "manual", testTime, "")
	raw := ReadingHash(1, testOrg, testUnit, &testUser, 36.04, "manual", testTime, "")
	if stored != raw {
		t.Errorf("hash differs between raw temp and its 1-dp rounding (%q vs %q); "+
			"persisted value and hashed value could diverge", raw, stored)
	}
}

func TestComputeStatus(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	f := func(v float64) *float64 { return &v }
	at := func(d time.Duration) *time.Time { tm := now.Add(d); return &tm }

	const (
		minF     = 33.0
		maxF     = 40.0
		interval = 60 // minutes
	)

	cases := []struct {
		name   string
		temp   *float64
		at     *time.Time
		want   string
	}{
		{"no reading temp", nil, at(-1 * time.Minute), "no_data"},
		{"no reading time", f(36), nil, "no_data"},
		{"in range and recent", f(36), at(-10 * time.Minute), "ok"},
		{"at min boundary is ok", f(minF), at(-10 * time.Minute), "ok"},
		{"at max boundary is ok", f(maxF), at(-10 * time.Minute), "ok"},
		{"below min", f(32.9), at(-10 * time.Minute), "out_of_range"},
		{"above max", f(40.1), at(-10 * time.Minute), "out_of_range"},
		{"exactly at interval is not overdue", f(36), at(-interval * time.Minute), "ok"},
		{"past interval is overdue", f(36), at(-(interval + 1) * time.Minute), "overdue"},
		{"out of range beats overdue", f(50), at(-(interval + 1) * time.Minute), "out_of_range"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeStatus(minF, maxF, interval, c.temp, c.at, now)
			if got != c.want {
				t.Errorf("ComputeStatus = %q, want %q", got, c.want)
			}
		})
	}
}
