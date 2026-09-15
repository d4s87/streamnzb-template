#!/usr/bin/env python3
"""
Phase 2 focused tests: version-suggestion enforcement, prep-pr mode, and
branch construction. Offline/fixture-based -- no live `gh` calls, no
mutation of the repository's real .release/ directory (temp-dir-isolated,
same pattern already used in tests/test_check_release.py).
"""

import json
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402


# ---------------------------------------------------------------------------
# Branch construction
# ---------------------------------------------------------------------------

assert cr.branch_name_for_version("6.1.0") == "docs/prepare-release-6-1-0"
assert cr.branch_name_for_version("10.20.30") == "docs/prepare-release-10-20-30"

for bad in ("v6.1.0", "6.1", "6.1.0-rc1", "not-a-version", "6.1.0; rm -rf /"):
    try:
        cr.validate_version(bad)
    except cr.CheckError:
        pass
    else:
        raise AssertionError(f"invalid version {bad!r} was accepted before branch construction")

print("PASS: branch_name_for_version constructs docs/prepare-release-X-Y-Z; invalid versions rejected upstream")


# ---------------------------------------------------------------------------
# Version-suggestion enforcement
# ---------------------------------------------------------------------------

assert cr.canonical_bump("6.0.1", "patch") == "6.0.2"
assert cr.canonical_bump("6.0.1", "minor") == "6.1.0"
assert cr.canonical_bump("6.0.1", "major") == "7.0.0"

assert cr.classify_requested_bump("6.0.2", "6.0.1") == "patch"
assert cr.classify_requested_bump("6.1.0", "6.0.1") == "minor"
assert cr.classify_requested_bump("7.0.0", "6.0.1") == "major"
assert cr.classify_requested_bump("6.0.5", "6.0.1") is None  # skips semver
assert cr.classify_requested_bump("6.2.0", "6.0.1") is None  # skips a minor
assert cr.classify_requested_bump("6.0.0", "6.0.1") is None  # decrease

print("PASS: canonical_bump / classify_requested_bump")

status, detail = cr.enforce_version_suggestion("6.0.2", "6.0.1", "patch")
assert status == "pass", detail
status, detail = cr.enforce_version_suggestion("6.1.0", "6.0.1", "minor")
assert status == "pass", detail
status, detail = cr.enforce_version_suggestion("7.0.0", "6.0.1", "major")
assert status == "pass", detail

status, detail = cr.enforce_version_suggestion("7.0.0", "6.0.1", "minor")
assert status == "warn", detail
status, detail = cr.enforce_version_suggestion("6.1.0", "6.0.1", "patch")
assert status == "warn", detail

status, detail = cr.enforce_version_suggestion("6.0.2", "6.0.1", "minor")
assert status == "fail", detail
status, detail = cr.enforce_version_suggestion("6.1.0", "6.0.1", "major")
assert status == "fail", detail

status, detail = cr.enforce_version_suggestion("6.0.5", "6.0.1", "minor")
assert status == "fail", detail
assert "canonical" in detail

status, detail = cr.enforce_version_suggestion("6.0.2", "6.0.1", None)
assert status == "pass", detail

print("PASS: enforce_version_suggestion (exact patch/minor/major pass, larger warns, smaller/malformed fail, no-suggestion accepts)")


# ---------------------------------------------------------------------------
# prep-pr mode -- version "6.0.1" reuses the real, current README.md/
# CHANGELOG.md (both already reference 6.0.1), so only the .release/
# artifacts and GitHub state need faking. Temp-dir-isolated RELEASE_DIR;
# never touches the repository's real .release/.
# ---------------------------------------------------------------------------

REAL_COMPAT = cr.load_compatibility_baseline()
REAL_COUNTS = cr.load_current_counts()

MAIN_SHA = "8fea02c16c204988af3a1acf08dd303ada9f0414"

