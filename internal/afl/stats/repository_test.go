package stats

import (
	"path/filepath"
	"testing"
)

func TestNewRepository(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "afl")
	repo, err := NewRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := repo.ResolveTeam("Brisbane Lions")
	if err != nil {
		t.Fatal(err)
	}
	if id != "BRI" {
		t.Fatalf("expected BRI, got %s", id)
	}
	stats, ok := repo.Team("BRI")
	if !ok {
		t.Fatal("missing BRI stats")
	}
	if stats.Inside50Efficiency <= 0 {
		t.Fatal("expected positive inside50 efficiency")
	}
}
