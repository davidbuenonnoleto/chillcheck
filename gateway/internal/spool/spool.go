package spool

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"

	"chillcheck-gateway/internal/reading"
)

// Spool persists readings that couldn't be delivered so a network outage never
// loses data (gaps fail a compliance audit). It is a simple append-only JSON
// lines file, trimmed to a maximum number of records.
type Spool struct {
	path string
	max  int
	mu   sync.Mutex
}

func New(path string, max int) *Spool {
	if max <= 0 {
		max = 20000
	}
	return &Spool{path: path, max: max}
}

func (s *Spool) Append(rs []reading.Reading) error {
	if len(rs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, r := range rs {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return s.trimLocked()
}

func (s *Spool) LoadAll() ([]reading.Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Spool) loadLocked() ([]reading.Reading, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []reading.Reading
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r reading.Reading
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out, sc.Err()
}

func (s *Spool) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// trimLocked keeps only the newest `max` records if the file has grown too large.
func (s *Spool) trimLocked() error {
	all, err := s.loadLocked()
	if err != nil || len(all) <= s.max {
		return err
	}
	all = all[len(all)-s.max:]

	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, r := range all {
		b, _ := json.Marshal(r)
		w.Write(b)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, s.path)
}
