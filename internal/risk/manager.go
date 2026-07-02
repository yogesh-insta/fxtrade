package risk

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

type EntryRequest struct {
	CorrelationID   string
	SpreadPips      float64
	OpenPositions   int
	Confidence      float64
	AccountBalance  float64
	StopDistance    float64
}

type Manager struct {
	cfg   config.RiskConfig
	mu    sync.Mutex
	state State
}

type State struct {
	DayStart        time.Time `json:"day_start"`
	WeekStart       time.Time `json:"week_start"`
	DailyPnL        float64   `json:"daily_pnl"`
	WeeklyPnL       float64   `json:"weekly_pnl"`
	TradesThisMonth int       `json:"trades_this_month"`
	MonthKey        string    `json:"month_key"`
	LastLossAt      time.Time `json:"last_loss_at,omitempty"`
	TradesOpened    int       `json:"trades_opened"`
}

func NewManager(cfg config.RiskConfig) *Manager {
	now := time.Now()
	return &Manager{
		cfg: cfg,
		state: State{
			DayStart:  startOfDay(now),
			WeekStart: startOfWeek(now),
			MonthKey:  now.Format("2006-01"),
		},
	}
}

func (m *Manager) HaltFile() string {
	return m.cfg.HaltFile
}

func (m *Manager) IsHalted() bool {
	_, err := os.Stat(m.cfg.HaltFile)
	return err == nil
}

func (m *Manager) ActivateKillSwitch() error {
	return os.WriteFile(m.cfg.HaltFile, []byte("halted\n"), 0o600)
}

func (m *Manager) AllowEntry(ctx context.Context, req EntryRequest) error {
	_ = ctx
	if m.IsHalted() {
		return fmt.Errorf("kill switch active (%s)", m.cfg.HaltFile)
	}
	if req.OpenPositions >= m.cfg.MaxOpenPositions {
		return fmt.Errorf("max open positions (%d) reached", m.cfg.MaxOpenPositions)
	}
	if req.SpreadPips > m.cfg.MaxSpreadPips {
		return fmt.Errorf("spread %.1f pips exceeds max %.1f", req.SpreadPips, m.cfg.MaxSpreadPips)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollPeriods(time.Now())

	if m.state.DailyPnL <= -m.dailyLossLimit(req.AccountBalance) {
		return fmt.Errorf("daily loss limit reached (%.2f)", m.state.DailyPnL)
	}
	if m.state.WeeklyPnL <= -m.weeklyLossLimit(req.AccountBalance) {
		return fmt.Errorf("weekly loss limit reached (%.2f)", m.state.WeeklyPnL)
	}
	if m.state.TradesThisMonth >= m.cfg.MaxTradesPerMonth {
		return fmt.Errorf("max trades per month (%d) reached", m.cfg.MaxTradesPerMonth)
	}
	if !m.state.LastLossAt.IsZero() {
		cooldown := time.Duration(m.cfg.CooldownAfterLossDays) * 24 * time.Hour
		if time.Since(m.state.LastLossAt) < cooldown {
			return fmt.Errorf("cooldown after loss until %s", m.state.LastLossAt.Add(cooldown))
		}
	}
	return nil
}

func (m *Manager) SizeUnits(balance, stopDistance, confidence float64) int64 {
	if balance <= 0 || stopDistance <= 0 {
		return 0
	}
	riskPct := m.cfg.RiskPerTradePctBase
	if confidence >= m.cfg.HighConfThreshold {
		riskPct = m.cfg.RiskPerTradePctHighConf
	}
	riskAmount := balance * riskPct / 100
	units := riskAmount / stopDistance
	if units < 1 {
		return 0
	}
	return int64(math.Floor(units))
}

func (m *Manager) RecordTradeOpened() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollPeriods(time.Now())
	m.state.TradesOpened++
	m.state.TradesThisMonth++
}

func (m *Manager) RecordTradeClosed(pl float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollPeriods(time.Now())
	m.state.DailyPnL += pl
	m.state.WeeklyPnL += pl
	if pl < 0 {
		m.state.LastLossAt = time.Now()
	}
}

func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollPeriods(time.Now())
	return m.state
}

func (m *Manager) ExportState() State {
	return m.Snapshot()
}

func (m *Manager) RestoreState(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
	m.rollPeriods(time.Now())
}

func (m *Manager) dailyLossLimit(balance float64) float64 {
	return balance * m.cfg.MaxDailyLossPct / 100
}

func (m *Manager) weeklyLossLimit(balance float64) float64 {
	return balance * m.cfg.MaxWeeklyLossPct / 100
}

func (m *Manager) rollPeriods(now time.Time) {
	day := startOfDay(now)
	if day.After(m.state.DayStart) {
		m.state.DayStart = day
		m.state.DailyPnL = 0
	}
	week := startOfWeek(now)
	if week.After(m.state.WeekStart) {
		m.state.WeekStart = week
		m.state.WeeklyPnL = 0
	}
	month := now.Format("2006-01")
	if month != m.state.MonthKey {
		m.state.MonthKey = month
		m.state.TradesThisMonth = 0
	}
}

func startOfDay(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	day := startOfDay(t)
	for day.Weekday() != time.Monday {
		day = day.AddDate(0, 0, -1)
	}
	return day
}
