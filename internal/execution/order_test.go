package execution_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestBuildMarketOrderLong(t *testing.T) {
	tp := 0.70000
	order, err := execution.BuildMarketOrder(execution.MarketOrderParams{
		Instrument: oanda.DefaultInstrument,
		Direction:  execution.DirectionLong,
		Units:      1000,
		StopLoss:   0.68000,
		TakeProfit: &tp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Order.Units != "1000" {
		t.Fatalf("units: %s", order.Order.Units)
	}
	if order.Order.StopLossOnFill == nil {
		t.Fatal("expected stop loss")
	}
	if order.Order.TakeProfitOnFill == nil {
		t.Fatal("expected take profit")
	}
}

func TestBuildMarketOrderShort(t *testing.T) {
	order, err := execution.BuildMarketOrder(execution.MarketOrderParams{
		Direction: execution.DirectionShort,
		Units:     500,
		StopLoss:  0.70000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Order.Units != "-500" {
		t.Fatalf("units: %s", order.Order.Units)
	}
}

func TestBuildMarketOrderRejectsMissingStop(t *testing.T) {
	_, err := execution.BuildMarketOrder(execution.MarketOrderParams{
		Direction: execution.DirectionLong,
		Units:     100,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildMarketOrderFractionalBTC(t *testing.T) {
	order, err := execution.BuildMarketOrder(execution.MarketOrderParams{
		Instrument: "BTC_USD",
		Direction:  execution.DirectionLong,
		UnitsStr:   "0.042",
		StopLoss:   100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Order.Units != "0.042" {
		t.Fatalf("units: %s", order.Order.Units)
	}

	short, err := execution.BuildMarketOrder(execution.MarketOrderParams{
		Instrument: "BTC_USD",
		Direction:  execution.DirectionShort,
		UnitsStr:   "0.042",
		StopLoss:   100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if short.Order.Units != "-0.042" {
		t.Fatalf("units: %s", short.Order.Units)
	}
}
