package scanner

import (
	"fmt"
	"time"

	"github.com/ym/fxtrade/internal/oanda"
)

type OpeningRange struct {
	High        float64
	Low         float64
	RangePips   float64
	SessionOpen time.Time
	CandleCount int
}

func candleTime(c oanda.Candle) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, c.Time)
	if err != nil {
		return time.Parse(time.RFC3339, c.Time)
	}
	return t, err
}

func candleOHLC(c oanda.Candle) (o, h, l, cl float64, err error) {
	o, err = oanda.ParsePrice(c.Mid.O)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	h, err = oanda.ParsePrice(c.Mid.H)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	l, err = oanda.ParsePrice(c.Mid.L)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	cl, err = oanda.ParsePrice(c.Mid.C)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return o, h, l, cl, nil
}

func BuildOpeningRange(candles []oanda.Candle, sessionOpen time.Time, count int, pipSize float64) (OpeningRange, bool, string) {
	if count <= 0 {
		count = 2
	}
	if pipSize <= 0 {
		pipSize = oanda.PipSize
	}

	var selected []oanda.Candle
	for _, c := range candles {
		if !c.Complete {
			continue
		}
		t, err := candleTime(c)
		if err != nil {
			continue
		}
		if t.Before(sessionOpen) {
			continue
		}
		selected = append(selected, c)
		if len(selected) >= count {
			break
		}
	}
	if len(selected) < count {
		return OpeningRange{}, false, fmt.Sprintf("need %d complete M15 candles after session open, have %d", count, len(selected))
	}

	high := -1.0
	low := 1e18
	for _, c := range selected {
		_, h, l, _, err := candleOHLC(c)
		if err != nil {
			return OpeningRange{}, false, "parse candle OHLC"
		}
		if h > high {
			high = h
		}
		if l < low {
			low = l
		}
	}
	if high <= low {
		return OpeningRange{}, false, "degenerate opening range"
	}

	rangePips := (high - low) / pipSize
	return OpeningRange{
		High:        high,
		Low:         low,
		RangePips:   rangePips,
		SessionOpen: sessionOpen,
		CandleCount: len(selected),
	}, true, ""
}

func H1TrendBias(candles []oanda.Candle) int {
	if len(candles) < 2 {
		return 0
	}
	first := candles[0]
	last := candles[len(candles)-1]
	fo, err1 := oanda.ParsePrice(first.Mid.O)
	lc, err2 := oanda.ParsePrice(last.Mid.C)
	if err1 != nil || err2 != nil {
		return 0
	}
	switch {
	case lc > fo*1.0001:
		return 1
	case lc < fo*0.9999:
		return -1
	default:
		return 0
	}
}
