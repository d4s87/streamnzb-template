#!/usr/bin/env python3

import json
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402
import prepare_release_housekeeping as prh  # noqa: E402


# ---------------------------------------------------------------------------
# validate_version
# ---------------------------------------------------------------------------

assert cr.validate_version("6.0.2") == "6.0.2"
assert cr.validate_version("0.0.1") == "0.0.1"
assert cr.validate_version("12.34.567") == "12.34.567"

for bad in (
    "v6.0.2",           # v-prefix rejected
    "6.0.2 ",           # trailing junk
    " 6.0.2",           # leading junk
    "6.0",               # not 3 components
    "6.0.2.1",           # too many components
    "6.0.2-rc1",         # prerelease metadata rejected
    "6.0.2+build.5",     # build metadata rejected
    "6.0.2; rm -rf /",   # shell-injection-shaped
    "6.0.2`whoami`",     # shell-injection-shaped
    "$(echo 6.0.2)",     # shell-injection-shaped
    "",
):
    try:
        cr.validate_version(bad)
    except cr.CheckError:
        pass
    else:
        raise AssertionError(f"invalid version {bad!r} was accepted")

print("PASS: validate_version accepts bare semver only, rejects junk/prerelease/injection shapes")


# ---------------------------------------------------------------------------
# validate_sha
# ---------------------------------------------------------------------------

sha40 = "8fea02c16c204988af3a1acf08dd303ada9f0414"
assert cr.validate_sha(sha40) == sha40
assert cr.validate_sha(sha40.upper()) == sha40  # normalizes to lowercase

for bad in (
    sha40[:39],        # too short
    sha40 + "a",       # too long
    "g" * 40,          # non-hex
    sha40 + "; rm -rf /",
    "",
):
    try:
        cr.validate_sha(bad)
    except cr.CheckError:
        pass
    else:
        raise AssertionError(f"invalid sha {bad!r} was accepted")

print("PASS: validate_sha accepts exactly-40-hex only (case-insensitive), rejects short/long/non-hex/injection")


# ---------------------------------------------------------------------------
# version_key ordering (used by semver-progression / suggestion logic)
# ---------------------------------------------------------------------------

assert cr.version_key("6.0.2") > cr.version_key("6.0.1")
assert cr.version_key("6.1.0") > cr.version_key("6.0.99")
assert cr.version_key("7.0.0") > cr.version_key("6.99.99")

print("PASS: version_key orders semver components numerically, not lexicographically")


# ---------------------------------------------------------------------------
# count_defines
# ---------------------------------------------------------------------------

GOOD_DEFINES = (
    "# comment, ignored\n"
    "\n"
    'Anime Dubs Only: define if (releaseName matches "(?i)dual")\n'
    'Movies LQ Release Title [movie]: define if (releaseName matches "(?i)lq")\n'
)
assert cr.count_defines(GOOD_DEFINES) == 2

try:
    cr.count_defines(GOOD_DEFINES + "not a valid define line\n")
except cr.CheckError as exc:
    assert "unrecognized Define Library line" in str(exc)
else:
    raise AssertionError("malformed Define line was not detected")

print("PASS: count_defines counts real lines, fails closed on unrecognized syntax")


# ---------------------------------------------------------------------------
# parse_changelog / find_changelog_section / changelog_section_body
# ---------------------------------------------------------------------------

SLUG = "d4s87/streamnzb-template"
CHANGELOG_FIXTURE = (
    "# Changelog\n"
    "\n"
    f"## [Unreleased](https://github.com/{SLUG}/compare/6.0.1...HEAD)\n"
    "\n"
    f"{cr.NOTICE_LINE}\n"
    "\n"
    f"## [6.0.1](https://github.com/{SLUG}/compare/6.0.0...6.0.1) (2026-09-15)\n"
    "\n"
    "### Bug Fixes\n"
    "\n"
    "- fixed a thing.\n"
    "\n"
    f"## [6.0.0](https://github.com/{SLUG}/compare/v5.2...6.0.0) (2026-09-14)\n"
    "\n"
    "older body.\n"
)

parsed = cr.parse_changelog(CHANGELOG_FIXTURE)
assert parsed["previous_version"] == "6.0.1"
assert [s["version"] for s in parsed["dated_sections"]] == ["6.0.1", "6.0.0"]
assert parsed["dated_sections"][0]["previous"] == "6.0.0"
assert parsed["dated_sections"][0]["date"] == "2026-09-15"

