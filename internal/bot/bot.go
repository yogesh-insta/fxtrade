package bot

import (
	"context"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
)

// Meta describes a registered bot for logs, health, and deployment.
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Status is the runtime snapshot exposed on /health.
type Status struct {
	Meta
	Running        bool      `json:"running"`
	Halted         bool      `json:"halted"`
	OpenPositions  int       `json:"open_positions"`
	Detail         string    `json:"detail,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	LastCycleOKAt  time.Time `json:"last_cycle_ok_at,omitempty"`
}

// Deps are shared platform services passed to every bot.
type Deps struct {
	CFG      *config.Config
	Client   *oanda.Client
	Stream   *oanda.Stream
	Notifier notify.Notifier
	DryRun   bool
}

// Handle is returned after a bot starts; the orchestrator uses it for health and shutdown.
type Handle struct {
	Meta          Meta
	Instruments   []string
	Status        func() Status
	Halt          func() error
	OpenPositions func() int
	SaveState     func() error
}

// Bot is a deployable trading strategy. Each implementation runs concurrently with others.
type Bot interface {
	Meta() Meta
	Start(ctx context.Context, deps *Deps) (*Handle, error)
}

type Factory func(deps *Deps) (Bot, error)
