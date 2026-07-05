package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/bots/btc_cfd"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dbPath := flag.String("db", "", "SQLite trades.db (default from btc_cfd config)")
	dateStr := flag.String("date", "", "report UTC date YYYY-MM-DD (default: previous UTC day)")
	printOnly := flag.Bool("print", false, "print email body instead of sending")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	path := *dbPath
	if path == "" {
		path = cfg.BtcCfd.DBPath
	}

	store, err := sqlite.Open(path)
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	reportDate := btc_cfd.ReportDayUTC(time.Now().UTC())
	if *dateStr != "" {
		t, err := time.Parse("2006-01-02", *dateStr)
		if err != nil {
			slog.Error("parse date", "value", *dateStr, "error", err)
			os.Exit(1)
		}
		reportDate = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}

	day, err := store.DayMetricsUTC(reportDate)
	if err != nil {
		slog.Error("day metrics", "error", err)
		os.Exit(1)
	}
	all, err := store.AllTimeDayMetrics()
	if err != nil {
		slog.Error("all-time metrics", "error", err)
		os.Exit(1)
	}

	var open *btc_cfd.OpenPosition
	if cfg.OANDA.Token != "" {
		client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
		open, err = btc_cfd.FetchOpenPosition(context.Background(), client, cfg.BtcCfd.Instrument)
		if err != nil {
			slog.Warn("open position lookup failed", "error", err)
		}
	}

	subject, body := btc_cfd.DailyEmail(reportDate, day, all, open)
	if *printOnly {
		fmtPrint(subject, body)
		return
	}

	if !cfg.Email.Enabled() {
		slog.Error("email not configured (need smtp_host, username, password, alert_to)")
		os.Exit(1)
	}

	n := notify.New(cfg.Email)
	n.Send(context.Background(), subject, body)
	slog.Info("daily summary sent", "subject", subject, "to", cfg.Email.AlertTo, "date", reportDate.Format("2006-01-02"))
}

func fmtPrint(subject, body string) {
	os.Stdout.WriteString("Subject: " + subject + "\n\n")
	os.Stdout.WriteString(body)
}
