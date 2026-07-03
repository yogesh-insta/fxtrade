package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	At         time.Time      `json:"at"`
	Instrument string         `json:"instrument,omitempty"`
	Mode       string         `json:"mode"`
	Action     string         `json:"action"`
	Reason     string         `json:"reason,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

type Writer struct {
	dir string
}

func New(dir string) *Writer {
	if dir == "" {
		dir = "logs/trades"
	}
	return &Writer{dir: dir}
}

func (w *Writer) Append(e Entry) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(w.dir, "journal.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
