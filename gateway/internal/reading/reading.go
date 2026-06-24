package reading

import "time"

// Reading is one decoded sensor measurement, ready to send to ChillCheck.
// JSON matches the /api/ingest/readings contract. Temperatures are Fahrenheit
// (sensors report Celsius; the decoders convert at the edge).
type Reading struct {
	MAC   string    `json:"mac"`
	TempF float64   `json:"temp_f"`
	At    time.Time `json:"recorded_at"`
}

func CtoF(c float64) float64 { return c*9.0/5.0 + 32.0 }
