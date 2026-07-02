package notify

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"github.com/ym/fxtrade/internal/config"
)

type Notifier interface {
	Send(ctx context.Context, subject, body string)
}

type LogNotifier struct{}

func (LogNotifier) Send(ctx context.Context, subject, body string) {
	_ = ctx
	slog.Info("notification", "subject", subject, "body", strings.TrimSpace(body))
}

type EmailNotifier struct {
	cfg config.EmailConfig
}

func New(cfg config.EmailConfig) Notifier {
	if cfg.Enabled() {
		return &EmailNotifier{cfg: cfg}
	}
	return LogNotifier{}
}

func (e *EmailNotifier) Send(ctx context.Context, subject, body string) {
	_ = ctx
	addr := fmt.Sprintf("%s:%d", e.cfg.SMTPHost, e.cfg.SMTPPort)
	msg := strings.Join([]string{
		fmt.Sprintf("To: %s", e.cfg.AlertTo),
		fmt.Sprintf("From: %s", e.cfg.Username),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	auth := smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.SMTPHost)
	if err := smtp.SendMail(addr, auth, e.cfg.Username, []string{e.cfg.AlertTo}, []byte(msg)); err != nil {
		slog.Error("email send failed", "subject", subject, "error", err)
		return
	}
	slog.Info("email sent", "subject", subject, "to", e.cfg.AlertTo)
}
