package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

type silentNotifier struct{}

func (silentNotifier) Send(context.Context, string, string) {}

var _ notify.Notifier = silentNotifier{}

func TestSeedOpenedTradeDetectsBlinkClose(t *testing.T) {
	var mu sync.Mutex
	openTrades := []oanda.Trade{} // empty: already stopped out before poll
	txs := []oanda.Transaction{{
		ID:   "349",
		Type: "ORDER_FILL",
		TradesClosed: []struct {
			TradeID    string `json:"tradeID"`
			Units      string `json:"units"`
			RealizedPL string `json:"realizedPL"`
		}{{
			TradeID:    "346",
			Units:      "625",
			RealizedPL: "-75.5367",
		}},
		Price: "58.340",
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v3/accounts/acc/openTrades":
			_ = json.NewEncoder(w).Encode(oanda.OpenTradesResponse{Trades: openTrades})
		case r.URL.Path == "/v3/accounts/acc/transactions":
			_ = json.NewEncoder(w).Encode(oanda.TransactionsResponse{Transactions: txs})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := oanda.NewClient(srv.URL, "acc", "token")
	rm := risk.NewManager(config.RiskConfig{})
	mon := New(client, rm, silentNotifier{}, time.Second)

	var closedID string
	var closedPL float64
	mon.SetOnTradeClosed(func(tradeID, _ string, pl float64) {
		closedID = tradeID
		closedPL = pl
	})

	// Executor fill seeds known — trade never appears in openTrades (21s SL).
	mon.SeedOpenedTrade(oanda.Trade{
		ID:           "346",
		Instrument:   "XAG_USD",
		CurrentUnits: "-625",
		Price:        "58.219",
	})

	mon.poll(context.Background())

	if closedID != "346" {
		t.Fatalf("want close of trade 346, got %q", closedID)
	}
	if closedPL < -75.54 || closedPL > -75.53 {
		t.Fatalf("want ~-75.5367 P/L, got %v", closedPL)
	}
	if mon.OpenCount() != 0 {
		t.Fatalf("want no open trades after blink close, got %d", mon.OpenCount())
	}
}

func TestNoteTradeOpenedStubStillCloses(t *testing.T) {
	var mu sync.Mutex
	txs := []oanda.Transaction{{
		Type: "ORDER_FILL",
		TradesClosed: []struct {
			TradeID    string `json:"tradeID"`
			Units      string `json:"units"`
			RealizedPL string `json:"realizedPL"`
		}{{
			TradeID:    "99",
			Units:      "100",
			RealizedPL: "1.25",
		}},
		Price: "1.10",
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v3/accounts/acc/openTrades":
			_ = json.NewEncoder(w).Encode(oanda.OpenTradesResponse{})
		case r.URL.Path == "/v3/accounts/acc/transactions":
			_ = json.NewEncoder(w).Encode(oanda.TransactionsResponse{Transactions: txs})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := oanda.NewClient(srv.URL, "acc", "token")
	rm := risk.NewManager(config.RiskConfig{})
	mon := New(client, rm, silentNotifier{}, time.Second)

	var closed bool
	mon.SetOnTradeClosed(func(tradeID, _ string, pl float64) {
		if tradeID == "99" && pl == 1.25 {
			closed = true
		}
	})
	mon.NoteTradeOpened("99")
	mon.poll(context.Background())
	if !closed {
		t.Fatal("ID-only NoteTradeOpened should still detect external close")
	}
}

func TestSeedOpenedTradeIdempotent(t *testing.T) {
	rm := risk.NewManager(config.RiskConfig{})
	mon := New(nil, rm, silentNotifier{}, time.Second)
	opens := 0
	mon.SetOnTradeOpened(func(oanda.Trade) { opens++ })

	t0 := oanda.Trade{ID: "1", Instrument: "USD_JPY", CurrentUnits: "625", Price: "162.0"}
	mon.SeedOpenedTrade(t0)
	mon.SeedOpenedTrade(t0)
	if opens != 1 {
		t.Fatalf("onTradeOpened want 1, got %d", opens)
	}
}

func TestOwnerTagSkipsForeignOpen(t *testing.T) {
	var mu sync.Mutex
	opens := []oanda.Trade{{
		ID:           "100",
		Instrument:   "AUD_USD",
		CurrentUnits: "1000",
		Price:        "0.65",
		ClientExtensions: &oanda.ClientExtensions{
			ID:  "universe_scanner:scan-AUD_USD-1",
			Tag: "universe_scanner",
		},
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v3/accounts/acc/openTrades" {
			_ = json.NewEncoder(w).Encode(oanda.OpenTradesResponse{Trades: opens})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := oanda.NewClient(srv.URL, "acc", "token")
	rm := risk.NewManager(config.RiskConfig{})
	mon := New(client, rm, silentNotifier{}, time.Second, "AUD_USD")
	mon.SetOwnerTag("fx_sentiment")

	adopted := 0
	mon.SetOnTradeOpened(func(oanda.Trade) { adopted++ })
	mon.poll(context.Background())
	if adopted != 0 {
		t.Fatalf("fx_sentiment must not adopt scanner trade, got %d opens", adopted)
	}
	if mon.OpenCount() != 0 {
		t.Fatalf("want empty known, got %d", mon.OpenCount())
	}
}

func TestOwnerTagAdoptsOwnOpen(t *testing.T) {
	opens := []oanda.Trade{{
		ID:           "200",
		Instrument:   "EUR_USD",
		CurrentUnits: "1000",
		Price:        "1.10",
		ClientExtensions: &oanda.ClientExtensions{
			ID:  "fx_sentiment:trend-EUR_USD-1",
			Tag: "fx_sentiment",
		},
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v3/accounts/acc/openTrades" {
			_ = json.NewEncoder(w).Encode(oanda.OpenTradesResponse{Trades: opens})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client := oanda.NewClient(srv.URL, "acc", "token")
	rm := risk.NewManager(config.RiskConfig{})
	mon := New(client, rm, silentNotifier{}, time.Second, "EUR_USD")
	mon.SetOwnerTag("fx_sentiment")

	adopted := 0
	mon.SetOnTradeOpened(func(oanda.Trade) { adopted++ })
	mon.poll(context.Background())
	if adopted != 1 {
		t.Fatalf("want adopt own trade once, got %d", adopted)
	}
	if mon.OpenCount() != 1 {
		t.Fatalf("want 1 open, got %d", mon.OpenCount())
	}
}
