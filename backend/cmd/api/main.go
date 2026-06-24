package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"chillcheck/internal/alerts"
	"chillcheck/internal/api"
	"chillcheck/internal/config"
	"chillcheck/internal/email"
	"chillcheck/internal/store"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	mailer := buildMailer(cfg)

	// Background alert engine (out-of-range / overdue -> notify).
	if cfg.AlertsEnabled {
		engine := alerts.NewEngine(st, mailer, cfg.AlertInterval)
		go engine.Run(context.Background())
	}

	srv := api.NewServer(st, cfg, mailer)

	addr := ":" + cfg.Port
	log.Printf("ChillCheck API listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatal(err)
	}
}

// buildMailer sends email via SMTP if SMTP_HOST is set, otherwise logs messages.
// Used for alerts, team invites, and password resets.
func buildMailer(cfg config.Config) email.Mailer {
	if cfg.SMTPHost != "" {
		log.Printf("email: sending via %s:%s", cfg.SMTPHost, cfg.SMTPPort)
		return email.SMTPMailer{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.AlertFrom,
		}
	}
	log.Println("email: no SMTP_HOST set, logging messages instead of sending")
	return email.LogMailer{}
}
