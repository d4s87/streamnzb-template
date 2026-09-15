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

SLUG = "d4s87/streamnzb-template"


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
# prep-pr mode -- models an explicit, isolated "6.2.0 prepared, 6.1.0
# predecessor" scenario. README/CHANGELOG are FAKED via temp-file/
# monkeypatch isolation (not read from the live repository) so this
# scenario stays true regardless of what version main actually happens to
# be prepared through at the time these tests run -- see the recurring
# "bump hardcoded tests every time Prepare Release runs or is reverted"
# failure class documented in backlog-roadmap.md. Only the .release/
# artifacts, README version, and CHANGELOG text are faked; GitHub state is
# faked via FakeGithubAdapter as before. Never touches the repository's
# real README.md/CHANGELOG.md/.release/.
# ---------------------------------------------------------------------------

REAL_COMPAT = cr.load_compatibility_baseline()
REAL_COUNTS = cr.load_current_counts()

# A real, resolvable tag/commit pair: 6.1.0 (tag) and MAIN_SHA (the 6.0.1
# release commit, an ancestor of 6.1.0) -- compute_release_delta() shells
# out to real `git log`, so it needs real refs. The resulting range is
# empty (MAIN_SHA precedes 6.1.0), which is fine: an empty delta yields no
# suggested bump, and enforce_version_suggestion() always passes when no
# suggestion is made (see test_check_release_phase2.py's own
# enforce_version_suggestion coverage above).
MAIN_SHA = "8fea02c16c204988af3a1acf08dd303ada9f0414"

FAKE_README_VERSION = "6.2.0"
FAKE_CHANGELOG_TEXT = (
    "# Changelog\n\n"
    f"## [Unreleased](https://github.com/{SLUG}/compare/6.2.0...HEAD)\n\n"
    f"{cr.NOTICE_LINE}\n\n"
    f"## [6.2.0](https://github.com/{SLUG}/compare/6.1.0...6.2.0) (2026-09-15)\n\n"
    "### Added\n\n- Fake curated body for isolated fixture testing.\n\n"
    f"## [6.1.0](https://github.com/{SLUG}/compare/6.0.1...6.1.0) (2026-09-15)\n\n"
    "Body.\n"
)

GOOD_NOTE = "# DraCuLa StreamNZB Template 6.2.0\n\nCurated public prose. Mentions 6.2.0.\n"
DRAFT_NOTE = f"# DraCuLa StreamNZB Template 6.2.0 {cr.DRAFT_HEADING_MARKER}\n\n{cr.DRAFT_NOTICE}\n\nBody.\n"

GOOD_PROVENANCE = {
    "schema_version": 1,
    "version": "6.2.0",
    "previous_version": "6.1.0",
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
    """Runs prep-pr mode against a fully isolated fixture: fake README
    version, fake CHANGELOG text, and temp-dir .release/ artifacts. None of
    this reads or depends on the live repository's actual prepared state."""
    original_release_dir = cr.RELEASE_DIR
    original_changelog_path = cr.CHANGELOG_PATH
    original_parse_readme_version = cr.parse_readme_version
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir) / "release"
        tmp_release_dir.mkdir()
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        (tmp_release_dir / f"{version}.json").write_text(json.dumps(provenance), encoding="utf-8")

        changelog_path = Path(tmpdir) / "CHANGELOG.md"
        changelog_path.write_text(FAKE_CHANGELOG_TEXT, encoding="utf-8")

        # A real fake README.md, parsed by the real parse_readme_version()
        # regex (routed at tmpdir) rather than a constant stub -- this way
        # the prep-pr scenario still exercises the actual anchor-matching
        # logic (fails closed on zero/multiple anchors) instead of bypassing
        # it entirely.
        (Path(tmpdir) / "README.md").write_text(
            f"**Current version: {FAKE_README_VERSION}**  \n", encoding="utf-8"
        )

        cr.RELEASE_DIR = tmp_release_dir
        cr.CHANGELOG_PATH = changelog_path
        cr.parse_readme_version = lambda *a, **k: original_parse_readme_version(root=Path(tmpdir))
        try:
            return cr.run_prep_pr(version, github, offline)
        finally:
            cr.RELEASE_DIR = original_release_dir
            cr.CHANGELOG_PATH = original_changelog_path
            cr.parse_readme_version = original_parse_readme_version


