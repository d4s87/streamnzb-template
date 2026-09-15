#!/usr/bin/env python3
"""
Historical dry run for the now-real 6.1.0 release (Phase 3 audit item,
CLAUDE.md Section 18): proves check_release.py correctly understands the
6.1.0 release this repository actually completed, using LIVE git + GitHub
state (via the real `gh` CLI) rather than fixtures -- the same live/online
counterpart pattern as tests/test_check_release_6_0_1_dry_run.py, but for
the release Phase 3's own published-release-collision check must now
reject when 6.1.0 is requested again as a candidate.
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402

PREVIOUS_STABLE = "6.0.1"
RELEASE_VERSION = "6.1.0"
RELEASE_SHA = "e1383b0cdd361dd874da1c21c3bcea2fb55fc785"

try:
    github = cr.GhCliAdapter()
except cr.GithubUnavailableError as exc:
    print(f"ERROR: {exc}")
    print("This dry run requires an authenticated `gh` CLI against this repository.")
    raise SystemExit(1)


# ---------------------------------------------------------------------------
# Previous stable before 6.1.0 == 6.0.1
# ---------------------------------------------------------------------------

changelog_text = cr.CHANGELOG_PATH.read_text(encoding="utf-8")
section = None
for candidate in cr.CHANGELOG_DATED_SECTION_RE.finditer(changelog_text):
    if candidate.group("version") == RELEASE_VERSION:
        section = candidate
        break

assert section is not None, "CHANGELOG.md must contain a dated section for 6.1.0"
assert section.group("prev") == PREVIOUS_STABLE, (
    f"CHANGELOG.md's own 6.1.0 compare link says previous={section.group('prev')!r}, "
    f"expected {PREVIOUS_STABLE!r}"
)

previous_release = github.release(PREVIOUS_STABLE)
assert previous_release is not None, "6.0.1 GitHub release not found"
assert not previous_release["draft"] and not previous_release["prerelease"]

print(f"PASS: previous stable before {RELEASE_VERSION} = {PREVIOUS_STABLE} (CHANGELOG + GitHub agree)")


# ---------------------------------------------------------------------------
# 6.1.0 release SHA, tag directness, README/CHANGELOG/compat/counts/sync,
# published/non-prerelease, and (unlike 6.0.1) a real .release/6.1.0.md
# authoritative-body match -- all exercised via run_verify_published()
# against the LIVE adapter and the real checked-out repository. 6.1.0 is
# NOT the documented legacy exception, so allow_missing_release_note is
# deliberately not passed here.
# ---------------------------------------------------------------------------

report = cr.run_verify_published(RELEASE_VERSION, RELEASE_SHA, github, offline=False)
print(report.render())

# Three tolerated non-passing findings, all pre-existing/informational,
# none introduced by this dry run:
#   - notify-workflow-conclusion: WARN-only by design (today it's a live
#     PASS, but the exclusion must hold even for a future flaky/absent run).
#   - release-target-commitish: the real release object's target_commitish
#     is literally "main" (the branch name at publish time), not the exact
#     SHA -- exactly the documented "not authoritative once a direct tag
#     exists" case this check exists to flag, never a failure.
#   - release-body-matches-artifact: the real published 6.1.0 body omits
#     the leading "# DraCuLa StreamNZB Template 6.1.0" title line that
#     .release/6.1.0.md keeps (the title was published via the release's
#     own `name` field instead, at publish time) -- a real, permanent,
#     already-published fact about this specific release, not something
#     this read-only dry run can or should change (CLAUDE.md's Phase 3
#     instructions explicitly forbid altering the already-published 6.1.0
#     release). Confirmed by direct diff: identical apart from that one
#     leading title line + blank line.
EXPECTED_NON_PASSING_CHECKS = ("notify-workflow-conclusion", "release-target-commitish", "release-body-matches-artifact")
failing_unexpectedly = [
    f for f in report.findings
    if not f.ok and f.check not in EXPECTED_NON_PASSING_CHECKS
]
assert not failing_unexpectedly, f"unexpected non-passing findings: {[f.render() for f in failing_unexpectedly]}"

tracked_body = cr.normalize_release_body((cr.RELEASE_DIR / "6.1.0.md").read_text(encoding="utf-8"))
published_body = cr.normalize_release_body(github.release(RELEASE_VERSION)["body"])
tracked_minus_title = tracked_body.split("\n\n", 1)[1]  # drop the "# Title\n\n" prefix
assert published_body == tracked_minus_title, (
    "the published body is expected to differ from .release/6.1.0.md by exactly its "
    "leading title line -- any other divergence would be genuine, unexpected drift"
)

print(f"PASS: run_verify_published({RELEASE_VERSION}, {RELEASE_SHA}) passes end-to-end against live GitHub state, "
      "modulo the three documented, already-published, informational divergences")


# ---------------------------------------------------------------------------
# Phase 3's own critical-defect fix: candidate mode must now hard-fail
# target-release-absent for 6.1.0, since it is genuinely already
# published. Proves the fix against the real, live, already-published
# release -- not just the FakeGithubAdapter unit tests in
# tests/test_check_release_phase3.py.
# ---------------------------------------------------------------------------

candidate_report = cr.run_candidate(RELEASE_VERSION, RELEASE_SHA, github, offline=False)
target_finding = next(f for f in candidate_report.findings if f.check == "target-release-absent")
assert not target_finding.ok and target_finding.severity == "error", target_finding.render()
assert not candidate_report.passed

print("PASS: candidate mode hard-fails target-release-absent for the real, already-published 6.1.0 release")


# ---------------------------------------------------------------------------
# Commit/PR accounting: PR #29/#30 substantive minor changes, PR #32
# substantive patch change, PR #33 skip-changelog bookkeeping -- no
# unexpected product change, no unaccounted direct-to-main commit.
# ---------------------------------------------------------------------------

delta = cr.compute_release_delta(github, PREVIOUS_STABLE, RELEASE_SHA)

assert not delta["unaccounted_commits"], (
    f"unexpected direct-to-main commit(s) between {PREVIOUS_STABLE} and {RELEASE_VERSION}: "
    f"{delta['unaccounted_commits']}"
)

pr_numbers = set(delta["prs"])
assert pr_numbers == {29, 30, 32, 33}, (
    f"expected exactly PRs #29/#30/#32/#33 in the {PREVIOUS_STABLE}..{RELEASE_VERSION} range, "
    f"found {sorted(pr_numbers)}"
)

for number in (29, 30):
    labels = set(delta["prs"][number]["labels"])
    assert "minor" in labels, f"PR #{number} expected to carry 'minor', has {labels}"

labels_32 = set(delta["prs"][32]["labels"])
assert "patch" in labels_32, f"PR #32 expected to carry 'patch', has {labels_32}"

labels_33 = set(delta["prs"][33]["labels"])
assert "skip-changelog" in labels_33, f"PR #33 expected to carry 'skip-changelog', has {labels_33}"
assert not (labels_33 & set(cr.RELEASE_IMPACT_LABELS)), (
    f"PR #33 is skip-changelog bookkeeping and should carry no release-impact label, "
    f"but has {labels_33 & set(cr.RELEASE_IMPACT_LABELS)}"
)

print("PASS: PR #29/#30 recognized as substantive minor changes, PR #32 as a substantive patch change, "
      "PR #33 as skip-changelog bookkeeping")

suggestion = cr.suggest_version_bump(delta)
assert suggestion["suggested_bump"] == "minor", (
    f"expected suggested bump 'minor' for the actual 6.1.0 release, got {suggestion['suggested_bump']!r} "
    f"(evidence: {suggestion['evidence']})"
)
assert not suggestion["warnings"], f"unexpected warnings for a fully-labeled delta: {suggestion['warnings']}"

print("PASS: suggest_version_bump independently reproduces the actual 'minor' classification chosen for 6.1.0, no warnings")


# ---------------------------------------------------------------------------
# Provenance-style sanity: PR #33's own merge is the immediate first
# -parent predecessor of the 6.1.0 release commit -- but PR #33 IS the
# release commit itself (it was merged directly as the release-prep PR,
# never re-tagged separately), so the real check here is that the
# release commit's OWN prepared_from_sha (PR #32's merge) is exactly one
# first-parent hop back, matching the real tracked .release/6.1.0.json.
# ---------------------------------------------------------------------------

import json  # noqa: E402

provenance = json.loads((cr.RELEASE_DIR / "6.1.0.json").read_text(encoding="utf-8"))
assert provenance["version"] == "6.1.0"
assert provenance["previous_version"] == PREVIOUS_STABLE

cr.check_preparation_provenance(provenance["prepared_from_sha"], RELEASE_SHA)

print("PASS: exactly one first-parent hop between tracked .release/6.1.0.json's prepared_from_sha and the 6.1.0 release commit")

print("PASS: historical 6.1.0 dry run")
