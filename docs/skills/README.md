# Cursor skills index

Skills are stored in **`.cursor/skills/`** at the repo root so Cursor can auto-attach them. This page is a human-readable catalog.

---

## Workflow skills

| Skill | Path | Trigger / use |
|-------|------|----------------|
| **code-quality** | [`.cursor/skills/code-quality/SKILL.md`](../../.cursor/skills/code-quality/SKILL.md) | Pre-PR checks: `go vet`, `test`, `gofmt`, `staticcheck` |
| **maintain-bot-specs** | [`.cursor/skills/maintain-bot-specs/SKILL.md`](../../.cursor/skills/maintain-bot-specs/SKILL.md) | Update specs/skills when bot code changes |
| **analyze-pl-tweaks** | [`.cursor/skills/analyze-pl-tweaks/SKILL.md`](../../.cursor/skills/analyze-pl-tweaks/SKILL.md) | P/L analysis, `bot-analyze`, demo tweaks |
| *(reference)* | [`.cursor/skills/analyze-pl-tweaks/reference.md`](../../.cursor/skills/analyze-pl-tweaks/reference.md) | Deep reference for analyze-pl-tweaks |

---

## Bot logic skills

Each pairs with a spec under **`docs/specs/`**.

| Bot | Skill | Spec |
|-----|-------|------|
| `universe_scanner` | [universe-scanner-logic](../../.cursor/skills/universe-scanner-logic/SKILL.md) | [universe_scanner_bot_spec.md](../specs/universe_scanner_bot_spec.md) |
| `fx_sentiment` | [fx-sentiment-logic](../../.cursor/skills/fx-sentiment-logic/SKILL.md) | [fx_sentiment_bot_spec.md](../specs/fx_sentiment_bot_spec.md) |
| `btc_cfd` | — | [btc_cfd_bot_spec.md](../specs/btc_cfd_bot_spec.md) |

---

## Adding a new skill

1. Create `.cursor/skills/<name>/SKILL.md` with YAML frontmatter (`name`, `description`)
2. Add a row to this file
3. Add to [docs/README.md](../README.md) if it's a primary workflow or bot skill
4. If it references repo paths, add parent docs to `scripts/verify-bot-spec-paths.sh` when applicable

See [guides/bot_specs_maintenance.md](../guides/bot_specs_maintenance.md) for bot logic skills specifically.
