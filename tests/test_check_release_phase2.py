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

print("PASS: check_release_phase2 tests")
