// Package afl implements the AFLPulse value-betting engine.
package afl

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const floatEpsilon = 1e-9

// TeamID identifies an AFL club.
type TeamID string

func (t TeamID) String() string { return string(t) }

func (t TeamID) Validate() error {
	if strings.TrimSpace(string(t)) == "" {
		return fmt.Errorf("empty team id")
	}
	return nil
}

// VenueID identifies an AFL ground.
type VenueID string

func (v VenueID) String() string { return string(v) }

func (v VenueID) Validate() error {
	if strings.TrimSpace(string(v)) == "" {
		return fmt.Errorf("empty venue id")
	}
	return nil
}

// VenueDimension classifies ground playing dimensions.
type VenueDimension string

const (
	VenueNarrow   VenueDimension = "narrow"
	VenueStandard VenueDimension = "standard"
	VenueWide     VenueDimension = "wide"
)

// PlayerRole classifies structural player impact.
type PlayerRole string

const (
	RoleMidfielder  PlayerRole = "midfielder"
	RoleKeyForward  PlayerRole = "key_forward"
	RoleKeyDefender PlayerRole = "key_defender"
)

// TeamStats holds rolling team performance indicators.
type TeamStats struct {
	TeamID                  TeamID
	FormWinsLast10          int
	FormLossesLast10        int
	Inside50Efficiency      float64 // 0-1
	ClearanceRate           float64 // per game normalized 0-1
	ContestedPossessionRate float64 // 0-1
	DisposalRate            float64 // 0-1 normalized
	H2HWinsVsOpponent       int
	H2HGamesVsOpponent      int
}

// VenueProfile describes ground characteristics and home advantage data.
type VenueProfile struct {
	ID              VenueID
	Name            string
	Dimension       VenueDimension
	Latitude        float64
	Longitude       float64
	Interstate      bool
	HomeWinRates    map[TeamID]float64 // team-specific home win rate at this venue
	PrimaryHomeTeam TeamID
}

// PlayerImpact describes a single player's structural weight.
type PlayerImpact struct {
	PlayerID    string
	TeamID      TeamID
	Role        PlayerRole
	ImpactScore float64 // 0-1 premium weight
	Available   bool
}

// PlayerAvailabilityMatrix tracks availability per team for a match.
type PlayerAvailabilityMatrix struct {
	Home []PlayerImpact
	Away []PlayerImpact
}

// WeatherMetrics captures match-day conditions.
type WeatherMetrics struct {
	RainMM               float64
	WindKPH              float64
	Humidity             float64
	ContestFavorability  float64 // derived 0-1
	TotalPointsFactor    float64 // derived multiplier for expected scoring
}

// MatchDayContext aggregates all match metadata for feature engineering.
type MatchDayContext struct {
	EventID       string
	HomeTeam      TeamID
	AwayTeam      TeamID
	Venue         VenueProfile
	Kickoff       time.Time
	TravelKM      float64
	TravelHours   float64
	CrowdEstimate int
	Weather       WeatherMetrics
	Players       PlayerAvailabilityMatrix
	HomeStats     TeamStats
	AwayStats     TeamStats
}

// MarketOdds is a single bookmaker outcome for one side of a match.
type MarketOdds struct {
	EventID     string
	HomeTeam    TeamID
	AwayTeam    TeamID
	Bookmaker   string
	Team        TeamID
	DecimalOdds float64
	UpdatedAt   time.Time
	Kickoff     time.Time
}

func (m MarketOdds) ImpliedProb() float64 {
	if m.DecimalOdds <= 1.0 {
		return 0
	}
	return 1.0 / m.DecimalOdds
}

func (m MarketOdds) Validate() error {
	if err := m.HomeTeam.Validate(); err != nil {
		return fmt.Errorf("home team: %w", err)
	}
	if err := m.AwayTeam.Validate(); err != nil {
		return fmt.Errorf("away team: %w", err)
	}
	if err := m.Team.Validate(); err != nil {
		return fmt.Errorf("outcome team: %w", err)
	}
	if m.DecimalOdds <= 1.0+floatEpsilon {
		return fmt.Errorf("invalid decimal odds %.4f", m.DecimalOdds)
	}
	if math.IsNaN(m.DecimalOdds) || math.IsInf(m.DecimalOdds, 0) {
		return fmt.Errorf("non-finite decimal odds")
	}
	return nil
}

// FeatureVector is the fixed-length input to the prediction model.
type FeatureVector struct {
	Values []float64
}

// Feature index constants — stable contract for offline training export.
const (
	FeatHomeForm = iota
	FeatAwayForm
	FeatHomeInside50
	FeatAwayInside50
	FeatHomeClearance
	FeatAwayClearance
	FeatHomeContested
	FeatAwayContested
	FeatHomeDisposal
	FeatAwayDisposal
	FeatH2HHomeRate
	FeatVenueStyleHome
	FeatVenueStyleAway
	FeatTravelFatigue
	FeatCrowdBias
	FeatWeatherContest
	FeatWeatherTotalPts
	FeatHomePlayerAvail
	FeatAwayPlayerAvail
	FeatHomeAdvantage
	FeatureCount
)

// ValueBet is a confirmed positive-EV opportunity.
type ValueBet struct {
	EventID      string
	HomeTeam     TeamID
	AwayTeam     TeamID
	Team         TeamID
	Bookmaker    string
	ModelProb    float64
	ImpliedProb  float64
	DecimalOdds  float64
	EV           float64
	EdgePct      float64
	Reasons      []string
	Kickoff      time.Time
	IsHomePick   bool
}

// ComputeEV returns expected value: (modelProb * decimalOdds) - 1.0
func ComputeEV(modelProb, decimalOdds float64) float64 {
	if decimalOdds <= 1.0 || modelProb < 0 || modelProb > 1 {
		return -1
	}
	return (modelProb * decimalOdds) - 1.0
}

// Clamp01 restricts v to [0, 1].
func Clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// SafeDiv returns a/b with zero guard.
func SafeDiv(a, b float64) float64 {
	if math.Abs(b) < floatEpsilon {
		return 0
	}
	return a / b
}
