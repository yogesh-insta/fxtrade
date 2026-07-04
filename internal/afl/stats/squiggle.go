package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/afl"
)

const squiggleBaseURL = "https://api.squiggle.com.au/"

// LiveStatsMeta records when live stats were refreshed and from where.
type LiveStatsMeta struct {
	Source string
	AsOf   time.Time
	Year   int
	Round  int
}

// SquiggleClient fetches completed games and ladder data from api.squiggle.com.au.
type SquiggleClient struct {
	HTTPClient *http.Client
	UserAgent  string
}

type squiggleGame struct {
	HTeam    string `json:"hteam"`
	ATeam    string `json:"ateam"`
	HScore   int    `json:"hscore"`
	AScore   int    `json:"ascore"`
	Winner   string `json:"winner"`
	Complete int    `json:"complete"`
	Round    int    `json:"round"`
	Date     string `json:"date"`
	Venue    string `json:"venue"`
}

// HistoricalGame is a completed Squiggle fixture used for training.
type HistoricalGame struct {
	Year     int
	HTeam    string
	ATeam    string
	HScore   int
	AScore   int
	Winner   string
	Complete int
	Round    int
	Date     string
	Venue    string
}

// FetchHistoricalGames returns completed games for a season.
func (c *SquiggleClient) FetchHistoricalGames(ctx context.Context, year int) ([]HistoricalGame, error) {
	games, err := c.fetchGames(ctx, year)
	if err != nil {
		return nil, err
	}
	out := make([]HistoricalGame, 0, len(games))
	for _, g := range games {
		out = append(out, HistoricalGame{
			Year: year, HTeam: g.HTeam, ATeam: g.ATeam, HScore: g.HScore, AScore: g.AScore,
			Winner: g.Winner, Complete: g.Complete, Round: g.Round, Date: g.Date, Venue: g.Venue,
		})
	}
	return out, nil
}

type squiggleStanding struct {
	Name       string  `json:"name"`
	Rank       int     `json:"rank"`
	Percentage float64 `json:"percentage"`
	For        int     `json:"for"`
	Against    int     `json:"against"`
	Played     int     `json:"played"`
}

type squiggleGamesResponse struct {
	Games []squiggleGame `json:"games"`
}

type squiggleStandingsResponse struct {
	Standings []squiggleStanding `json:"standings"`
}

// RefreshLiveStats overlays team form, efficiency proxies, and H2H from Squiggle.
func (r *Repository) RefreshLiveStats(ctx context.Context, year int, client *SquiggleClient) (LiveStatsMeta, error) {
	if client == nil {
		client = &SquiggleClient{HTTPClient: http.DefaultClient}
	}
	if client.HTTPClient == nil {
		client.HTTPClient = http.DefaultClient
	}
	if client.UserAgent == "" {
		client.UserAgent = "AFLPulse/1.0 fxtrade"
	}

	games, err := client.fetchGames(ctx, year)
	if err != nil {
		return LiveStatsMeta{}, err
	}
	r.fixtureVenues = indexFixtureVenues(games, r.ResolveTeam, r.ResolveVenue)
	round := maxRound(games)
	standings, err := client.fetchStandings(ctx, year, round)
	if err != nil {
		return LiveStatsMeta{}, err
	}

	teamStats, h2h, venueAvgs, err := buildLiveStats(completedGames(games), standings, r.ResolveTeam, r.ResolveVenue)
	if err != nil {
		return LiveStatsMeta{}, err
	}

	for id, live := range teamStats {
		base, ok := r.teams[id]
		if !ok {
			r.teams[id] = live
			continue
		}
		base.FormWinsLast10 = live.FormWinsLast10
		base.FormLossesLast10 = live.FormLossesLast10
		base.Inside50Efficiency = live.Inside50Efficiency
		base.ClearanceRate = live.ClearanceRate
		base.ContestedPossessionRate = live.ContestedPossessionRate
		base.DisposalRate = live.DisposalRate
		base.PointsForPerGame = live.PointsForPerGame
		base.PointsAgainstPerGame = live.PointsAgainstPerGame
		base.LadderPosition = live.LadderPosition
		r.teams[id] = base
	}
	rolling := rollingScoringFromGames(games, r.ResolveTeam)
	for id, roll := range rolling {
		base, ok := r.teams[id]
		if !ok {
			continue
		}
		r.teams[id] = mergeRollingScoring(base, roll)
	}
	for vid, avg := range venueAvgs {
		v, ok := r.venues[vid]
		if !ok {
			continue
		}
		v.AvgTotalScore = avg
		r.venues[vid] = v
	}
	r.h2h = h2h
	meta := LiveStatsMeta{
		Source: "Squiggle API (api.squiggle.com.au)",
		AsOf:   time.Now().UTC(),
		Year:   year,
		Round:  round,
	}
	r.liveMeta = meta
	return meta, nil
}

