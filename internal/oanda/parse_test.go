package oanda_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/oanda"
)

func TestClientPriceToUpdate(t *testing.T) {
	p := oanda.ClientPrice{
		Instrument: "AUD_USD",
		Time:       "2026-07-02T10:00:00.000000000Z",
		Tradeable:  true,
		Bids:       []oanda.PriceTick{{Price: "0.69000"}},
		Asks:       []oanda.PriceTick{{Price: "0.69008"}},
	}
	tick, err := p.ToUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if tick.Bid != 0.69 || tick.Ask != 0.69008 {
		t.Fatalf("unexpected bid/ask: %v %v", tick.Bid, tick.Ask)
	}
	if tick.Spread <= 0 {
		t.Fatalf("expected positive spread, got %v", tick.Spread)
	}
}

func TestParsePrice(t *testing.T) {
	v, err := oanda.ParsePrice("1.23456")
	if err != nil {
		t.Fatal(err)
	}
	if v != 1.23456 {
		t.Fatalf("got %v", v)
	}
}
