# Branch protection setup (repo admins)

Enable these settings so **all work goes through PRs** and **CI must pass** before merge.

**Cursor agents:** always follow `.cursor/rules/pr-workflow.mdc` (branch + PR; never push `main` directly), even before GitHub enforcement is on.

## GitHub plan note (private repos)

On a **private** repository, classic branch protection and rulesets require **GitHub Pro** (or an org plan that includes them). Free personal accounts only get this for **public** repos.

| Option | Effect |
|--------|--------|
| Upgrade to GitHub Pro | Enable protection/rulesets on this private repo |
| Make the repo public | Protection available on Free |
| Keep Free + private | GitHub cannot block direct pushes; rely on CONTRIBUTING + Cursor rule |

Check current plan: GitHub → **Settings → Billing**.

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
| Include administrators | **Enabled** (blocks accidental direct push even for you) |
| Allow force pushes | Disabled |
| Allow deletions | Disabled |

### Finding the CI check name

The required check name is the **job** name in `.github/workflows/ci.yml`:

```yaml
jobs:
  test-and-build:   # <-- this name appears in GitHub PR checks
```

After the first PR runs CI, you can also pick **`test-and-build`** from the status-check dropdown in the branch protection UI.

## Enable via CLI (after Pro / public)

```bash
gh api -X PUT repos/yogesh-insta/fxtrade/branches/main/protection \
  -H "Accept: application/vnd.github+json" \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["test-and-build"]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
```

Prefer the GitHub UI if preferred; the table above is the source of truth.

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