section = cr.find_changelog_section(CHANGELOG_FIXTURE, "6.0.1")
assert section.startswith(f"## [6.0.1](https://github.com/{SLUG}/compare/6.0.0...6.0.1) (2026-09-15)")
assert "### Bug Fixes" in section
assert "## [6.0.0]" not in section

body = cr.changelog_section_body(CHANGELOG_FIXTURE, "6.0.1")
assert body == "### Bug Fixes\n\n- fixed a thing."

assert cr.find_changelog_section(CHANGELOG_FIXTURE, "9.9.9") is None
assert cr.changelog_section_body(CHANGELOG_FIXTURE, "9.9.9") is None

try:
    cr.parse_changelog(CHANGELOG_FIXTURE.replace("## [Unreleased]", "## [WIP]"))
except cr.CheckError as exc:
    assert "expected exactly 1" in str(exc)
else:
    raise AssertionError("missing Unreleased header was not detected")

print("PASS: parse_changelog / find_changelog_section / changelog_section_body")


# ---------------------------------------------------------------------------
# Repository baseline: load_current_counts / load_compatibility_baseline
# against the *real* checked-out repository. This doubles as a live
# reconciliation check against the stable-baseline figures this Phase-1
# work was scoped against.
# ---------------------------------------------------------------------------

counts = cr.load_current_counts()
assert counts == {
    "samsung": 153,
    "neutral": 152,
    "core": 130,
    "presentation": 22,
    "device": 1,
    "defines": 62,
}, f"unexpected baseline counts: {counts}"

baseline = cr.load_compatibility_baseline()
assert baseline["streamnzb_version"] == "6.2.0"
assert baseline["jhin_version"] == "0.8.0"
assert baseline["jhin_version_go_mod"] == "0.8.0"
assert baseline["streamnzb_sha"] == "c5aa001b0051c0f866768e30c97292c7d1a6c216"
cr.check_compatibility_internal_consistency(baseline)  # must not raise

print("PASS: load_current_counts / load_compatibility_baseline match the current stable baseline (6.0.1)")

try:
    cr.check_compatibility_internal_consistency(
        {**baseline, "jhin_version": "9.9.9"}
    )
except cr.CheckError as exc:
    assert "Jhin" in str(exc)
else:
    raise AssertionError("README/go.mod Jhin-version drift was not detected")

print("PASS: check_compatibility_internal_consistency fails closed on README/go.mod drift")

readme_version = cr.parse_readme_version()
assert readme_version == "6.3.0"  # main's real, currently checked-in README version

print("PASS: parse_readme_version reads the current stable version")


# ---------------------------------------------------------------------------
# suggest_version_bump -- pure function, hand-built delta fixtures.
# ---------------------------------------------------------------------------

def _delta(prs, unaccounted=()):
    return {
        "previous_tag": "6.0.1",
        "candidate_sha": "0" * 40,
        "commits": [],
        "prs": prs,
        "unaccounted_commits": list(unaccounted),
    }


def _pr(number, title, labels):
    return {"number": number, "title": title, "labels": labels, "merged_at": "x", "merge_commit_sha": "y"}


patch_only = _delta({27: _pr(27, "fix: x", ["patch"])})
result = cr.suggest_version_bump(patch_only)
assert result["suggested_bump"] == "patch"
assert not result["warnings"]

minor_and_patch = _delta({1: _pr(1, "feat: a", ["minor"]), 2: _pr(2, "fix: b", ["patch"])})
result = cr.suggest_version_bump(minor_and_patch)
assert result["suggested_bump"] == "minor"

major_and_minor = _delta({1: _pr(1, "feat!: a", ["major"]), 2: _pr(2, "feat: b", ["minor"])})
result = cr.suggest_version_bump(major_and_minor)
assert result["suggested_bump"] == "major"

skip_only = _delta({28: _pr(28, "docs: prepare release", ["docs", "skip-changelog"])})
result = cr.suggest_version_bump(skip_only)
assert result["suggested_bump"] is None
assert not result["warnings"]
assert any("no product changes" in e or "no release-impact" in e for e in result["evidence"])

unlabeled = _delta({1: _pr(1, "chore: something", ["automation"])})
result = cr.suggest_version_bump(unlabeled)
assert result["suggested_bump"] is None
assert any("no recognized release-impact label" in w for w in result["warnings"])

direct_to_main = _delta({}, unaccounted=[{"sha": "a" * 40, "subject": "oops direct push"}])
result = cr.suggest_version_bump(direct_to_main)
assert result["suggested_bump"] is None
assert any("direct-to-main" in w for w in result["warnings"])

