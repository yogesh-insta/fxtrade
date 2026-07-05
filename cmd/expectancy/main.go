package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"os"

	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/report"
)

func main() {
	tradesPath := flag.String("trades", "logs/fx_sentiment/trades.jsonl", "path to trades.jsonl")
	flag.Parse()

	trades, err := journal.ReadTrades(*tradesPath)
	if err != nil {
		slog.Error("read trades", "error", err)
		os.Exit(1)
	}

	r := report.ExpectancyFromTrades(trades)
	fmtJSON, _ := json.MarshalIndent(r, "", "  ")
	slog.Info("expectancy report", "summary", r.String())
	os.Stdout.Write(fmtJSON)
	os.Stdout.Write([]byte("\n"))
}
