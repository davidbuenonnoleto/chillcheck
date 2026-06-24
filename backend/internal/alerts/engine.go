package alerts

import (
	"context"
	"fmt"
	"log"
	"time"

	"chillcheck/internal/email"
	"chillcheck/internal/store"
)

// Engine periodically evaluates every unit and reconciles alerts: it opens an
// alert (and notifies) when a unit first goes out of range or overdue, and
// resolves it when the unit recovers. One open alert per unit per kind, so a
// sustained problem notifies once, not every tick.
type Engine struct {
	store    *store.Store
	mailer   email.Mailer
	interval time.Duration
}

func NewEngine(st *store.Store, m email.Mailer, interval time.Duration) *Engine {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Engine{store: st, mailer: m, interval: interval}
}

func (e *Engine) Run(ctx context.Context) {
	log.Printf("alert engine started (evaluating every %s)", e.interval)
	t := time.NewTicker(e.interval)
	defer t.Stop()
	e.tick(ctx) // evaluate immediately on startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

// alertLeaderLockKey is a fixed advisory-lock id ("chillcheck:alerts") so that
// across multiple API replicas only the lock holder evaluates each tick.
const alertLeaderLockKey int64 = 0x43484b414c525421 // "CHKALRT!"

func (e *Engine) tick(ctx context.Context) {
	ran, err := e.store.WithLeaderLock(ctx, alertLeaderLockKey, e.evaluate)
	if err != nil {
		log.Printf("alert engine: leader lock: %v", err)
		return
	}
	if !ran {
		// Another replica holds the lock and is evaluating this tick.
		return
	}
}

func (e *Engine) evaluate(ctx context.Context) error {
	evals, err := e.store.EvaluateUnits(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, ev := range evals {
		e.reconcile(ctx, ev, now)
	}
	return nil
}

func (e *Engine) reconcile(ctx context.Context, ev store.UnitEval, now time.Time) {
	status := store.ComputeStatus(ev.MinTempF, ev.MaxTempF, ev.LogIntervalMinutes, ev.LatestTempF, ev.LatestAt, now)
	problem := ""
	if status == "out_of_range" || status == "overdue" {
		problem = status
	}

	open, err := e.store.ListOpenAlertsForUnit(ctx, ev.UnitID)
	if err != nil {
		log.Printf("alert engine: list open alerts: %v", err)
		return
	}

	hasCurrent := false
	for _, a := range open {
		if problem != "" && a.Kind == problem {
			hasCurrent = true
			continue
		}
		// The condition cleared, or the unit switched to a different problem kind.
		if err := e.store.ResolveAlert(ctx, a.ID); err != nil {
			log.Printf("alert engine: resolve alert: %v", err)
		}
	}

	if problem == "" || hasCurrent {
		return
	}

	alert, err := e.store.CreateAlert(ctx, ev.OrgID, ev.UnitID, problem, formatDetail(problem, ev, now))
	if err != nil {
		log.Printf("alert engine: create alert: %v", err)
		return
	}
	e.notify(ctx, ev, alert)
}

func (e *Engine) notify(ctx context.Context, ev store.UnitEval, a store.Alert) {
	recipients, err := e.store.AdminEmailsByOrg(ctx, ev.OrgID)
	if err != nil {
		log.Printf("alert engine: recipients: %v", err)
	}
	subject := fmt.Sprintf("ChillCheck alert: %s (%s)", ev.UnitName, kindLabel(a.Kind))
	body := fmt.Sprintf("%s at %s\n\n%s: %s\n",
		ev.UnitName, ev.LocationName, kindLabel(a.Kind), a.Detail)

	if len(recipients) == 0 {
		log.Printf("[ALERT] %s / %s — %s: %s (no admin recipients)", ev.LocationName, ev.UnitName, a.Kind, a.Detail)
	} else if err := e.mailer.Send(ctx, recipients, subject, body); err != nil {
		log.Printf("alert engine: notify: %v", err)
		return
	}
	if err := e.store.MarkAlertNotified(ctx, a.ID); err != nil {
		log.Printf("alert engine: mark notified: %v", err)
	}
}

func formatDetail(kind string, ev store.UnitEval, now time.Time) string {
	switch kind {
	case "out_of_range":
		if ev.LatestTempF != nil {
			return fmt.Sprintf("%.1f\u00B0F, outside safe range %.0f\u2013%.0f\u00B0F",
				*ev.LatestTempF, ev.MinTempF, ev.MaxTempF)
		}
		return "temperature out of range"
	case "overdue":
		if ev.LatestAt != nil {
			return fmt.Sprintf("no reading in %d min (expected every %d)",
				int(now.Sub(*ev.LatestAt).Minutes()), ev.LogIntervalMinutes)
		}
		return "no recent reading"
	}
	return ""
}

func kindLabel(kind string) string {
	switch kind {
	case "out_of_range":
		return "temperature out of range"
	case "overdue":
		return "check overdue"
	}
	return kind
}
