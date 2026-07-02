package sentiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func AppendAudit(dir string, record AuditRecord) error {
	if dir == "" {
		dir = "logs/sentiment"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := record.At.UTC().Format("2006-01-02") + ".jsonl"
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return err
	}
	return nil
}