// LiveStatsMeta returns metadata from the most recent live stats refresh.
func (r *Repository) LiveStatsMeta() (LiveStatsMeta, bool) {
	if r.liveMeta.Year == 0 {
		return LiveStatsMeta{}, false
	}
	return r.liveMeta, true
}

// HeadToHead returns home-team wins and total games vs away in the refreshed season data.
func (r *Repository) HeadToHead(home, away afl.TeamID) (wins, games int) {
	if r.h2h == nil {
		return 0, 0
	}
	key := h2hKey(home, away)
	v, ok := r.h2h[key]
	if !ok {
		return 0, 0
	}
	return v[0], v[1]
}

func (c *SquiggleClient) fetchGames(ctx context.Context, year int) ([]squiggleGame, error) {
	url := fmt.Sprintf("%s?q=games;year=%d", squiggleBaseURL, year)
	var resp squiggleGamesResponse
	if err := c.getJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("squiggle games: %w", err)
	}
	return resp.Games, nil
}

func (c *SquiggleClient) fetchStandings(ctx context.Context, year, round int) ([]squiggleStanding, error) {
	url := fmt.Sprintf("%s?q=standings;year=%d;round=%d", squiggleBaseURL, year, round)
	var resp squiggleStandingsResponse
	if err := c.getJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("squiggle standings: %w", err)
	}
	return resp.Standings, nil
}

func (c *SquiggleClient) getJSON(ctx context.Context, url string, dest any) error {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return err
	}
	return nil
}

type teamResolver func(name string) (afl.TeamID, error)
type venueResolver func(name string) (afl.VenueID, bool)

func buildLiveStats(games []squiggleGame, standings []squiggleStanding, resolve teamResolver, resolveVenue venueResolver) (map[afl.TeamID]afl.TeamStats, map[string][2]int, map[afl.VenueID]float64, error) {
	form := formFromGames(games, resolve)
	h2h := h2hFromGames(games, resolve)
	venueAvgs := venueAvgTotalsFromGames(games, resolveVenue)

	out := make(map[afl.TeamID]afl.TeamStats)
	for i, s := range standings {
		id, err := resolve(s.Name)
		if err != nil {
			continue
		}
		rank := s.Rank
		if rank <= 0 {
			rank = i + 1
		}
		metrics := efficiencyFromLadder(s.Percentage, s.For, s.Against, s.Played)
		rec := form[id]
		pf, pa := 0.0, 0.0
		if s.Played > 0 {
			pf = float64(s.For) / float64(s.Played)
			pa = float64(s.Against) / float64(s.Played)
		}
		out[id] = afl.TeamStats{
			TeamID:                  id,
			FormWinsLast10:          rec[0],
			FormLossesLast10:        rec[1],
			Inside50Efficiency:      metrics[0],
			ClearanceRate:           metrics[1],
			ContestedPossessionRate: metrics[2],
			DisposalRate:            metrics[3],
			PointsForPerGame:        pf,
			PointsAgainstPerGame:    pa,
			LadderPosition:          rank,
		}
	}
	if len(out) == 0 {
		return nil, nil, nil, fmt.Errorf("no squiggle teams matched local aliases")
	}
	return out, h2h, venueAvgs, nil
}

func venueAvgTotalsFromGames(games []squiggleGame, resolveVenue venueResolver) map[afl.VenueID]float64 {
	type agg struct {
		sum float64
		n   int
	}
	byVenue := make(map[afl.VenueID]agg)
	for _, g := range games {
		if g.Complete < 100 {
			continue
		}
		vid, ok := resolveVenue(g.Venue)
		if !ok {
			continue
		}
		a := byVenue[vid]
		a.sum += float64(g.HScore + g.AScore)
		a.n++
		byVenue[vid] = a
	}
	out := make(map[afl.VenueID]float64)
	for vid, a := range byVenue {
		if a.n > 0 {
			out[vid] = a.sum / float64(a.n)
		}
	}
	return out
}

