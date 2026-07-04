package stats

import (
	"sort"

	"github.com/ym/fxtrade/internal/afl"
)

// teamGameLog tracks completed games for rolling and season aggregates.
type teamGameLog struct {
	results []teamGameResult
}

type teamGameResult struct {
	forPts     int
	againstPts int
	date       string
}

type teamGameRec struct {
	forPts, againstPts int
	opponent           afl.TeamID
	date               string
	venue              string
}

// RollingTracker builds walk-forward team stats before each fixture.
type RollingTracker struct {
	teams map[afl.TeamID]*teamGameLog
	h2h   map[string][2]int
}

// NewRollingTracker creates an empty walk-forward state.
func NewRollingTracker() *RollingTracker {
	return &RollingTracker{
		teams: make(map[afl.TeamID]*teamGameLog),
		h2h:   make(map[string][2]int),
	}
}

// Snapshot returns team stats for a matchup using only games played before the next fixture.
func (rt *RollingTracker) Snapshot(home, away afl.TeamID) (homeStats, awayStats afl.TeamStats, h2hWins, h2hGames int) {
	homeStats = rt.teamStats(home)
	awayStats = rt.teamStats(away)
	if v, ok := rt.h2h[h2hKey(home, away)]; ok {
		h2hWins, h2hGames = v[0], v[1]
	}
	homeStats.H2HWinsVsOpponent = h2hWins
	homeStats.H2HGamesVsOpponent = h2hGames
	homeStats.TeamID = home
	awayStats.TeamID = away
	return homeStats, awayStats, h2hWins, h2hGames
}

func (rt *RollingTracker) teamStats(id afl.TeamID) afl.TeamStats {
	log := rt.teams[id]
	if log == nil || len(log.results) == 0 {
		return afl.TeamStats{TeamID: id}
	}

	var seasonFor, seasonAgainst int
	for _, r := range log.results {
		seasonFor += r.forPts
		seasonAgainst += r.againstPts
	}
	played := len(log.results)
	seasonPF := float64(seasonFor) / float64(played)
	seasonPA := float64(seasonAgainst) / float64(played)

	recent := log.results
	if len(recent) > afl.RollingFormGames {
		recent = recent[len(recent)-afl.RollingFormGames:]
	}
	var recentFor, recentAgainst int
	for _, r := range recent {
		recentFor += r.forPts
		recentAgainst += r.againstPts
	}
	n := float64(len(recent))
	recentPF := float64(recentFor) / n
	recentPA := float64(recentAgainst) / n

	metrics := efficiencyFromLadder(percentageFromRates(seasonPF, seasonPA), seasonFor, seasonAgainst, played)

	wins, losses := formWinsLosses(log.results, 10)

	return afl.TeamStats{
		TeamID:                     id,
		FormWinsLast10:             wins,
		FormLossesLast10:           losses,
		Inside50Efficiency:         metrics[0],
		ClearanceRate:              metrics[1],
		ContestedPossessionRate:    metrics[2],
		DisposalRate:               metrics[3],
		PointsForPerGame:           seasonPF,
		PointsAgainstPerGame:       seasonPA,
		RecentPointsForPerGame:     recentPF,
		RecentPointsAgainstPerGame: recentPA,
		ScoringTrendFor:            recentPF - seasonPF,
		ScoringTrendAgainst:        recentPA - seasonPA,
	}
}

// RecordGame appends a completed result (call after prediction for walk-forward).
func (rt *RollingTracker) RecordGame(home, away afl.TeamID, homeScore, awayScore int, homeWin bool) {
	rt.ensure(home).results = append(rt.ensure(home).results, teamGameResult{
		forPts: homeScore, againstPts: awayScore, date: "",
	})
	rt.ensure(away).results = append(rt.ensure(away).results, teamGameResult{
		forPts: awayScore, againstPts: homeScore, date: "",
	})
	key := h2hKey(home, away)
	v := rt.h2h[key]
	v[1]++
	if homeWin {
		v[0]++
	}
	rt.h2h[key] = v
}

func (rt *RollingTracker) ensure(id afl.TeamID) *teamGameLog {
	if rt.teams[id] == nil {
		rt.teams[id] = &teamGameLog{}
	}
	return rt.teams[id]
}

