package afl

import (
	"fmt"
	"math"
)

// LeagueBaselineTotal is the typical combined AFL match score used as a starting point.
const LeagueBaselineTotal = 170.0

// LeagueBaselineTeamPoints is the per-team fallback when scoring rates are unknown.
const LeagueBaselineTeamPoints = LeagueBaselineTotal / 2.0

// MaxExpectedMargin caps the projected winning margin derived from win probability and profiles.
const MaxExpectedMargin = 35.0

// Total blend weights: team scoring rates vs venue historical pace.
const (
	TeamScoringWeight = 0.60
	VenuePaceWeight   = 0.40
	SeasonScoringWeight = 0.45
	RecentScoringWeight = 0.55
)

// TotalsPredictor optionally overrides heuristic total projection when trained.
var TotalsPredictor *LinearPredictor

// Score bounds for projected match totals.
const (
	MinMatchTotal = 120.0
	MaxMatchTotal = 210.0
)

// ScoreProjection holds predicted match scoring outcomes.
type ScoreProjection struct {
	HomeScore       int
	AwayScore       int
	TotalScore      int
	Margin          int
	PredictedWinner TeamID
	HomeWinProb     float64
	Breakdown       ScoreBreakdown
}

// ScoreBreakdown explains how the projected total and margin were derived.
type ScoreBreakdown struct {
	BaselineTotal       float64
	WeatherFactor       float64
	HomeExpected        float64
	AwayExpected        float64
	TeamBasedTotal      float64
	VenueAvgTotal       float64
	BlendedTotal        float64
	AdjustedTotal       float64
	HomeOffensive       float64
	AwayOffensive       float64
	ProfileEdge         float64
	WinEdge             float64
	CombinedEdge        float64
	RawMargin           float64
	HomeGoalsBehind     string
	AwayGoalsBehind     string
}

// ProjectScore estimates team scores using independent total and margin models.
func ProjectScore(ctx MatchDayContext, homeWinProb float64) ScoreProjection {
	homeWinProb = Clamp01(homeWinProb)
	totalBD := ProjectTotal(ctx)
	marginBD := projectMargin(ctx, homeWinProb)

	total := totalBD.AdjustedTotal
	margin := marginBD.RawMargin

	homeScore := math.Round((total + margin) / 2.0)
	awayScore := math.Round((total - margin) / 2.0)
	if homeScore < 30 {
		homeScore = 30
	}
	if awayScore < 30 {
		awayScore = 30
	}

	winner := ctx.HomeTeam
	if awayScore > homeScore {
		winner = ctx.AwayTeam
	} else if homeScore == awayScore {
		if homeWinProb >= 0.5 {
			winner = ctx.HomeTeam
		} else {
			winner = ctx.AwayTeam
		}
	}

	bd := totalBD
	bd.HomeOffensive = marginBD.HomeOffensive
	bd.AwayOffensive = marginBD.AwayOffensive
	bd.ProfileEdge = marginBD.ProfileEdge
	bd.WinEdge = marginBD.WinEdge
	bd.CombinedEdge = marginBD.CombinedEdge
	bd.RawMargin = marginBD.RawMargin

	return ScoreProjection{
		HomeScore:       int(homeScore),
		AwayScore:       int(awayScore),
		TotalScore:      int(homeScore + awayScore),
		Margin:          int(math.Abs(homeScore - awayScore)),
		PredictedWinner: winner,
		HomeWinProb:     homeWinProb,
		Breakdown: ScoreBreakdown{
			BaselineTotal:   bd.BaselineTotal,
			WeatherFactor:   bd.WeatherFactor,
			HomeExpected:    bd.HomeExpected,
			AwayExpected:    bd.AwayExpected,
			TeamBasedTotal:  bd.TeamBasedTotal,
			VenueAvgTotal:   bd.VenueAvgTotal,
			BlendedTotal:    bd.BlendedTotal,
			AdjustedTotal:   bd.AdjustedTotal,
			HomeOffensive:   bd.HomeOffensive,
			AwayOffensive:   bd.AwayOffensive,
			ProfileEdge:     bd.ProfileEdge,
			WinEdge:         bd.WinEdge,
			CombinedEdge:    bd.CombinedEdge,
			RawMargin:       bd.RawMargin,
			HomeGoalsBehind: formatGoalsBehind(int(homeScore)),
			AwayGoalsBehind: formatGoalsBehind(int(awayScore)),
		},
	}
}

// ProjectTotal estimates combined match points from team scoring rates, venue pace, and weather.
func ProjectTotal(ctx MatchDayContext) ScoreBreakdown {
	if TotalsPredictor != nil {
		return projectTotalLearned(ctx)
	}
	return projectTotalHeuristic(ctx)
}

