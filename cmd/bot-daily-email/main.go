package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dateStr := flag.String("date", "", "report UTC date YYYY-MM-DD (default: previous UTC day, or today when -same-day)")
	sameDay := flag.Bool("same-day", true, "report current UTC day (use with evening cron after force-flat)")
	printOnly := flag.Bool("print", false, "print email body instead of sending")
	navPath := flag.String("nav-snapshots", "data/account_nav_snapshots.json", "path to NAV snapshot file for daily account P/L")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	now := time.Now().UTC()
	reportDate := report.ReportDayUTC(now, *sameDay)
	if *dateStr != "" {
		t, err := time.Parse("2006-01-02", *dateStr)
		if err != nil {
			slog.Error("parse date", "value", *dateStr, "error", err)
			os.Exit(1)
		}
		reportDate = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}

	ctx := context.Background()
	acct := report.AccountDailyContext{}
	navStore := report.NewNavSnapshotStore(*navPath)

	if cfg.OANDA.Token != "" {
		client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
		acct.Reconcile = report.ReconcileAllBots(ctx, cfg, client)
		if acct.Reconcile.TotalReconciled > 0 {
			slog.Info("reconciled trades before daily email", "count", acct.Reconcile.TotalReconciled)
		}
		if cfg.OANDA.InitialCapitalAUD > 0 {
			if navSummary, err := client.AccountSummary(ctx); err == nil {
				nav, _ := oanda.ParsePrice(navSummary.Account.NAV)
				unrealized, _ := oanda.ParsePrice(navSummary.Account.UnrealizedPL)
				currency := navSummary.Account.Currency
				if currency == "" {
					currency = "AUD"
				}
				acct.NAV = nav
				acct.Currency = currency
				acct.Baseline = cfg.OANDA.InitialCapitalAUD
				acct.TotalPL = nav - acct.Baseline
				acct.UnrealizedPL = unrealized
				if prior, priorDate, ok := navStore.PriorNAV(reportDate, currency); ok {
					acct.DailyPLKnown = true
					acct.PriorNAV = prior
					acct.PriorDate = priorDate
					acct.DailyPL = nav - prior
				}
			} else {
				slog.Warn("account summary for daily email", "error", err)
			}
		}
	}

	sections := make([]report.BotDailySection, 0, len(report.TradingBotIDs()))
	for _, botID := range report.TradingBotIDs() {
		sections = append(sections, loadBotSection(botID, report.BotDBPath(cfg, botID), reportDate))
	}

	analysis := report.FormatDailyTweaks(cfg, now, reportDate)
	if note := reconcileAnalysisNote(acct.Reconcile); note != "" {
		analysis = note + "\n" + analysis
	}

	subject, body := report.DailyEmail(reportDate, sections, acct, analysis)
	if *printOnly {
		fmtPrint(subject, body)
		return
	}

	if !cfg.Email.Enabled() {
		slog.Error("email not configured (need smtp_host, username, password, alert_to)")
		os.Exit(1)
	}

	n := notify.New(cfg.Email)
	n.Send(ctx, subject, body)
	if acct.NAV > 0 {
		if err := navStore.Record(reportDate, acct.NAV, acct.Currency); err != nil {
			slog.Warn("nav snapshot save failed", "error", err)
		}
	}
	slog.Info("daily summary sent", "subject", subject, "to", cfg.Email.AlertTo, "date", reportDate.Format("2006-01-02"))
}

func reconcileAnalysisNote(summary report.ReconcileSummary) string {
	if summary.AttemptedTotal() == 0 {
		return ""
	}
	msg := fmt.Sprintf("Reconciled %d trade(s) from OANDA before this report.", summary.TotalReconciled)
	if missing := summary.StillMissingTotal(); missing > 0 {
		msg += fmt.Sprintf(" %d row(s) still missing P/L — often older trades or trade IDs not in the last 30 days of OANDA history.", missing)
	}
	return msg
}

func loadBotSection(botID, dbPath string, day time.Time) report.BotDailySection {
	sec := report.BotDailySection{BotID: botID, DBPath: dbPath}

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

	dm, err := store.DayMetricsUTC(day)
	if err != nil {
		sec.Err = err
		return sec
	}
	sec.Day = dm

	all, err := store.AllTimeDayMetrics()
	if err != nil {
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
