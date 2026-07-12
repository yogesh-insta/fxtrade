package execution

import (
	"fmt"
	"strconv"

	"github.com/ym/fxtrade/internal/oanda"
)

type LimitOrderParams struct {
	Instrument     string
	Direction      string
	Units          int64
	Price          float64
	StopLoss       float64
	TakeProfit     *float64
	TimeInForce    string
	ClientOrderID  string
	ClientOrderTag string
}

func BuildLimitOrder(p LimitOrderParams) (oanda.CreateOrderRequest, error) {
	if p.Instrument == "" {
		p.Instrument = oanda.DefaultInstrument
	}
	if p.Units <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("units must be positive")
	}
	if p.Price <= 0 || p.StopLoss <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("price and stop-loss are required")
	}
	tif := p.TimeInForce
	if tif == "" {
		tif = oanda.TimeInForceGTC
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
			Type:         oanda.OrderTypeLimit,
			Instrument:   p.Instrument,
			Units:        strconv.FormatInt(units, 10),
			Price:        oanda.FormatPrice(p.Price),
			TimeInForce:  tif,
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
	if p.ClientOrderID != "" || p.ClientOrderTag != "" {
		order.Order.ClientExtensions = &oanda.ClientExtensions{
			ID:  p.ClientOrderID,
			Tag: p.ClientOrderTag,
		}
	}
	return order, nil
}