GOOD_NOTE = "# DraCuLa StreamNZB Template 6.0.1\n\nCurated public prose. Mentions 6.0.1.\n"
DRAFT_NOTE = f"# DraCuLa StreamNZB Template 6.0.1 {cr.DRAFT_HEADING_MARKER}\n\n{cr.DRAFT_NOTICE}\n\nBody.\n"

GOOD_PROVENANCE = {
    "schema_version": 1,
    "version": "6.0.1",
    "previous_version": "6.0.0",
    "prepared_from_sha": MAIN_SHA,
    "prepared_at_date": "2026-09-15",
    "compatibility": {
        "streamnzb_version": REAL_COMPAT["streamnzb_version"],
        "streamnzb_sha": REAL_COMPAT["streamnzb_sha"],
        "jhin_version": REAL_COMPAT["jhin_version"],
    },
    "expected_counts": dict(REAL_COUNTS),
}


def _run_prep_pr(version, github, note_text, provenance, offline=False):
    original_release_dir = cr.RELEASE_DIR
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir)
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        (tmp_release_dir / f"{version}.json").write_text(json.dumps(provenance), encoding="utf-8")
        cr.RELEASE_DIR = tmp_release_dir
        try:
            return cr.run_prep_pr(version, github, offline)
        finally:
            cr.RELEASE_DIR = original_release_dir


def _adapter(main_sha=MAIN_SHA, tag=None, release=None):
    return cr.FakeGithubAdapter(
        main_sha=main_sha,
        tags={"6.0.1": tag} if tag else {},
        releases={"6.0.1": release} if release else {},
    )


def _finding(report, check):
    return next(f for f in report.findings if f.check == check)


# a. fresh main passes freshness
report = _run_prep_pr("6.0.1", _adapter(main_sha=MAIN_SHA), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "preparation-freshness")
assert f.ok, f.render()

print("PASS: prep-pr freshness check passes when prepared_from_sha == live main")

# b. changed main fails freshness
report = _run_prep_pr("6.0.1", _adapter(main_sha="c" * 40), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "preparation-freshness")
assert not f.ok and f.severity == "error", f.render()
assert "stale" in f.detail

print("PASS: prep-pr freshness check fails closed when main has advanced")

# c. uncurated release note fails
report = _run_prep_pr("6.0.1", _adapter(), DRAFT_NOTE, GOOD_PROVENANCE)
f = _finding(report, "release-note-curated")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr hard-fails release-note-curated on an untouched generated draft")

# d. curated note passes
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "release-note-curated")
assert f.ok, f.render()

print("PASS: prep-pr passes release-note-curated on a curated note")

# e. provenance version mismatch fails
bad_version_provenance = {**GOOD_PROVENANCE, "version": "9.9.9"}
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, bad_version_provenance)
f = _finding(report, "provenance-version-matches")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails on provenance version mismatch")

# f. count/compat snapshot drift fails
drifted_compat_provenance = {
    **GOOD_PROVENANCE,
    "compatibility": {**GOOD_PROVENANCE["compatibility"], "jhin_version": "0.0.0"},
}
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, drifted_compat_provenance)
f = _finding(report, "provenance-compatibility-snapshot-matches")
assert not f.ok and f.severity == "error", f.render()

drifted_counts_provenance = {
    **GOOD_PROVENANCE,
    "expected_counts": {**REAL_COUNTS, "samsung": 1},
}
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, drifted_counts_provenance)
f = _finding(report, "provenance-counts-snapshot-matches")
assert not f.ok and f.severity == "error", f.render()

# Non-drifted snapshot passes both.
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
assert _finding(report, "provenance-compatibility-snapshot-matches").ok
assert _finding(report, "provenance-counts-snapshot-matches").ok

print("PASS: prep-pr fails on stored compatibility/counts snapshot drift from current repo state, passes when matching")

# g. tag collision fails
report = _run_prep_pr("6.0.1", _adapter(tag={"sha": MAIN_SHA, "type": "commit"}), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "tag-not-yet-created")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails when the tag already exists (unexpected before publish)")

