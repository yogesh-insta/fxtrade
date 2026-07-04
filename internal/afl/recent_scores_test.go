package afl

import (
	"strings"
	"testing"
)

func TestFormatTeamLast5(t *testing.T) {
	line := FormatTeamLast5("ESS", []RecentMatchScore{
		{Opponent: "SYD", For: 59, Against: 102, Venue: "S.C.G.", Total: 161},
		{Opponent: "CARL", For: 72, Against: 88, Venue: "M.C.G.", Total: 160},
	})
	if line == "" {
		t.Fatal("expected non-empty line")
	}
	for _, want := range []string{"ESS last 2", "59-102 vs SYD @ SCG (total 161, L)", "72-88 vs CARL @ MCG (total 160, L)"} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %q in %q", want, line)
		}
	}
}
