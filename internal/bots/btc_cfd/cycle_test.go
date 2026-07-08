package btc_cfd

import "testing"

func TestCandlesRequestCount(t *testing.T) {
	if got := candlesRequestCount(200); got != 201 {
		t.Fatalf("candlesRequestCount(200) = %d, want 201", got)
	}
}
