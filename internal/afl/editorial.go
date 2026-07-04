package afl

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EditorialPick is a curated high-confidence betting selection for a fixture.
type EditorialPick struct {
	Type      string   `json:"type"` // h2h, margin, totals
	Selection string   `json:"selection"`
	Odds      *float64 `json:"odds,omitempty"`
	Label     string   `json:"label"`
	Why       string   `json:"why,omitempty"`
}

// PlayerPropPick is a curated player-prop consideration (Same Game Multi).
type PlayerPropPick struct {
	Player string `json:"player"`
	Team   TeamID `json:"team"`
	Market string `json:"market"`
	Why    string `json:"why,omitempty"`
}

// FixtureEditorial holds human-curated picks and notes for one match.
type FixtureEditorial struct {
	MatchLabel  string           `json:"match_label"`
	Home        TeamID           `json:"home"`
	Away        TeamID           `json:"away"`
	SmartPicks  []EditorialPick  `json:"smart_picks"`
	PlayerProps []PlayerPropPick `json:"player_props"`
	Notes       []string         `json:"notes"`
}

// RoundEditorial is the weekly editorial picks file.
type RoundEditorial struct {
	Round    int                `json:"round"`
	Fixtures []FixtureEditorial `json:"fixtures"`
}

type roundEditorialFile struct {
	Round    int                `json:"round"`
	Fixtures []FixtureEditorial `json:"fixtures"`
}

// LoadRoundEditorial reads editorial picks from path. Missing file returns empty, not error.
func LoadRoundEditorial(path string) (RoundEditorial, error) {
	if path == "" {
		return RoundEditorial{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RoundEditorial{}, nil
		}
		return RoundEditorial{}, fmt.Errorf("editorial picks %s: %w", path, err)
	}
	var f roundEditorialFile
	if err := json.Unmarshal(data, &f); err != nil {
		return RoundEditorial{}, fmt.Errorf("editorial picks %s: %w", path, err)
	}
	return RoundEditorial{Round: f.Round, Fixtures: f.Fixtures}, nil
}

func fixtureEditorialKey(home, away TeamID) string {
	return string(home) + "|" + string(away)
}

// LookupFixtureEditorial finds editorial content for a home/away pairing.
func (e RoundEditorial) LookupFixtureEditorial(home, away TeamID) *FixtureEditorial {
	for i := range e.Fixtures {
		f := &e.Fixtures[i]
		if f.Home == home && f.Away == away {
			return f
		}
	}
	return nil
}

// AttachEditorialPicks merges editorial content into match reports by fixture.
func AttachEditorialPicks(reports []MatchReport, editorial RoundEditorial) {
	for i := range reports {
		ctx := reports[i].Context
		if ed := editorial.LookupFixtureEditorial(ctx.HomeTeam, ctx.AwayTeam); ed != nil {
			copy := *ed
			reports[i].Editorial = &copy
		}
	}
}

// FormatSmartPickLine renders one smart pick for email output.
func (p EditorialPick) FormatSmartPickLine() string {
	label := strings.TrimSpace(p.Label)
	if label == "" {
		label = p.Selection
	}
	if p.Odds != nil && *p.Odds > 1 {
		return fmt.Sprintf("%s ($%.2f)", label, *p.Odds)
	}
	return label
}
