package agent

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed skills/*/SKILL.md
var skillsFS embed.FS

// LoadSystemPrompt concatenates all skill bodies into the agent system instruction.
func LoadSystemPrompt() (string, error) {
	var names []string
	err := fs.WalkDir(skillsFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if path.Base(p) == "SKILL.md" {
			names = append(names, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no skills found under skills/")
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(baseSystemPrompt)
	b.WriteString("\n\n")
	for _, name := range names {
		raw, err := skillsFS.ReadFile(name)
		if err != nil {
			return "", err
		}
		body := stripFrontmatter(string(raw))
		skillName := path.Base(path.Dir(name))
		fmt.Fprintf(&b, "## Skill: %s\n\n%s\n\n", skillName, strings.TrimSpace(body))
	}
	return strings.TrimSpace(b.String()), nil
}

// baseSystemPrompt is the thin wrapper around skills.
const baseSystemPrompt = `You are the BTC/USD sentiment agent running on Cloud Run.
You operate only via the provided tools. Follow every loaded skill below.
Never place trades. Never invent market data. Always finish with emit_sentiment.`

func stripFrontmatter(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := s[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return s
	}
	return strings.TrimSpace(rest[idx+4:])
}