def _adapter(main_sha=MAIN_SHA, tag=None, release=None):
    return cr.FakeGithubAdapter(
        main_sha=main_sha,
        tags={"6.2.0": tag} if tag else {},
        releases={"6.2.0": release} if release else {},
    )


def _finding(report, check):
    return next(f for f in report.findings if f.check == check)


# a. fresh main passes freshness
report = _run_prep_pr("6.2.0", _adapter(main_sha=MAIN_SHA), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "preparation-freshness")
assert f.ok, f.render()

print("PASS: prep-pr freshness check passes when prepared_from_sha == live main")

# b. changed main fails freshness
report = _run_prep_pr("6.2.0", _adapter(main_sha="c" * 40), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "preparation-freshness")
assert not f.ok and f.severity == "error", f.render()
assert "stale" in f.detail

print("PASS: prep-pr freshness check fails closed when main has advanced")

# c. uncurated release note fails
report = _run_prep_pr("6.2.0", _adapter(), DRAFT_NOTE, GOOD_PROVENANCE)
f = _finding(report, "release-note-curated")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr hard-fails release-note-curated on an untouched generated draft")

# d. curated note passes
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "release-note-curated")
assert f.ok, f.render()

print("PASS: prep-pr passes release-note-curated on a curated note")

# e. provenance version mismatch fails
bad_version_provenance = {**GOOD_PROVENANCE, "version": "9.9.9"}
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, bad_version_provenance)
f = _finding(report, "provenance-version-matches")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails on provenance version mismatch")

# f. count/compat snapshot drift fails
drifted_compat_provenance = {
    **GOOD_PROVENANCE,
    "compatibility": {**GOOD_PROVENANCE["compatibility"], "jhin_version": "0.0.0"},
}
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, drifted_compat_provenance)
f = _finding(report, "provenance-compatibility-snapshot-matches")
assert not f.ok and f.severity == "error", f.render()

drifted_counts_provenance = {
    **GOOD_PROVENANCE,
    "expected_counts": {**REAL_COUNTS, "samsung": 1},
}
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, drifted_counts_provenance)
f = _finding(report, "provenance-counts-snapshot-matches")
assert not f.ok and f.severity == "error", f.render()

# Non-drifted snapshot passes both.
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
assert _finding(report, "provenance-compatibility-snapshot-matches").ok
assert _finding(report, "provenance-counts-snapshot-matches").ok

print("PASS: prep-pr fails on stored compatibility/counts snapshot drift from current repo state, passes when matching")

# g. tag collision fails
report = _run_prep_pr("6.2.0", _adapter(tag={"sha": MAIN_SHA, "type": "commit"}), GOOD_NOTE, GOOD_PROVENANCE)
f = _finding(report, "tag-not-yet-created")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails when the tag already exists (unexpected before publish)")

# h. published release collision fails
report = _run_prep_pr(
    "6.2.0",
    _adapter(release={"tag_name": "6.2.0", "target_commitish": MAIN_SHA, "draft": False, "prerelease": False, "published_at": "x", "body": ""}),
    GOOD_NOTE,
    GOOD_PROVENANCE,
)
f = _finding(report, "release-not-yet-published")
assert not f.ok and f.severity == "error", f.render()

print("PASS: prep-pr fails when a published release already exists (unexpected before publish)")

# Fully-good fixture passes end to end.
report = _run_prep_pr("6.2.0", _adapter(), GOOD_NOTE, GOOD_PROVENANCE)
assert report.passed, report.render()

print("PASS: prep-pr passes end to end for a fresh, curated, non-colliding preparation")

# offline mode skips GitHub-dependent checks explicitly, not silently.
report = _run_prep_pr("6.2.0", None, GOOD_NOTE, GOOD_PROVENANCE, offline=True)
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


