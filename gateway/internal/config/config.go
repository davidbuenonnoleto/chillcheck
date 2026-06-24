package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIURL         string   `yaml:"api_url"`
	GatewayKey     string   `yaml:"gateway_key"`
	SampleInterval string   `yaml:"sample_interval"`
	SpoolPath      string   `yaml:"spool_path"`
	SpoolMax       int      `yaml:"spool_max"`
	Simulate       bool     `yaml:"simulate"`
	SimulateMACs   []string `yaml:"simulate_macs"`

	Interval time.Duration `yaml:"-"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		APIURL:         "http://localhost:8080",
		SampleInterval: "5m",
		SpoolPath:      "./chillcheck-spool.jsonl",
		SpoolMax:       20000,
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}

	// Env overrides keep the key out of the config file if preferred.
	if v := os.Getenv("CHILLCHECK_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("CHILLCHECK_GATEWAY_KEY"); v != "" {
		cfg.GatewayKey = v
	}

	d, err := time.ParseDuration(cfg.SampleInterval)
	if err != nil {
		return cfg, fmt.Errorf("sample_interval %q: %w", cfg.SampleInterval, err)
	}
	cfg.Interval = d

	if cfg.GatewayKey == "" {
		return cfg, fmt.Errorf("gateway_key is required (set it in the config or CHILLCHECK_GATEWAY_KEY)")
	}
	return cfg, nil
}
