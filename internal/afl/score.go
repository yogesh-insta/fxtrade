package afl

import "math"

// LeagueBaselineTotal is the typical combined AFL match score used as a starting point.
const LeagueBaselineTotal = 170.0

// MaxExpectedMargin caps the projected winning margin derived from win probability and profiles.
const MaxExpectedMargin = 35.0

// ScoreProjection holds predicted match scoring outcomes.
type ScoreProjection struct {
	HomeScore       int
	AwayScore       int
	TotalScore      int
	Margin          int
	PredictedWinner TeamID
	HomeWinProb     float64
}

// ProjectScore estimates team scores from match context and the model's home win probability.
// Total points start at the league baseline (~170) adjusted by weather, then split using
// offensive profiles (inside50, form, clearances, venue fit, availability) and homeWinProb.
func ProjectScore(ctx MatchDayContext, homeWinProb float64) ScoreProjection {
	homeWinProb = Clamp01(homeWinProb)

	homeWeather := WeatherProfileAdjust(ctx.Weather, ctx.HomeStats)
	awayWeather := WeatherProfileAdjust(ctx.Weather, ctx.AwayStats)
	weatherFactor := (homeWeather.TotalPointsFactor + awayWeather.TotalPointsFactor) / 2.0
	if weatherFactor <= 0 {
		weatherFactor = 1.0
	}
	total := LeagueBaselineTotal * weatherFactor

	homeOff := teamOffensiveProfile(ctx, ctx.HomeStats, ctx.Players.Home)
	awayOff := teamOffensiveProfile(ctx, ctx.AwayStats, ctx.Players.Away)
	profileEdge := SafeDiv(homeOff-awayOff, homeOff+awayOff)
	winEdge := (homeWinProb - 0.5) * 2.0
	combinedEdge := 0.4*profileEdge + 0.6*winEdge
	margin := combinedEdge * MaxExpectedMargin

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

	return ScoreProjection{
		HomeScore:       int(homeScore),
		AwayScore:       int(awayScore),
		TotalScore:      int(homeScore + awayScore),
		Margin:          int(math.Abs(homeScore - awayScore)),
		PredictedWinner: winner,
		HomeWinProb:     homeWinProb,
	}
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
