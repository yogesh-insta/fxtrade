package oanda

import (
	"fmt"
	"strconv"
	"time"
)

const (
	PipSize     = 0.0001
	MinStopPips = 10 // minimum stop distance for entries (10 pips)
)

// MinStopDistance returns the smallest allowed stop-loss distance in price units.
func MinStopDistance() float64 {
	return MinStopPips * PipSize
}

func FormatPrice(p float64) string {
	return strconv.FormatFloat(p, 'f', 5, 64)
}

func SpreadPips(spread float64) float64 {
	return spread / PipSize
}

func ParsePrice(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse price %q: %w", s, err)
	}
	return v, nil
}

func (p ClientPrice) ToUpdate() (PriceUpdate, error) {
	if len(p.Bids) == 0 || len(p.Asks) == 0 {
		return PriceUpdate{}, fmt.Errorf("missing bid/ask for %s", p.Instrument)
	}
	bid, err := ParsePrice(p.Bids[0].Price)
	if err != nil {
		return PriceUpdate{}, err
	}
	ask, err := ParsePrice(p.Asks[0].Price)
	if err != nil {
		return PriceUpdate{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, p.Time)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, p.Time)
	}
	return PriceUpdate{
		Instrument: p.Instrument,
		Bid:        bid,
		Ask:        ask,
		Spread:     ask - bid,
		Time:       t,
		Tradeable:  p.Tradeable,
	}, nil
}
