// Command seed loads a demo restaurant so you can click through the app right away.
// Run it after the schema is loaded:  go run ./cmd/seed
// It is idempotent — it deletes and recreates the "Maple Street Diner" org each run.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"chillcheck/internal/auth"
	"chillcheck/internal/config"
	"chillcheck/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	demoOrg   = "Maple Street Diner"
	demoEmail = "demo@chillcheck.app"
	demoPass  = "chillcheck123"
	demoName  = "Sam Rivera"
)

type unitSpec struct {
	name     string
	kind     string
	min, max float64
	interval int
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	// Idempotent: wipe any previous demo org (cascades to users/locations/units/readings).
	if _, err := conn.Exec(ctx, `DELETE FROM organizations WHERE name = $1`, demoOrg); err != nil {
		log.Fatalf("cleanup: %v", err)
	}

	hash, err := auth.HashPassword(demoPass)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}

	var orgID, userID, locID uuid.UUID
	if err := conn.QueryRow(ctx, `INSERT INTO organizations (name) VALUES ($1) RETURNING id`, demoOrg).Scan(&orgID); err != nil {
		log.Fatalf("org: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role) VALUES ($1,$2,$3,$4,'admin') RETURNING id`,
		orgID, demoEmail, demoName, hash,
	).Scan(&userID); err != nil {
		log.Fatalf("user: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, timezone) VALUES ($1,$2,'America/Los_Angeles') RETURNING id`,
		orgID, demoOrg,
	).Scan(&locID); err != nil {
		log.Fatalf("location: %v", err)
	}

	units := []unitSpec{
		{"Walk-in cooler", "fridge", 33, 40, 240},
		{"Reach-in fridge", "fridge", 34, 40, 240},
		{"Chest freezer", "freezer", -10, 10, 240},
		{"Hot line", "hot_hold", 135, 165, 120},
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(42))

	for _, u := range units {
		var unitID uuid.UUID
		if err := conn.QueryRow(ctx,
			`INSERT INTO units (org_id, location_id, name, kind, min_temp_f, max_temp_f, log_interval_minutes)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			orgID, locID, u.name, u.kind, u.min, u.max, u.interval,
		).Scan(&unitID); err != nil {
			log.Fatalf("unit %s: %v", u.name, err)
		}

		// Each unit demonstrates a different board status (see below).
		mid := (u.min + u.max) / 2
		switch u.name {
		case "Walk-in cooler":
			// 3 days of healthy readings, most recent ~1h ago -> "in range"
			seedSeries(ctx, conn, orgID, unitID, userID, now, 72, 4, 1, func() float64 { return mid + jitter(rng, 1.5) }, "")

		case "Reach-in fridge":
			// healthy history, but the latest reading is above max -> "out of range"
			seedSeries(ctx, conn, orgID, unitID, userID, now, 72, 4, 4, func() float64 { return mid + jitter(rng, 1.5) }, "")
			insertReading(ctx, conn, orgID, unitID, userID, u.max+4, now.Add(-30*time.Minute), "door left ajar after delivery")

		case "Chest freezer":
			// readings stop 6h ago, interval is 4h -> "check overdue"
			seedSeries(ctx, conn, orgID, unitID, userID, now, 72, 4, 6, func() float64 { return mid + jitter(rng, 2.5) }, "")

		case "Hot line":
			// intentionally no readings -> "no readings yet"
		}
	}

	fmt.Println("Seeded demo data.")
	fmt.Printf("  Sign in:  %s  /  %s\n", demoEmail, demoPass)
	fmt.Println("  Statuses on the board: Walk-in (in range), Reach-in (out of range), Chest freezer (overdue), Hot line (no data).")
}

// seedSeries inserts a reading every stepH hours, from startAgoH hours ago up to
// endAgoH hours ago, using temp() for each value.
func seedSeries(ctx context.Context, conn *pgx.Conn, orgID, unitID, userID uuid.UUID, now time.Time, startAgoH, stepH, endAgoH int, temp func() float64, note string) {
	for h := startAgoH; h >= endAgoH; h -= stepH {
		insertReading(ctx, conn, orgID, unitID, userID, round1(temp()), now.Add(-time.Duration(h)*time.Hour), note)
	}
}

// chain state for seeded readings (single run, single org)
var (
	chainSeq  int64
	chainPrev string
)

func insertReading(ctx context.Context, conn *pgx.Conn, orgID, unitID, userID uuid.UUID, tempF float64, at time.Time, note string) {
	var n any
	if note != "" {
		n = note
	}
	chainSeq++
	at = at.UTC().Truncate(time.Microsecond)
	rowHash := store.ReadingHash(chainSeq, orgID, unitID, &userID, tempF, "manual", at, chainPrev)
	if _, err := conn.Exec(ctx,
		`INSERT INTO readings (org_id, unit_id, temp_f, source, note, recorded_by, recorded_at, chain_seq, prev_hash, row_hash)
		 VALUES ($1,$2,$3,'manual',$4,$5,$6,$7,$8,$9)`,
		orgID, unitID, tempF, n, userID, at, chainSeq, chainPrev, rowHash,
	); err != nil {
		log.Fatalf("reading: %v", err)
	}
	chainPrev = rowHash
}

func jitter(rng *rand.Rand, spread float64) float64 { return (rng.Float64()*2 - 1) * spread }
func round1(f float64) float64                      { return float64(int(f*10+0.5)) / 10 }
