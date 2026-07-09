package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultNavSnapshotPath = "data/account_nav_snapshots.json"

// NavSnapshotStore records end-of-day NAV for daily P&L deltas.
type NavSnapshotStore struct {
	path string
}

type navSnapshotFile struct {
	Currency string             `json:"currency"`
	ByDate   map[string]float64 `json:"by_date"`
}

// NewNavSnapshotStore opens or creates the snapshot file at path (default data/account_nav_snapshots.json).
func NewNavSnapshotStore(path string) *NavSnapshotStore {
	if path == "" {
		path = defaultNavSnapshotPath
	}
	return &NavSnapshotStore{path: path}
}

func (s *NavSnapshotStore) load() (navSnapshotFile, error) {
	var f navSnapshotFile
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return navSnapshotFile{ByDate: make(map[string]float64)}, nil
		}
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("parse %s: %w", s.path, err)
	}
	if f.ByDate == nil {
		f.ByDate = make(map[string]float64)
	}
	return f, nil
}

func (s *NavSnapshotStore) save(f navSnapshotFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}

// PriorNAV returns the stored NAV for the UTC calendar day before reportDate.
func (s *NavSnapshotStore) PriorNAV(reportDate time.Time, currency string) (nav float64, priorDate string, ok bool) {
	f, err := s.load()
	if err != nil {
		return 0, "", false
	}
	prior := reportDate.UTC().Add(-24 * time.Hour)
	key := prior.Format("2006-01-02")
	nav, ok = f.ByDate[key]
	if !ok {
		return 0, key, false
	}
	if currency != "" && f.Currency != "" && f.Currency != currency {
		return nav, key, true
	}
	return nav, key, true
}

// Record stores NAV for reportDate (typically after sending the daily email).
func (s *NavSnapshotStore) Record(reportDate time.Time, nav float64, currency string) error {
	f, err := s.load()
	if err != nil {
		return err
	}
	if currency != "" {
		f.Currency = currency
	}
	key := reportDate.UTC().Format("2006-01-02")
	f.ByDate[key] = nav
	return s.save(f)
}
