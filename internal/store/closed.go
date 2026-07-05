package store

import (
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

// ClosedTradeRow builds a SQLite trade row from close context and optional entry meta.
func ClosedTradeRow(instrument, tradeID, correlationID string, pl float64, meta TradeMeta, hasMeta bool) sqlite.Trade {
	if correlationID == "" && hasMeta {
		correlationID = meta.CorrelationID
	}
	row := sqlite.Trade{
		ClosedAt:      time.Now().UTC(),
		Instrument:    instrument,
		TradeID:       tradeID,
		CorrelationID: correlationID,
		RealizedPL:    pl,
		NetPL:         pl,
	}
	if instrument == "" && hasMeta {
		instrument = meta.Instrument
		row.Instrument = instrument
	}
	if hasMeta {
		row.Direction = meta.Direction
		row.SignalPrice = meta.SignalPrice
		row.FillPrice = meta.FillPrice
		row.StopLoss = meta.StopLoss
		row.TakeProfit = meta.TakeProfit
		row.Units = meta.Units
		row.SlippagePct = SlippagePct(meta.SignalPrice, meta.FillPrice)
	}
	return row
}