func formWinsLosses(results []teamGameResult, n int) (wins, losses int) {
	slice := results
	if len(slice) > n {
		slice = slice[len(slice)-n:]
	}
	for _, r := range slice {
		if r.forPts > r.againstPts {
			wins++
		} else if r.forPts < r.againstPts {
			losses++
		}
	}
	return wins, losses
}

func percentageFromRates(forPG, againstPG float64) float64 {
	if againstPG <= 0 {
		return 100
	}
	return forPG / againstPG * 100
}

// rollingScoringFromGames computes per-team season and recent scoring from completed games.
func rollingScoringFromGames(games []squiggleGame, resolve teamResolver) map[afl.TeamID]afl.TeamStats {
	byTeam := make(map[afl.TeamID][]teamGameRec)
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
		byTeam[homeID] = append(byTeam[homeID], teamGameRec{g.HScore, g.AScore, awayID, g.Date, g.Venue})
		byTeam[awayID] = append(byTeam[awayID], teamGameRec{g.AScore, g.HScore, homeID, g.Date, g.Venue})
	}

	out := make(map[afl.TeamID]afl.TeamStats)
	for id, recs := range byTeam {
		sort.Slice(recs, func(i, j int) bool { return recs[i].date < recs[j].date })
		var seasonFor, seasonAgainst int
		for _, r := range recs {
			seasonFor += r.forPts
			seasonAgainst += r.againstPts
		}
		played := len(recs)
		seasonPF := float64(seasonFor) / float64(played)
		seasonPA := float64(seasonAgainst) / float64(played)

		recent := recs
		if len(recent) > afl.RollingFormGames {
			recent = recent[len(recent)-afl.RollingFormGames:]
		}
		var recentFor, recentAgainst int
		for _, r := range recent {
			recentFor += r.forPts
			recentAgainst += r.againstPts
		}
		n := float64(len(recent))
		recentPF := float64(recentFor) / n
		recentPA := float64(recentAgainst) / n

		wins, losses := 0, 0
		formSlice := recs
		if len(formSlice) > 10 {
			formSlice = formSlice[len(formSlice)-10:]
		}
		for _, r := range formSlice {
			if r.forPts > r.againstPts {
				wins++
			} else if r.forPts < r.againstPts {
				losses++
			}
		}

		metrics := efficiencyFromLadder(percentageFromRates(seasonPF, seasonPA), seasonFor, seasonAgainst, played)
		out[id] = afl.TeamStats{
			TeamID:                     id,
			FormWinsLast10:             wins,
			FormLossesLast10:           losses,
			Inside50Efficiency:         metrics[0],
			ClearanceRate:              metrics[1],
			ContestedPossessionRate:    metrics[2],
			DisposalRate:               metrics[3],
			PointsForPerGame:           seasonPF,
			PointsAgainstPerGame:       seasonPA,
			RecentPointsForPerGame:     recentPF,
			RecentPointsAgainstPerGame: recentPA,
			ScoringTrendFor:            recentPF - seasonPF,
			ScoringTrendAgainst:        recentPA - seasonPA,
			Last5Scores:                last5ScoresFromRecs(recs),
		}
	}
	return out
}

func last5ScoresFromRecs(recs []teamGameRec) []afl.RecentMatchScore {
	if len(recs) == 0 {
		return nil
	}
	slice := recs
	if len(slice) > afl.RollingFormGames {
		slice = slice[len(slice)-afl.RollingFormGames:]
	}
	out := make([]afl.RecentMatchScore, len(slice))
	for i, r := range slice {
		out[i] = afl.RecentMatchScore{
			Opponent: r.opponent,
			For:      r.forPts,
			Against:  r.againstPts,
			Venue:    r.venue,
			Total:    r.forPts + r.againstPts,
		}
	}
	return out
}

// mergeRollingScoring overlays recent scoring fields onto existing team stats.
func mergeRollingScoring(base afl.TeamStats, rolling afl.TeamStats) afl.TeamStats {
	base.RecentPointsForPerGame = rolling.RecentPointsForPerGame
	base.RecentPointsAgainstPerGame = rolling.RecentPointsAgainstPerGame
	base.ScoringTrendFor = rolling.ScoringTrendFor
	base.ScoringTrendAgainst = rolling.ScoringTrendAgainst
	base.Last5Scores = rolling.Last5Scores
	if rolling.FormWinsLast10+rolling.FormLossesLast10 > 0 {
		base.FormWinsLast10 = rolling.FormWinsLast10
		base.FormLossesLast10 = rolling.FormLossesLast10
	}
	return base
}