# ---------------------------------------------------------------------------
# run_prepare's release-drafter-drafts-diagnostic must never report a
# numeric draft count. GitHub's List Releases endpoint omits draft
# releases for callers without push access, and Prepare's preflight
# intentionally runs under a read-only GITHUB_TOKEN -- a live 2026-09-15
# Prepare run against this repo demonstrated the exact failure mode: a
# real 6.1.0 draft (id 389039961) existed, but the old code reported
# "0 draft releases" because the read-only credential's API response
# came back empty, not because none existed. The fix removes the live
# matching_draft_releases() call from run_prepare entirely and always
# emits a fixed visibility-limitation warning instead. Uses the real
# 6.0.1 -> 6.1.0 commit range (both real, known SHAs already on `main`)
# so compute_release_delta's local git commands work unmodified; README/
# CHANGELOG are isolated fixtures (see _run_prepare_isolated below), never
# the live repository's actual prepared state -- this models an explicit
# "main's CHANGELOG has already rolled Unreleased forward to a further,
# not-yet-published 6.2.0" scenario rather than depending on whatever main
# happens to be prepared through when these tests run.
# ---------------------------------------------------------------------------

PREPARE_MAIN_SHA = "4deacb17c47b32e204197fb0f524ed00449e63d9"  # real historical main (PR #35 merge, 6.2.0 prepared-not-published)
PREPARE_PREVIOUS_SHA = "e1383b0cdd361dd874da1c21c3bcea2fb55fc785"  # real 6.1.0 tag commit

EXPECTED_DRAFT_DIAGNOSTIC = (
    "Release Drafter draft inventory is not authoritative in Prepare preflight: the "
    "read-only GITHUB_TOKEN may omit draft releases. Draft selection/hygiene remains a "
    "Phase 3 responsibility."
)

PREPARE_FAKE_README_TEXT = "**Current version: 6.2.0**  \n**Compatibility: StreamNZB 6.1.0 / Jhin 0.7.1**\n"
PREPARE_FAKE_CHANGELOG_TEXT = (
    "# Changelog\n\n"
    f"## [Unreleased](https://github.com/{SLUG}/compare/6.2.0...HEAD)\n\n"
    f"{cr.NOTICE_LINE}\n\n"
    f"## [6.2.0](https://github.com/{SLUG}/compare/6.1.0...6.2.0) (2026-09-15)\n\n"
    "Body.\n\n"
    f"## [6.1.0](https://github.com/{SLUG}/compare/6.0.1...6.1.0) (2026-09-15)\n\n"
    "Body.\n"
)


def _run_prepare_isolated(version, github, offline=False):
    """Runs `prepare` mode against the fake README/CHANGELOG fixture above,
    isolated from the live repository's actual prepared state -- same
    rationale/pattern as _run_prep_pr's isolation above."""
    original_readme_path = cr.README_PATH
    original_changelog_path = cr.CHANGELOG_PATH
    with tempfile.TemporaryDirectory() as tmpdir:
        readme_path = Path(tmpdir) / "README.md"
        readme_path.write_text(PREPARE_FAKE_README_TEXT, encoding="utf-8")
        changelog_path = Path(tmpdir) / "CHANGELOG.md"
        changelog_path.write_text(PREPARE_FAKE_CHANGELOG_TEXT, encoding="utf-8")
        cr.README_PATH = readme_path
        cr.CHANGELOG_PATH = changelog_path
        try:
            return cr.run_prepare(version, github, offline)
        finally:
            cr.README_PATH = original_readme_path
            cr.CHANGELOG_PATH = original_changelog_path


class _NoDraftCallAdapter(cr.FakeGithubAdapter):
    """Proves run_prepare's fixed diagnostic never calls
    matching_draft_releases() -- any call is a regression back to the old,
    credential-dependent numeric-count behavior this fix removes."""

    def matching_draft_releases(self, tag_name):
        raise AssertionError(
            "run_prepare must not call matching_draft_releases(): its result is not "
            "authoritative under Prepare's read-only preflight GITHUB_TOKEN"
        )


