package store

import (
	"testing"
	"time"
)

func TestPct(t *testing.T) {
	cases := []struct {
		num, den, want int
	}{
		{8, 10, 80},
		{1, 3, 33},
		{2, 3, 67},
		{10, 10, 100},
		{11, 10, 100}, // capped
		{0, 5, 0},
		{0, 0, 0},  // no readings -> 0, not 100
		{5, 0, 0},  // defensive: empty denominator
	}
	for _, c := range cases {
		if got := pct(c.num, c.den); got != c.want {
			t.Errorf("pct(%d,%d) = %d, want %d", c.num, c.den, got, c.want)
		}
	}
}

func TestBuildTrend_FillsEmptyDays(t *testing.T) {
	from := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC) // exclusive
	counts := map[string]dayCount{
		"2026-06-01": {InRange: 8, Total: 10},
		"2026-06-03": {InRange: 5, Total: 5},
		// 2026-06-02 intentionally absent
	}

	got := buildTrend(from, to, counts)

	want := []TrendBucket{
		{Date: "2026-06-01", InRange: 8, Total: 10, Pct: 80},
		{Date: "2026-06-02", InRange: 0, Total: 0, Pct: 0},
		{Date: "2026-06-03", InRange: 5, Total: 5, Pct: 100},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d buckets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bucket %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBuildTrend_SingleDay(t *testing.T) {
	from := time.Date(2026, 6, 1, 6, 30, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 18, 0, 0, 0, time.UTC)
	got := buildTrend(from, to, map[string]dayCount{"2026-06-01": {InRange: 3, Total: 4}})
	if len(got) != 1 {
		t.Fatalf("got %d buckets, want 1: %+v", len(got), got)
	}
	if got[0] != (TrendBucket{Date: "2026-06-01", InRange: 3, Total: 4, Pct: 75}) {
		t.Errorf("bucket = %+v", got[0])
	}
}
