package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type Status struct {
	OK                 bool      `json:"ok"`
	StartedAt          time.Time `json:"started_at"`
	LastTickAt         time.Time `json:"last_tick_at,omitempty"`
	LastTickInst       string    `json:"last_tick_instrument,omitempty"`
	TicksReceived      uint64    `json:"ticks_received"`
	StreamConnected    bool      `json:"stream_connected"`
	Halted             bool      `json:"halted"`
	OpenPositions      int       `json:"open_positions"`
	SentimentEnabled   bool      `json:"sentiment_enabled"`
	SentimentDirection string    `json:"sentiment_direction,omitempty"`
	SentimentConfidence float64  `json:"sentiment_confidence,omitempty"`
	SentimentAt        time.Time `json:"sentiment_at,omitempty"`
	TradingMode        string    `json:"trading_mode,omitempty"`
}

type Server struct {
	startedAt time.Time
	ticks     atomic.Uint64
	lastTick  atomic.Value
	lastInst  atomic.Value
	connected atomic.Bool
	haltedFn    func() bool
	killFn      func() error
	openPosFn   func() int
	sentimentFn func() (enabled bool, direction string, confidence float64, at time.Time)
	tradingModeFn func() string
}

func NewServer() *Server {
	s := &Server{startedAt: time.Now()}
	s.lastTick.Store(time.Time{})
	s.lastInst.Store("")
	return s
}

func (s *Server) SetHaltedCheck(fn func() bool) {
	s.haltedFn = fn
}

func (s *Server) SetKillHandler(fn func() error) {
	s.killFn = fn
}

func (s *Server) SetOpenPositions(fn func() int) {
	s.openPosFn = fn
}

func (s *Server) SetSentimentStatus(fn func() (bool, string, float64, time.Time)) {
	s.sentimentFn = fn
}

func (s *Server) SetTradingMode(fn func() string) {
	s.tradingModeFn = fn
}

func (s *Server) SetStreamConnected(v bool) {
	s.connected.Store(v)
}

func (s *Server) RecordTick(instrument string, at time.Time) {
	s.ticks.Add(1)
	s.lastTick.Store(at)
	s.lastInst.Store(instrument)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /kill", s.handleKill)
	return mux
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	if s.killFn == nil {
		http.Error(w, "kill switch not configured", http.StatusServiceUnavailable)
		return
	}
	if err := s.killFn(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "halted": true})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	last, _ := s.lastTick.Load().(time.Time)
	inst, _ := s.lastInst.Load().(string)

	st := Status{
		OK:              true,
		StartedAt:       s.startedAt,
		LastTickAt:      last,
		LastTickInst:    inst,
		TicksReceived:   s.ticks.Load(),
		StreamConnected: s.connected.Load(),
	}
	if s.haltedFn != nil {
		st.Halted = s.haltedFn()
	}
	if s.openPosFn != nil {
		st.OpenPositions = s.openPosFn()
	}
	if s.sentimentFn != nil {
		enabled, dir, conf, at := s.sentimentFn()
		st.SentimentEnabled = enabled
		st.SentimentDirection = dir
		st.SentimentConfidence = conf
		st.SentimentAt = at
	}
	if s.tradingModeFn != nil {
		st.TradingMode = s.tradingModeFn()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}
