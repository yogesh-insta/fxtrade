package stats

import (
	"path/filepath"
	"testing"

	"github.com/ym/fxtrade/internal/afl"
)

func TestAwayTravelKMStKToMCGViaRepository(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "afl")
	repo, err := NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	stkBase := repo.DefaultVenueForTeam("STK")
	matchVenue := repo.DefaultVenueForTeam("ESS") // ESS home at MCG
	km := afl.AwayTravelKM(stkBase, matchVenue)
	if km >= afl.MinMeaningfulTravelKM {
		t.Fatalf("STK→MCG travel = %.1f km, want < %d km", km, afl.MinMeaningfulTravelKM)
	}
	if km > 10 {
		t.Fatalf("STK→MCG expected <10 km, got %.1f km", km)
	}
}

func TestDefaultVenueForTeamUsesDefaultVenueField(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "afl")
	repo, err := NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	stk := repo.DefaultVenueForTeam("STK")
	if stk.ID != "DOCKLANDS" {
		t.Fatalf("STK default venue = %s, want DOCKLANDS", stk.ID)
	}
	ess := repo.DefaultVenueForTeam("ESS")
	if ess.ID != "MCG" {
		t.Fatalf("ESS default venue = %s, want MCG", ess.ID)
	}
}

func TestMatchVenueESSSTKRound17(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "afl")
	repo, err := NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo.fixtureVenues = map[string]afl.VenueID{
		h2hKey("ESS", "STK"): "DOCKLANDS",
	}
	v, ok := repo.MatchVenue("ESS", "STK")
	if !ok {
		t.Fatal("expected fixture venue for ESS vs STK")
	}
	if v.ID != "DOCKLANDS" || v.Name != "Marvel Stadium" {
		t.Fatalf("ESS vs STK venue = %+v, want Marvel Stadium (DOCKLANDS)", v)
	}
	if v.Dimension != afl.VenueClosed {
		t.Fatalf("DOCKLANDS dimension = %s, want closed", v.Dimension)
	}
}

func TestNewRepository(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "afl")
	repo, err := NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := repo.ResolveTeam("Brisbane Lions")
	if err != nil {
		t.Fatal(err)
	}
	if id != "BRI" {
		t.Fatalf("expected BRI, got %s", id)
	}
	stats, ok := repo.Team("BRI")
	if !ok {
		t.Fatal("missing BRI stats")
	}
	if stats.Inside50Efficiency <= 0 {
		t.Fatal("expected positive inside50 efficiency")
	}
}
