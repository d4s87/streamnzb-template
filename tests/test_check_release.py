#!/usr/bin/env python3

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import check_release as cr  # noqa: E402


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
    "samsung": 151,
    "neutral": 150,
    "core": 129,
    "presentation": 21,
    "device": 1,
    "defines": 62,
}, f"unexpected baseline counts: {counts}"

baseline = cr.load_compatibility_baseline()
assert baseline["streamnzb_version"] == "6.1.0"
assert baseline["jhin_version"] == "0.7.1"
assert baseline["jhin_version_go_mod"] == "0.7.1"
assert baseline["streamnzb_sha"] == "2ff93449e59a6597f25fd008e3440920772578c3"
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
assert readme_version == "6.0.1"

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


def _make_adapter(release=None, tag=None, main_sha=PR28_MERGE, checks=None):
    return cr.FakeGithubAdapter(
        main_sha=main_sha,
        releases={"6.0.1": release} if release else {},
        tags={"6.0.1": tag} if tag else {},
        check_conclusions_by_sha={PR28_MERGE: checks or {"validate": "success"}},
    )


good_adapter = _make_adapter(release=GOOD_RELEASE, tag=GOOD_TAG)
report = cr.run_verify_published("6.0.1", PR28_MERGE, good_adapter, offline=False, allow_missing_release_note=True)
assert report.passed, report.render()

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

print("PASS: check_release tests")