skip_and_patch = _delta({1: _pr(1, "docs: prepare", ["skip-changelog", "patch"])})
result = cr.suggest_version_bump(skip_and_patch)
assert result["suggested_bump"] is None  # skip-changelog never drives, regardless of co-applied label
assert any("contradictory" in w for w in result["warnings"])

print("PASS: suggest_version_bump (patch-only, minor+patch, major+minor, skip-only, unlabeled, direct-to-main, contradictory)")


# ---------------------------------------------------------------------------
# check_preparation_freshness -- pure equality check.
# ---------------------------------------------------------------------------

ok, detail = cr.check_preparation_freshness("a" * 40, "a" * 40)
assert ok
ok, detail = cr.check_preparation_freshness("a" * 40, "b" * 40)
assert not ok
assert "stale" in detail

print("PASS: check_preparation_freshness is equality-based, not ancestry-based")


# ---------------------------------------------------------------------------
# check_preparation_provenance -- uses the real repository's own git
# history (6.0.0..6.0.1 first-parent chain), no live GitHub call needed.
# ---------------------------------------------------------------------------

PR27_MERGE = "d030429"  # tip of main immediately before PR #28 merged
PR26_MERGE = "31c4bd1"  # two merges earlier
PR28_MERGE = "8fea02c16c204988af3a1acf08dd303ada9f0414"

pr27_full = cr._git("rev-parse", PR27_MERGE)
pr26_full = cr._git("rev-parse", PR26_MERGE)

cr.check_preparation_provenance(pr27_full, PR28_MERGE)  # must not raise: exactly one hop

try:
    cr.check_preparation_provenance(pr26_full, PR28_MERGE)
except cr.CheckError as exc:
    assert "expected exactly 1" in str(exc)
else:
    raise AssertionError("stale prepared_from_sha (2 hops back) was not detected")

try:
    cr.check_preparation_provenance(PR28_MERGE, PR28_MERGE)
except cr.CheckError as exc:
    assert "nothing was merged" in str(exc)
else:
    raise AssertionError("prepared_from_sha == candidate_sha was not detected")

print("PASS: check_preparation_provenance against real 6.0.0..6.0.1 first-parent history")


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

report = cr.Report()
report.add_pass("a", "ok")
report.add_fail("b", "bad")
report.add_warn("c", "meh")
assert not report.passed
assert len(report.errors) == 1
assert len(report.warnings) == 1
rendered = report.render()
assert "[PASS] a: ok" in rendered
assert "[FAIL] b: bad" in rendered
assert "[WARN] c: meh" in rendered

clean_report = cr.Report()
clean_report.add_pass("a", "ok")
clean_report.add_warn("c", "meh")
assert clean_report.passed  # warnings alone do not fail a report

print("PASS: Report tracks pass/fail/warning and 'passed' ignores warnings")


# ---------------------------------------------------------------------------
# FakeGithubAdapter-driven integration tests of run_candidate /
# run_verify_published, replicating the real 6.0.1 shape with deliberate
# mutations to prove each check actually fires on drift.
# ---------------------------------------------------------------------------

GOOD_RELEASE = {
    "tag_name": "6.0.1",
    "target_commitish": PR28_MERGE,
    "draft": False,
    "prerelease": False,
    "published_at": "2026-09-15T09:13:37Z",
    "body": "placeholder",
}
GOOD_TAG = {"sha": PR28_MERGE, "type": "commit"}


def _make_adapter(release=None, tag=None, main_sha=PR28_MERGE, checks=None, version="6.0.1"):
    return cr.FakeGithubAdapter(
        main_sha=main_sha,
        releases={version: release} if release else {},
        tags={version: tag} if tag else {},
        check_conclusions_by_sha={PR28_MERGE: checks or {"validate": "success"}},
    )


good_adapter = _make_adapter(release=GOOD_RELEASE, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, good_adapter, offline=False, allow_missing_release_note=True)
# 6.0.1 is no longer the live README/CHANGELOG version (6.1.0 has since
# shipped, permanently -- main only moves forward), so readme-version
# -matches now legitimately fails here too, alongside the two pre
# -existing legacy-6.0.1 warnings (missing release note, no notify run
# in this fixture). This is exactly and only that -- everything else
# about this fixture (tag, release draft/prerelease/timestamp/body) is
# genuinely clean, which is the actual point of this test.
EXPECTED_NON_PASSING_FOR_HISTORICAL_6_0_1 = {"readme-version-matches", "release-note-artifact", "notify-workflow-conclusion"}
assert {f.check for f in report.findings if not f.ok} == EXPECTED_NON_PASSING_FOR_HISTORICAL_6_0_1, report.render()

