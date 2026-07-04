package odds

import (
	"strings"
	"testing"

	"github.com/ym/fxtrade/internal/afl"
)

func TestParseEvents(t *testing.T) {
	data := []byte(`[{
		"id": "ev1",
		"sport_key": "aussierules_afl",
		"commence_time": "2025-04-17T09:30:00Z",
		"home_team": "Brisbane Lions",
		"away_team": "Collingwood Magpies",
		"bookmakers": [{
			"key": "sportsbet",
			"title": "SportsBet",
			"markets": [{
				"key": "h2h",
				"outcomes": [
					{"name": "Brisbane Lions", "price": 1.53},
					{"name": "Collingwood Magpies", "price": 2.51}
				]
			}]
		}]
	}]`)

	resolve := func(name string) (afl.TeamID, error) {
		m := map[string]afl.TeamID{
			"brisbane lions":      "BRI",
			"collingwood magpies": "COLL",
		}
		key := strings.ToLower(strings.TrimSpace(name))
		if id, ok := m[key]; ok {
			return id, nil
		}
		return "", nil
	}

	out, err := ParseEvents(data, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.H2H) != 2 {
		t.Fatalf("expected 2 h2h outcomes, got %d", len(out.H2H))
	}
	if out.H2H[0].DecimalOdds != 1.53 {
		t.Fatalf("unexpected odds %.2f", out.H2H[0].DecimalOdds)
	}
}

func TestParseEventsTotals(t *testing.T) {
	data := []byte(`[{
		"id": "ev1",
		"commence_time": "2025-04-17T09:30:00Z",
		"home_team": "Richmond Tigers",
		"away_team": "Carlton Blues",
		"bookmakers": [{
			"key": "sportsbet",
			"markets": [{
				"key": "totals",
				"outcomes": [
					{"name": "Over", "price": 1.90, "point": 168.5},
					{"name": "Under", "price": 1.90, "point": 168.5}
				]
			}]
		}]
	}]`)

	resolve := func(name string) (afl.TeamID, error) {
		m := map[string]afl.TeamID{
			"richmond tigers": "RICH",
			"carlton blues":   "CARL",
		}
		key := strings.ToLower(strings.TrimSpace(name))
		return m[key], nil
	}

	out, err := ParseEvents(data, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Totals) != 1 {
		t.Fatalf("expected 1 totals line, got %d", len(out.Totals))
	}
	if out.Totals[0].Line != 168.5 {
		t.Fatalf("unexpected line %.1f", out.Totals[0].Line)
	}
}
