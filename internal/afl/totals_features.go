package afl

// Totals feature indices for the trained linear scoring model.
const (
	TotalsFeatHomeSeasonFor = iota
	TotalsFeatAwaySeasonFor
	TotalsFeatHomeSeasonAgainst
	TotalsFeatAwaySeasonAgainst
	TotalsFeatHomeRecentFor
	TotalsFeatAwayRecentFor
	TotalsFeatHomeRecentAgainst
	TotalsFeatAwayRecentAgainst
	TotalsFeatHomeTrend
	TotalsFeatAwayTrend
	TotalsFeatVenueAvg
	TotalsFeatureCount
)

// BuildTotalsFeatureVector assembles inputs for the match total points model.
func BuildTotalsFeatureVector(ctx MatchDayContext) FeatureVector {
	vals := make([]float64, TotalsFeatureCount)
	h, a := ctx.HomeStats, ctx.AwayStats
	vals[TotalsFeatHomeSeasonFor] = scalePts(teamPointsForSeason(h))
	vals[TotalsFeatAwaySeasonFor] = scalePts(teamPointsForSeason(a))
	vals[TotalsFeatHomeSeasonAgainst] = scalePts(teamPointsAgainstSeason(h))
	vals[TotalsFeatAwaySeasonAgainst] = scalePts(teamPointsAgainstSeason(a))
	vals[TotalsFeatHomeRecentFor] = scalePts(h.RecentPointsForPerGame)
	vals[TotalsFeatAwayRecentFor] = scalePts(a.RecentPointsForPerGame)
	vals[TotalsFeatHomeRecentAgainst] = scalePts(h.RecentPointsAgainstPerGame)
	vals[TotalsFeatAwayRecentAgainst] = scalePts(a.RecentPointsAgainstPerGame)
	vals[TotalsFeatHomeTrend] = NormScoringTrend(h.ScoringTrendFor)
	vals[TotalsFeatAwayTrend] = NormScoringTrend(a.ScoringTrendFor)
	vals[TotalsFeatVenueAvg] = scalePts(ctx.Venue.AvgTotalScore)
	return FeatureVector{Values: vals}
}

func scalePts(v float64) float64 {
	if v <= 0 {
		return LeagueBaselineTeamPoints / 120.0
	}
	return v / 120.0
}

func teamPointsForSeason(s TeamStats) float64 {
	if s.PointsForPerGame > 0 {
		return s.PointsForPerGame
	}
	return LeagueBaselineTeamPoints
}

func teamPointsAgainstSeason(s TeamStats) float64 {
	if s.PointsAgainstPerGame > 0 {
		return s.PointsAgainstPerGame
	}
	return LeagueBaselineTeamPoints
}