annotated_adapter = _make_adapter(release=GOOD_RELEASE, tag={"sha": PR28_MERGE, "type": "tag"})
report = cr.run_verify_published("6.0.1", PR28_MERGE, annotated_adapter, offline=False, allow_missing_release_note=True)
assert not report.passed
assert any("tag-directness" in f.check and not f.ok for f in report.findings)

wrong_sha_adapter = _make_adapter(release=GOOD_RELEASE, tag={"sha": "f" * 40, "type": "commit"})
report = cr.run_verify_published("6.0.1", PR28_MERGE, wrong_sha_adapter, offline=False, allow_missing_release_note=True)
assert not report.passed
assert any("tag-sha-matches" in f.check and not f.ok for f in report.findings)

draft_adapter = _make_adapter(release={**GOOD_RELEASE, "draft": True}, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, draft_adapter, offline=False, allow_missing_release_note=True)
assert not report.passed
assert any("release-draft-false" in f.check and not f.ok for f in report.findings)

prerelease_adapter = _make_adapter(release={**GOOD_RELEASE, "prerelease": True}, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, prerelease_adapter, offline=False, allow_missing_release_note=True)
assert not report.passed
assert any("release-prerelease-false" in f.check and not f.ok for f in report.findings)

missing_release_adapter = _make_adapter(release=None, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, missing_release_adapter, offline=False, allow_missing_release_note=True)
assert not report.passed
assert any("release-exists" in f.check and not f.ok for f in report.findings)

missing_note_adapter = _make_adapter(release=GOOD_RELEASE, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, missing_note_adapter, offline=False, allow_missing_release_note=False)
assert not report.passed
assert any("release-note-artifact" in f.check and not f.ok for f in report.findings)

print("PASS: run_verify_published fires on annotated tag / wrong sha / draft / prerelease / missing release / missing legacy-note-flag")

# candidate mode: main != expected sha must fail; tag-already-exists must fail.
wrong_main_adapter = _make_adapter(release=None, tag=None, main_sha="c" * 40)
report = cr.run_candidate("9.9.9", PR28_MERGE, wrong_main_adapter, offline=False)
assert any("main-equals-candidate" in f.check and not f.ok for f in report.findings)

tag_exists_adapter = _make_adapter(release=None, tag={"sha": PR28_MERGE, "type": "commit"}, main_sha=PR28_MERGE)
report = cr.run_candidate("6.0.1", PR28_MERGE, tag_exists_adapter, offline=False)
assert any("tag-still-absent" in f.check and not f.ok for f in report.findings)

print("PASS: run_candidate fires on main!=candidate and tag-already-exists")


# ---------------------------------------------------------------------------
# Release-note readiness: check_release_note_curated() -- exact-marker
# matching only, shared constants (DRAFT_NOTICE / DRAFT_HEADING_MARKER)
# live in this module and are imported by prepare_release_housekeeping.py,
# never duplicated.
# ---------------------------------------------------------------------------

GENERATED_DRAFT_NOTE = prh.generate_release_note(
    "6.0.2",
    "6.0.1",
    "### Bug Fixes\n\n- fixed a thing.",
    {
        "streamnzb_version": "6.1.0",
        "streamnzb_sha": "2ff93449e59a6597f25fd008e3440920772578c3",
        "jhin_version": "0.7.1",
    },
    {"samsung": 151, "neutral": 150, "core": 129, "presentation": 21, "device": 1, "defines": 62},
    "d4s87/streamnzb-template",
)
assert cr.DRAFT_NOTICE in GENERATED_DRAFT_NOTE
assert cr.DRAFT_HEADING_MARKER in GENERATED_DRAFT_NOTE

# A. generated draft rejection: the exact generator output must fail readiness.
ok, detail = cr.check_release_note_curated(GENERATED_DRAFT_NOTE)
assert not ok
assert "DRAFT_NOTICE" in detail or "draft marker" in detail

print("PASS: check_release_note_curated rejects an untouched generator-produced draft")

# B. curated note acceptance: strip only the two generator markers, keep
# version/compatibility/counts/changelog-link content intact.
CURATED_NOTE = GENERATED_DRAFT_NOTE.replace(
    f"# DraCuLa StreamNZB Template 6.0.2 {cr.DRAFT_HEADING_MARKER}",
    "# DraCuLa StreamNZB Template 6.0.2",
).replace(cr.DRAFT_NOTICE + "\n\n", "")

