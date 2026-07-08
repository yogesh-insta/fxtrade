# Keeping bot specs and Cursor skills up to date

Bot logic lives in Go; human/agent-readable summaries live in **spec docs** and **skills**. This guide keeps them aligned without re-searching the repo every time.

## What to maintain

| Artifact | Role | Update when |
|----------|------|-------------|
| `docs/*_bot_spec.md` | Canonical logic reference (you `@` this in chat) | Behavior, defaults, config keys, or file layout changes |
| `.cursor/skills/*-logic/SKILL.md` | Routes agents to the spec + source map | Same as spec; keep **one-paragraph summary** and **key defaults** in sync |
| `.cursor/skills/maintain-bot-specs/SKILL.md` | Tells agents to update docs in the same PR as code | Rarely — only when the process changes |
| `scripts/verify-bot-spec-paths.sh` | CI check: referenced paths still exist | Add paths to specs when you add modules |
| `scripts/code-quality.sh` | Full pre-PR / CI gate | Update when adding new required checks |
| `CONTRIBUTING.md` | Repo workflow for humans and agents | Update when process changes |

**Rule of thumb:** any PR that changes **what the bot does** should update the matching spec (and skill summary/defaults if those changed). Pure refactors that move files must update path tables and pass `./scripts/code-quality.sh`.

---

## Bot → doc map

| Bot ID | Spec | Skill |
|--------|------|-------|
| `universe_scanner` | `docs/universe_scanner_bot_spec.md` | `.cursor/skills/universe-scanner-logic/SKILL.md` |
| `fx_sentiment` | `docs/fx_sentiment_bot_spec.md` | `.cursor/skills/fx-sentiment-logic/SKILL.md` |
| `btc_cfd` | `docs/btc_cfd_bot_spec.md` | *(no logic skill yet — add when needed)* |
| P/L / tuning ops | — | `.cursor/skills/analyze-pl-tweaks/SKILL.md` |

---

## PR checklist (code change → doc change)

When you touch bot logic, walk this list before merging:

- [ ] **Source map** — new/moved/renamed files reflected in spec table and skill source map
- [ ] **Cycle / flow** — ASCII or numbered steps match `engine.go` (or main cycle file)
- [ ] **Config keys** — new or renamed JSON keys documented; defaults match `internal/config/*.go` `Default*Config()`
- [ ] **Skill summary** — one-paragraph + key defaults bullets updated (skills are what agents read first)
- [ ] **Cross-links** — related bot specs still linked if behavior diverged
- [ ] **Verify script** — `./scripts/code-quality.sh` passes locally
- [ ] **Runtime probe** — run the bot’s one-shot CLI and confirm output still matches the doc’s story

### Code area → spec section

**universe_scanner**

| Code change in | Update spec section |
|----------------|---------------------|
| `internal/scanner/engine.go` | Runtime loop, entry/exit, force-flat |
| `internal/scanner/scan.go`, `rank.go`, `range.go` | Per-symbol scan, scoring, breakout |
| `internal/scanner/session.go`, `universe.go` | Sessions, universe |
| `internal/config/scanner.go` | Key config knobs, defaults |

**fx_sentiment**

| Code change in | Update spec section |
|----------------|---------------------|
| `internal/strategy/engine.go` | Strategy cycle, RANGE/TREND behavior |
| `internal/strategy/range.go`, `gates.go` | Mode detection, range band, gates |
| `internal/sentiment/worker.go` | Sentiment pipeline |
| `internal/config/strategy.go` | Key config knobs, defaults |

---

## Automated checks

### Path verification (CI)

```bash
./scripts/code-quality.sh
```

Fails if a path referenced in a bot spec or logic skill does not exist (catches renames/moves). Also runs `go vet`, `go test`, `gofmt` (changed files), and `staticcheck` (touched packages). Runs on every PR in GitHub Actions (`ci.yml` job `test-and-build`).

### Config default audit (manual / agent-assisted)

Defaults drift silently. After changing `Default*Config()` in Go, ask Cursor:

```
Compare DefaultScannerConfig in internal/config/scanner.go with the
"Key defaults" section in docs/universe_scanner_bot_spec.md and list mismatches.
```

Same pattern for `DefaultRangeModeConfig`, `DefaultTrendModeConfig`, etc.

---

## Periodic review (monthly or after a big refactor)

1. Run `./scripts/verify-bot-spec-paths.sh`
2. For each enabled bot, run its one-shot test and read the cycle email/log:
   - `universe_scanner` → `go run ./cmd/scanner-test`
   - `fx_sentiment` → `go run ./cmd/strategy-test` and `go run ./cmd/sentiment-test`
3. Skim spec **Runtime loop** / **Strategy cycle** against the main engine file (5 min per bot)
4. If `plan.md` or `btc_cfd_bot_spec.md` contradict the code, note it — `plan.md` is historical for fx_sentiment; code wins

---

## Asking Cursor to refresh a spec

Use a single message with the spec attached:

```
@docs/fx_sentiment_bot_spec.md
Read internal/strategy/engine.go and internal/strategy/gates.go.
Update the spec and .cursor/skills/fx-sentiment-logic/SKILL.md for any
logic or default changes. Run ./scripts/verify-bot-spec-paths.sh.
```

Attach only the files that changed in your PR to keep the diff focused.

---

## What not to duplicate

| Source of truth | Docs should… |
|-----------------|--------------|
| Go code | Describe behavior, not copy entire functions |
| `.credentials.example` | Reference keys; don’t duplicate the full JSON block in specs |
| `README.md` | Link to specs for logic; README stays ops/quickstart |
| `plan.md` | Historical design; don’t merge back into fx_sentiment spec unless promoting a deliberate behavior change |

---

## Adding a new bot spec

1. Add `docs/<bot_id>_bot_spec.md` (copy structure from `universe_scanner_bot_spec.md`)
2. Add `.cursor/skills/<bot-id>-logic/SKILL.md` with source map + link to spec
3. Add the new files to `scripts/verify-bot-spec-paths.sh` `SPEC_FILES` array
4. Cross-link from sibling specs’ “vs other bots” tables
