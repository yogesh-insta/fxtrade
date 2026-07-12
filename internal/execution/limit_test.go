package execution_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestBuildLimitOrderClientExtensions(t *testing.T) {
	tp := 0.70000
	order, err := execution.BuildLimitOrder(execution.LimitOrderParams{
		Instrument:     oanda.DefaultInstrument,
		Direction:      execution.DirectionLong,
		Units:          1000,
		Price:          0.69000,
		StopLoss:       0.68000,
		TakeProfit:     &tp,
		ClientOrderID:  "fx_sentiment:range-buy-AUD_USD-1",
		ClientOrderTag: "fx_sentiment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Order.ClientExtensions == nil {
		t.Fatal("expected client extensions on limit order")
	}
	if order.Order.ClientExtensions.ID != "fx_sentiment:range-buy-AUD_USD-1" {
		t.Fatalf("client id: %s", order.Order.ClientExtensions.ID)
	}
	if order.Order.ClientExtensions.Tag != "fx_sentiment" {
		t.Fatalf("client tag: %s", order.Order.ClientExtensions.Tag)
	}
}