assert cr.DRAFT_NOTICE not in CURATED_NOTE
assert cr.DRAFT_HEADING_MARKER not in CURATED_NOTE
assert "6.0.2" in CURATED_NOTE
assert "StreamNZB: 6.1.0" in CURATED_NOTE
assert "Samsung QN90A: **151**" in CURATED_NOTE
assert "compare/6.0.1...6.0.2" in CURATED_NOTE

ok, detail = cr.check_release_note_curated(CURATED_NOTE)
assert ok, detail

print("PASS: check_release_note_curated accepts a curated note with markers removed and content intact")


# ---------------------------------------------------------------------------
# C. candidate integration: an untouched generated draft under
# .release/<version>.md must produce a hard FAIL "release-note-curated";
# a curated note at the same path must pass it. Uses a temporary directory
# swapped in for cr.RELEASE_DIR -- never touches the repository's real
# .release/ directory.
# ---------------------------------------------------------------------------

def _run_candidate_with_release_dir(version, sha, github, note_text, provenance):
    original_release_dir = cr.RELEASE_DIR
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir)
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        (tmp_release_dir / f"{version}.json").write_text(json.dumps(provenance), encoding="utf-8")
        cr.RELEASE_DIR = tmp_release_dir
        try:
            return cr.run_candidate(version, sha, github, offline=False)
        finally:
            cr.RELEASE_DIR = original_release_dir


fixture_provenance = {
    "schema_version": 1,
    "version": "6.0.1",
    "previous_version": "6.0.0",
    "prepared_from_sha": pr27_full,
    "prepared_at_date": "2026-09-15",
    "compatibility": {},
    "expected_counts": {},
}
candidate_adapter = _make_adapter(release=None, tag=None, main_sha=PR28_MERGE)

draft_report = _run_candidate_with_release_dir(
    "6.0.1", PR28_MERGE, candidate_adapter, GENERATED_DRAFT_NOTE.replace("6.0.2", "6.0.1"), fixture_provenance
)
draft_finding = next(f for f in draft_report.findings if f.check == "release-note-curated")
assert not draft_finding.ok and draft_finding.severity == "error", draft_finding.render()
assert "edit the generated public release note" in draft_finding.detail

curated_report = _run_candidate_with_release_dir(
    "6.0.1", PR28_MERGE, candidate_adapter, CURATED_NOTE.replace("6.0.2", "6.0.1").replace("6.0.1...6.0.2", "6.0.0...6.0.1"), fixture_provenance
)
curated_finding = next(f for f in curated_report.findings if f.check == "release-note-curated")
assert curated_finding.ok, curated_finding.render()

print("PASS: run_candidate hard-fails 'release-note-curated' on an untouched draft, passes on a curated note (temp-isolated, no repo .release/ mutation)")


# ---------------------------------------------------------------------------
# D. verify-published body mismatch: for a future-style release with
# .release/<version>.md present, an exact-match body passes and a
# differing GitHub release body is now a hard FAIL (was a warning).
# ---------------------------------------------------------------------------

def _run_verify_published_with_release_dir(version, sha, github, note_text, **kwargs):
    original_release_dir = cr.RELEASE_DIR
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_release_dir = Path(tmpdir)
        (tmp_release_dir / f"{version}.md").write_text(note_text, encoding="utf-8")
        cr.RELEASE_DIR = tmp_release_dir
        try:
            return cr.run_verify_published(version, sha, github, offline=False, **kwargs)
        finally:
            cr.RELEASE_DIR = original_release_dir


FUTURE_NOTE = "# DraCuLa StreamNZB Template 6.0.2\n\nSome curated public prose.\n"

exact_match_adapter = _make_adapter(
    release={**GOOD_RELEASE, "tag_name": "6.0.2", "body": FUTURE_NOTE.strip()}, tag=GOOD_TAG, version="6.0.2"
)
report = _run_verify_published_with_release_dir("6.0.2", PR28_MERGE, exact_match_adapter, FUTURE_NOTE)
finding = next(f for f in report.findings if f.check == "release-body-matches-artifact")
assert finding.ok, finding.render()

mismatched_adapter = _make_adapter(
    release={**GOOD_RELEASE, "tag_name": "6.0.2", "body": "some other body entirely"}, tag=GOOD_TAG, version="6.0.2"
)
report = _run_verify_published_with_release_dir("6.0.2", PR28_MERGE, mismatched_adapter, FUTURE_NOTE)
finding = next(f for f in report.findings if f.check == "release-body-matches-artifact")
assert not finding.ok and finding.severity == "error", finding.render()
assert "authoritative" in finding.detail

