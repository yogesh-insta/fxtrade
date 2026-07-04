package afl

import "math"

// WeatherAdjustment captures team-specific weather impact.
type WeatherAdjustment struct {
	TeamStrengthFactor float64 // multiplier for team profile fit
	TotalPointsFactor  float64 // match scoring expectation multiplier
}

// TravelFatigue returns a 0-1 fatigue penalty for the away team based on travel distance.
// Longer interstate trips (e.g. VIC→WA) produce higher fatigue scores.
func TravelFatigue(away TeamID, venue VenueProfile, travelKM float64) float64 {
	if travelKM <= 0 {
		return 0
	}
	base := math.Log1p(travelKM/500.0) / math.Log1p(6) // ~0 at 0km, ~1 near 3000km
	if venue.Interstate {
		base *= 1.15
	}
	_ = away // reserved for team-specific travel matrices in future
	return Clamp01(base)
}

// CrowdBias returns a home-ground advantage multiplier centered at 1.0.
func CrowdBias(home, away TeamID, venue VenueProfile) float64 {
	bias := 1.0
	if rate, ok := venue.HomeWinRates[home]; ok && rate > 0 {
		bias += (rate - 0.5) * 0.4
	} else if venue.PrimaryHomeTeam == home {
		bias += 0.08
	}
	if venue.Interstate && away != home {
		bias += 0.04
	}
	if bias < 0.85 {
		return 0.85
	}
	if bias > 1.25 {
		return 1.25
	}
	return bias
}

// WeatherProfileAdjust down-weights disposal-heavy teams in wet/windy conditions.
func WeatherProfileAdjust(weather WeatherMetrics, team TeamStats) WeatherAdjustment {
	wetWind := Clamp01(weather.RainMM/8.0 + weather.WindKPH/40.0)
	disposalHeavy := team.DisposalRate
	contestHeavy := team.ContestedPossessionRate

	// Wet/windy favors contested ball winners over uncontested disposal chains.
	profileFit := 1.0 - wetWind*(disposalHeavy*0.35-contestHeavy*0.25)
	profileFit = Clamp01(profileFit)

	totalPts := weather.TotalPointsFactor
	if totalPts <= 0 {
		totalPts = 1.0 - wetWind*0.25
	}
	if totalPts < 0.6 {
		totalPts = 0.6
	}
	return WeatherAdjustment{
		TeamStrengthFactor: profileFit,
		TotalPointsFactor:  totalPts,
	}
}

// VenueStyleFit scores how well a team's profile suits the venue dimensions.
func VenueStyleFit(team TeamStats, venue VenueProfile) float64 {
	switch venue.Dimension {
	case VenueNarrow:
		// Narrow grounds reward contested work and inside-50 efficiency.
		return Clamp01(team.ContestedPossessionRate*0.5 + team.Inside50Efficiency*0.5)
	case VenueWide:
		// Wide expanses favor clearance chains and disposal volume.
		return Clamp01(team.ClearanceRate*0.4 + team.DisposalRate*0.6)
	default:
		return Clamp01((team.Inside50Efficiency + team.ClearanceRate + team.ContestedPossessionRate) / 3.0)
	}
}

// PlayerAvailabilityAdjust returns effective team strength 0-1 after absences.
func PlayerAvailabilityAdjust(impacts []PlayerImpact) float64 {
	if len(impacts) == 0 {
		return 1.0
	}
	var missingWeight float64
	for _, p := range impacts {
		if p.Available {
			continue
		}
		w := p.ImpactScore
		switch p.Role {
		case RoleMidfielder:
			w *= 1.1
		case RoleKeyForward, RoleKeyDefender:
			w *= 1.2
		}
		missingWeight += Clamp01(w)
	}
	strength := 1.0 - missingWeight*0.15
	return Clamp01(strength)
}

// FormScore converts W/L record to 0-1.
func FormScore(wins, losses int) float64 {
	total := wins + losses
	if total == 0 {
		return 0.5
	}
	return Clamp01(float64(wins) / float64(total))
}

// H2HHomeRate returns home team's historical win rate vs opponent.
func H2HHomeRate(home TeamStats) float64 {
	if home.H2HGamesVsOpponent == 0 {
		return 0.5
	}
	return Clamp01(float64(home.H2HWinsVsOpponent) / float64(home.H2HGamesVsOpponent))
}
