package afl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultPregameKickoffDrift = 15 * time.Minute

// SquiggleFixture is a scheduled Squiggle game used for pregame matching (avoids import cycle with stats).
type SquiggleFixture struct {
	ID       int
	HomeTeam string
	AwayTeam string
	Kickoff  time.Time
	Round    int
	Venue    string
}

// PregameState tracks pre-game emails sent by Squiggle game ID.
type PregameState struct {
	Sent    map[string]time.Time `json:"sent"`
	SavedAt time.Time          `json:"saved_at"`
}

// LoadPregameState reads dedup state from disk. Missing files start empty.
func LoadPregameState(path string) (*PregameState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PregameState{Sent: make(map[string]time.Time)}, nil
		}
		return nil, err
	}
	var st PregameState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse pregame state %s: %w", path, err)
	}
	if st.Sent == nil {
		st.Sent = make(map[string]time.Time)
	}
	return &st, nil
}

// Save writes dedup state atomically.
func (s *PregameState) Save(path string) error {
	if s.Sent == nil {
		s.Sent = make(map[string]time.Time)
	}
	s.SavedAt = time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WasSent reports whether a Squiggle game ID already triggered an email.
func (s *PregameState) WasSent(id int) bool {
	if s == nil || s.Sent == nil {
		return false
	}
	_, ok := s.Sent[strconv.Itoa(id)]
	return ok
}

// MarkSent records a successful pre-game email for a Squiggle game ID.
func (s *PregameState) MarkSent(id int) {
	if s.Sent == nil {
		s.Sent = make(map[string]time.Time)
	}
	s.Sent[strconv.Itoa(id)] = time.Now().UTC()
}

// Prune removes entries older than cutoff to keep the state file small.
func (s *PregameState) Prune(cutoff time.Time) {
	if s == nil || s.Sent == nil {
		return
	}
	for id, sentAt := range s.Sent {
		if sentAt.Before(cutoff) {
			delete(s.Sent, id)
		}
	}
}

// MatchSquiggleToOdds finds odds markets for a Squiggle upcoming game.
func MatchSquiggleToOdds(game SquiggleFixture, h2h []MarketOdds, totals []TotalsOdds, resolve func(string) (TeamID, error)) (Fixture, []MarketOdds, []TotalsOdds, bool) {
	homeID, err := resolve(game.HomeTeam)
	if err != nil {
		return Fixture{}, nil, nil, false
	}
	awayID, err := resolve(game.AwayTeam)
	if err != nil {
		return Fixture{}, nil, nil, false
	}

	var fixture Fixture
	var matchedH2H []MarketOdds
	for _, mo := range h2h {
		if mo.HomeTeam != homeID || mo.AwayTeam != awayID {
			continue
		}
		if !kickoffCloseEnough(game.Kickoff, mo.Kickoff, defaultPregameKickoffDrift) {
			continue
		}
		matchedH2H = append(matchedH2H, mo)
		if fixture.EventID == "" {
			fixture = Fixture{
				EventID:  mo.EventID,
				HomeTeam: homeID,
				AwayTeam: awayID,
				Kickoff:  mo.Kickoff,
			}
		}
	}
	if fixture.EventID == "" {
		return Fixture{}, nil, nil, false
	}

	var matchedTotals []TotalsOdds
	for _, t := range totals {
		if t.EventID != fixture.EventID {
			continue
		}
		if t.HomeTeam != homeID || t.AwayTeam != awayID {
			continue
		}
		matchedTotals = append(matchedTotals, t)
	}
	return fixture, matchedH2H, matchedTotals, true
}

func kickoffCloseEnough(expected, actual time.Time, drift time.Duration) bool {
	if expected.IsZero() || actual.IsZero() {
		return true
	}
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	return diff <= drift
}

// DefaultPregameStatePath returns the repo-relative dedup file path.
func DefaultPregameStatePath(statsDir string) string {
	base := strings.TrimSpace(statsDir)
	if base == "" {
		base = "data/afl"
	}
	return filepath.Join(base, "pregame-sent.json")
}