print("PASS: run_verify_published hard-fails 'release-body-matches-artifact' on drift, passes on an exact match")


# ---------------------------------------------------------------------------
# E. legacy behavior: 6.0.1 with --allow-missing-release-note still passes
# with a WARNING (not a failure, not silently absent) for the missing
# artifact -- unaffected by the hard-fail change above, since that change
# only fires when .release/<version>.md exists.
# ---------------------------------------------------------------------------

legacy_report = cr.run_verify_published(
    "6.0.1", PR28_MERGE, good_adapter, offline=False, allow_missing_release_note=True
)
legacy_finding = next(f for f in legacy_report.findings if f.check == "release-note-artifact")
assert not legacy_finding.ok and legacy_finding.severity == "warning", legacy_finding.render()
# A warning alone must not fail the report -- but 6.0.1 is also, by now,
# permanently a historical (non-live) version, so readme-version-matches
# legitimately fails here too (see EXPECTED_NON_PASSING_FOR_HISTORICAL_6_0_1
# above). Assert precisely that set rather than a blanket `.passed`, which
# can never hold again for this fixture once main advances past 6.0.1.
assert {f.check for f in legacy_report.findings if not f.ok} == EXPECTED_NON_PASSING_FOR_HISTORICAL_6_0_1, legacy_report.render()

# Without the legacy flag, the same missing artifact must be a hard FAIL.
no_legacy_report = cr.run_verify_published(
    "6.0.1", PR28_MERGE, good_adapter, offline=False, allow_missing_release_note=False
)
no_legacy_finding = next(f for f in no_legacy_report.findings if f.check == "release-note-artifact")
assert not no_legacy_finding.ok and no_legacy_finding.severity == "error"
assert not no_legacy_report.passed

print("PASS: legacy --allow-missing-release-note still warns (not fails) only for 6.0.1-style explicit opt-in")

# The legacy exception must not generalize to any other version: passing
# --allow-missing-release-note for a future release must still hard-fail
# (CodeRabbit finding -- the flag previously bypassed the requirement for
# every version, not just the documented 6.0.1 exception).
future_adapter = _make_adapter(release=None, tag=None, main_sha=PR28_MERGE, version="9.9.9")
future_legacy_report = cr.run_verify_published(
    "9.9.9", PR28_MERGE, future_adapter, offline=False, allow_missing_release_note=True
)
future_finding = next(f for f in future_legacy_report.findings if f.check == "release-note-artifact")
assert not future_finding.ok and future_finding.severity == "error", future_finding.render()
assert not future_legacy_report.passed
assert cr.LEGACY_RELEASE_NOTE_EXCEPTION_VERSION in future_finding.detail

print("PASS: --allow-missing-release-note does not generalize past the documented 6.0.1 exception")


# ---------------------------------------------------------------------------
# SHA case normalization (CodeRabbit finding): validate_sha lowercases its
# input, but the normalized value must actually be used for comparisons --
# an operator passing --sha with uppercase hex must not see spurious
# main-equals-candidate / tag-sha-matches / provenance-freshness failures.
# ---------------------------------------------------------------------------

uppercase_sha = PR28_MERGE.upper()
report = cr.run_verify_published(
    "6.0.1", uppercase_sha, good_adapter, offline=False, allow_missing_release_note=True
)
# Same historical-6.0.1 caveat as above: only readme-version-matches/
# release-note-artifact/notify-workflow-conclusion are expected to be
# non-passing; everything SHA-case-sensitive (tag-exists/tag-directness/
# tag-sha-matches/release-target-commitish) must be clean, which is what
# this test actually verifies.
assert {f.check for f in report.findings if not f.ok} == EXPECTED_NON_PASSING_FOR_HISTORICAL_6_0_1, report.render()
tag_sha_finding = next(f for f in report.findings if f.check == "tag-sha-matches")
assert tag_sha_finding.ok, tag_sha_finding.render()

candidate_report = cr.run_candidate("6.0.1", uppercase_sha, candidate_adapter, offline=False)
main_finding = next(f for f in candidate_report.findings if f.check == "main-equals-candidate")
assert main_finding.ok, main_finding.render()

print("PASS: uppercase --sha input is normalized before comparison in both verify-published and candidate")

print("PASS: check_release tests")
