#!/usr/bin/env bash
#
# Proves .github/workflows/sync-vidhin.yml's configured PR branch/title
# pass the real validate-pr-metadata.yml governance logic (the actual
# `run:` block, not a hand-maintained regex copy -- same extraction
# approach as test_validate_pr_metadata.sh), and that the exact
# non-compliant shape PR #64 shipped with (branch `automation/vidhin-sync`,
# untyped title) stays rejected, so that governance mismatch can never
# silently come back.

root="$(cd "$(dirname "$0")/.." && pwd)"
sync_workflow="$root/.github/workflows/sync-vidhin.yml"
metadata_workflow="$root/.github/workflows/validate-pr-metadata.yml"

for f in "$sync_workflow" "$metadata_workflow"; do
  if [ ! -f "$f" ]; then
    echo "ERROR: workflow file not found: $f"
    exit 2
  fi
done

tmpdir="$(mktemp -d)"
extracted_script="$tmpdir/validate.sh"

awk '
  found && /^      - name:/ { exit }
  found { print }
  /run: \|$/ { found = 1 }
' "$metadata_workflow" >"$extracted_script"

if [ ! -s "$extracted_script" ]; then
  echo "ERROR: failed to extract run block from $metadata_workflow"
  exit 2
fi

configured_branch="$(sed -n 's/^[[:space:]]*branch: \(.*\)$/\1/p' "$sync_workflow" | head -1)"
configured_title="$(sed -n 's/^[[:space:]]*title: "\(.*\)"$/\1/p' "$sync_workflow" | head -1)"

if [ -z "$configured_branch" ] || [ -z "$configured_title" ]; then
  echo "FAIL - could not extract configured branch/title from $sync_workflow"
  exit 1
fi

failures=0

run_case() {
  local desc="$1" branch="$2" title="$3" want_rc="$4"
  PR_TITLE="$title" PR_BRANCH="$branch" PR_BODY="non-empty description" \
    bash "$extracted_script" >/dev/null 2>&1
  local got_rc=$?

  if [ "$got_rc" -eq "$want_rc" ]; then
    printf 'ok - %s\n' "$desc"
  else
    printf 'FAIL - %s: got rc=%s, want rc=%s (branch=%q title=%q)\n' \
      "$desc" "$got_rc" "$want_rc" "$branch" "$title"
    failures=$((failures + 1))
  fi
}

run_case "sync-vidhin.yml configured branch is accepted: $configured_branch" \
  "$configured_branch" "chore: placeholder title" 0

run_case "sync-vidhin.yml configured title is accepted: $configured_title" \
  "chore/placeholder-branch" "$configured_title" 0

# The exact shape PR #64 shipped with -- must stay rejected so this
# specific regression can never silently return.
run_case "prohibited legacy branch stays rejected: automation/vidhin-sync" \
  "automation/vidhin-sync" "chore: placeholder title" 1

run_case "prohibited legacy title stays rejected: untyped 'Sync Vidhin release-group rules'" \
  "chore/placeholder-branch" "Sync Vidhin release-group rules" 1

echo "---"

if [ "$failures" -ne 0 ]; then
  echo "FAIL: $failures assertion(s) failed"
  exit 1
fi

echo "PASS: sync-vidhin.yml PR branch/title governance tests"
exit 0
