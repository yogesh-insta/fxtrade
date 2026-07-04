package afl

import (
	"fmt"
	"math"
	"strings"
)

// BuildMatchPredictionReasons returns human-readable bullets explaining the fixture prediction.
func BuildMatchPredictionReasons(r MatchReport) []string {
	ctx := r.Context
	h, a := ctx.HomeStats, ctx.AwayStats
	winner := r.Score.PredictedWinner
	winProb := r.WinnerWinProbability()

	var reasons []string

	// Winner narrative
	if winner == ctx.HomeTeam {
		reasons = append(reasons, fmt.Sprintf(
			"Model favours %s to win (%.0f%%) — home win probability %.0f%% from trained H2H model",
			winner, winProb*100, r.HomeWinProb*100))
	} else {
		reasons = append(reasons, fmt.Sprintf(
			"Model favours %s to win (%.0f%%) — away side rated %.0f%% despite playing at %s",
			winner, winProb*100, (1-r.HomeWinProb)*100, ctx.Venue.Name))
	}

	reasons = append(reasons, formCompareReason(ctx.HomeTeam, h, ctx.AwayTeam, a)...)

	if h.H2HGamesVsOpponent > 0 {
		awayWins := h.H2HGamesVsOpponent - h.H2HWinsVsOpponent
		reasons = append(reasons, fmt.Sprintf(
			"Season head-to-head at home: %s %d–%d %s (%d meetings)",
			ctx.HomeTeam, h.H2HWinsVsOpponent, awayWins, ctx.AwayTeam, h.H2HGamesVsOpponent))
	}

	if trend := scoringTrendReason(ctx.HomeTeam, h, ctx.AwayTeam, a); trend != "" {
		reasons = append(reasons, trend)
	}

	if venue := venueReason(ctx); venue != "" {
		reasons = append(reasons, venue)
	}

	if MeaningfulTravel(ctx.TravelKM) {
		travel := TravelFatigue(ctx.AwayTeam, ctx.Venue, ctx.TravelKM)
		if travel > 0.12 {
			reasons = append(reasons, fmt.Sprintf(
				"Away travel: %s ~%.0f km to %s (fatigue factor %.0f%%)",
				ctx.AwayTeam, ctx.TravelKM, ctx.Venue.Name, travel*100))
		}
	}

	if wx := weatherReason(ctx); wx != "" {
		reasons = append(reasons, wx)
	}

	homeAvail := PlayerAvailabilityAdjust(ctx.Players.Home)
	awayAvail := PlayerAvailabilityAdjust(ctx.Players.Away)
	if homeAvail < 0.92 || awayAvail < 0.92 {
		reasons = append(reasons, fmt.Sprintf(
			"Squad availability: %s %.0f%%, %s %.0f%% (injuries file applied if configured)",
			ctx.HomeTeam, homeAvail*100, ctx.AwayTeam, awayAvail*100))
	}

	reasons = append(reasons, totalReasons(r)...)

	if margin := marginReason(r); margin != "" {
		reasons = append(reasons, margin)
	}

	if len(r.ValueBets) > 0 {
		vb := r.ValueBets[0]
		if vb.Team != winner {
			reasons = append(reasons, fmt.Sprintf(
				"Note: best H2H value is %s @ %.2f (%s) — model %.0f%% vs market %.0f%% (winner pick and value bet differ)",
				vb.Team, vb.DecimalOdds, vb.Bookmaker, vb.ModelProb*100, vb.ImpliedProb*100))
		} else {
			reasons = append(reasons, fmt.Sprintf(
				"Aligned value: %s @ %.2f (%s) EV +%.0f%%",
				vb.Team, vb.DecimalOdds, vb.Bookmaker, vb.EV*100))
		}
	}

	return reasons
}

func formCompareReason(homeName TeamID, h TeamStats, awayName TeamID, a TeamStats) []string {
	var out []string
	hForm := FormScore(h.FormWinsLast10, h.FormLossesLast10)
	aForm := FormScore(a.FormWinsLast10, a.FormLossesLast10)
	diff := hForm - aForm
	if math.Abs(diff) < 0.08 {
		out = append(out, fmt.Sprintf(
			"Recent results similar: %s %d–%d, %s %d–%d (last 10)",
			homeName, h.FormWinsLast10, h.FormLossesLast10,
			awayName, a.FormWinsLast10, a.FormLossesLast10))
		return out
	}
	fav, dog := homeName, awayName
	favW, favL, dogW, dogL := h.FormWinsLast10, h.FormLossesLast10, a.FormWinsLast10, a.FormLossesLast10
	if diff < 0 {
		fav, dog = awayName, homeName
		favW, favL, dogW, dogL = a.FormWinsLast10, a.FormLossesLast10, h.FormWinsLast10, h.FormLossesLast10
	}
	out = append(out, fmt.Sprintf(
		"Form edge %s: %d–%d vs %s %d–%d over last 10",
		fav, favW, favL, dog, dogW, dogL))
	return out
}

