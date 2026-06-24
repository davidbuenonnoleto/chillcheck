package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

// These tests exercise the tamper-evident hash chain against a real Postgres.
// They are skipped unless TEST_DATABASE_URL points at a database with schema.sql
// loaded, so `go test ./...` stays green without a database. Run with e.g.:
//
//	TEST_DATABASE_URL="postgres://chillcheck:chillcheck@localhost:5433/chillcheck?sslmode=disable" \
//	    go test ./internal/store/ -run Integration -v

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run chain integration tests")
	}
	st, err := New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// seedChain creates a fresh org with a unit and four chained readings
// (three manual, one sensor) and returns the org id.
func seedChain(t *testing.T, st *Store) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	email := "chain+" + uuid.NewString() + "@test.local"
	user, org, err := st.CreateOrgWithAdmin(ctx, "Chain Test Co", "Admin", email, "x")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	loc, err := st.CreateLocation(ctx, org.ID, "Kitchen", "UTC")
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	unit, err := st.CreateUnit(ctx, org.ID, loc.ID, "Walk-in", "fridge", 33, 40, 240)
	if err != nil {
		t.Fatalf("create unit: %v", err)
	}

	for _, temp := range []float64{36.0, 37.5, 35.5} {
		if _, err := st.CreateReading(ctx, org.ID, unit.ID, user.ID, temp, ""); err != nil {
			t.Fatalf("create reading: %v", err)
		}
	}
	if err := st.CreateSensorReading(ctx, org.ID, unit.ID, 38.1, testTime); err != nil {
		t.Fatalf("create sensor reading: %v", err)
	}
	return org.ID
}

// WithLeaderLock must let only one holder run at a time: while one call holds
// the advisory lock, a concurrent call returns ran=false; after release the
// lock is acquirable again. This is what stops multiple API replicas from
// evaluating (and emailing) the same alert tick.
func TestIntegration_LeaderLockSerializes(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	const key int64 = 0x1234abcd

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	var heldRan bool
	var heldErr error
	go func() {
		heldRan, heldErr = st.WithLeaderLock(ctx, key, func(context.Context) error {
			close(started)
			<-release // hold the lock until the main goroutine says go
			return nil
		})
		close(done)
	}()

	<-started // lock is now held by the goroutine

	ran, err := st.WithLeaderLock(ctx, key, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("contended attempt errored: %v", err)
	}
	if ran {
		t.Error("expected ran=false while the lock is held by another caller")
	}

	close(release)
	<-done
	if heldErr != nil || !heldRan {
		t.Fatalf("holder should have run: ran=%v err=%v", heldRan, heldErr)
	}

	ran, err = st.WithLeaderLock(ctx, key, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("post-release attempt errored: %v", err)
	}
	if !ran {
		t.Error("expected lock to be acquirable again after release")
	}
}

func TestIntegration_ChainVerifiesIntact(t *testing.T) {
	st := testStore(t)
	org := seedChain(t, st)

	got, err := st.VerifyReadingChain(context.Background(), org)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !got.OK {
		t.Errorf("expected intact chain, got broken at %v", got.BrokenAtSeq)
	}
	if got.Count != 4 {
		t.Errorf("expected 4 readings, got %d", got.Count)
	}
	if got.FirstSeq != 1 || got.LastSeq != 4 {
		t.Errorf("expected seq 1..4, got %d..%d", got.FirstSeq, got.LastSeq)
	}
}

// Editing a past reading's value directly in the database must break the chain
// at exactly that row — the core compliance claim.
func TestIntegration_ChainDetectsEditedReading(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	org := seedChain(t, st)

	// Tamper: change the temperature of chain_seq=2 in place, leaving row_hash
	// untouched (as a careless DB edit would).
	if _, err := st.pool.Exec(ctx,
		`UPDATE readings SET temp_f = temp_f + 5 WHERE org_id = $1 AND chain_seq = 2`, org); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	got, err := st.VerifyReadingChain(ctx, org)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.OK {
		t.Fatal("expected broken chain after editing a reading, got OK")
	}
	if got.BrokenAtSeq == nil || *got.BrokenAtSeq != 2 {
		t.Errorf("expected break at seq 2, got %v", got.BrokenAtSeq)
	}
}

// Deleting a past reading must break the chain at the following row, because its
// stored prev_hash no longer matches the recomputed running hash.
func TestIntegration_ChainDetectsDeletedReading(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	org := seedChain(t, st)

	if _, err := st.pool.Exec(ctx,
		`DELETE FROM readings WHERE org_id = $1 AND chain_seq = 2`, org); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := st.VerifyReadingChain(ctx, org)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.OK {
		t.Fatal("expected broken chain after deleting a reading, got OK")
	}
	if got.BrokenAtSeq == nil || *got.BrokenAtSeq != 3 {
		t.Errorf("expected break detected at seq 3 (row after the deleted one), got %v", got.BrokenAtSeq)
	}
}
