package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// round1 rounds to one decimal place, half away from zero, matching how
// numeric(5,1) stores temp_f. We store the rounded value so the hashed input and
// the persisted value can never diverge.
func round1(f float64) float64 {
	if f < 0 {
		return math.Ceil(f*10-0.5) / 10
	}
	return math.Floor(f*10+0.5) / 10
}

// ReadingHash is the canonical hash of a reading's immutable fields chained to
// the previous row's hash. Insert and verify MUST produce identical input here.
func ReadingHash(seq int64, orgID, unitID uuid.UUID, recordedBy *uuid.UUID, tempF float64, source string, recordedAt time.Time, prevHash string) string {
	by := ""
	if recordedBy != nil {
		by = recordedBy.String()
	}
	payload := fmt.Sprintf("%d\n%s\n%s\n%s\n%.1f\n%s\n%s\n%s",
		seq, orgID, unitID, by, tempF, source,
		recordedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano), prevHash)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// insertChainedReading appends a reading as the next link in the org's hash
// chain. An advisory lock serializes appends per org so chain_seq and prev_hash
// stay consistent under concurrency (reading volume is low).
func (s *Store) insertChainedReading(ctx context.Context, orgID, unitID uuid.UUID, tempF float64, source, note string, recordedBy *uuid.UUID, recordedAt *time.Time) (Reading, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reading{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, "readings:"+orgID.String()); err != nil {
		return Reading{}, err
	}

	var lastSeq int64
	var prevHash string
	err = tx.QueryRow(ctx,
		`SELECT chain_seq, row_hash FROM readings
		 WHERE org_id = $1 AND chain_seq IS NOT NULL
		 ORDER BY chain_seq DESC LIMIT 1`, orgID,
	).Scan(&lastSeq, &prevHash)
	if errors.Is(err, pgx.ErrNoRows) {
		lastSeq, prevHash = 0, ""
	} else if err != nil {
		return Reading{}, err
	}

	seq := lastSeq + 1
	at := time.Now().UTC().Truncate(time.Microsecond)
	if recordedAt != nil {
		at = recordedAt.UTC().Truncate(time.Microsecond)
	}
	temp := round1(tempF)
	rowHash := ReadingHash(seq, orgID, unitID, recordedBy, temp, source, at, prevHash)

	var rbArg any
	if recordedBy != nil {
		rbArg = *recordedBy
	}

	var r Reading
	err = tx.QueryRow(ctx,
		`INSERT INTO readings (org_id, unit_id, temp_f, source, note, recorded_by, recorded_at, chain_seq, prev_hash, row_hash)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, unit_id, temp_f, source, COALESCE(note,''), recorded_at`,
		orgID, unitID, temp, source, nullify(note), rbArg, at, seq, prevHash, rowHash,
	).Scan(&r.ID, &r.UnitID, &r.TempF, &r.Source, &r.Note, &r.RecordedAt)
	if err != nil {
		return Reading{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reading{}, err
	}
	return r, nil
}

// ChainStatus is the result of verifying an org's reading hash chain.
type ChainStatus struct {
	OK          bool   `json:"ok"`
	Count       int    `json:"count"`
	FirstSeq    int64  `json:"first_seq"`
	LastSeq     int64  `json:"last_seq"`
	BrokenAtSeq *int64 `json:"broken_at_seq"`
}

// VerifyReadingChain walks the org's chain in order, recomputing each hash and
// checking its link to the previous row. A break (edited field, deleted row,
// reordered row) is reported at the first offending chain_seq.
func (s *Store) VerifyReadingChain(ctx context.Context, orgID uuid.UUID) (ChainStatus, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT chain_seq, unit_id, temp_f, source, recorded_by, recorded_at, prev_hash, row_hash
		 FROM readings WHERE org_id = $1 AND chain_seq IS NOT NULL
		 ORDER BY chain_seq`, orgID)
	if err != nil {
		return ChainStatus{}, err
	}
	defer rows.Close()

	var st ChainStatus
	prev := ""
	first := true
	for rows.Next() {
		var (
			seq        int64
			unitID     uuid.UUID
			tempF      float64
			source     string
			recordedBy *uuid.UUID
			at         time.Time
			storedPrev string
			storedHash string
		)
		if err := rows.Scan(&seq, &unitID, &tempF, &source, &recordedBy, &at, &storedPrev, &storedHash); err != nil {
			return ChainStatus{}, err
		}
		if first {
			st.FirstSeq = seq
			first = false
		}
		st.LastSeq = seq
		st.Count++

		want := ReadingHash(seq, orgID, unitID, recordedBy, tempF, source, at, prev)
		if (storedPrev != prev || storedHash != want) && st.BrokenAtSeq == nil {
			sq := seq
			st.BrokenAtSeq = &sq
		}
		prev = storedHash
	}
	if err := rows.Err(); err != nil {
		return ChainStatus{}, err
	}
	st.OK = st.BrokenAtSeq == nil
	return st, nil
}
