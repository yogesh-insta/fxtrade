package stats

import (
	"testing"

	"github.com/ym/fxtrade/internal/afl"
)

func TestFormFromGamesLast10(t *testing.T) {
	games := []squiggleGame{
		{HTeam: "Richmond", ATeam: "Carlton", Winner: "Richmond", Complete: 100, Date: "2026-03-01"},
		{HTeam: "Carlton", ATeam: "Richmond", Winner: "Carlton", Complete: 100, Date: "2026-03-08"},
		{HTeam: "Richmond", ATeam: "Collingwood", Winner: "Collingwood", Complete: 100, Date: "2026-03-15"},
	}
	resolve := func(name string) (afl.TeamID, error) {
		switch name {
		case "Richmond":
			return "RICH", nil
		case "Carlton":
			return "CARL", nil
		case "Collingwood":
			return "COLL", nil
		default:
			return "", nil
		}
	}
	form := formFromGames(games, resolve)
	rich := form["RICH"]
	if rich[0] != 1 || rich[1] != 2 {
		t.Fatalf("expected RICH 1-2, got %d-%d", rich[0], rich[1])
	}
}

func TestH2HFromGames(t *testing.T) {
	games := []squiggleGame{
		{HTeam: "Richmond", ATeam: "Carlton", Winner: "Richmond", Complete: 100},
		{HTeam: "Carlton", ATeam: "Richmond", Winner: "Carlton", Complete: 100},
	}
	resolve := func(name string) (afl.TeamID, error) {
		if name == "Richmond" {
			return "RICH", nil
		}
		return "CARL", nil
	}
	h2h := h2hFromGames(games, resolve)
	key := h2hKey("RICH", "CARL")
	if h2h[key] != [2]int{1, 1} {
		t.Fatalf("expected 1-1 h2h at home, got %+v", h2h[key])
	}
}

func TestEfficiencyFromLadder(t *testing.T) {
	top := efficiencyFromLadder(140, 1500, 1000, 14)
	bottom := efficiencyFromLadder(65, 900, 1400, 14)
	if top[0] <= bottom[0] {
		t.Fatalf("top team inside50 should exceed bottom: %.3f vs %.3f", top[0], bottom[0])
	}
}

func TestVenueAvgTotalsFromGames(t *testing.T) {
	games := []squiggleGame{
		{Venue: "M.C.G.", HScore: 80, AScore: 90, Complete: 100},
		{Venue: "M.C.G.", HScore: 70, AScore: 70, Complete: 100},
		{Venue: "Gabba", HScore: 100, AScore: 95, Complete: 100},
	}
	resolve := func(name string) (afl.VenueID, bool) {
		switch normalizeName(name) {
		case "m.c.g.":
			return "MCG", true
		case "gabba":
			return "GABBA", true
		default:
			return "", false
		}
	}
	avgs := venueAvgTotalsFromGames(games, resolve)
	if avgs["MCG"] != 155 {
		t.Fatalf("MCG avg expected 155, got %.0f", avgs["MCG"])
	}
	if avgs["GABBA"] != 195 {
		t.Fatalf("GABBA avg expected 195, got %.0f", avgs["GABBA"])
	}
}