# h. published release collision fails
report = _run_prep_pr(
    "6.0.1",
    _adapter(release={"tag_name": "6.0.1", "target_commitish": MAIN_SHA, "draft": False, "prerelease": False, "published_at": "x", "body": ""}),
    GOOD_NOTE,
    GOOD_PROVENANCE,
)
f = _finding(report, "release-not-yet-published")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails when a published release already exists (unexpected before publish)")

# Fully-good fixture passes end to end.
report = _run_prep_pr("6.0.1", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
assert report.passed, report.render()

print("PASS: prep-pr passes end to end for a fresh, curated, non-colliding preparation")

# offline mode skips GitHub-dependent checks explicitly, not silently.
report = _run_prep_pr("6.0.1", None, GOOD_NOTE, GOOD_PROVENANCE, offline=True)
assert report.passed
f = _finding(report, "github-dependent-checks")
assert not f.ok and f.severity == "warning"

print("PASS: prep-pr --offline skips GitHub-dependent checks explicitly (warning, not silent)")


# ---------------------------------------------------------------------------
# render_prepare_release_pr_body.render_body -- CodeRabbit finding: a PR
# carrying both an impact label and skip-changelog must render as
# bookkeeping, matching suggest_version_bump()'s own precedence (a
# skip-changelog PR never drives the release, even with a co-applied
# label) rather than contradicting it.
# ---------------------------------------------------------------------------

sys.path.insert(0, str(ROOT / "scripts"))
import render_prepare_release_pr_body as rprb  # noqa: E402


def _delta_with_prs(prs):
    return {"previous_tag": "6.0.1", "candidate_sha": "0" * 40, "commits": [], "prs": prs, "unaccounted_commits": []}


ordinary_patch_pr = {1: {"number": 1, "title": "fix: a", "labels": ["patch"]}}
body = rprb.render_body("6.1.0", "6.0.1", MAIN_SHA, _delta_with_prs(ordinary_patch_pr), {"suggested_bump": "patch", "evidence": [], "warnings": []})
assert "#1 'fix: a' (patch)" in body.split("### Bookkeeping")[0]

contradictory_pr = {2: {"number": 2, "title": "docs: prepare", "labels": ["patch", "skip-changelog"]}}
body = rprb.render_body("6.1.0", "6.0.1", MAIN_SHA, _delta_with_prs(contradictory_pr), {"suggested_bump": None, "evidence": [], "warnings": []})
substantive_section = body.split("### Bookkeeping")[0]
bookkeeping_section = body.split("### Bookkeeping")[1]
assert "#2" not in substantive_section
assert "#2 'docs: prepare' (patch, skip-changelog)" in bookkeeping_section

print("PASS: render_body classifies a skip-changelog PR as bookkeeping even alongside an impact label")


# ---------------------------------------------------------------------------
# prep-pr must revalidate the release-impact/version policy against LIVE
# main, not just at Prepare-dispatch time -- architect finding: the
# stale-main equality gate does NOT catch a human hand-editing the open
# PR's README/CHANGELOG/.release/<version>.{json,md} *consistently* to an
# under-classified version while main stays untouched (freshness would
# still pass). Uses the real 6.0.0..6.0.1 commit range (5 real, known
# SHAs) with FABRICATED PR-label associations via FakeGithubAdapter, so
# the resulting "suggested bump" is fully controlled without touching
# live GitHub state.
# ---------------------------------------------------------------------------

REAL_RANGE_COMMIT_SHAS = [
    "8fea02c16c204988af3a1acf08dd303ada9f0414",
    "d030429c1878a497759a89605668353684ca3b3d",
    "31c4bd1df9a572a93484d9913b6b705f2dc37599",
    "0c5bbc48879018145565ec0a78ee624229281d2c",
    "1c8c954775d5763752cf2d7264c9c43e6a7dd946",
]
SLUG = "d4s87/streamnzb-template"


def _fake_changelog_text(version, previous="6.0.0"):
    return (
        "# Changelog\n\n"
        f"## [Unreleased](https://github.com/{SLUG}/compare/{version}...HEAD)\n\n"
        f"{cr.NOTICE_LINE}\n\n"
        f"## [{version}](https://github.com/{SLUG}/compare/{previous}...{version}) (2026-09-20)\n\n"
        "Body.\n"
    )


def _fake_prs_by_sha(label):
    pr = {"number": 999, "title": "fake pr for policy test", "labels": [label], "merged_at": "x", "merge_commit_sha": "y"}
    return {sha: [pr] for sha in REAL_RANGE_COMMIT_SHAS}


def _policy_adapter(suggested_label, main_sha=MAIN_SHA):
    return cr.FakeGithubAdapter(main_sha=main_sha, prs_by_sha=_fake_prs_by_sha(suggested_label))


def _note_for(version):
    return f"# DraCuLa StreamNZB Template {version}\n\nCurated public prose.\n"


def _provenance_for(version, prepared_from_sha, previous="6.0.0"):
    return {
        "schema_version": 1,
        "version": version,
        "previous_version": previous,
        "prepared_from_sha": prepared_from_sha,
        "prepared_at_date": "2026-09-20",
        "compatibility": dict(REAL_COMPAT),
        "expected_counts": dict(REAL_COUNTS),
    }


def _run_prep_pr_with_fake_changelog(version, github, note_text, provenance, previous="6.0.0"):
    original_release_dir = cr.RELEASE_DIR
    original_changelog_path = cr.CHANGELOG_PATH
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir) / "release"
        tmp_release_dir.mkdir()
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        (tmp_release_dir / f"{version}.json").write_text(json.dumps(provenance), encoding="utf-8")

        changelog_path = Path(tmpdir) / "CHANGELOG.md"
        changelog_path.write_text(_fake_changelog_text(version, previous), encoding="utf-8")

        cr.RELEASE_DIR = tmp_release_dir
        cr.CHANGELOG_PATH = changelog_path
        try:
            return cr.run_prep_pr(version, github, offline=False)
        finally:
            cr.RELEASE_DIR = original_release_dir
            cr.CHANGELOG_PATH = original_changelog_path


