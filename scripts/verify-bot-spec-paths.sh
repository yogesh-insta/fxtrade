#!/usr/bin/env bash
# Verify paths referenced in bot spec docs and logic skills still exist.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SPEC_FILES=(
  docs/README.md
  docs/skills/README.md
  docs/specs/universe_scanner_bot_spec.md
  docs/specs/fx_sentiment_bot_spec.md
  docs/guides/bot_specs_maintenance.md
  .cursor/skills/universe-scanner-logic/SKILL.md
  .cursor/skills/fx-sentiment-logic/SKILL.md
  .cursor/skills/maintain-bot-specs/SKILL.md
)

missing=0
checked=()

normalize_path() {
  local path="$1"
  path="${path%/}"
  path="${path%%[;:)\`]*}"
  printf '%s' "$path"
}

should_check() {
  local path="$1"
  [[ "$path" == internal/* || "$path" == cmd/* || "$path" == docs/* || "$path" == scripts/* || "$path" == deploy/* || "$path" == .cursor/* ]]
  [[ "$path" == *.go || "$path" == *.md || "$path" == *.sh || "$path" == *.json ]]
}

check_path() {
  local path="$1"
  local from="$2"
  path="$(normalize_path "$path")"
  [[ -z "$path" ]] && return
  should_check "$path" || return
  for seen in "${checked[@]:-}"; do
    [[ "$seen" == "$path" ]] && return
  done
  checked+=("$path")
  if [[ ! -e "$path" ]]; then
    echo "MISSING: $path (referenced in $from)"
    missing=$((missing + 1))
  fi
}

for f in "${SPEC_FILES[@]}"; do
  if [[ ! -f "$f" ]]; then
    echo "MISSING SPEC FILE: $f"
    missing=$((missing + 1))
    continue
  fi
  while IFS= read -r path; do
    check_path "$path" "$f"
  done < <(grep -oE '(internal|cmd|docs|scripts|deploy|\.cursor)/[a-zA-Z0-9_./-]+\.(go|md|sh)' "$f" | sort -u)
done

if [[ $missing -gt 0 ]]; then
  echo ""
  echo "$missing path(s) missing. Update specs/skills or restore files."
  echo "See docs/guides/bot_specs_maintenance.md"
  exit 1
fi

echo "OK: all paths referenced in bot specs and skills exist ($(printf '%s\n' "${checked[@]}" | wc -l) unique paths)."