func formFromGames(games []squiggleGame, resolve teamResolver) map[afl.TeamID][2]int {
	type result struct {
		win  bool
		date string
	}
	byTeam := make(map[afl.TeamID][]result)

	for _, g := range games {
		if g.Complete < 100 {
			continue
		}
		homeID, err := resolve(g.HTeam)
		if err != nil {
			continue
		}
		awayID, err := resolve(g.ATeam)
		if err != nil {
			continue
		}
		homeWin := normalizeName(g.Winner) == normalizeName(g.HTeam)
		byTeam[homeID] = append(byTeam[homeID], result{win: homeWin, date: g.Date})
		byTeam[awayID] = append(byTeam[awayID], result{win: !homeWin, date: g.Date})
	}

	out := make(map[afl.TeamID][2]int)
	for id, results := range byTeam {
		sort.Slice(results, func(i, j int) bool { return results[i].date < results[j].date })
		if len(results) > 10 {
			results = results[len(results)-10:]
		}
		wins, losses := 0, 0
		for _, r := range results {
			if r.win {
				wins++
			} else {
				losses++
			}
		}
		out[id] = [2]int{wins, losses}
	}
	return out
}

func h2hFromGames(games []squiggleGame, resolve teamResolver) map[string][2]int {
	out := make(map[string][2]int)
	for _, g := range games {
		if g.Complete < 100 {
			continue
		}
		homeID, err := resolve(g.HTeam)
		if err != nil {
			continue
		}
		awayID, err := resolve(g.ATeam)
		if err != nil {
			continue
		}
		key := h2hKey(homeID, awayID)
		v := out[key]
		v[1]++
		if normalizeName(g.Winner) == normalizeName(g.HTeam) {
			v[0]++
		}
		out[key] = v
	}
	return out
}

func h2hKey(home, away afl.TeamID) string {
	return string(home) + "|" + string(away)
}

// efficiencyFromLadder maps ladder percentage and scoring rates to 0-1 model features.
// Squiggle does not publish inside-50 or clearance stats; these are scaled proxies.
func efficiencyFromLadder(percentage float64, pointsFor, pointsAgainst, played int) [4]float64 {
	pctFactor := afl.Clamp01((percentage - 70) / 80)
	inside := 0.35 + 0.65*pctFactor

	scoringBias := 0.5
	if played > 0 {
		pf := float64(pointsFor) / float64(played)
		pa := float64(pointsAgainst) / float64(played)
		scoringBias = afl.Clamp01((pf - pa + 20) / 60)
	}

	clearance := afl.Clamp01(inside*0.95 + scoringBias*0.05)
	contested := afl.Clamp01(inside*0.90 + (1-scoringBias)*0.10)
	disposal := afl.Clamp01(inside*0.85 + scoringBias*0.15)
	return [4]float64{inside, clearance, contested, disposal}
}

func completedGames(games []squiggleGame) []squiggleGame {
	out := make([]squiggleGame, 0, len(games))
	for _, g := range games {
		if g.Complete >= 100 {
			out = append(out, g)
		}
	}
	return out
}

// indexFixtureVenues maps upcoming home|away pairs to venue IDs from Squiggle.
func indexFixtureVenues(games []squiggleGame, resolve teamResolver, resolveVenue venueResolver) map[string]afl.VenueID {
	out := make(map[string]afl.VenueID)
	for _, g := range games {
		if g.Complete >= 100 {
			continue
		}
		if g.HTeam == "" || g.ATeam == "" || strings.EqualFold(g.HTeam, "none") {
			continue
		}
		homeID, err := resolve(g.HTeam)
		if err != nil {
			continue
		}
		awayID, err := resolve(g.ATeam)
		if err != nil {
			continue
		}
		vid, ok := resolveVenue(g.Venue)
		if !ok {
			continue
		}
		out[h2hKey(homeID, awayID)] = vid
	}
	return out
}

func maxRound(games []squiggleGame) int {
	round := 0
	for _, g := range games {
		if g.Round > round {
			round = g.Round
		}
	}
	if round == 0 {
		round = 1
	}
	return round
}
