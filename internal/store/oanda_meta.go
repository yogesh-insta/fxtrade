package store

import (
	"strconv"
	"time"

	"github.com/ym/fxtrade/internal/oanda"
)

// MetaFromTrade extracts entry context from an OANDA open trade.
func MetaFromTrade(t oanda.Trade) TradeMeta {
	units, _ := strconv.ParseInt(t.CurrentUnits, 10, 64)
	direction := "LONG"
	if units < 0 {
		direction = "SHORT"
		units = -units
	}
	fill, _ := oanda.ParsePrice(t.Price)
	var sl, tp float64
	if t.StopLossOrder != nil {
		sl, _ = oanda.ParsePrice(t.StopLossOrder.Price)
	}
	if t.TakeProfitOrder != nil {
		tp, _ = oanda.ParsePrice(t.TakeProfitOrder.Price)
	}
	corr := ""
	if t.ClientExtensions != nil {
		corr = t.ClientExtensions.ID
	}
	openAt := time.Now().UTC()
	if t.OpenTime != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, t.OpenTime); err == nil {
			openAt = parsed.UTC()
		}
	}
	return TradeMeta{
		Instrument:    t.Instrument,
		CorrelationID: corr,
		Direction:     direction,
		FillPrice:     fill,
		SignalPrice:   fill,
		StopLoss:      sl,
		TakeProfit:    tp,
		Units:         units,
		OpenedAt:      openAt,
	}
}
