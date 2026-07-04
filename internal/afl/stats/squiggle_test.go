package stats

import (
	"fmt"
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

func TestBuildLiveStatsLadderPosition(t *testing.T) {
	standings := []squiggleStanding{
		{Name: "Essendon", Rank: 15, Percentage: 85, For: 1200, Against: 1400, Played: 14},
		{Name: "St Kilda", Rank: 8, Percentage: 105, For: 1300, Against: 1250, Played: 14},
	}
	resolve := func(name string) (afl.TeamID, error) {
		switch name {
		case "Essendon":
			return "ESS", nil
		case "St Kilda":
			return "STK", nil
		default:
			return "", fmt.Errorf("unknown")
		}
	}
	stats, _, _, err := buildLiveStats(nil, standings, resolve, func(string) (afl.VenueID, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if stats["ESS"].LadderPosition != 15 {
		t.Fatalf("ESS ladder = %d, want 15", stats["ESS"].LadderPosition)
	}
	if stats["STK"].LadderPosition != 8 {
		t.Fatalf("STK ladder = %d, want 8", stats["STK"].LadderPosition)
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

func TestIndexFixtureVenuesUpcomingOnly(t *testing.T) {
	games := []squiggleGame{
		{HTeam: "Essendon", ATeam: "St Kilda", Venue: "Docklands", Complete: 0},
		{HTeam: "Essendon", ATeam: "Carlton", Venue: "M.C.G.", Complete: 100},
		{HTeam: "None", ATeam: "None", Venue: "M.C.G.", Complete: 0},
	}
	resolve := func(name string) (afl.TeamID, error) {
		switch name {
		case "Essendon":
			return "ESS", nil
		case "St Kilda":
			return "STK", nil
		case "Carlton":
			return "CARL", nil
		default:
			return "", fmt.Errorf("unknown")
		}
	}
	resolveVenue := func(name string) (afl.VenueID, bool) {
		switch normalizeName(name) {
		case "docklands":
			return "DOCKLANDS", true
		case "m.c.g.":
			return "MCG", true
		default:
			return "", false
		}
	}
	idx := indexFixtureVenues(games, resolve, resolveVenue)
	if idx[h2hKey("ESS", "STK")] != "DOCKLANDS" {
		t.Fatalf("ESS vs STK venue = %q, want DOCKLANDS", idx[h2hKey("ESS", "STK")])
	}
	if _, ok := idx[h2hKey("ESS", "CARL")]; ok {
		t.Fatal("completed fixture should not be indexed")
	}
}
