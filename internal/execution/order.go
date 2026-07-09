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
	Instrument     string
	Direction      string
	Units          int64
	UnitsStr         string // fractional units (e.g. BTC); takes precedence over Units when set
	StopLoss       float64
	TakeProfit     *float64
	ClientOrderID  string
	ClientOrderTag string
}

func BuildMarketOrder(p MarketOrderParams) (oanda.CreateOrderRequest, error) {
	if p.Instrument == "" {
		p.Instrument = oanda.DefaultInstrument
	}
	if p.UnitsStr == "" && p.Units <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("units must be positive")
	}
	if p.StopLoss <= 0 {
		return oanda.CreateOrderRequest{}, fmt.Errorf("stop-loss price is required")
	}

	unitsField, err := signedOrderUnits(p.Direction, p.UnitsStr, p.Units)
	if err != nil {
		return oanda.CreateOrderRequest{}, err
	}

	order := oanda.CreateOrderRequest{
		Order: oanda.OrderSpec{
			Type:         oanda.OrderTypeMarket,
			Instrument:   p.Instrument,
			Units:        unitsField,
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

	if p.ClientOrderID != "" || p.ClientOrderTag != "" {
		order.Order.ClientExtensions = &oanda.ClientExtensions{
			ID:  p.ClientOrderID,
			Tag: p.ClientOrderTag,
		}
	}

	return order, nil
}

func signedOrderUnits(direction, unitsStr string, units int64) (string, error) {
	if unitsStr != "" {
		u, err := strconv.ParseFloat(unitsStr, 64)
		if err != nil {
			return "", fmt.Errorf("parse units: %w", err)
		}
		if u <= 0 {
			return "", fmt.Errorf("units must be positive")
		}
		switch direction {
		case DirectionLong:
			return unitsStr, nil
		case DirectionShort:
			return strconv.FormatFloat(-u, 'f', -1, 64), nil
		default:
			return "", fmt.Errorf("direction must be LONG or SHORT")
		}
	}
	switch direction {
	case DirectionLong:
	case DirectionShort:
		units = -units
	default:
		return "", fmt.Errorf("direction must be LONG or SHORT")
	}
	return strconv.FormatInt(units, 10), nil
}
