# Branch protection setup (repo admins)

Enable these settings so **all work goes through PRs** and **CI must pass** before merge.

## GitHub UI

1. Open **Settings → Branches** (or **Rules → Rulesets** on newer GitHub).
2. Add a branch protection rule (or ruleset) for **`main`**.

### Required settings

| Option | Recommended value |
|--------|-------------------|
| Require a pull request before merging | Enabled |
| Required approvals | `0` (solo) or `1` (team) |
| Dismiss stale pull request approvals when new commits are pushed | Enabled if using approvals |
| Require status checks to pass before merging | Enabled |
| Require branches to be up to date before merging | Enabled |
| Status checks that are required | **`test-and-build`** |
| Require conversation resolution before merging | Optional |
| Include administrators | Your choice (enforcing for admins prevents accidental direct push) |
| Allow force pushes | Disabled |
| Allow deletions | Disabled |

### Finding the CI check name

The required check name is the **job** name in `.github/workflows/ci.yml`:

```yaml
jobs:
  test-and-build:   # <-- this name appears in GitHub PR checks
```

After the first PR runs CI, you can also pick **`test-and-build`** from the status-check dropdown in the branch protection UI.

## Verify it works

1. Try pushing directly to `main` — should be rejected.
2. Open a PR with a failing test — merge button should stay blocked until CI passes.
3. Confirm required check shows green: `test-and-build`.

## What CI runs

See `CONTRIBUTING.md` and `scripts/code-quality.sh`. Summary:

- `go vet`, `go test`
- Bot spec path verification
- Builds
- `gofmt` on changed Go files
- `staticcheck` on touched packages

## Bypass (emergencies only)

If administrators are not subject to rules, document any hotfix process in the PR after an emergency merge. Prefer fixing forward via PR when possible.
