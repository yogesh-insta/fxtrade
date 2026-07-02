package market

import (
	"context"
	"fmt"

	"github.com/ym/fxtrade/internal/oanda"
)

type Snapshot struct {
	Instrument   string
	Bid          float64
	Ask          float64
	SpreadPips   float64
	Mid          float64
	Weekly       []oanda.Candle
	Daily        []oanda.Candle
	H4           []oanda.Candle
	EMA20W       float64
	EMA50W       float64
	Week52High   float64
	Week52Low    float64
	ATR14Daily   float64
	EMA20H4      float64
	RSI14H4      float64
	LastDailyClose float64
	LastH4Close    float64
	LastWeeklyClose float64
}

func LoadSnapshot(ctx context.Context, client *oanda.Client, instrument string) (Snapshot, error) {
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}

	pricing, err := client.Pricing(ctx, instrument)
	if err != nil {
		return Snapshot{}, err
	}
	if len(pricing.Prices) == 0 {
		return Snapshot{}, fmt.Errorf("no pricing for %s", instrument)
	}
	tick, err := pricing.Prices[0].ToUpdate()
	if err != nil {
		return Snapshot{}, err
	}

	weekly, err := client.Candles(ctx, instrument, "W", 52)
	if err != nil {
		return Snapshot{}, err
	}
	daily, err := client.Candles(ctx, instrument, "D", 100)
	if err != nil {
		return Snapshot{}, err
	}
	h4, err := client.Candles(ctx, instrument, "H4", 60)
	if err != nil {
		return Snapshot{}, err
	}

	wCloses := candleCloses(weekly.Candles)
	dHighs, dLows, dCloses := candleOHLC(daily.Candles)
	h4Closes := candleCloses(h4.Candles)

	snap := Snapshot{
		Instrument: instrument,
		Bid:        tick.Bid,
		Ask:        tick.Ask,
		SpreadPips: oanda.SpreadPips(tick.Spread),
		Mid:        (tick.Bid + tick.Ask) / 2,
		Weekly:     weekly.Candles,
		Daily:      daily.Candles,
		H4:         h4.Candles,
		EMA20W:     EMA(wCloses, 20),
		EMA50W:     EMA(wCloses, 50),
		ATR14Daily: ATR(dHighs, dLows, dCloses, 14),
		EMA20H4:    EMA(h4Closes, 20),
		RSI14H4:    RSI(h4Closes, 14),
	}
	snap.Week52High, snap.Week52Low = HighLow(wCloses)
	if len(dCloses) > 0 {
		snap.LastDailyClose = dCloses[len(dCloses)-1]
	}
	if len(h4Closes) > 0 {
		snap.LastH4Close = h4Closes[len(h4Closes)-1]
	}
	if len(wCloses) > 0 {
		snap.LastWeeklyClose = wCloses[len(wCloses)-1]
	}
	return snap, nil
}
