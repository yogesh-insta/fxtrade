package oanda

import "testing"

func TestTradeDirection(t *testing.T) {
	if TradeDirection(62500) != "LONG" {
		t.Fatal("expected LONG")
	}
	if TradeDirection(-62500) != "SHORT" {
		t.Fatal("expected SHORT")
	}
}

func TestEstimateNotionalUSD(t *testing.T) {
	usd, ok := EstimateNotionalUSD("GBP_USD", 62500, 1.33890)
	if !ok {
		t.Fatal("expected ok")
	}
	want := 62500.0 * 1.33890
	if usd < want-0.01 || usd > want+0.01 {
		t.Fatalf("got %.2f want %.2f", usd, want)
	}
	usd, ok = EstimateNotionalUSD("USD_JPY", 625, 161.90)
	if !ok || usd != 625 {
		t.Fatalf("USD base: got %.2f ok=%v", usd, ok)
	}
}

func TestClosedTradeInstrument(t *testing.T) {
	txs := []Transaction{
		{
			Instrument: "GBP_USD",
			TradesClosed: []struct {
				TradeID    string `json:"tradeID"`
				Units      string `json:"units"`
				RealizedPL string `json:"realizedPL"`
			}{
				{TradeID: "42", Units: "1000", RealizedPL: "1.50"},
			},
		},
	}
	inst, ok := ClosedTradeInstrument(txs, "42")
	if !ok || inst != "GBP_USD" {
		t.Fatalf("got %q ok=%v", inst, ok)
	}
}
