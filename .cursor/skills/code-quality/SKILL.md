---
name: code-quality
description: Runs fxtrade code quality checks before opening or updating a PR. Use when preparing a pull request, fixing CI failures, or when the user asks about code quality, formatting, vet, staticcheck, or pre-merge checks.
---

# Code Quality (pre-PR)

Run the same checks as CI **before** pushing or opening a PR.

## Command

From repo root:

```bash
./scripts/code-quality.sh
```

Against a specific base branch (e.g. before opening a PR to `main`):

```bash
CODE_QUALITY_BASE=origin/main ./scripts/code-quality.sh
```

## What it runs

| Check | Scope |
|-------|--------|
| `go vet ./...` | Whole module |
| `go test ./...` | Whole module |
| `verify-bot-spec-paths.sh` | Bot specs + logic skills |
| `go build` | `fxtrade`, `scanner-test`, `strategy-test`, `sentiment-test` |
| `gofmt` | **Changed** `.go` files only (vs merge base) |
| `staticcheck` | **Packages** with changed `.go` files only |

## If something fails

| Failure | Fix |
|---------|-----|
| `gofmt` | `gofmt -w <file>` on listed files |
| `go vet` / `go test` | Fix the reported issue; add tests if behavior changed |
| `staticcheck` | Fix or justify; avoid drive-by refactors outside the PR scope |
| `verify-bot-spec-paths` | Update spec/skill paths — see `docs/guides/bot_specs_maintenance.md` |

## PR workflow

1. Branch from `main`: `git checkout -b your-name/short-description`
2. Make changes; update bot specs if logic changed (`maintain-bot-specs` skill)
3. Run `./scripts/code-quality.sh`
4. Commit, push, open PR — **do not push directly to `main`**
5. Wait for GitHub Actions **CI** job to pass

See **`CONTRIBUTING.md`** for full repo workflow.