# 1. suggested minor, requested 6.1.0 -> PASS.
report = _run_prep_pr_with_fake_changelog(
    "6.1.0", _policy_adapter("minor"), _note_for("6.1.0"), _provenance_for("6.1.0", MAIN_SHA)
)
f = _finding(report, "version-suggestion-policy")
assert f.ok, f.render()
assert "matches suggested minor" in f.detail

print("PASS: prep-pr version-suggestion-policy passes when requested bump matches the live-recomputed suggestion")

# 2 & 5. suggested minor, but the open PR's artifacts were hand-edited
# *consistently* to under-classified 6.0.1 (the canonical patch bump of
# previous 6.0.0 -- deliberately not the real published 6.0.1 tag: this
# adapter is fully fake and never touches live GitHub, only the string
# happens to coincide), with main entirely unchanged (prepared_from_sha ==
# live main, so freshness independently passes) -- must still hard FAIL,
# specifically via version-suggestion-policy.
report = _run_prep_pr_with_fake_changelog(
    "6.0.1", _policy_adapter("minor"), _note_for("6.0.1"), _provenance_for("6.0.1", MAIN_SHA)
)
freshness = _finding(report, "preparation-freshness")
policy = _finding(report, "version-suggestion-policy")
assert freshness.ok, f"main is unchanged; freshness must independently pass: {freshness.render()}"
assert not policy.ok and policy.severity == "error", policy.render()
assert "smaller" in policy.detail

print("PASS: prep-pr hard-fails version-suggestion-policy on an under-classified hand-edit even though main never advanced (freshness alone would have missed this)")

# 3. suggested patch, requested changed to larger 6.1.0 -> WARN, allowed.
report = _run_prep_pr_with_fake_changelog(
    "6.1.0", _policy_adapter("patch"), _note_for("6.1.0"), _provenance_for("6.1.0", MAIN_SHA)
)
f = _finding(report, "version-suggestion-policy")
assert not f.ok and f.severity == "warning", f.render()
# A warning-severity finding alone must never appear in report.errors
# (README.md legitimately fails readme-version-matches here since this
# fixture doesn't fake that file too -- irrelevant to this check).
assert f not in report.errors

