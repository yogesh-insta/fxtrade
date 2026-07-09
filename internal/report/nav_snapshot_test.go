package report

import (
	"testing"
	"time"
)

func TestNavSnapshotStore(t *testing.T) {
	path := t.TempDir() + "/nav.json"
	store := NewNavSnapshotStore(path)
	day := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	if err := store.Record(day.Add(-24*time.Hour), 99200, "AUD"); err != nil {
		t.Fatal(err)
	}
	nav, priorDate, ok := store.PriorNAV(day, "AUD")
	if !ok || nav != 99200 || priorDate != "2026-07-07" {
		t.Fatalf("prior: nav=%v date=%q ok=%v", nav, priorDate, ok)
	}

	if err := store.Record(day, 99348.4, "AUD"); err != nil {
		t.Fatal(err)
	}
	nav2, _, ok2 := store.PriorNAV(day.Add(24*time.Hour), "AUD")
	if !ok2 || nav2 != 99348.4 {
		t.Fatalf("next day prior: %v ok=%v", nav2, ok2)
	}
}
