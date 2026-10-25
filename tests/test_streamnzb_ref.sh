#!/usr/bin/env bash

# Regression coverage for STREAMNZB_REF resolution: commit SHAs, lightweight
# and annotated tags must all resolve to the same commit, and anything else
# must fail closed.

set -u

root="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck source=scripts/streamnzb_ref.sh
source "$root/scripts/streamnzb_ref.sh" || exit 1

tmpdir="$(mktemp -d)" || exit 1
trap 'rm -rf "$tmpdir"' EXIT

origin="$tmpdir/origin"
checkout="$tmpdir/checkout"
failures=0

g() {
  git -c user.name=test -c user.email=test@example.invalid \
    -c commit.gpgsign=false -c tag.gpgsign=false "$@"
}

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

setup_rc=0
{
  g init --quiet "$origin" &&
    g -C "$origin" commit --quiet --allow-empty -m first &&
    g -C "$origin" commit --quiet --allow-empty -m second &&
    g -C "$origin" config uploadpack.allowAnySHA1InWant true &&
    g -C "$origin" tag light HEAD~1 &&
    g -C "$origin" tag -a annotated -m annotated HEAD~1 &&
    g -C "$origin" tag -a tree-tag -m tree "HEAD~1^{tree}" &&
    g clone --quiet "$origin" "$checkout"
} >/dev/null 2>&1 || setup_rc=$?

if [ "$setup_rc" -ne 0 ]; then
  echo "ERROR: failed to create test repositories"
  exit "$setup_rc"
fi

first="$(git -C "$origin" rev-parse HEAD~1)"
second="$(git -C "$origin" rev-parse HEAD)"

expect_commit() {
  local name="$1" ref="$2" want="$3" got

  if ! got="$(resolve_streamnzb_ref "$checkout" "$ref" 2>/dev/null)"; then
    fail "$name: resolution failed"
    return
  fi

  if [ "$got" != "$want" ]; then
    fail "$name: resolved $got, want $want"
    return
  fi

  if ! git -C "$checkout" checkout --quiet --detach "$got" ||
    ! verify_streamnzb_checkout "$checkout" "$ref" "$want" 2>/dev/null; then
    fail "$name: checkout verification failed"
    return
  fi

  pass "$name"
}

expect_rejected() {
  local name="$1" ref="$2"

  if resolve_streamnzb_ref "$checkout" "$ref" >/dev/null 2>&1; then
    fail "$name: unexpectedly resolved"
  else
    pass "$name"
  fi
}

expect_commit "full commit SHA" "$first" "$first"
expect_commit "lightweight tag" light "$first"
expect_commit "annotated tag peels to commit" annotated "$first"

expect_rejected "tag pointing at a tree" tree-tag
expect_rejected "missing tag" does-not-exist
expect_rejected "unknown commit SHA" 0123456789abcdef0123456789abcdef01234567

# The upstream tag moved; the fetched value must beat the stale local tag.
g -C "$origin" tag -f light "$second" >/dev/null 2>&1
expect_commit "moved tag uses fetched value" light "$second"

git -C "$checkout" checkout --quiet --detach "$second"
if verify_streamnzb_checkout "$checkout" light "$first" 2>/dev/null; then
  fail "mismatched checkout: unexpectedly verified"
else
  pass "mismatched checkout rejected"
fi

if [ "$failures" -ne 0 ]; then
  echo "ERROR: $failures StreamNZB ref resolution check(s) failed"
  exit 1
fi

echo "PASS: StreamNZB ref resolution"
