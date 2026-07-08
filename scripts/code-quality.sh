#!/usr/bin/env bash
# Local and CI code-quality gate. Run from repo root: ./scripts/code-quality.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_REF="${CODE_QUALITY_BASE:-}"
if [[ -z "$BASE_REF" ]]; then
  if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
    BASE_REF="origin/${GITHUB_BASE_REF}"
  elif git rev-parse --verify origin/main >/dev/null 2>&1; then
    BASE_REF="origin/main"
  else
    BASE_REF="HEAD~1"
  fi
fi

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...

echo "==> verify bot spec paths"
bash scripts/verify-bot-spec-paths.sh

echo "==> build main binaries"
go build -o /dev/null ./cmd/fxtrade
go build -o /dev/null ./cmd/scanner-test
go build -o /dev/null ./cmd/strategy-test
go build -o /dev/null ./cmd/sentiment-test

changed_go_files() {
  if ! git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
    return
  fi
  git diff --name-only --diff-filter=ACMR "${BASE_REF}"...HEAD -- '*.go' 2>/dev/null || \
    git diff --name-only --diff-filter=ACMR "${BASE_REF}" HEAD -- '*.go' 2>/dev/null || true
}

echo "==> gofmt (changed .go files vs ${BASE_REF})"
fmt_fail=0
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  if [[ -n "$(gofmt -l "$f")" ]]; then
    echo "  not formatted: $f (run: gofmt -w $f)"
    fmt_fail=1
  fi
done < <(changed_go_files)

if [[ $fmt_fail -ne 0 ]]; then
  echo "gofmt check failed on changed files"
  exit 1
fi

echo "==> staticcheck (packages with changed .go files)"
if ! command -v staticcheck >/dev/null 2>&1; then
  go install honnef.co/go/tools/cmd/staticcheck@latest
  export PATH="$(go env GOPATH)/bin:$PATH"
fi

declare -A pkgs=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  dir="$(dirname "$f")"
  if [[ "$dir" == "." ]]; then
    pkgs["."]=1
  else
    pkgs["./${dir}/..."]=1
  fi
done < <(changed_go_files)

if [[ ${#pkgs[@]} -eq 0 ]]; then
  echo "  no changed Go files; skipping staticcheck"
else
  for pkg in "${!pkgs[@]}"; do
    echo "  staticcheck $pkg"
    staticcheck "$pkg"
  done
fi

echo "OK: code quality checks passed"
