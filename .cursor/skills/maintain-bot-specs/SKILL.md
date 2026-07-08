---
name: maintain-bot-specs
description: Keeps bot spec docs and logic skills in sync with code changes. Use when editing universe_scanner, fx_sentiment, or btc_cfd strategy logic, or when the user asks to update or refresh bot documentation, specs, or skills.
---

# Maintain Bot Specs and Skills

When you change bot **behavior**, **config defaults**, or **file layout**, update docs in the **same PR** as the code.

## Process

1. Read **`docs/bot_specs_maintenance.md`** for the full checklist.
2. Identify the bot → update its **spec** and **logic skill** (see map below).
3. Update only what changed: source map paths, cycle description, config keys, key defaults, one-paragraph skill summary.
4. Run **`./scripts/verify-bot-spec-paths.sh`** — must pass.
5. Do not paste large code blocks into specs; describe behavior and point to files.

## Bot map

| Bot | Spec | Skill |
|-----|------|-------|
| `universe_scanner` | `docs/universe_scanner_bot_spec.md` | `.cursor/skills/universe-scanner-logic/SKILL.md` |
| `fx_sentiment` | `docs/fx_sentiment_bot_spec.md` | `.cursor/skills/fx-sentiment-logic/SKILL.md` |
| `btc_cfd` | `docs/btc_cfd_bot_spec.md` | — |

## Trigger → what to edit

| You changed | Update |
|-------------|--------|
| Main cycle / entry / exit | Spec: runtime loop section; skill: one-paragraph summary |
| Scoring, gates, mode detection | Spec: relevant section; skill: key defaults if thresholds changed |
| `Default*Config()` in `internal/config/` | Spec: key defaults table; skill: key defaults bullets |
| Moved or renamed Go files | Spec + skill source maps; run verify script |
| New bot | New spec + skill; add files to `scripts/verify-bot-spec-paths.sh` |

## Verify

```bash
chmod +x scripts/verify-bot-spec-paths.sh   # once per clone
./scripts/verify-bot-spec-paths.sh
```

For default drift after config changes, diff `internal/config/*.go` defaults against the spec’s defaults section.