func projectTotalLearned(ctx MatchDayContext) ScoreBreakdown {
	fv := BuildTotalsFeatureVector(ctx)
	raw, err := TotalsPredictor.Predict(fv)
	if err != nil {
		return projectTotalHeuristic(ctx)
	}
	weatherFactor := matchWeatherFactor(ctx)
	adjusted := clampTotal(raw * weatherFactor)

	hbd := projectTotalHeuristic(ctx)
	return ScoreBreakdown{
		BaselineTotal:  LeagueBaselineTotal,
		WeatherFactor:  weatherFactor,
		HomeExpected:   hbd.HomeExpected,
		AwayExpected:   hbd.AwayExpected,
		TeamBasedTotal: hbd.TeamBasedTotal,
		VenueAvgTotal:  ctx.Venue.AvgTotalScore,
		BlendedTotal:   raw,
		AdjustedTotal:  adjusted,
	}
}

func projectTotalHeuristic(ctx MatchDayContext) ScoreBreakdown {
	homePF := teamPointsFor(ctx.HomeStats)
	homePA := teamPointsAgainst(ctx.HomeStats)
	awayPF := teamPointsFor(ctx.AwayStats)
	awayPA := teamPointsAgainst(ctx.AwayStats)

	homeExpected := (homePF + awayPA) / 2.0
	awayExpected := (awayPF + homePA) / 2.0
	teamBased := homeExpected + awayExpected

	venueAvg := ctx.Venue.AvgTotalScore
	blended := teamBased
	if venueAvg > 0 {
		blended = TeamScoringWeight*teamBased + VenuePaceWeight*venueAvg
	}

	weatherFactor := matchWeatherFactor(ctx)
	adjusted := clampTotal(blended * weatherFactor)

	return ScoreBreakdown{
		BaselineTotal:  LeagueBaselineTotal,
		WeatherFactor:  weatherFactor,
		HomeExpected:   homeExpected,
		AwayExpected:   awayExpected,
		TeamBasedTotal: teamBased,
		VenueAvgTotal:  venueAvg,
		BlendedTotal:   blended,
		AdjustedTotal:  adjusted,
	}
}

func projectMargin(ctx MatchDayContext, homeWinProb float64) ScoreBreakdown {
	homeOff := teamOffensiveProfile(ctx, ctx.HomeStats, ctx.Players.Home)
	awayOff := teamOffensiveProfile(ctx, ctx.AwayStats, ctx.Players.Away)
	profileEdge := SafeDiv(homeOff-awayOff, homeOff+awayOff)
	winEdge := (homeWinProb - 0.5) * 2.0
	combinedEdge := 0.4*profileEdge + 0.6*winEdge
	margin := combinedEdge * MaxExpectedMargin

	return ScoreBreakdown{
		HomeOffensive: homeOff,
		AwayOffensive: awayOff,
		ProfileEdge:   profileEdge,
		WinEdge:       winEdge,
		CombinedEdge:  combinedEdge,
		RawMargin:     margin,
	}
}

func teamPointsFor(stats TeamStats) float64 {
	season := teamPointsForSeason(stats)
	recent := stats.RecentPointsForPerGame
	if recent <= 0 {
		return season
	}
	blended := SeasonScoringWeight*season + RecentScoringWeight*recent
	return blended + stats.ScoringTrendFor*0.15
}

func teamPointsAgainst(stats TeamStats) float64 {
	season := teamPointsAgainstSeason(stats)
	recent := stats.RecentPointsAgainstPerGame
	if recent <= 0 {
		return season
	}
	blended := SeasonScoringWeight*season + RecentScoringWeight*recent
	return blended + stats.ScoringTrendAgainst*0.15
}

func matchWeatherFactor(ctx MatchDayContext) float64 {
	homeWeather := WeatherProfileAdjust(ctx.Weather, ctx.HomeStats)
	awayWeather := WeatherProfileAdjust(ctx.Weather, ctx.AwayStats)
	factor := (homeWeather.TotalPointsFactor + awayWeather.TotalPointsFactor) / 2.0
	if factor <= 0 {
		factor = 1.0
	}
	return factor
}

func clampTotal(v float64) float64 {
	if v < MinMatchTotal {
		return MinMatchTotal
	}
	if v > MaxMatchTotal {
		return MaxMatchTotal
	}
	return v
}

func teamOffensiveProfile(ctx MatchDayContext, stats TeamStats, players []PlayerImpact) float64 {
	form := FormScore(stats.FormWinsLast10, stats.FormLossesLast10)
	inside := stats.Inside50Efficiency
	clearance := stats.ClearanceRate
	venueFit := VenueStyleFit(stats, ctx.Venue)
	avail := PlayerAvailabilityAdjust(players)

	raw := form*0.25 + inside*0.30 + clearance*0.20 + venueFit*0.15 + avail*0.10
	if raw < 0.35 {
		return 0.35
	}
	if raw > 1.0 {
		return 1.0
	}
	return raw
}

func formatGoalsBehind(score int) string {
	if score < 0 {
		score = 0
	}
	goals := score / 6
	behinds := score % 6
	return fmt.Sprintf("%d.%d", goals, behinds)
}
