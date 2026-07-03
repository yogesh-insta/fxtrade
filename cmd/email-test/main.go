package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

func main() {
	credentials := ".credentials"
	if len(os.Args) > 1 {
		credentials = os.Args[1]
	}
	cfg, err := config.Load(credentials)
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	if !cfg.Email.Enabled() {
		fmt.Println("email not configured (need smtp_host, username, password, alert_to)")
		os.Exit(1)
	}
	n := notify.New(cfg.Email)
	n.Send(context.Background(), "fxtrade: email test", "If you received this, SMTP credentials are working.\n")
	fmt.Println("done — check", cfg.Email.AlertTo)
}
