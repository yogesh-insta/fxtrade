package btc_cfd

import (
	"sync"
	"testing"
	"time"
)

func TestCandlesRequestCount(t *testing.T) {
	if got := candlesRequestCount(200); got != 201 {
		t.Fatalf("candlesRequestCount(200) = %d, want 201", got)
	}
}

func TestMarkCycleOKCalled(t *testing.T) {
	var mu sync.Mutex
	var got time.Time
	e := &cycleEngine{
		markCycleOK: func() {
			mu.Lock()
			got = time.Now()
			mu.Unlock()
		},
	}
	e.markCycleOK()
	mu.Lock()
	defer mu.Unlock()
	if got.IsZero() {
		t.Fatal("expected markCycleOK to record timestamp")
	}
}
