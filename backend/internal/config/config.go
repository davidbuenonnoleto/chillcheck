package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	CORSOrigin  string

	// Alerting
	AlertsEnabled bool
	AlertInterval time.Duration
	SMTPHost      string
	SMTPPort      string
	SMTPUser      string
	SMTPPass      string
	AlertFrom     string

	// Billing (Stripe)
	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceID       string
	AppBaseURL          string
}

// BillingEnabled reports whether Stripe is configured. When false, the app skips
// all billing/entitlement gating so it runs with zero setup.
func (c Config) BillingEnabled() bool {
	return c.StripeSecretKey != "" && c.StripePriceID != ""
}

func Load() Config {
	cfg := Config{
		DatabaseURL: getenv("DATABASE_URL", "postgres://chillcheck:chillcheck@localhost:5432/chillcheck?sslmode=disable"),
		JWTSecret:   getenv("JWT_SECRET", "dev-secret-change-me"),
		Port:        getenv("PORT", "8080"),
		CORSOrigin:  getenv("CORS_ORIGIN", "http://localhost:5173"),

		AlertsEnabled: getbool("ALERTS_ENABLED", true),
		AlertInterval: getdur("ALERT_INTERVAL", time.Minute),
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      getenv("SMTP_PORT", "587"),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		AlertFrom:     getenv("ALERT_FROM", "alerts@chillcheck.local"),

		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceID:       os.Getenv("STRIPE_PRICE_ID"),
		AppBaseURL:          getenv("APP_BASE_URL", "http://localhost:5173"),
	}
	if cfg.JWTSecret == "dev-secret-change-me" {
		log.Println("WARNING: using the default JWT secret. Set JWT_SECRET in production.")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getbool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getdur(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
