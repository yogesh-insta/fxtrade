package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ym/fxtrade/internal/afl"
)

// Repository loads static AFL stats and reference data from JSON files.
type Repository struct {
	dir      string
	venues           map[afl.VenueID]afl.VenueProfile
	teams            map[afl.TeamID]afl.TeamStats
	teamDefaultVenue map[afl.TeamID]afl.VenueID
	players  map[afl.TeamID][]afl.PlayerImpact
	aliases  map[string]afl.TeamID
	venueAlias map[string]afl.VenueID
	h2h      map[string][2]int
	liveMeta LiveStatsMeta
}

type venuesFile struct {
	Venues []venueJSON `json:"venues"`
}

type venueJSON struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Dimension       afl.VenueDimension     `json:"dimension"`
	Latitude        float64            `json:"latitude"`
	Longitude       float64            `json:"longitude"`
	Interstate      bool               `json:"interstate"`
	PrimaryHomeTeam string             `json:"primary_home_team"`
	HomeWinRates    map[string]float64 `json:"home_win_rates"`
}

type teamsFile struct {
	Teams []teamJSON `json:"teams"`
}

type teamJSON struct {
	ID                      string  `json:"id"`
	FormWinsLast10          int     `json:"form_wins_last_10"`
	FormLossesLast10        int     `json:"form_losses_last_10"`
	Inside50Efficiency      float64 `json:"inside50_efficiency"`
	ClearanceRate           float64 `json:"clearance_rate"`
	ContestedPossessionRate float64 `json:"contested_possession_rate"`
	DisposalRate            float64 `json:"disposal_rate"`
	H2HWinsVsOpponent       int     `json:"h2h_wins_vs_opponent"`
	H2HGamesVsOpponent      int     `json:"h2h_games_vs_opponent"`
	DefaultVenue            string  `json:"default_venue"`
}

type playersFile struct {
	Players []playerJSON `json:"players"`
}

type playerJSON struct {
	PlayerID    string     `json:"player_id"`
	TeamID      string     `json:"team_id"`
	Role        afl.PlayerRole `json:"role"`
	ImpactScore float64    `json:"impact_score"`
}

type aliasesFile struct {
	Aliases map[string]string `json:"aliases"`
}

type venueAliasesFile struct {
	Aliases map[string]string `json:"aliases"`
}

type injuriesFile struct {
	Unavailable []struct {
		PlayerID string `json:"player_id"`
		TeamID   string `json:"team_id"`
	} `json:"unavailable"`
}

// NewRepository loads all JSON seed data from dir.
func NewRepository(dir string) (*Repository, error) {
	r := &Repository{
		dir:     dir,
		venues:           make(map[afl.VenueID]afl.VenueProfile),
		teams:            make(map[afl.TeamID]afl.TeamStats),
		teamDefaultVenue: make(map[afl.TeamID]afl.VenueID),
		players: make(map[afl.TeamID][]afl.PlayerImpact),
		aliases: make(map[string]afl.TeamID),
		venueAlias: make(map[string]afl.VenueID),
	}
	if err := r.loadVenues(filepath.Join(dir, "venues.json")); err != nil {
		return nil, err
	}
	if err := r.loadTeams(filepath.Join(dir, "teams.json")); err != nil {
		return nil, err
	}
	if err := r.loadPlayers(filepath.Join(dir, "players.json")); err != nil {
		return nil, err
	}
	if err := r.loadAliases(filepath.Join(dir, "team_aliases.json")); err != nil {
		return nil, err
	}
	_ = r.loadVenueAliases(filepath.Join(dir, "venue_aliases.json"))
	return r, nil
}

func (r *Repository) loadVenues(path string) error {
	var f venuesFile
	if err := readJSON(path, &f); err != nil {
		return err
	}
	for _, v := range f.Venues {
		rates := make(map[afl.TeamID]float64)
		for k, val := range v.HomeWinRates {
			rates[afl.TeamID(k)] = val
		}
		r.venues[afl.VenueID(v.ID)] = afl.VenueProfile{
			ID:              afl.VenueID(v.ID),
			Name:            v.Name,
			Dimension:       v.Dimension,
			Latitude:        v.Latitude,
			Longitude:       v.Longitude,
			Interstate:      v.Interstate,
			PrimaryHomeTeam: afl.TeamID(v.PrimaryHomeTeam),
			HomeWinRates:    rates,
		}
	}
	return nil
}

