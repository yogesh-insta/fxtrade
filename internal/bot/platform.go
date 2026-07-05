package bot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/health"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
)

type Options struct {
	HealthAddr string
	// BotFilter limits to one bot (for GCP: one systemd unit per bot).
	BotFilter string
	DryRun    bool
}

type runningBot struct {
	id     string
	handle *Handle
}

// RunPlatform starts all enabled bots concurrently on shared OANDA connectivity.
func RunPlatform(cfg *config.Config, opts Options) error {
	enabled := cfg.EnabledBots()
	if opts.BotFilter != "" {
		enabled = []string{opts.BotFilter}
	}
	if len(enabled) == 0 {
		return fmt.Errorf("no bots enabled — set bots.enabled in .credentials or strategy.mode/strategy.enabled")
	}

	for _, id := range enabled {
		found := false
		for _, reg := range RegisteredIDs() {
			if reg == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown bot %q (registered: %v)", id, RegisteredIDs())
		}
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	stream := oanda.NewStream(cfg.OANDA.StreamBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	notifier := notify.WithTradeOnly(notify.New(cfg.Email), cfg.Notifications.TradeOnlyEmail())
	deps := &Deps{CFG: cfg, Client: client, Stream: stream, Notifier: notifier, DryRun: opts.DryRun}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := startupChecks(ctx, client, notifier, cfg, enabled); err != nil {
		return err
	}

	healthSrv := health.NewServer()
	healthSrv.SetDryRun(opts.DryRun)
	var runners []runningBot
	var mu sync.Mutex

	for _, id := range enabled {
		b, err := New(id, deps)
		if err != nil {
			return err
		}
		meta := b.Meta()
		slog.Info("starting bot", "id", meta.ID, "name", meta.Name)

		handle, err := b.Start(ctx, deps)
		if err != nil {
			return fmt.Errorf("start bot %s: %w", id, err)
		}
		runners = append(runners, runningBot{id: id, handle: handle})
	}

	healthSrv.SetBotStatus(func() []health.BotStatus {
		mu.Lock()
		defer mu.Unlock()
		out := make([]health.BotStatus, 0, len(runners))
		for _, r := range runners {
			st := r.handle.Status()
			out = append(out, health.BotStatus{
				ID:            st.ID,
				Name:          st.Name,
				Description:   st.Description,
				Running:       st.Running,
				Halted:        st.Halted,
				OpenPositions: st.OpenPositions,
				Detail:        st.Detail,
				StartedAt:     st.StartedAt,
			})
		}
		return out
	})
	healthSrv.SetAvailableBots(RegisteredIDs())
	healthSrv.SetActiveBots(enabled)
	healthSrv.SetHaltedCheck(func() bool {
		for _, r := range runners {
			st := r.handle.Status()
			if st.Halted {
				return true
			}
		}
		return false
	})
	healthSrv.SetKillHandler(func() error {
		var first error
		for _, r := range runners {
			if err := r.handle.Halt(); err != nil && first == nil {
				first = err
			}
		}
		return first
	})
	healthSrv.SetOpenPositions(func() int {
		total := 0
		for _, r := range runners {
			total += r.handle.OpenPositions()
		}
		return total
	})

	instruments := mergeInstruments(runners)
	ticks := make(chan oanda.PriceUpdate, 256)
	go func() {
		healthSrv.SetStreamConnected(true)
		defer healthSrv.SetStreamConnected(false)
		if len(instruments) == 0 {
			<-ctx.Done()
			return
		}
		if err := stream.RunPricingStream(ctx, instruments, ticks); err != nil && ctx.Err() == nil {
			slog.Error("pricing stream exited", "error", err)
		}
	}()

	go func() {
		addr := opts.HealthAddr
		if addr == "" {
			addr = ":8080"
		}
		slog.Info("health server listening", "addr", addr)
		if err := http.ListenAndServe(addr, healthSrv.Handler()); err != nil {
			slog.Error("health server", "error", err)
			cancel()
		}
	}()

	slog.Info("fxtrade platform running",
		"dry_run", opts.DryRun,
		"environment", cfg.OANDA.Environment,
		"active_bots", enabled,
		"registered_bots", RegisteredIDs(),
		"stream_instruments", len(instruments),
		"email", cfg.Email.Enabled(),
	)

	for {
		select {
		case <-ctx.Done():
			for _, r := range runners {
				if r.handle.SaveState != nil {
					if err := r.handle.SaveState(); err != nil {
						slog.Warn("state save failed", "bot", r.id, "error", err)
					}
				}
			}
			notify.SendRoutine(notifier, ctx, "fxtrade: platform stopping", fmt.Sprintf("graceful shutdown\nactive_bots: %s\n", strings.Join(enabled, ", ")))
			slog.Info("shutting down")
			return nil
		case tick := <-ticks:
			healthSrv.RecordTick(tick.Instrument, tick.Time)
		}
	}
}

func mergeInstruments(runners []runningBot) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, r := range runners {
		for _, inst := range r.handle.Instruments {
			if _, ok := seen[inst]; ok {
				continue
			}
			seen[inst] = struct{}{}
			out = append(out, inst)
		}
	}
	return out
}

func startupChecks(ctx context.Context, client *oanda.Client, notifier notify.Notifier, cfg *config.Config, enabled []string) error {
	summary, err := client.AccountSummary(ctx)
	if err != nil {
		return err
	}
	slog.Info("account connected",
		"id", summary.Account.ID,
		"balance", summary.Account.Balance,
		"nav", summary.Account.NAV,
	)

	notify.SendRoutine(notifier, ctx, "fxtrade: platform started",
		fmt.Sprintf("fxtrade platform started.\n\nEnvironment: %s\nActive bots: %s\nRegistered: %s\n",
			cfg.OANDA.Environment, strings.Join(enabled, ", "), strings.Join(RegisteredIDs(), ", ")))
	return nil
}
