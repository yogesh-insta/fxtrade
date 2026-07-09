package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/report"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	botID := flag.String("bot", "", "bot id (default: all enabled bots)")
	installRoot := flag.String("root", "", "optional install root (e.g. /opt/fxtrade) to resolve relative db paths")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if *installRoot != "" {
		chdirTo(*installRoot)
	}

	now := time.Now().UTC()
	bots := cfg.EnabledBots()
	if *botID != "" {
		bots = []string{config.NormalizeBotID(*botID)}
	}
	if len(bots) == 0 {
		bots = report.TradingBotIDs()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "fxtrade bot analysis (as of %s UTC)\n\n", now.Format("2006-01-02 15:04"))

	for _, id := range bots {
		a := report.AnalyzeBot(cfg, id, now)
		b.WriteString(report.FormatAnalysis(a))
	}

	fmt.Print(b.String())
}

func chdirTo(root string) {
	if err := os.Chdir(root); err != nil {
		slog.Warn("chdir", "root", root, "error", err)
	}
}
