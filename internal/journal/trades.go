package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TradeRecord struct {
	At            time.Time `json:"at"`
	Instrument    string    `json:"instrument,omitempty"`
	TradeID       string    `json:"trade_id"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Direction     string    `json:"direction,omitempty"`
	Mode          string    `json:"mode,omitempty"`
	RealizedPL    float64   `json:"realized_pl"`
	RMultiple     float64   `json:"r_multiple,omitempty"`
	Confidence    float64   `json:"confidence,omitempty"`
}

func (w *Writer) AppendTrade(t TradeRecord) error {
	if t.At.IsZero() {
		t.At = time.Now().UTC()
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(w.dir, "trades.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func ReadTrades(path string) ([]TradeRecord, error) {
	if path == "" {
		path = "logs/trades/trades.jsonl"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []TradeRecord
	for _, line := range splitLines(data) {
		if line == "" {
			continue
		}
		var t TradeRecord
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
