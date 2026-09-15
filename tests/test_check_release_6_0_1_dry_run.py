#!/usr/bin/env python3
"""
Mandatory historical dry run (Phase 1, item E): prove check_release.py
correctly understands the 6.0.1 release this repository just completed,
using LIVE git + GitHub state (via the real `gh` CLI) rather than fixtures.

Requires `gh` installed and authenticated against this repository. This is
a live/online counterpart to tests/test_check_release.py's offline unit
tests, not a replacement for them.
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402

PREVIOUS_STABLE = "6.0.0"
RELEASE_VERSION = "6.0.1"
RELEASE_SHA = "8fea02c16c204988af3a1acf08dd303ada9f0414"

try:
    github = cr.GhCliAdapter()
except cr.GithubUnavailableError as exc:
    print(f"ERROR: {exc}")
    print("This dry run requires an authenticated `gh` CLI against this repository.")
    raise SystemExit(1)


# ---------------------------------------------------------------------------
# Previous stable before 6.0.1 == 6.0.0
# ---------------------------------------------------------------------------

changelog_text = cr.CHANGELOG_PATH.read_text(encoding="utf-8")
section = None
for candidate in cr.CHANGELOG_DATED_SECTION_RE.finditer(changelog_text):
    if candidate.group("version") == RELEASE_VERSION:
        section = candidate
        break

assert section is not None, "CHANGELOG.md must contain a dated section for 6.0.1"
assert section.group("prev") == PREVIOUS_STABLE, (
    f"CHANGELOG.md's own 6.0.1 compare link says previous={section.group('prev')!r}, "
    f"expected {PREVIOUS_STABLE!r}"
)

previous_release = github.release(PREVIOUS_STABLE)
assert previous_release is not None, "6.0.0 GitHub release not found"
assert not previous_release["draft"] and not previous_release["prerelease"]

print(f"PASS: previous stable before {RELEASE_VERSION} = {PREVIOUS_STABLE} (CHANGELOG + GitHub agree)")


# ---------------------------------------------------------------------------
# 6.0.1 release SHA, tag directness, README/CHANGELOG/compat/counts/sync,
# published/non-prerelease -- all exercised via run_verify_published()
# against the LIVE adapter and the real checked-out repository.
# ---------------------------------------------------------------------------

report = cr.run_verify_published(
    RELEASE_VERSION, RELEASE_SHA, github, offline=False, allow_missing_release_note=True
)
print(report.render())

# The only expected non-pass finding is the legacy release-note allowance
# itself (6.0.1 predates .release/<version>.md) and possibly the notify
# workflow conclusion (informational). No genuine FAIL is acceptable.
assert report.passed, "run_verify_published(6.0.1) must pass -- see findings above"

failing_but_not_legacy = [
    f for f in report.findings
    if not f.ok and f.check not in ("release-note-artifact",)
]
assert not failing_but_not_legacy, f"unexpected non-passing findings: {[f.render() for f in failing_but_not_legacy]}"

legacy_finding = next(f for f in report.findings if f.check == "release-note-artifact")
assert not legacy_finding.ok and legacy_finding.severity == "warning", (
    "6.0.1 predates .release/6.0.1.md; this must be an explicit WARN via "
    "--allow-missing-release-note, never a silent pass"
)

print("PASS: run_verify_published(6.0.1, 8fea02c...) passes end-to-end against live GitHub state")


# ---------------------------------------------------------------------------
# Commit/PR accounting: PR #26/#27 substantive patch changes, PR #28
# skip-changelog bookkeeping, PR #24/#25 recognized as pre-6.0.1 docs
# housekeeping (also skip-changelog) -- no unexpected product change, no
# unaccounted direct-to-main commit.
# ---------------------------------------------------------------------------

delta = cr.compute_release_delta(github, PREVIOUS_STABLE, RELEASE_SHA)

assert not delta["unaccounted_commits"], (
    f"unexpected direct-to-main commit(s) between {PREVIOUS_STABLE} and {RELEASE_VERSION}: "
    f"{delta['unaccounted_commits']}"
)

pr_numbers = set(delta["prs"])
assert pr_numbers == {24, 25, 26, 27, 28}, (
    f"expected exactly PRs #24-#28 in the {PREVIOUS_STABLE}..{RELEASE_VERSION} range, "
    f"found {sorted(pr_numbers)}"
)

for number in (26, 27):
    labels = set(delta["prs"][number]["labels"])
    assert "patch" in labels, f"PR #{number} expected to carry 'patch', has {labels}"

for number in (24, 25, 28):
    labels = set(delta["prs"][number]["labels"])
    assert "skip-changelog" in labels, f"PR #{number} expected to carry 'skip-changelog', has {labels}"
    assert not (labels & set(cr.RELEASE_IMPACT_LABELS)), (
        f"PR #{number} is skip-changelog bookkeeping and should carry no release-impact "
        f"label, but has {labels & set(cr.RELEASE_IMPACT_LABELS)}"
    )

print("PASS: PR #26/#27 recognized as substantive patch changes; PR #24/#25/#28 recognized as skip-changelog bookkeeping")

suggestion = cr.suggest_version_bump(delta)
assert suggestion["suggested_bump"] == "patch", (
    f"expected suggested bump 'patch' for the actual 6.0.1 release, got {suggestion['suggested_bump']!r} "
    f"(evidence: {suggestion['evidence']})"
)
assert not suggestion["warnings"], f"unexpected warnings for a fully-labeled delta: {suggestion['warnings']}"

print("PASS: suggest_version_bump independently reproduces the actual 'patch' classification chosen for 6.0.1, no warnings")


# ---------------------------------------------------------------------------
# Provenance-style sanity: the immediate first-parent predecessor of the
# 6.0.1 release commit is PR #27's merge (i.e. exactly one hop, no other
# main activity slipped in between PR #27 landing and PR #28 merging).
# ---------------------------------------------------------------------------

pr27_merge_sha = delta["prs"][27]["merge_commit_sha"]
cr.check_preparation_provenance(pr27_merge_sha, RELEASE_SHA)

print("PASS: exactly one first-parent hop between PR #27's merge and the 6.0.1 release commit (no stale drift)")

print("PASS: historical 6.0.1 dry run")
