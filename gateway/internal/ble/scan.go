package ble

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"chillcheck-gateway/internal/reading"

	"tinygo.org/x/bluetooth"
)

// Source produces decoded readings. Both the real BLE scanner and the simulator
// implement it, so main.go wires either one identically.
type Source interface {
	Run(ctx context.Context, handle func(reading.Reading)) error
}

// ---------- real BLE scanner ----------

type Scanner struct {
	decoders []Decoder
}

func NewScanner() *Scanner { return &Scanner{decoders: Decoders()} }

func (s *Scanner) Run(ctx context.Context, handle func(reading.Reading)) error {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = adapter.StopScan()
	}()

	return adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
		adv := buildAdv(result)
		for _, d := range s.decoders {
			if c, ok := d.DecodeC(adv); ok {
				handle(reading.Reading{MAC: adv.MAC, TempF: reading.CtoF(c), At: time.Now()})
				return
			}
		}
	})
}

func buildAdv(result bluetooth.ScanResult) Advertisement {
	adv := Advertisement{
		MAC:              normalizeMAC(result.Address.String()),
		LocalName:        result.LocalName(),
		RSSI:             result.RSSI,
		ServiceData:      map[uint16][]byte{},
		ManufacturerData: map[uint16][]byte{},
	}
	for _, sd := range result.ServiceData() {
		if sd.UUID.Is16Bit() {
			adv.ServiceData[sd.UUID.Get16Bit()] = sd.Data
		}
	}
	for _, md := range result.ManufacturerData() {
		adv.ManufacturerData[md.CompanyID] = md.Data
	}
	return adv
}

func normalizeMAC(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r - 32)
		}
	}
	h := b.String()
	if len(h) != 12 {
		return strings.ToUpper(s)
	}
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = h[i*2 : i*2+2]
	}
	return strings.Join(parts, ":")
}

// ---------- simulator (no hardware) ----------

// Simulator emits readings for a fixed set of MACs so the whole pipeline can be
// tested against a real backend with no Bluetooth present. Bind these MACs to
// units in the app to see sensor readings flow in.
type Simulator struct {
	MACs     []string
	Interval time.Duration
}

func (s *Simulator) Run(ctx context.Context, handle func(reading.Reading)) error {
	if s.Interval <= 0 {
		s.Interval = 10 * time.Second
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	// Give each sensor a steady baseline near typical fridge temperature.
	base := make([]float64, len(s.MACs))
	for i := range base {
		base[i] = 36 + float64(i)
	}
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for i, mac := range s.MACs {
				f := base[i] + (rng.Float64()*2 - 1) // +/- 1 F wander
				handle(reading.Reading{MAC: normalizeMAC(mac), TempF: round1(f), At: time.Now()})
			}
		}
	}
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