def _prepare_adapter(releases=None, tags=None, branches=None, open_prs_by_branch=None):
    return _NoDraftCallAdapter(
        main_sha=PREPARE_MAIN_SHA,
        # Default "latest stable" is the real 6.1.0 release -- deliberately
        # mismatched against PREPARE_FAKE_CHANGELOG_TEXT's Unreleased
        # compare-link previous ("6.2.0"), to exercise the
        # changelog-previous-version-agrees-with-github divergence check.
        # compute_release_delta's git-log call below needs a real,
        # resolvable tag, hence the real 6.1.0 tag rather than a fake one.
        releases=releases if releases is not None else {
            "6.1.0": {
                "tag_name": "6.1.0",
                "target_commitish": PREPARE_PREVIOUS_SHA,
                "draft": False,
                "prerelease": False,
                "published_at": "2026-09-15T16:44:55Z",
                "body": "",
            }
        },
        tags=tags or {},
        branches=branches or {},
        open_prs_by_branch=open_prs_by_branch or {},
    )


# a. Happy path: the diagnostic is a fixed, non-numeric, always-present
# warning that never blocks preparation, and matching_draft_releases() is
# never called (the adapter raises if it is). Uses "6.1.1" as the
# hypothetical next-version-to-prepare: a canonical patch bump of the
# fake "6.1.0 is latest stable" this adapter presents, with no real
# collision (6.1.1 was never used for anything real).
report = _run_prepare_isolated("6.1.1", _prepare_adapter(), offline=False)
f = _finding(report, "release-drafter-drafts-diagnostic")
assert f.severity == "warning" and not f.ok, f.render()
assert f.detail == EXPECTED_DRAFT_DIAGNOSTIC, f.render()
# The fixture's CHANGELOG.md Unreleased section deliberately names
# "previous" as 6.2.0 (a further, not-yet-published version) while the
# adapter's fake "latest published stable" is 6.1.0 --
# changelog-previous-version-agrees-with-github is EXPECTED to disagree
# for exactly that reason. This is an explicit, isolated scenario, not a
# read of whatever main happens to be prepared through; assert precisely
# that single expected divergence rather than a blanket `.passed`.
non_passing = {finding.check for finding in report.findings if not finding.ok}
assert non_passing == {
    "changelog-previous-version-agrees-with-github",
    "release-drafter-drafts-diagnostic",  # asserted individually above -- always-present warning
    "release-impact-accounting",  # WARN: the 2 real PR #34/#35 commits are unaccounted (this fake adapter never populates prs_by_sha)
}, report.render()

print("PASS: run_prepare's release-drafter-drafts-diagnostic is a fixed visibility warning (never a numeric claim), never blocks preparation, and never calls matching_draft_releases()")

# b. Published-release collision remains a hard failure, independent of
# the draft-diagnostic fix -- releases/tags/{tag} stays the authoritative
# check for an actually-published 6.1.0 release.
report = _run_prepare_isolated(
    "6.1.0",
    _prepare_adapter(releases={
        "6.0.1": {
            "tag_name": "6.0.1", "target_commitish": PREPARE_PREVIOUS_SHA,
            "draft": False, "prerelease": False, "published_at": "x", "body": "",
        },
        "6.1.0": {
            "tag_name": "6.1.0", "target_commitish": PREPARE_MAIN_SHA,
            "draft": False, "prerelease": False, "published_at": "x", "body": "",
        },
    }),
    offline=False,
)
f = _finding(report, "published-release-collision")
assert not f.ok and f.severity == "error", f.render()
assert not report.passed

print("PASS: run_prepare still hard-fails published-release-collision")

# c. Tag collision remains a hard failure.
report = _run_prepare_isolated(
    "6.1.0",
    _prepare_adapter(tags={"6.1.0": {"sha": "deadbeef", "type": "commit"}}),
    offline=False,
)
f = _finding(report, "tag-absent-remote")
assert not f.ok and f.severity == "error", f.render()
assert not report.passed

print("PASS: run_prepare still hard-fails tag-absent-remote")

print("PASS: check_release_phase2 tests")
