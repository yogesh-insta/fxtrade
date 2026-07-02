//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestOANDAPracticeConnectivity(t *testing.T) {
	path := os.Getenv("FXTRADE_CREDENTIALS")
	if path == "" {
		path = ".credentials"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no .credentials file for integration test")
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summary, err := client.AccountSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Account.ID == "" {
		t.Fatal("empty account id")
	}

	pricing, err := client.Pricing(ctx, oanda.DefaultInstrument)
	if err != nil {
		t.Fatal(err)
	}
	if len(pricing.Prices) == 0 {
		t.Fatal("no prices")
	}

	candles, err := client.Candles(ctx, oanda.DefaultInstrument, "D", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles.Candles) < 5 {
		t.Fatalf("expected candles, got %d", len(candles.Candles))
	}
}

func TestOANDAPendingOrdersList(t *testing.T) {
	path := os.Getenv("FXTRADE_CREDENTIALS")
	if path == "" {
		path = ".credentials"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no .credentials file")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err = client.PendingOrders(ctx)
	if err != nil {
		t.Fatal(err)
	}
}
