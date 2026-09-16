#!/usr/bin/env bash
#
# Exercises the actual branch/title validation logic embedded in
# .github/workflows/validate-pr-metadata.yml -- extracts and runs the real
# `run:` block script (not a hand-maintained regex copy that could drift).

root="$(cd "$(dirname "$0")/.." && pwd)"
workflow="$root/.github/workflows/validate-pr-metadata.yml"

if [ ! -f "$workflow" ]; then
  echo "ERROR: workflow file not found: $workflow"
  exit 2
fi

tmpdir="$(mktemp -d)"
tmp_rc=$?

if [ "$tmp_rc" -ne 0 ]; then
  echo "ERROR: failed to create temp directory"
  exit "$tmp_rc"
fi

extracted_script="$tmpdir/validate.sh"

# The `run: |` block is the last step of the file today; stop early at the
# next step header too, so this stays correct if a step is appended later.
awk '
  found && /^      - name:/ { exit }
  found { print }
  /run: \|$/ { found = 1 }
' "$workflow" >"$extracted_script"

if [ ! -s "$extracted_script" ]; then
  echo "ERROR: failed to extract run block from $workflow"
  exit 2
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

types=(feat fix chore docs test ci refactor perf)

for t in "${types[@]}"; do
  run_case "accepted branch: $t/x" "$t/x" "chore: valid title" 0
  run_case "accepted title: $t: description" "chore/x" "$t: description" 0
done

# Rejected branches: the whole point of this change -- automation/... must
# no longer validate now that it has no title-type counterpart.
run_case "rejected branch: automation/x"        "automation/x"        "chore: valid title" 1
run_case "rejected branch: Automation/x"         "Automation/x"        "chore: valid title" 1
run_case "rejected branch: random/x"             "random/x"            "chore: valid title" 1
run_case "rejected branch: uppercase suffix"     "fix/BadSuffix"       "chore: valid title" 1
run_case "rejected branch: illegal characters"   "fix/bad_suffix!"     "chore: valid title" 1
run_case "rejected branch: no suffix"            "fix/"                "chore: valid title" 1

# Rejected titles: automation: must stay unsupported (governance intent is
# a symmetric type set, not a new type), and malformed titles still reject.
run_case "rejected title: automation:"           "chore/x" "automation: something"  1
run_case "rejected title: unknown type"           "chore/x" "wip: something"         1
run_case "rejected title: missing type"           "chore/x" "just a description"     1
run_case "rejected title: missing separator space" "chore/x" "chore:no-space"        1

# The governance point of this change: the accepted branch-prefix type set
# and the accepted title-prefix type set must be exactly the same set.
title_re="$(sed -n "s/^[[:space:]]*title_re='\(.*\)'\$/\1/p" "$workflow")"
branch_re="$(sed -n "s/^[[:space:]]*branch_re='\(.*\)'\$/\1/p" "$workflow")"

if [ -z "$title_re" ] || [ -z "$branch_re" ]; then
  echo "FAIL - could not extract title_re/branch_re from $workflow"
  failures=$((failures + 1))
else
  type_group_re='\(([^)]+)\)'

  [[ "$title_re" =~ $type_group_re ]]
  title_types_raw="${BASH_REMATCH[1]}"
  [[ "$branch_re" =~ $type_group_re ]]
  branch_types_raw="${BASH_REMATCH[1]}"

  title_types_sorted="$(printf '%s\n' "${title_types_raw//|/$'\n'}" | sort)"
  branch_types_sorted="$(printf '%s\n' "${branch_types_raw//|/$'\n'}" | sort)"

  if [ "$title_types_sorted" = "$branch_types_sorted" ]; then
    printf 'ok - branch-type set and title-type set are symmetric (%s)\n' \
      "$(printf '%s' "$title_types_sorted" | tr '\n' ',')"
  else
    printf 'FAIL - branch-type set and title-type set differ:\n  title:  %s\n  branch: %s\n' \
      "$title_types_sorted" "$branch_types_sorted"
    failures=$((failures + 1))
  fi
fi

echo "---"

if [ "$failures" -ne 0 ]; then
  echo "FAIL: $failures assertion(s) failed"
  exit 1
fi

echo "PASS: PR metadata branch/title validation tests"
exit 0