func (r *Repository) loadTeams(path string) error {
	var f teamsFile
	if err := readJSON(path, &f); err != nil {
		return err
	}
	for _, t := range f.Teams {
		if t.DefaultVenue != "" {
			r.teamDefaultVenue[afl.TeamID(t.ID)] = afl.VenueID(t.DefaultVenue)
		}
		r.teams[afl.TeamID(t.ID)] = afl.TeamStats{
			TeamID:                  afl.TeamID(t.ID),
			FormWinsLast10:          t.FormWinsLast10,
			FormLossesLast10:        t.FormLossesLast10,
			Inside50Efficiency:      t.Inside50Efficiency,
			ClearanceRate:           t.ClearanceRate,
			ContestedPossessionRate: t.ContestedPossessionRate,
			DisposalRate:            t.DisposalRate,
			H2HWinsVsOpponent:       t.H2HWinsVsOpponent,
			H2HGamesVsOpponent:      t.H2HGamesVsOpponent,
		}
	}
	return nil
}

func (r *Repository) loadPlayers(path string) error {
	var f playersFile
	if err := readJSON(path, &f); err != nil {
		return err
	}
	for _, p := range f.Players {
		tid := afl.TeamID(p.TeamID)
		r.players[tid] = append(r.players[tid], afl.PlayerImpact{
			PlayerID:    p.PlayerID,
			TeamID:      tid,
			Role:        p.Role,
			ImpactScore: p.ImpactScore,
			Available:   true,
		})
	}
	return nil
}

func (r *Repository) loadAliases(path string) error {
	var f aliasesFile
	if err := readJSON(path, &f); err != nil {
		return err
	}
	for name, id := range f.Aliases {
		r.aliases[normalizeName(name)] = afl.TeamID(id)
	}
	return nil
}

func (r *Repository) loadVenueAliases(path string) error {
	var f venueAliasesFile
	if err := readJSON(path, &f); err != nil {
		return nil // optional file
	}
	for name, id := range f.Aliases {
		r.venueAlias[normalizeName(name)] = afl.VenueID(id)
	}
	return nil
}

// ResolveVenue maps a Squiggle venue name to internal VenueID.
func (r *Repository) ResolveVenue(name string) (afl.VenueID, bool) {
	if id, ok := r.venueAlias[normalizeName(name)]; ok {
		if _, known := r.venues[id]; known {
			return id, true
		}
	}
	return "", false
}

// ResolveTeam maps an API team name to internal TeamID.
func (r *Repository) ResolveTeam(name string) (afl.TeamID, error) {
	if id, ok := r.aliases[normalizeName(name)]; ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown team name %q", name)
}

// Team returns stats for a team.
func (r *Repository) Team(id afl.TeamID) (afl.TeamStats, bool) {
	s, ok := r.teams[id]
	return s, ok
}

// Venue returns profile for a venue.
func (r *Repository) Venue(id afl.VenueID) (afl.VenueProfile, bool) {
	v, ok := r.venues[id]
	return v, ok
}

// DefaultVenueForTeam returns the team's home base venue for travel calculations.
func (r *Repository) DefaultVenueForTeam(team afl.TeamID) afl.VenueProfile {
	if vid, ok := r.teamDefaultVenue[team]; ok {
		if v, ok := r.venues[vid]; ok {
			return v
		}
	}
	for _, v := range r.venues {
		if v.PrimaryHomeTeam == team {
			return v
		}
	}
	return afl.VenueProfile{}
}

// PlayerMatrix builds availability for home/away with optional injuries overlay.
func (r *Repository) PlayerMatrix(home, away afl.TeamID, injuriesPath string) afl.PlayerAvailabilityMatrix {
	homePlayers := clonePlayers(r.players[home])
	awayPlayers := clonePlayers(r.players[away])
	if injuriesPath != "" {
		_, _ = applyInjuries(homePlayers, awayPlayers, injuriesPath)
	}
	return afl.PlayerAvailabilityMatrix{Home: homePlayers, Away: awayPlayers}
}

// CountInjuries returns how many players are listed unavailable in the injuries file.
func (r *Repository) CountInjuries(path string) (int, error) {
	var f injuriesFile
	if err := readJSON(path, &f); err != nil {
		return 0, err
	}
	return len(f.Unavailable), nil
}

func clonePlayers(src []afl.PlayerImpact) []afl.PlayerImpact {
	out := make([]afl.PlayerImpact, len(src))
	copy(out, src)
	return out
}

func applyInjuries(home, away []afl.PlayerImpact, path string) (int, error) {
	var f injuriesFile
	if err := readJSON(path, &f); err != nil {
		return 0, fmt.Errorf("injuries %s: %w", path, err)
	}
	unavail := make(map[string]struct{})
	for _, u := range f.Unavailable {
		unavail[u.PlayerID] = struct{}{}
	}
	n := markUnavailable(home, unavail) + markUnavailable(away, unavail)
	return n, nil
}

func markUnavailable(players []afl.PlayerImpact, unavail map[string]struct{}) int {
	n := 0
	for i := range players {
		if _, ok := unavail[players[i].PlayerID]; ok {
			players[i].Available = false
			n++
		}
	}
	return n
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
