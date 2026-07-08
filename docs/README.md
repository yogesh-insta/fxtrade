# fxtrade documentation

Central index for **bot specs**, **guides**, and **Cursor skills**. Start here instead of searching the repo.

---

## Bot specs (logic reference)

Use `@docs/specs/<bot>.md` in Cursor for logic questions.

| Bot | Spec | Cursor skill |
|-----|------|--------------|
| `universe_scanner` | [specs/universe_scanner_bot_spec.md](specs/universe_scanner_bot_spec.md) | [.cursor/skills/universe-scanner-logic/SKILL.md](../.cursor/skills/universe-scanner-logic/SKILL.md) |
| `fx_sentiment` | [specs/fx_sentiment_bot_spec.md](specs/fx_sentiment_bot_spec.md) | [.cursor/skills/fx-sentiment-logic/SKILL.md](../.cursor/skills/fx-sentiment-logic/SKILL.md) |
| `btc_cfd` | [specs/btc_cfd_bot_spec.md](specs/btc_cfd_bot_spec.md) | *(logic skill not yet added)* |

---

## Guides (workflows & upkeep)

| Guide | Purpose |
|-------|---------|
| [guides/bot_specs_maintenance.md](guides/bot_specs_maintenance.md) | Keep specs/skills in sync with code |
| [guides/demo_trading_loop.md](guides/demo_trading_loop.md) | Demo account P/L iteration, config tweaks |
| [../CONTRIBUTING.md](../CONTRIBUTING.md) | Branch, PR, code quality workflow |
| [../.github/BRANCH_PROTECTION.md](../.github/BRANCH_PROTECTION.md) | Enable mandatory PRs + CI on `main` |

---

## Cursor skills

Skills live under **`.cursor/skills/`** (Cursor auto-discovers them). Catalog: **[skills/README.md](skills/README.md)**.

| Skill | Use when |
|-------|----------|
| [code-quality](../.cursor/skills/code-quality/SKILL.md) | Before opening a PR |
| [maintain-bot-specs](../.cursor/skills/maintain-bot-specs/SKILL.md) | Bot logic changes — update docs in same PR |
| [universe-scanner-logic](../.cursor/skills/universe-scanner-logic/SKILL.md) | ORB scanner questions |
| [fx-sentiment-logic](../.cursor/skills/fx-sentiment-logic/SKILL.md) | Range/trend + sentiment questions |
| [analyze-pl-tweaks](../.cursor/skills/analyze-pl-tweaks/SKILL.md) | P/L analysis, algorithm tweaks |

---

## Deploy & ops (outside `docs/`)

| Doc | Path |
|-----|------|
| VM deploy | [deploy/gcp/DEPLOY.md](../deploy/gcp/DEPLOY.md) |
| Quickstart | [README.md](../README.md) |
| Historical fx_sentiment design | [plan.md](../plan.md) |

---

## Folder layout

```
docs/
├── README.md              ← you are here
├── specs/                 # bot logic specs (canonical)
├── guides/                # workflows & maintenance
└── skills/                # index → .cursor/skills/
```
