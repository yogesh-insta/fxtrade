package afl

import "fmt"

// BuildHomeFeatureVector assembles model input for the home team's win probability.
func BuildHomeFeatureVector(ctx MatchDayContext) FeatureVector {
	travelFatigue := TravelFatigue(ctx.AwayTeam, ctx.Venue, ctx.TravelKM)
	crowdBias := CrowdBias(ctx.HomeTeam, ctx.AwayTeam, ctx.Venue)
	homeVenueFit := VenueStyleFit(ctx.HomeStats, ctx.Venue)
	awayVenueFit := VenueStyleFit(ctx.AwayStats, ctx.Venue)
	homeWeather := WeatherProfileAdjust(ctx.Weather, ctx.HomeStats)
	awayWeather := WeatherProfileAdjust(ctx.Weather, ctx.AwayStats)
	homePlayers := PlayerAvailabilityAdjust(ctx.Players.Home)
	awayPlayers := PlayerAvailabilityAdjust(ctx.Players.Away)

	homeAdvantage := crowdBias * homeVenueFit / (awayVenueFit + floatEpsilon)
	homeAdvantage = Clamp01(homeAdvantage * homeWeather.TeamStrengthFactor / (awayWeather.TeamStrengthFactor + floatEpsilon))
	homeAdvantage *= homePlayers / (awayPlayers + floatEpsilon)
	homeAdvantage *= 1.0 - travelFatigue*0.12
	homeAdvantage = Clamp01(homeAdvantage)

	vals := make([]float64, FeatureCount)
	vals[FeatHomeForm] = FormScore(ctx.HomeStats.FormWinsLast10, ctx.HomeStats.FormLossesLast10)
	vals[FeatAwayForm] = FormScore(ctx.AwayStats.FormWinsLast10, ctx.AwayStats.FormLossesLast10)
	vals[FeatHomeInside50] = ctx.HomeStats.Inside50Efficiency
	vals[FeatAwayInside50] = ctx.AwayStats.Inside50Efficiency
	vals[FeatHomeClearance] = ctx.HomeStats.ClearanceRate
	vals[FeatAwayClearance] = ctx.AwayStats.ClearanceRate
	vals[FeatHomeContested] = ctx.HomeStats.ContestedPossessionRate
	vals[FeatAwayContested] = ctx.AwayStats.ContestedPossessionRate
	vals[FeatHomeDisposal] = ctx.HomeStats.DisposalRate
	vals[FeatAwayDisposal] = ctx.AwayStats.DisposalRate
	vals[FeatH2HHomeRate] = H2HHomeRate(ctx.HomeStats)
	vals[FeatVenueStyleHome] = homeVenueFit
	vals[FeatVenueStyleAway] = awayVenueFit
	vals[FeatTravelFatigue] = travelFatigue
	vals[FeatCrowdBias] = crowdBias
	vals[FeatWeatherContest] = ctx.Weather.ContestFavorability
	vals[FeatWeatherTotalPts] = homeWeather.TotalPointsFactor
	vals[FeatHomePlayerAvail] = homePlayers
	vals[FeatAwayPlayerAvail] = awayPlayers
	vals[FeatHomeAdvantage] = homeAdvantage
	return FeatureVector{Values: vals}
}

// BuildValueBetReasons returns human-readable bullets for a value bet.
func BuildValueBetReasons(ctx MatchDayContext, bet ValueBet) []string {
	reasons := []string{
		fmt.Sprintf("Model win probability %.1f%% vs implied %.1f%% (edge %.1f%%)",
			bet.ModelProb*100, bet.ImpliedProb*100, bet.EdgePct*100),
		fmt.Sprintf("Decimal odds %.2f at %s", bet.DecimalOdds, bet.Bookmaker),
	}
	travel := TravelFatigue(ctx.AwayTeam, ctx.Venue, ctx.TravelKM)
	if travel > 0.15 {
		reasons = append(reasons, fmt.Sprintf("Away travel fatigue %.0f%% (%.0f km)", travel*100, ctx.TravelKM))
	}
	crowd := CrowdBias(ctx.HomeTeam, ctx.AwayTeam, ctx.Venue)
	if crowd > 1.05 {
		reasons = append(reasons, fmt.Sprintf("Home crowd/venue bias multiplier %.2f at %s", crowd, ctx.Venue.Name))
	}
	if ctx.Weather.RainMM > 2 || ctx.Weather.WindKPH > 25 {
		reasons = append(reasons, fmt.Sprintf("Weather: rain %.1fmm, wind %.0f km/h — favors contested profiles", ctx.Weather.RainMM, ctx.Weather.WindKPH))
	}
	homeAvail := PlayerAvailabilityAdjust(ctx.Players.Home)
	awayAvail := PlayerAvailabilityAdjust(ctx.Players.Away)
	if homeAvail < 0.9 || awayAvail < 0.9 {
		reasons = append(reasons, fmt.Sprintf("Player availability: home %.0f%%, away %.0f%%", homeAvail*100, awayAvail*100))
	}
	return reasons
}