func scoringTrendReason(homeName TeamID, h TeamStats, awayName TeamID, a TeamStats) string {
	if h.RecentPointsForPerGame <= 0 && a.RecentPointsForPerGame <= 0 {
		return ""
	}
	var parts []string
	if h.RecentPointsForPerGame > 0 {
		parts = append(parts, fmt.Sprintf("%s %.0f pts/game last 5 (trend %+.0f vs season)",
			homeName, h.RecentPointsForPerGame, h.ScoringTrendFor))
	}
	if a.RecentPointsForPerGame > 0 {
		parts = append(parts, fmt.Sprintf("%s %.0f pts/game last 5 (trend %+.0f vs season)",
			awayName, a.RecentPointsForPerGame, a.ScoringTrendFor))
	}
	if h.ScoringTrendFor < -5 && a.ScoringTrendFor < -5 {
		return "Scoring slump both sides: " + strings.Join(parts, "; ") + " — supports a lower total"
	}
	if h.ScoringTrendFor > 5 && a.ScoringTrendFor > 5 {
		return "Scoring surge both sides: " + strings.Join(parts, "; ") + " — supports a higher total"
	}
	return strings.Join(parts, "; ")
}

func venueReason(ctx MatchDayContext) string {
	v := ctx.Venue
	line := fmt.Sprintf("Venue %s (%s)", v.Name, v.Dimension)
	if v.AvgTotalScore > 0 {
		line += fmt.Sprintf(" — season avg total %.0f pts at this ground", v.AvgTotalScore)
	}
	if rate, ok := v.HomeWinRates[ctx.HomeTeam]; ok && rate > 0.55 {
		line += fmt.Sprintf("; %s home win rate here %.0f%%", ctx.HomeTeam, rate*100)
	} else if v.PrimaryHomeTeam == ctx.HomeTeam {
		line += fmt.Sprintf("; %s primary home ground", ctx.HomeTeam)
	}
	return line
}

func weatherReason(ctx MatchDayContext) string {
	if VenueIsClosed(ctx.Venue) {
		return fmt.Sprintf("Weather: rain %.1f mm, wind %.0f km/h outdoors — closed roof, no scoring impact",
			ctx.Weather.RainMM, ctx.Weather.WindKPH)
	}
	w := ctx.Weather
	if w.RainMM < 1 && w.WindKPH < 20 {
		return fmt.Sprintf("Weather mild: rain %.1f mm, wind %.0f km/h — limited scoring impact", w.RainMM, w.WindKPH)
	}
	factor := w.TotalPointsFactor
	if factor <= 0 {
		homeW := WeatherProfileAdjust(w, ctx.HomeStats)
		factor = homeW.TotalPointsFactor
	}
	bias := "neutral"
	if factor < 0.95 {
		bias = "lower scoring"
	} else if factor > 1.02 {
		bias = "higher scoring"
	}
	return fmt.Sprintf("Weather: rain %.1f mm, wind %.0f km/h — %s conditions (total factor ~%.2f)",
		w.RainMM, w.WindKPH, bias, factor)
}

func totalReasons(r MatchReport) []string {
	ctx := r.Context
	d := r.Score.Breakdown
	var out []string

	if d.TeamBasedTotal > 0 {
		out = append(out, fmt.Sprintf(
			"Total %d: matchup scoring %s %.0f + %s %.0f expected (season+recent blend) before venue/weather",
			r.Score.TotalScore, ctx.HomeTeam, d.HomeExpected, ctx.AwayTeam, d.AwayExpected))
	} else {
		out = append(out, fmt.Sprintf("Total %d from baseline model × weather %.2f", r.Score.TotalScore, d.WeatherFactor))
	}

	if r.TotalsLine != nil {
		diff := float64(r.Score.TotalScore) - r.TotalsLine.Line
		var lean string
		switch {
		case diff > 5:
			lean = fmt.Sprintf("model %d pts above book line %.1f — lean OVER if betting totals", r.Score.TotalScore, r.TotalsLine.Line)
		case diff < -5:
			lean = fmt.Sprintf("model %d pts below book line %.1f — lean UNDER if betting totals", r.Score.TotalScore, r.TotalsLine.Line)
		default:
			lean = fmt.Sprintf("model %d pts near book line %.1f (Δ %+.0f)", r.Score.TotalScore, r.TotalsLine.Line, diff)
		}
		out = append(out, lean+" @ "+r.TotalsLine.Bookmaker)
	}

	return out
}

func marginReason(r MatchReport) string {
	d := r.Score.Breakdown
	if d.RawMargin == 0 && d.ProfileEdge == 0 {
		return ""
	}
	return fmt.Sprintf(
		"Margin %d (%s): offensive profiles %+.2f, win-probability edge %+.2f → projected spread %+.1f",
		r.Score.Margin, r.Score.PredictedWinner, d.ProfileEdge, d.WinEdge, d.RawMargin)
}
