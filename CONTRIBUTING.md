# Contributing to fxtrade

All changes land through **pull requests**. Do not commit or push directly to `main`.

**Pre-PR checks:** `./scripts/code-quality.sh` — see also `.cursor/skills/code-quality/SKILL.md`.

**Cursor:** `.cursor/rules/pr-workflow.mdc` is always-on — agents must open a PR instead of pushing `main`.

---

## Quick start

```bash
git clone https://github.com/yogesh-insta/fxtrade.git
cd fxtrade
cp .credentials.example .credentials   # never commit .credentials
go mod download
```

---

## Workflow

### 1. Branch from `main`

```bash
git checkout main
git pull origin main
git checkout -b your-name/short-description
```

Use a descriptive branch name (`fix/scanner-force-flat`, `feat/fx-sentiment-cooldown`).

### 2. Make changes

| Change type | Also update |
|-------------|-------------|
| Bot behavior (`universe_scanner`, `fx_sentiment`, `btc_cfd`) | Matching `docs/specs/*_bot_spec.md` + `.cursor/skills/*-logic/SKILL.md` |
| Config defaults in `internal/config/` | Spec defaults section + skill bullets |
| Moved/renamed Go files | Source maps in specs/skills |

See **`docs/guides/bot_specs_maintenance.md`** for the doc checklist.

**Secrets:** never commit `.credentials`, API keys, or tokens. Use `.credentials.example` for structure only.

### 3. Run code quality locally

```bash
chmod +x scripts/code-quality.sh   # once per clone
./scripts/code-quality.sh
```

This mirrors the CI job: `go vet`, `go test`, bot-spec path verify, builds, `gofmt` on changed files, `staticcheck` on touched packages.

Cursor agents: use **`.cursor/skills/code-quality/SKILL.md`** before opening a PR.

### 4. Commit

- One logical change per commit when possible
- Clear message: `Fix force-flat for INDEX instruments` not `updates`
- Bot logic + doc updates can be in the same commit or same PR

### 5. Push and open a PR

```bash
git push -u origin your-name/short-description
```

Open a PR to **`main`**. Fill in the PR template checklist.

### 6. CI must pass

GitHub Actions runs **`CI`** on every PR:

- `go vet ./...`
- `go test ./...`
- `./scripts/verify-bot-spec-paths.sh`
- Build `fxtrade` + key test CLIs
- `gofmt` on changed `.go` files
- `staticcheck` on packages with changed `.go` files

Fix failures locally with `./scripts/code-quality.sh`, push again.

### 7. Merge

- Squash or merge per maintainer preference
- Delete the branch after merge

Deploy to the VM is separate (`deploy.yml` on `main`); see `deploy/gcp/DEPLOY.md`.

---

## Repository layout (where to work)

| Area | Path |
|------|------|
| Main daemon | `cmd/fxtrade/` |
| Platform bots | `internal/bots/` |
| Universe scanner logic | `internal/scanner/` |
| FX sentiment strategy | `internal/strategy/`, `internal/sentiment/` |
| BTC CFD bot | `internal/bots/btc_cfd/` |
| Shared platform | `internal/oanda/`, `risk/`, `execution/`, `monitor/`, `journal/` |
| Config | `internal/config/`, `.credentials.example` |
| Bot logic docs | `docs/specs/`, `docs/README.md` |
| Cursor skills | `.cursor/skills/` |
| GCP deploy | `deploy/gcp/` |

---

## Documentation and Cursor skills

| Purpose | File |
|---------|------|
| **Documentation index** | `docs/README.md` |
| How to keep specs current | `docs/guides/bot_specs_maintenance.md` |
| Universe scanner logic | `docs/specs/universe_scanner_bot_spec.md` |
| FX sentiment logic | `docs/specs/fx_sentiment_bot_spec.md` |
| BTC CFD spec | `docs/specs/btc_cfd_bot_spec.md` |
| Skills catalog | `docs/skills/README.md` |
| P/L analysis workflow | `.cursor/skills/analyze-pl-tweaks/SKILL.md` |
| VM access (Cloud Agents) | `docs/guides/vm_access_via_actions.md` |
| Pre-PR checks | `.cursor/skills/code-quality/SKILL.md` |
| Doc updates with code | `.cursor/skills/maintain-bot-specs/SKILL.md` |

In Cursor chat, `@docs/specs/fx_sentiment_bot_spec.md` or `@docs/README.md` for logic questions instead of searching the whole repo.

---

## Testing

```bash
go test ./...                                    # all tests
go run ./cmd/scanner-test                        # universe_scanner one cycle (dry-run default)
go run ./cmd/strategy-test                       # fx_sentiment strategy cycle
go run ./cmd/sentiment-test                      # fx_sentiment sentiment cycle
go run ./cmd/bot-metrics -bot universe_scanner   # SQLite stats
```

Add tests when you change non-trivial behavior (`internal/*_test.go`).

---

## Branch protection (required checks)

Repo admins: enable protection on `main` so PRs are mandatory and CI must pass.

**Private repo on GitHub Free:** classic branch protection is unavailable until you upgrade to **Pro** or make the repo **public**. Until then, rely on this CONTRIBUTING workflow and `.cursor/rules/pr-workflow.mdc`.

**Settings → Branches → Add rule for `main`:**

| Setting | Value |
|---------|--------|
| Require a pull request before merging | Yes |
| Require approvals | Optional (0–1 for solo maintainer) |
| Require status checks to pass | Yes |
| Required check | **`test-and-build`** (job name in `.github/workflows/ci.yml`) |
| Require branches to be up to date | Recommended |
| Do not allow bypassing / Include administrators | **Yes** (recommended) |
| Restrict direct pushes | Yes (no direct push to `main`) |

Detailed steps: **`.github/BRANCH_PROTECTION.md`**

---

## Bots and dry-run

- Local paper trading: `go run ./cmd/fxtrade -bot universe_scanner --dry-run`
- VM: `FXTRADE_DRY_RUN=--dry-run` in systemd unit
- Practice account only until validated; see `docs/guides/demo_trading_loop.md`

---

## Questions

- Ops / VM: `deploy/gcp/DEPLOY.md`, `README.md`
- Strategy design (historical): `plan.md` (fx_sentiment); **code wins** if they differ
- Open an issue or PR discussion for larger design changes before large refactors