print("PASS: prep-pr warns-but-allows a larger-than-suggested requested bump")

# 4. non-canonical edited version (skips semver relative to previous 6.0.0) -> FAIL regardless of suggestion.
report = _run_prep_pr_with_fake_changelog(
    "6.0.5", _policy_adapter("minor"), _note_for("6.0.5"), _provenance_for("6.0.5", MAIN_SHA)
)
f = _finding(report, "version-suggestion-policy")
assert not f.ok and f.severity == "error", f.render()
assert "canonical" in f.detail

print("PASS: prep-pr hard-fails a non-canonical requested version regardless of the live suggestion")

# 6. stale-main freshness remains independently enforced: main HAS
# advanced (prepared_from_sha != live main) while the requested version is
# otherwise correctly classified -- freshness must fail, and
# version-suggestion-policy must still be evaluated on its own (not
# skipped just because freshness failed), proving the two checks are
# independent.
report = _run_prep_pr_with_fake_changelog(
    "6.1.0", _policy_adapter("minor"), _note_for("6.1.0"), _provenance_for("6.1.0", "c" * 40)
)
freshness = _finding(report, "preparation-freshness")
policy = _finding(report, "version-suggestion-policy")
assert not freshness.ok and freshness.severity == "error", freshness.render()
assert policy.ok, f"version-suggestion-policy must still be evaluated independently of freshness: {policy.render()}"

print("PASS: prep-pr's stale-main freshness check fails independently of version-suggestion-policy (neither masks the other)")


# ---------------------------------------------------------------------------
# CodeRabbit follow-up: _policy_adapter above assigns the SAME label to
# every commit in the range, so it can't distinguish "the whole delta was
# scanned" from "only the first (or last) commit was inspected". Prove the
# full range actually matters: mix labels across the 5 real commits (only
# the MIDDLE one carries the higher-impact label) and confirm the
# recomputed suggestion -- and therefore version-suggestion-policy's
# verdict -- reflects that middle commit, not just an endpoint.
# ---------------------------------------------------------------------------

def _fake_mixed_prs_by_sha():
    patch_pr = {"number": 101, "title": "patch pr", "labels": ["patch"], "merged_at": "x", "merge_commit_sha": "y"}
    minor_pr = {"number": 102, "title": "minor pr", "labels": ["minor"], "merged_at": "x", "merge_commit_sha": "y"}
    # REAL_RANGE_COMMIT_SHAS[2] is neither the first nor the last commit in
    # the range -- a "only check commits[0]" or "only check commits[-1]"
    # bug would both miss it and wrongly conclude "patch".
    return {
        sha: [minor_pr if sha == REAL_RANGE_COMMIT_SHAS[2] else patch_pr]
        for sha in REAL_RANGE_COMMIT_SHAS
    }


mixed_adapter = cr.FakeGithubAdapter(main_sha=MAIN_SHA, prs_by_sha=_fake_mixed_prs_by_sha())

# A requested patch bump must now be rejected as smaller than the mixed
# range's true suggestion (minor, driven by the one middle commit).
report = _run_prep_pr_with_fake_changelog(
    "6.0.1", mixed_adapter, _note_for("6.0.1"), _provenance_for("6.0.1", MAIN_SHA)
)
f = _finding(report, "version-suggestion-policy")
assert not f.ok and f.severity == "error", f.render()
assert "smaller" in f.detail and "minor" in f.detail

# The matching minor request must pass against that same mixed range.
report = _run_prep_pr_with_fake_changelog(
    "6.1.0", mixed_adapter, _note_for("6.1.0"), _provenance_for("6.1.0", MAIN_SHA)
)
f = _finding(report, "version-suggestion-policy")
assert f.ok, f.render()

print("PASS: prep-pr's recomputed suggestion reflects the entire commit range, not just its first or last commit")

print("PASS: check_release_phase2 tests")
