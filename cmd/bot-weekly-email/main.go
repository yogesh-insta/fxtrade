package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	endDateStr := flag.String("end-date", "", "report period end UTC date YYYY-MM-DD (exclusive; default: today UTC)")
	printOnly := flag.Bool("print", false, "print email body instead of sending")
	dryRun := flag.Bool("dry-run", false, "alias for -print")
	flag.Parse()

	if *dryRun {
		*printOnly = true
	}

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	now := time.Now().UTC()
	weekStart, weekEnd := report.ReportWeekUTC(now)
	if *endDateStr != "" {
		t, err := time.Parse("2006-01-02", *endDateStr)
		if err != nil {
			slog.Error("parse end-date", "value", *endDateStr, "error", err)
			os.Exit(1)
		}
		weekEnd = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		weekStart = weekEnd.Add(-7 * 24 * time.Hour)
	}

	sections := make([]report.BotWeeklySection, 0, len(report.TradingBotIDs()))
	for _, botID := range report.TradingBotIDs() {
		sections = append(sections, loadBotSection(botID, report.BotDBPath(cfg, botID), weekStart, weekEnd))
	}

	subject, body := report.WeeklyEmail(weekStart, weekEnd, sections)
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
	slog.Info("weekly summary sent",
		"subject", subject,
		"to", cfg.Email.AlertTo,
		"start", weekStart.Format("2006-01-02"),
		"end", weekEnd.Add(-24*time.Hour).Format("2006-01-02"),
	)
}

func loadBotSection(botID, dbPath string, start, end time.Time) report.BotWeeklySection {
	sec := report.BotWeeklySection{BotID: botID, DBPath: dbPath}

	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			sec.Err = fmt.Errorf("database not found at %s", dbPath)
		} else {
			sec.Err = err
		}
		return sec
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		sec.Err = err
		return sec
	}
	defer store.Close()

	pm, err := store.PeriodMetricsUTC(start, end)
	if err != nil {
		sec.Err = err
		return sec
	}
	sec.Period = pm

	all, err := store.AllTimeDayMetrics()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		sec.Err = err
		return sec
	}
	sec.AllTime = all
	return sec
}

func fmtPrint(subject, body string) {
	os.Stdout.WriteString("Subject: " + subject + "\n\n")
	os.Stdout.WriteString(body)
}
