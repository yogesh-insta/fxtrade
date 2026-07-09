package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

const smtpTimeout = 30 * time.Second

type Notifier interface {
	Send(ctx context.Context, subject, body string)
}

type digestNotifier interface {
	SendDigest(ctx context.Context, digestKey, subject, body string)
}

// SendDigest delivers routine status emails, respecting min_interval_minutes when configured.
func SendDigest(n Notifier, ctx context.Context, digestKey, subject, body string) {
	if routineEmailsSuppressed(n) {
		slog.Info("routine email suppressed", "subject", subject, "digest_key", digestKey)
		return
	}
	if d, ok := n.(digestNotifier); ok {
		d.SendDigest(ctx, digestKey, subject, body)
		return
	}
	n.Send(ctx, subject, body)
}

// SendRoutine delivers operational emails that are not tied to a trade event.
// Suppressed when trade-only or daily-summary-only email mode is enabled.
func SendRoutine(n Notifier, ctx context.Context, subject, body string) {
	if routineEmailsSuppressed(n) {
		slog.Info("routine email suppressed", "subject", subject)
		return
	}
	n.Send(ctx, subject, body)
}

// WithTradeOnly wraps a notifier so routine digests and SendRoutine calls are
// suppressed while trade-event Send calls still deliver.
func WithTradeOnly(n Notifier, tradeOnly bool) Notifier {
	if !tradeOnly {
		return n
	}
	return &tradeGatedNotifier{inner: n, tradeOnly: true}
}

// WithDailySummaryOnly wraps a notifier so all bot emails are suppressed except
// cron-driven summaries that use an unwrapped notifier.
func WithDailySummaryOnly(n Notifier, active bool) Notifier {
	if !active {
		return n
	}
	return &dailySummaryOnlyNotifier{inner: n, active: true}
}

type tradeGatedNotifier struct {
	inner     Notifier
	tradeOnly bool
}

func (g *tradeGatedNotifier) Send(ctx context.Context, subject, body string) {
	g.inner.Send(ctx, subject, body)
}

func (g *tradeGatedNotifier) SendDigest(ctx context.Context, digestKey, subject, body string) {
	slog.Info("routine email suppressed", "subject", subject, "digest_key", digestKey)
}

type dailySummaryOnlyNotifier struct {
	inner  Notifier
	active bool
}

func (d *dailySummaryOnlyNotifier) Send(ctx context.Context, subject, body string) {
	if d.active {
		slog.Info("email suppressed (daily_summary_only)", "subject", subject)
		return
	}
	d.inner.Send(ctx, subject, body)
}

func (d *dailySummaryOnlyNotifier) SendDigest(ctx context.Context, digestKey, subject, body string) {
	if d.active {
		slog.Info("email suppressed (daily_summary_only)", "subject", subject, "digest_key", digestKey)
		return
	}
	if dig, ok := d.inner.(digestNotifier); ok {
		dig.SendDigest(ctx, digestKey, subject, body)
		return
	}
	d.inner.Send(ctx, subject, body)
}

func routineEmailsSuppressed(n Notifier) bool {
	switch g := n.(type) {
	case *tradeGatedNotifier:
		return g.tradeOnly
	case *dailySummaryOnlyNotifier:
		return g.active
	}
	return false
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
	var base Notifier = LogNotifier{}
	if cfg.Enabled() {
		base = &EmailNotifier{cfg: cfg}
	}
	if cfg.MinIntervalMinutes > 0 {
		return &rateLimitedNotifier{
			inner:       base,
			minInterval: time.Duration(cfg.MinIntervalMinutes) * time.Minute,
		}
	}
	return base
}

type rateLimitedNotifier struct {
	inner       Notifier
	minInterval time.Duration
	mu          sync.Mutex
	lastDigest  map[string]time.Time
}

func (r *rateLimitedNotifier) Send(ctx context.Context, subject, body string) {
	r.inner.Send(ctx, subject, body)
}

func (r *rateLimitedNotifier) SendDigest(ctx context.Context, digestKey, subject, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lastDigest == nil {
		r.lastDigest = make(map[string]time.Time)
	}
	if last, ok := r.lastDigest[digestKey]; ok && time.Since(last) < r.minInterval {
		slog.Info("email digest suppressed",
			"key", digestKey,
			"subject", subject,
			"retry_in", (r.minInterval - time.Since(last)).Round(time.Second),
		)
		return
	}
	r.lastDigest[digestKey] = time.Now()
	r.inner.Send(ctx, subject, body)
}

func (e *EmailNotifier) Send(ctx context.Context, subject, body string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()

	if err := e.send(ctx, subject, body); err != nil {
		slog.Error("email send failed", "subject", subject, "error", err)
		return
	}
	slog.Info("email sent", "subject", subject, "to", e.cfg.AlertTo)
}

func (e *EmailNotifier) send(ctx context.Context, subject, body string) error {
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
	return sendMail(ctx, addr, auth, e.cfg.Username, []string{e.cfg.AlertTo}, []byte(msg))
}

func sendMail(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
