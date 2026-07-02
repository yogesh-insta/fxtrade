package execution

import (
	"fmt"
	"strconv"

	"github.com/ym/fxtrade/internal/oanda"
)

const (
	DirectionLong  = "LONG"
	DirectionShort = "SHORT"
)

type MarketOrderParams struct {
	Instrument string
	Direction  string
	Units      int64
	StopLoss   float64
	TakeProfit *float64
}

func BuildMarketOrder(p MarketOrderParams) (oanda.CreateOrderRequest, error) {
	if p.Instrument == "" {
		p.Instrument = oanda.DefaultInstrument
	}
	if p.Units <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("units must be positive")
	}
	if p.StopLoss <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("stop-loss price is required")
	}

	units := p.Units
	switch p.Direction {
	case DirectionLong:
	case DirectionShort:
		units = -units
	default:
		return oanda.CreateOrderRequest{}, fmt.Errorf("direction must be LONG or SHORT")
	}

	order := oanda.CreateOrderRequest{
		Order: oanda.OrderSpec{
			Type:         oanda.OrderTypeMarket,
			Instrument:   p.Instrument,
			Units:        strconv.FormatInt(units, 10),
			TimeInForce:  oanda.TimeInForceFOK,
			PositionFill: oanda.PositionFillDefault,
			StopLossOnFill: &oanda.OnFillStopLoss{
				Price:       oanda.FormatPrice(p.StopLoss),
				TimeInForce: oanda.TimeInForceGTC,
			},
		},
	}

	if p.TakeProfit != nil {
		order.Order.TakeProfitOnFill = &oanda.OnFillTakeProfit{
			Price:       oanda.FormatPrice(*p.TakeProfit),
			TimeInForce: oanda.TimeInForceGTC,
		}
	}

	return order, nil
}
