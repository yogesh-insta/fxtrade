package backtest

import (
	"fmt"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/sentiment"
	"github.com/ym/fxtrade/internal/strategy"
)

type Result struct {
	Days           int
	RangeDays      int
	TrendDays      int
	StandAsideDays int
	Gate0Blocked   int
	TrendLongOK    int
	TrendShortOK   int
}

func Run(candles []oanda.Candle, weekly []oanda.Candle, h4 []oanda.Candle, cfg *config.Config) Result {
	if len(candles) < 30 {
		return Result{}
	}

	var res Result
	rangeCfg := cfg.RangeMode
	trendCfg := cfg.TrendMode
	riskCfg := cfg.Risk

	for i := 30; i < len(candles); i++ {
		dailySlice := candles[:i+1]
		wSlice := weeklySubset(weekly, dailySlice[len(dailySlice)-1].Time)
		h4Slice := h4Subset(h4, dailySlice[len(dailySlice)-1].Time)

		close, _ := oanda.ParsePrice(dailySlice[len(dailySlice)-1].Mid.C)
		dHighs, dLows, dCloses := ohlc(dailySlice)
		wCloses := closes(wSlice)
		h4Closes := closes(h4Slice)

		snap := market.Snapshot{
			Mid:             close,
			Bid:             close - 0.00004,
			Ask:             close + 0.00004,
			SpreadPips:      0.8,
			Daily:           dailySlice,
			Weekly:          wSlice,
			H4:              h4Slice,
			EMA20W:          market.EMA(wCloses, 20),
			EMA50W:          market.EMA(wCloses, 50),
			Week52High:      high(wCloses),
			Week52Low:       low(wCloses),
			ATR14Daily:      market.ATR(dHighs, dLows, dCloses, 14),
			EMA20H4:         market.EMA(h4Closes, 20),
			RSI14H4:         market.RSI(h4Closes, 14),
			LastDailyClose:  close,
			LastH4Close:     last(h4Closes),
			LastWeeklyClose: last(wCloses),
		}

		t, _ := time.Parse(time.RFC3339, dailySlice[len(dailySlice)-1].Time)
		if t.IsZero() {
			t = time.Now()
		}

		band := strategy.DetectRange(snap, rangeCfg)
		mode := strategy.DetectMode(snap, band, trendCfg)
		res.Days++

		switch mode {
		case strategy.ModeRange:
			res.RangeDays++
		case strategy.ModeTrend:
			res.TrendDays++
		default:
			res.StandAsideDays++
		}

		if ok, _ := strategy.CheckMarketConditions(snap, sentiment.Empty(), false, riskCfg, t); !ok {
			res.Gate0Blocked++
			continue
		}

		if mode == strategy.ModeTrend {
			dir, ok, _ := strategy.TrendEntry(snap, band, trendCfg)
			if ok {
				if dir == "LONG" {
					res.TrendLongOK++
				} else {
					res.TrendShortOK++
				}
			}
		}
	}

	return res
}

func (r Result) Summary() string {
	return fmt.Sprintf(
		"days=%d range=%d trend=%d stand_aside=%d gate0_blocked=%d trend_long_signals=%d trend_short_signals=%d",
		r.Days, r.RangeDays, r.TrendDays, r.StandAsideDays, r.Gate0Blocked, r.TrendLongOK, r.TrendShortOK,
	)
}

func weeklySubset(weekly []oanda.Candle, asOf string) []oanda.Candle {
	return subsetBefore(weekly, asOf)
}

func h4Subset(h4 []oanda.Candle, asOf string) []oanda.Candle {
	return subsetBefore(h4, asOf)
}

func subsetBefore(candles []oanda.Candle, asOf string) []oanda.Candle {
	out := make([]oanda.Candle, 0, len(candles))
	for _, c := range candles {
		if c.Time <= asOf {
			out = append(out, c)
		}
	}
	return out
}

func ohlc(candles []oanda.Candle) (highs, lows, closes []float64) {
	for _, c := range candles {
		h, e1 := oanda.ParsePrice(c.Mid.H)
		l, e2 := oanda.ParsePrice(c.Mid.L)
		cl, e3 := oanda.ParsePrice(c.Mid.C)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		highs = append(highs, h)
		lows = append(lows, l)
		closes = append(closes, cl)
	}
	return highs, lows, closes
}

func closes(candles []oanda.Candle) []float64 {
	out := make([]float64, 0, len(candles))
	for _, c := range candles {
		if p, err := oanda.ParsePrice(c.Mid.C); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func high(vals []float64) float64 {
	h, _ := market.HighLow(vals)
	return h
}

func low(vals []float64) float64 {
	_, l := market.HighLow(vals)
	return l
}

func last(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	return vals[len(vals)-1]
}
