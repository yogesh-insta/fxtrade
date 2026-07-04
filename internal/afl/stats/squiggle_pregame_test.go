package stats

import (
	"testing"
	"time"
)

func TestGamesInPregameWindow(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	games := []UpcomingGame{
		{ID: 1, Kickoff: now.Add(30 * time.Minute)},
		{ID: 2, Kickoff: now.Add(34 * time.Minute)},
		{ID: 3, Kickoff: now.Add(35 * time.Minute)},
		{ID: 4, Kickoff: now.Add(29 * time.Minute)},
	}
	got := GamesInPregameWindow(games, now, 30*time.Minute, 5*time.Minute)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("window = %+v", got)
	}
}

func TestGamesInPregameWindowEdge(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	lead := 30 * time.Minute
	window := 5 * time.Minute
	games := []UpcomingGame{
		{ID: 1, Kickoff: now.Add(lead)},           // inclusive lower bound
		{ID: 2, Kickoff: now.Add(lead + window)}, // exclusive upper bound
	}
	got := GamesInPregameWindow(games, now, lead, window)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseSquiggleKickoffUnix(t *testing.T) {
	g := squiggleGame{Unixtime: 1751696400}
	tm, ok := parseSquiggleKickoff(g)
	if !ok || tm.IsZero() {
		t.Fatal("expected unix kickoff")
	}
}
