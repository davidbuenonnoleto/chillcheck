package sampler

import (
	"context"
	"sync"
	"time"

	"chillcheck-gateway/internal/reading"
)

// Sampler throttles the firehose of BLE broadcasts (every few seconds) down to
// one reading per sensor per interval. Observe() is called for every decoded
// advertisement; on each tick the newest reading per MAC is emitted.
type Sampler struct {
	interval time.Duration
	mu       sync.Mutex
	latest   map[string]reading.Reading
	out      chan []reading.Reading
}

func New(interval time.Duration) *Sampler {
	return &Sampler{
		interval: interval,
		latest:   map[string]reading.Reading{},
		out:      make(chan []reading.Reading, 8),
	}
}

func (s *Sampler) Observe(r reading.Reading) {
	s.mu.Lock()
	s.latest[r.MAC] = r // newest wins
	s.mu.Unlock()
}

// Batches is the stream of sampled readings to deliver.
func (s *Sampler) Batches() <-chan []reading.Reading { return s.out }

func (s *Sampler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.flush() // emit whatever we have on shutdown
			close(s.out)
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

func (s *Sampler) flush() {
	s.mu.Lock()
	if len(s.latest) == 0 {
		s.mu.Unlock()
		return
	}
	batch := make([]reading.Reading, 0, len(s.latest))
	for _, r := range s.latest {
		batch = append(batch, r)
	}
	s.latest = map[string]reading.Reading{}
	s.mu.Unlock()

	select {
	case s.out <- batch:
	default: // sender is briefly busy; drop into next tick rather than block scanning
	}
}
