#!/usr/bin/env python3
"""
Canonical release-readiness checker (Phase 1: read-only foundation).

Three modes, matching the two future human-triggered workflow gates plus
post-publication verification:

    prepare           -- before generating release housekeeping
    candidate         -- after the release-prep PR has merged to main,
                          before tagging/publishing
    verify-published  -- after a release has been published (or, with
                          --allow-missing-release-note, for a historical
                          release that predates this tooling)

This script never creates branches, pushes, tags, or publishes anything.
See CLAUDE.md's release-automation section for the full two-gate design.
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README_PATH = ROOT / "README.md"
CHANGELOG_PATH = ROOT / "CHANGELOG.md"
VARIANTS_PATH = ROOT / "profiles" / "variants.json"
RULES_PATH = ROOT / "profiles" / "rules.json"
DEFINES_PATH = ROOT / "generated" / "streamnzb-defines.txt"
GO_MOD_PATH = ROOT / "tests" / "streamnzb_compat" / "go.mod"
COMPAT_SCRIPT_PATH = ROOT / "scripts" / "test_streamnzb_compat.sh"
RELEASE_DIR = ROOT / ".release"
PREPARE_WORKFLOW_FILE = "prepare-release.yml"
PUBLISH_WORKFLOW_FILE = "publish-release.yml"

VERSION_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+$")
SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")

RELEASE_IMPACT_LABELS = ("major", "minor", "patch")
SKIP_LABEL = "skip-changelog"

# The one documented legacy release predating .release/<version>.md (see
# .release/README.md). --allow-missing-release-note is scoped to exactly
# this version -- it must never let a later release skip the requirement.
LEGACY_RELEASE_NOTE_EXCEPTION_VERSION = "6.0.1"

NOTICE_LINE = (
    "Changes in this section are under development and are not part of "
    "the latest stable release."
)

# Generator-owned markers (scripts/prepare_release_housekeeping.py) that
# identify an untouched .release/<version>.md draft seed. Defined here, not
# in the generator module, so both the generator and this checker can
# import them from one place without a circular import (the generator
# already imports from this module). Exact-marker matching only -- no
# heuristic "does this look edited" prose detection.
DRAFT_HEADING_MARKER = "— release notes draft"
DRAFT_NOTICE = (
    "<!-- Auto-generated draft, seeded verbatim from the CHANGELOG section below. "
    "Condense for public release-note tone before merging the release-preparation "
    "PR -- this is a review starting point, not final prose. -->"
)

CHANGELOG_UNRELEASED_RE = re.compile(
    r"^## \[Unreleased\]\(https://github\.com/(?P<slug>[^/]+/[^/]+)"
    r"/compare/(?P<prev>[0-9]+\.[0-9]+\.[0-9]+)\.\.\.HEAD\)[ \t]*$",
    re.MULTILINE,
)
CHANGELOG_DATED_SECTION_RE = re.compile(
    r"^## \[(?P<version>[0-9]+\.[0-9]+\.[0-9]+)\]"
    r"\(https://github\.com/(?P<slug>[^/]+/[^/]+)/compare/"
    # The left side of a historical compare link may still be an old
    # v-prefixed tag (e.g. "v5.2", pre-6.0.0 convention); the section's own
    # version and the right side of its own compare link are always bare
    # semver, both historically and going forward -- see the "bare-semver
    # convention" check in candidate/verify-published modes.
    r"(?P<prev>v?[0-9]+\.[0-9]+(?:\.[0-9]+)?)\.\.\.(?P<cur>[0-9]+\.[0-9]+\.[0-9]+)\)"
    r" \((?P<date>\d{4}-\d{2}-\d{2})\)[ \t]*$",
    re.MULTILINE,
)
README_VERSION_RE = re.compile(
    r"^(\*\*Current version: )([0-9]+\.[0-9]+\.[0-9]+)(\*\*  )$",
    re.MULTILINE,
)
README_COMPAT_RE = re.compile(
    r"^\*\*Compatibility: StreamNZB (?P<streamnzb_version>[0-9]+\.[0-9]+\.[0-9]+)"
    r" / Jhin (?P<jhin_version>[0-9]+\.[0-9]+\.[0-9]+)\*\*\s*$",
    re.MULTILINE,
)
GO_MOD_JHIN_RE = re.compile(
    r"github\.com/dreulavelle/jhin v(?P<version>[0-9]+\.[0-9]+\.[0-9]+)"
)
COMPAT_SCRIPT_SHA_RE = re.compile(
    r'STREAMNZB_REF="\$\{STREAMNZB_REF:-(?P<sha>[0-9a-f]{40})\}"'
)
DEFINE_LINE_RE = re.compile(
    r"^(?P<name>.+?)(?: \[(?P<scope>[^\]]+)\])?: define if (?P<condition>.+)$"
)


class CheckError(Exception):
    """A release check failed; message is human-readable."""


class GithubUnavailableError(CheckError):
    """`gh` is missing/unauthenticated but a check needs GitHub state."""


# ---------------------------------------------------------------------------
# Input validation -- never interpolate unvalidated version/sha into shell
# fragments; these are the single choke point every mode routes through.
# ---------------------------------------------------------------------------

def validate_version(value):
    if not isinstance(value, str) or not VERSION_RE.match(value):
        raise CheckError(
            f"invalid version {value!r}: must match {VERSION_RE.pattern} "
            "(bare semver, no 'v' prefix, no prerelease/build metadata)"
        )
    return value


def validate_sha(value):
    if not isinstance(value, str) or not SHA_RE.match(value):
        raise CheckError(
            f"invalid sha {value!r}: must be exactly 40 hexadecimal characters"
        )
    return value.lower()


def version_key(version):
    return tuple(int(part) for part in version.split("."))


def branch_name_for_version(version):
    return "docs/prepare-release-" + version.replace(".", "-")


# ---------------------------------------------------------------------------
# Local git helpers -- argument arrays only, never shell=True.
# ---------------------------------------------------------------------------

def _run(args, cwd=ROOT):
    return subprocess.run(
        args, cwd=cwd, capture_output=True, text=True, check=False
    )


def _git(*args):
    result = _run(["git", *args])
    if result.returncode != 0:
        raise CheckError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def git_local_main_sha():
    """Local clone's own `main` ref. May be stale relative to GitHub if this
    clone hasn't fetched -- callers needing authoritative truth should use
    GithubAdapter.main_sha() instead."""
    return _git("rev-parse", "main")


def git_local_branch_exists(name):
    result = _run(["git", "rev-parse", "--verify", "--quiet", f"refs/heads/{name}"])
    return result.returncode == 0


def git_tag_exists_locally(tag):
    result = _run(["git", "rev-parse", "--verify", "--quiet", f"refs/tags/{tag}"])
    return result.returncode == 0


def git_tag_target_sha(tag):
    return _git("rev-list", "-n1", tag)


def git_tag_object_type(tag):
    """'commit' for a lightweight/direct tag, 'tag' for an annotated tag
    object -- project convention requires the former (see CLAUDE.md)."""
    return _git("cat-file", "-t", tag)


def git_first_parent_log(rev_range):
    out = _git("log", "--first-parent", "--format=%H", rev_range)
    return [line for line in out.splitlines() if line]


def resolve_repo_slug():
    url = _git("config", "--get", "remote.origin.url")
    match = re.search(r"github\.com[:/](?P<slug>[^/]+/[^/]+?)(?:\.git)?$", url)
    if not match:
        raise CheckError(f"cannot resolve GitHub repo slug from remote.origin.url={url!r}")
    return match.group("slug")


# ---------------------------------------------------------------------------
# GitHub adapter abstraction. One checker, not separate local/Actions
# implementations: core validation functions take a GithubAdapter; the
# production adapter shells out to `gh`, tests inject a deterministic fake.
# ---------------------------------------------------------------------------

class GithubAdapter:
    def main_sha(self):
        raise NotImplementedError

    def latest_stable_release(self):
        """-> {tag_name, target_commitish, draft, prerelease, published_at,
        body} for the newest non-draft, non-prerelease semver-tagged
        release, or None if there isn't one."""
        raise NotImplementedError

    def release(self, tag):
        """-> same shape as latest_stable_release() for one named tag, or
        None if no release exists for that tag."""
        raise NotImplementedError

    def tag_ref(self, tag):
        """-> {"sha": <sha the ref points at>, "type": "commit"|"tag"} or
        None if the tag doesn't exist."""
        raise NotImplementedError

    def associated_prs(self, sha):
        """-> list of {number, title, labels, merged_at, merge_commit_sha}
        for PRs GitHub associates with this commit, regardless of merge
        strategy (merge commit or squash)."""
        raise NotImplementedError

    def check_conclusions(self, sha):
        """-> {check_run_name: conclusion} for a commit SHA."""
        raise NotImplementedError

    def latest_workflow_run(self, workflow_file, event=None):
        """-> {conclusion, status, html_url} for the most recent run of a
        workflow file, optionally filtered by triggering event, or None."""
        raise NotImplementedError


class GhCliAdapter(GithubAdapter):
    """Production adapter: shells out to the `gh` CLI (argument arrays
    only). Fails loudly and immediately if `gh` is missing/unauthenticated
    rather than letting individual checks silently degrade."""

    def __init__(self, repo=None):
        self.repo = repo or resolve_repo_slug()
        self._ensure_available()

    def _ensure_available(self):
        try:
            result = _run(["gh", "auth", "status"])
        except FileNotFoundError as exc:
            raise GithubUnavailableError(
                "gh CLI is not installed; GitHub-dependent checks cannot run "
                "(pass --offline to skip them explicitly)"
            ) from exc
        if result.returncode != 0:
            raise GithubUnavailableError(
                "gh CLI is not authenticated; GitHub-dependent checks cannot run "
                "(pass --offline to skip them explicitly)"
            )

    def _api(self, path, allow_404=False):
        result = _run(["gh", "api", path])
        if result.returncode != 0:
            # Only an explicit 404 means "doesn't exist" -- gh reports it as
            # "gh: <message> (HTTP 404)" on stderr. Anything else (rate
            # limiting, 5xx, network failure, auth loss mid-run) must
            # propagate as a real error: a safety check that silently
            # treats "the API call failed" as "the tag/release is absent"
            # fails open exactly where it must fail closed.
            if allow_404 and "HTTP 404" in result.stderr:
                return None
            raise CheckError(f"gh api {path} failed: {result.stderr.strip()}")
        return json.loads(result.stdout)

    @staticmethod
    def _release_view(raw):
        return {
            "tag_name": raw["tag_name"],
            "target_commitish": raw["target_commitish"],
            "draft": raw["draft"],
            "prerelease": raw["prerelease"],
            "published_at": raw.get("published_at"),
            "body": raw.get("body") or "",
        }

    def main_sha(self):
        return self._api(f"repos/{self.repo}/git/refs/heads/main")["object"]["sha"]

    def latest_stable_release(self):
        releases = self._api(f"repos/{self.repo}/releases")
        stable = [
            r for r in releases
            if not r["draft"] and not r["prerelease"] and VERSION_RE.match(r["tag_name"])
        ]
        if not stable:
            return None
        stable.sort(key=lambda r: version_key(r["tag_name"]))
        return self._release_view(stable[-1])

    def release(self, tag):
        raw = self._api(f"repos/{self.repo}/releases/tags/{tag}", allow_404=True)
        if raw is None:
            return None
        return self._release_view(raw)

    def tag_ref(self, tag):
        raw = self._api(f"repos/{self.repo}/git/refs/tags/{tag}", allow_404=True)
        if raw is None:
            return None
        return {"sha": raw["object"]["sha"], "type": raw["object"]["type"]}

    def associated_prs(self, sha):
        prs = self._api(f"repos/{self.repo}/commits/{sha}/pulls")
        return [
            {
                "number": pr["number"],
                "title": pr["title"],
                "labels": sorted(label["name"] for label in pr.get("labels", [])),
                "merged_at": pr.get("merged_at"),
                "merge_commit_sha": pr.get("merge_commit_sha"),
            }
            for pr in prs
        ]

    def check_conclusions(self, sha):
        data = self._api(f"repos/{self.repo}/commits/{sha}/check-runs")
        return {run["name"]: run["conclusion"] for run in data.get("check_runs", [])}

    def latest_workflow_run(self, workflow_file, event=None):
        data = self._api(f"repos/{self.repo}/actions/workflows/{workflow_file}/runs")
        runs = data.get("workflow_runs", [])
        if event:
            runs = [r for r in runs if r.get("event") == event]
        if not runs:
            return None
        runs.sort(key=lambda r: r["run_number"], reverse=True)
        top = runs[0]
        return {
            "conclusion": top["conclusion"],
            "status": top["status"],
            "html_url": top["html_url"],
        }


class FakeGithubAdapter(GithubAdapter):
    """Deterministic in-memory adapter for tests. Construct with canned
    data; never touches the network or `gh`."""

    def __init__(
        self,
        main_sha=None,
        releases=None,
        tags=None,
        prs_by_sha=None,
        check_conclusions_by_sha=None,
        workflow_runs=None,
    ):
        self._main_sha = main_sha
        self._releases = releases or {}
        self._tags = tags or {}
        self._prs_by_sha = prs_by_sha or {}
        self._check_conclusions_by_sha = check_conclusions_by_sha or {}
        self._workflow_runs = workflow_runs or {}

    def main_sha(self):
        return self._main_sha

    def latest_stable_release(self):
        stable = [
            r for r in self._releases.values()
            if not r["draft"] and not r["prerelease"] and VERSION_RE.match(r["tag_name"])
        ]
        if not stable:
            return None
        stable.sort(key=lambda r: version_key(r["tag_name"]))
        return stable[-1]

    def release(self, tag):
        return self._releases.get(tag)

    def tag_ref(self, tag):
        return self._tags.get(tag)

    def associated_prs(self, sha):
        return self._prs_by_sha.get(sha, [])

    def check_conclusions(self, sha):
        return self._check_conclusions_by_sha.get(sha, {})

    def latest_workflow_run(self, workflow_file, event=None):
        return self._workflow_runs.get(workflow_file)


# ---------------------------------------------------------------------------
# Repository-local, deterministic readers (no GitHub state needed).
# ---------------------------------------------------------------------------

def load_current_counts(root=ROOT):
    variants = json.loads((root / "profiles" / "variants.json").read_text(encoding="utf-8"))
    rules = json.loads((root / "profiles" / "rules.json").read_text(encoding="utf-8"))

    expected_rules = {v["artifact"]: v["expected_rules"] for v in variants["variants"]}

    owners = {}
    for entry in rules["rules"]:
        owners[entry["owner"]] = owners.get(entry["owner"], 0) + 1

    defines_text = (root / "generated" / "streamnzb-defines.txt").read_text(encoding="utf-8")

    return {
        "samsung": expected_rules.get("profile.txt"),
        "neutral": expected_rules.get("profile-neutral.txt"),
        "core": owners.get("core", 0),
        "presentation": owners.get("presentation", 0),
        "device": owners.get("device:samsung-qn90a", 0),
        "defines": count_defines(defines_text),
    }


def count_defines(text):
    count = 0
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if not DEFINE_LINE_RE.match(line):
            raise CheckError(f"unrecognized Define Library line: {raw_line!r}")
        count += 1
    return count


def load_compatibility_baseline(root=ROOT):
    readme_text = (root / "README.md").read_text(encoding="utf-8")
    match = README_COMPAT_RE.search(readme_text)
    if not match:
        raise CheckError("README.md compatibility line not found or malformed")

    go_mod_text = (root / "tests" / "streamnzb_compat" / "go.mod").read_text(encoding="utf-8")
    go_match = GO_MOD_JHIN_RE.search(go_mod_text)
    if not go_match:
        raise CheckError("tests/streamnzb_compat/go.mod does not pin github.com/dreulavelle/jhin")

    compat_script_text = (root / "scripts" / "test_streamnzb_compat.sh").read_text(encoding="utf-8")
    sha_match = COMPAT_SCRIPT_SHA_RE.search(compat_script_text)
    if not sha_match:
        raise CheckError("scripts/test_streamnzb_compat.sh does not pin STREAMNZB_REF")

    return {
        "streamnzb_version": match.group("streamnzb_version"),
        "jhin_version": match.group("jhin_version"),
        "jhin_version_go_mod": go_match.group("version"),
        "streamnzb_sha": sha_match.group("sha"),
    }


def check_compatibility_internal_consistency(baseline):
    if baseline["jhin_version"] != baseline["jhin_version_go_mod"]:
        raise CheckError(
            f"README.md claims Jhin {baseline['jhin_version']!r} but "
            f"tests/streamnzb_compat/go.mod pins {baseline['jhin_version_go_mod']!r}"
        )


def parse_readme_version(root=ROOT):
    text = (root / "README.md").read_text(encoding="utf-8")
    matches = list(README_VERSION_RE.finditer(text))
    if len(matches) != 1:
        raise CheckError(
            f"README.md version anchor matched {len(matches)} times, expected exactly 1"
        )
    return matches[0].group(2)


def parse_changelog(text):
    """Return {previous_version, dated_sections: [{version, previous, date}]}."""
    unreleased_matches = list(CHANGELOG_UNRELEASED_RE.finditer(text))
    if len(unreleased_matches) != 1:
        raise CheckError(
            f"CHANGELOG.md Unreleased header matched {len(unreleased_matches)} times, "
            "expected exactly 1"
        )
    unreleased = unreleased_matches[0]

    after_header = text[unreleased.end():]
    notice_re = re.compile(r"^\n\n" + re.escape(NOTICE_LINE) + r"\n", re.MULTILINE)
    if not notice_re.match(after_header):
        raise CheckError("CHANGELOG.md Unreleased notice paragraph not found immediately after header")

    dated_sections = [
        {
            "version": m.group("version"),
            "previous": m.group("prev"),
            "date": m.group("date"),
        }
        for m in CHANGELOG_DATED_SECTION_RE.finditer(text)
    ]

    return {
        "previous_version": unreleased.group("prev"),
        "dated_sections": dated_sections,
    }


def find_changelog_section(text, version):
    """Return the section's own text (header line + body, up to the next
    '## [' heading), or None if no dated section exists for `version`."""
    match = None
    for candidate in CHANGELOG_DATED_SECTION_RE.finditer(text):
        if candidate.group("version") == version:
            match = candidate
            break
    if match is None:
        return None
    tail = text[match.end():]
    next_section = re.search(r"\n## \[", tail)
    body_end = match.end() + (next_section.start() if next_section else len(tail))
    return text[match.start():body_end].rstrip("\n")


def changelog_section_body(text, version):
    """Body text of a dated section, excluding its own header line."""
    section = find_changelog_section(text, version)
    if section is None:
        return None
    first_newline = section.find("\n")
    if first_newline == -1:
        return ""
    return section[first_newline + 1:].strip("\n")


def check_release_note_curated(text):
    """A generated .release/<version>.md draft is not publication-ready
    while either generator-owned marker remains. Exact-marker matching
    only -- not a "does this look edited" heuristic. Returns (ok, detail)."""
    if DRAFT_NOTICE in text:
        return False, "still contains the generator's DRAFT_NOTICE HTML comment"
    if DRAFT_HEADING_MARKER in text:
        return False, f"heading still contains the generated draft marker {DRAFT_HEADING_MARKER!r}"
    return True, "no generated draft markers present"


# ---------------------------------------------------------------------------
# Commit / PR accounting -- reusable release-delta model.
# ---------------------------------------------------------------------------

def compute_release_delta(github, previous_tag, candidate_sha):
    """First-parent commit range previous_tag..candidate_sha, with each
    commit's associated PR (regardless of merge-commit vs squash strategy)
    and any direct-to-main commit called out explicitly."""
    commit_shas = git_first_parent_log(f"{previous_tag}..{candidate_sha}")

    commits = []
    prs_by_number = {}
    unaccounted = []

    for sha in commit_shas:
        subject = _git("log", "-1", "--format=%s", sha)
        associated = github.associated_prs(sha)

        if not associated:
            unaccounted.append({"sha": sha, "subject": subject})
            commits.append({"sha": sha, "subject": subject, "prs": []})
            continue

        pr_numbers = []
        for pr in associated:
            prs_by_number[pr["number"]] = pr
            pr_numbers.append(pr["number"])

        commits.append({"sha": sha, "subject": subject, "prs": pr_numbers})

    return {
        "previous_tag": previous_tag,
        "candidate_sha": candidate_sha,
        "commits": commits,
        "prs": prs_by_number,
        "unaccounted_commits": unaccounted,
    }


def suggest_version_bump(delta):
    """Suggestion only -- never authoritative. Returns
    {suggested_bump, evidence: [...], warnings: [...]}."""
    evidence = []
    warnings = []
    impact_seen = set()

    for number, pr in sorted(delta["prs"].items()):
        labels = set(pr["labels"])
        impact_labels = labels & set(RELEASE_IMPACT_LABELS)
        is_skip = SKIP_LABEL in labels

        if is_skip:
            evidence.append(f"PR #{number} ({pr['title']!r}): skip-changelog, non-driving")
            if impact_labels:
                warnings.append(
                    f"PR #{number} carries both skip-changelog and "
                    f"{sorted(impact_labels)} -- contradictory metadata, review manually"
                )
            continue

        if len(impact_labels) == 0:
            warnings.append(
                f"PR #{number} ({pr['title']!r}) has no recognized release-impact "
                f"label ({RELEASE_IMPACT_LABELS}) and is not skip-changelog"
            )
            continue

        if len(impact_labels) > 1:
            warnings.append(
                f"PR #{number} ({pr['title']!r}) carries multiple release-impact "
                f"labels: {sorted(impact_labels)}"
            )

        for label in impact_labels:
            impact_seen.add(label)
            evidence.append(f"PR #{number} ({pr['title']!r}): {label}")

    if delta["unaccounted_commits"]:
        shas = ", ".join(c["sha"][:12] for c in delta["unaccounted_commits"])
        warnings.append(
            f"{len(delta['unaccounted_commits'])} direct-to-main commit(s) not "
            f"associated with any PR, cannot be auto-classified: {shas}"
        )

    if "major" in impact_seen:
        suggested = "major"
    elif "minor" in impact_seen:
        suggested = "minor"
    elif "patch" in impact_seen:
        suggested = "patch"
    elif delta["unaccounted_commits"]:
        suggested = None
        evidence.append("unresolved direct-to-main commits -- cannot suggest a bump")
    else:
        suggested = None
        evidence.append(f"no release-impact-labeled changes since {delta['previous_tag']}")

    return {"suggested_bump": suggested, "evidence": evidence, "warnings": warnings}


# ---------------------------------------------------------------------------
# Preparation-freshness invariant (Section 7): equality, not ancestry.
# Phase 2's future release-prep-PR CI step calls this directly against the
# live main SHA while the PR is still open.
# ---------------------------------------------------------------------------

def check_preparation_freshness(prepared_from_sha, live_main_sha):
    if prepared_from_sha == live_main_sha:
        return True, f"prepared_from_sha {prepared_from_sha} still equals live main"
    return False, (
        f"stale: prepared_from_sha {prepared_from_sha} != live main {live_main_sha}; "
        "release housekeeping must be regenerated from current main"
    )


def check_preparation_provenance(prepared_from_sha, candidate_sha):
    """Candidate-mode check: prepared_from_sha must be the immediate
    first-parent predecessor of the candidate (i.e. the only first-parent
    commit between them is the release-prep PR's own merge) -- proof that
    nothing else landed on main between generation and merge."""
    if prepared_from_sha == candidate_sha:
        raise CheckError("prepared_from_sha equals candidate_sha; nothing was merged")
    in_between = git_first_parent_log(f"{prepared_from_sha}..{candidate_sha}")
    if len(in_between) != 1:
        raise CheckError(
            f"expected exactly 1 first-parent commit between prepared_from_sha "
            f"and candidate ({len(in_between)} found: {in_between}); other main "
            "activity landed after preparation, regeneration required"
        )
    if in_between[0] != candidate_sha:
        raise CheckError(
            f"the sole commit between prepared_from_sha and candidate "
            f"({in_between[0]}) is not the candidate itself ({candidate_sha})"
        )


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

class Finding:
    def __init__(self, check, ok, detail, severity="error"):
        self.check = check
        self.ok = ok
        self.detail = detail
        self.severity = severity

    def render(self):
        if self.ok:
            mark = "PASS"
        elif self.severity == "warning":
            mark = "WARN"
        else:
            mark = "FAIL"
        return f"[{mark}] {self.check}: {self.detail}"


class Report:
    def __init__(self):
        self.findings = []

    def add(self, check, ok, detail, severity="error"):
        self.findings.append(Finding(check, ok, detail, severity))

    def add_pass(self, check, detail):
        self.add(check, True, detail)

    def add_fail(self, check, detail):
        self.add(check, False, detail, severity="error")

    def add_warn(self, check, detail):
        self.add(check, False, detail, severity="warning")

    @property
    def errors(self):
        return [f for f in self.findings if not f.ok and f.severity == "error"]

    @property
    def warnings(self):
        return [f for f in self.findings if not f.ok and f.severity == "warning"]

    @property
    def passed(self):
        return not self.errors

    def render(self):
        return "\n".join(f.render() for f in self.findings)


# ---------------------------------------------------------------------------
# Mode: prepare
# ---------------------------------------------------------------------------

def run_prepare(version, github, offline):
    report = Report()

    try:
        validate_version(version)
        report.add_pass("version-format", f"{version!r} matches {VERSION_RE.pattern}")
    except CheckError as exc:
        report.add_fail("version-format", str(exc))
        return report

    try:
        readme_text = README_PATH.read_text(encoding="utf-8")
        README_VERSION_RE_matches = list(README_VERSION_RE.finditer(readme_text))
        if len(README_VERSION_RE_matches) != 1:
            raise CheckError(
                f"README.md version anchor matched {len(README_VERSION_RE_matches)} times"
            )
        report.add_pass(
            "readme-anchor-parseable",
            f"current README version: {README_VERSION_RE_matches[0].group(2)}",
        )
    except CheckError as exc:
        report.add_fail("readme-anchor-parseable", str(exc))

    changelog_parsed = None
    try:
        changelog_text = CHANGELOG_PATH.read_text(encoding="utf-8")
        changelog_parsed = parse_changelog(changelog_text)
        report.add_pass(
            "changelog-parseable",
            f"Unreleased compare-link previous version: {changelog_parsed['previous_version']}, "
            f"{len(changelog_parsed['dated_sections'])} dated section(s) found",
        )
        if any(s["version"] == version for s in changelog_parsed["dated_sections"]):
            report.add_fail(
                "changelog-no-duplicate-section",
                f"CHANGELOG.md already contains a dated section for {version}",
            )
        else:
            report.add_pass("changelog-no-duplicate-section", f"no existing section for {version}")
    except CheckError as exc:
        report.add_fail("changelog-parseable", str(exc))

    try:
        counts = load_current_counts()
        report.add_pass("baseline-counts-readable", str(counts))
    except CheckError as exc:
        report.add_fail("baseline-counts-readable", str(exc))

    try:
        baseline = load_compatibility_baseline()
        check_compatibility_internal_consistency(baseline)
        report.add_pass(
            "compatibility-baseline-readable",
            f"StreamNZB {baseline['streamnzb_version']} ({baseline['streamnzb_sha']}) / "
            f"Jhin {baseline['jhin_version']}",
        )
    except CheckError as exc:
        report.add_fail("compatibility-baseline-readable", str(exc))

    release_note_path = RELEASE_DIR / f"{version}.md"
    if release_note_path.exists():
        report.add_fail("release-note-collision", f"{release_note_path} already exists")
    else:
        report.add_pass("release-note-collision", "no pre-existing release-note artifact")

    release_json_path = RELEASE_DIR / f"{version}.json"
    if release_json_path.exists():
        report.add_fail("release-provenance-collision", f"{release_json_path} already exists")
    else:
        report.add_pass("release-provenance-collision", "no pre-existing provenance artifact")

    branch_name = branch_name_for_version(version)
    if git_local_branch_exists(branch_name):
        report.add_fail("branch-collision", f"local branch {branch_name!r} already exists")
    else:
        report.add_pass(
            "branch-collision", f"branch name {branch_name!r} available (local check only)"
        )

    if git_tag_exists_locally(version):
        report.add_fail("tag-absent-local", f"tag {version!r} already exists locally")
    else:
        report.add_pass("tag-absent-local", f"tag {version!r} not found locally")

    if offline:
        report.add_warn(
            "github-dependent-checks",
            "offline mode requested; latest-stable-release, remote main SHA, remote tag "
            "presence, and PR-accounting checks were SKIPPED (not silently -- rerun without "
            "--offline before actually preparing a release)",
        )
        return report

    try:
        latest = github.latest_stable_release()
        if latest is None:
            report.add_fail("previous-stable-resolved", "no published stable release found on GitHub")
            return report
        report.add_pass(
            "previous-stable-resolved",
            f"latest published stable release: {latest['tag_name']} @ {latest['target_commitish']}",
        )
        previous_version = latest["tag_name"]
    except CheckError as exc:
        report.add_fail("previous-stable-resolved", str(exc))
        return report

    if changelog_parsed and changelog_parsed["previous_version"] != previous_version:
        report.add_fail(
            "changelog-previous-version-agrees-with-github",
            f"CHANGELOG.md Unreleased compare-link says {changelog_parsed['previous_version']!r}, "
            f"GitHub's latest stable release is {previous_version!r}",
        )
    elif changelog_parsed:
        report.add_pass(
            "changelog-previous-version-agrees-with-github",
            f"both agree: {previous_version}",
        )

    try:
        if version_key(version) <= version_key(previous_version):
            report.add_fail(
                "semver-progression",
                f"{version} does not exceed previous stable {previous_version}",
            )
        else:
            report.add_pass("semver-progression", f"{version} > {previous_version}")
    except ValueError as exc:
        report.add_fail("semver-progression", str(exc))

    try:
        remote_tag = github.tag_ref(version)
        if remote_tag is not None:
            report.add_fail("tag-absent-remote", f"tag {version!r} already exists on GitHub")
        else:
            report.add_pass("tag-absent-remote", f"tag {version!r} not found on GitHub")
    except CheckError as exc:
        report.add_fail("tag-absent-remote", str(exc))

    try:
        main_sha = github.main_sha()
        report.add_pass("main-sha-resolved", main_sha)
    except CheckError as exc:
        report.add_fail("main-sha-resolved", str(exc))
        return report

    try:
        delta = compute_release_delta(github, previous_version, main_sha)
        suggestion = suggest_version_bump(delta)
        report.add_pass(
            "release-impact-accounting",
            f"{len(delta['commits'])} commit(s) since {previous_version}; "
            f"suggested bump: {suggestion['suggested_bump']}; evidence: {suggestion['evidence']}",
        )
        for warning in suggestion["warnings"]:
            report.add_warn("release-impact-accounting", warning)
    except CheckError as exc:
        report.add_fail("release-impact-accounting", str(exc))

    return report


# ---------------------------------------------------------------------------
# Mode: candidate
# ---------------------------------------------------------------------------

def run_candidate(version, sha, github, offline):
    report = Report()

    try:
        version = validate_version(version)
        sha = validate_sha(sha)  # normalizes case -- comparisons below assume lowercase
        report.add_pass("input-format", f"version={version!r} sha={sha!r}")
    except CheckError as exc:
        report.add_fail("input-format", str(exc))
        return report

    try:
        actual_readme_version = parse_readme_version()
        if actual_readme_version != version:
            report.add_fail(
                "readme-version-matches",
                f"README.md says {actual_readme_version!r}, expected {version!r}",
            )
        else:
            report.add_pass("readme-version-matches", version)
    except CheckError as exc:
        report.add_fail("readme-version-matches", str(exc))

    changelog_text = CHANGELOG_PATH.read_text(encoding="utf-8")
    try:
        parsed = parse_changelog(changelog_text)
        if parsed["previous_version"] == version:
            report.add_pass(
                "changelog-unreleased-compare-link",
                f"Unreleased -> compare/{version}...HEAD",
            )
        else:
            report.add_fail(
                "changelog-unreleased-compare-link",
                f"Unreleased compare link references {parsed['previous_version']!r}, "
                f"expected {version!r} (i.e. '<version>...HEAD')",
            )
    except CheckError as exc:
        report.add_fail("changelog-unreleased-compare-link", str(exc))
        parsed = None

    section_match = None
    for candidate in CHANGELOG_DATED_SECTION_RE.finditer(changelog_text):
        if candidate.group("version") == version:
            section_match = candidate
            break

    if section_match is None:
        report.add_fail("changelog-section-exists", f"no dated section found for {version}")
        previous_version = None
    else:
        previous_version = section_match.group("prev")
        report.add_pass(
            "changelog-section-exists",
            f"## [{version}] found, compare link {previous_version}...{version}, "
            f"date {section_match.group('date')}",
        )
        if not VERSION_RE.match(previous_version):
            report.add_fail(
                "bare-semver-convention",
                f"compare-link previous version {previous_version!r} is not bare semver "
                "(the old v-prefixed tag style is only acceptable for already-published "
                "history, never for a new release's own compare link)",
            )
        else:
            report.add_pass("bare-semver-convention", previous_version)

        if section_match.group("cur") != version:
            report.add_fail(
                "changelog-release-compare-link",
                f"section heading version {version!r} but compare-link right side is "
                f"{section_match.group('cur')!r}",
            )
        else:
            report.add_pass("changelog-release-compare-link", f"{previous_version}...{version}")

    try:
        baseline = load_compatibility_baseline()
        check_compatibility_internal_consistency(baseline)
        report.add_pass(
            "compatibility-baseline-consistent",
            f"StreamNZB {baseline['streamnzb_version']} / Jhin {baseline['jhin_version']}",
        )
    except CheckError as exc:
        report.add_fail("compatibility-baseline-consistent", str(exc))

    try:
        counts = load_current_counts()
        report.add_pass("profile-define-counts", str(counts))
    except CheckError as exc:
        report.add_fail("profile-define-counts", str(exc))

    check_result = _run(["python3", str(ROOT / "scripts" / "build_profiles.py"), "check"])
    if check_result.returncode != 0:
        report.add_fail(
            "profile-generation-sync",
            f"build_profiles.py check failed: {check_result.stderr.strip() or check_result.stdout.strip()}",
        )
    else:
        report.add_pass("profile-generation-sync", "build_profiles.py check passed")

    formatter_check = _run(
        ["python3", str(ROOT / "scripts" / "build_formatter.py"), "--check"]
    )
    if formatter_check.returncode != 0:
        report.add_fail(
            "formatter-generation-sync",
            f"build_formatter.py --check failed: "
            f"{formatter_check.stderr.strip() or formatter_check.stdout.strip()}",
        )
    else:
        report.add_pass("formatter-generation-sync", "build_formatter.py --check passed")

    release_note_path = RELEASE_DIR / f"{version}.md"
    if not release_note_path.exists():
        report.add_fail("release-note-exists", f"{release_note_path} not found")
    elif not release_note_path.read_text(encoding="utf-8").strip():
        report.add_fail("release-note-exists", f"{release_note_path} is empty")
    else:
        note_text = release_note_path.read_text(encoding="utf-8")
        report.add_pass("release-note-exists", f"{release_note_path} present, non-empty")
        if version not in note_text:
            report.add_fail(
                "release-note-references-version",
                f"{release_note_path} does not mention {version!r}",
            )
        else:
            report.add_pass("release-note-references-version", version)

        curated_ok, curated_detail = check_release_note_curated(note_text)
        if curated_ok:
            report.add_pass("release-note-curated", curated_detail)
        else:
            report.add_fail(
                "release-note-curated",
                f"{release_note_path} {curated_detail} -- edit the generated public "
                "release note and remove the generated draft markers before this "
                "release-preparation PR may merge",
            )

    provenance_path = RELEASE_DIR / f"{version}.json"
    prepared_from_sha = None
    if not provenance_path.exists():
        report.add_fail("provenance-artifact-exists", f"{provenance_path} not found")
    else:
        try:
            provenance = json.loads(provenance_path.read_text(encoding="utf-8"))
            prepared_from_sha = provenance.get("prepared_from_sha")
            if provenance.get("version") != version:
                report.add_fail(
                    "provenance-version-matches",
                    f"{provenance_path} version={provenance.get('version')!r}, expected {version!r}",
                )
            else:
                report.add_pass("provenance-version-matches", version)
            if not prepared_from_sha or not SHA_RE.match(prepared_from_sha):
                report.add_fail(
                    "provenance-prepared-from-sha-present",
                    f"{provenance_path} missing a valid prepared_from_sha",
                )
            else:
                report.add_pass("provenance-prepared-from-sha-present", prepared_from_sha)
        except (json.JSONDecodeError, OSError) as exc:
            report.add_fail("provenance-artifact-exists", f"{provenance_path} unreadable: {exc}")

    if prepared_from_sha:
        try:
            check_preparation_provenance(prepared_from_sha, sha)
            report.add_pass(
                "provenance-freshness",
                f"exactly one first-parent commit ({sha}) landed after prepared_from_sha "
                f"({prepared_from_sha}) -- no stale/unrelated main activity slipped in",
            )
        except CheckError as exc:
            report.add_fail("provenance-freshness", str(exc))

    if offline:
        report.add_warn(
            "github-dependent-checks",
            "offline mode requested; main==sha, workflow-status, and tag-absence checks "
            "were SKIPPED (not silently)",
        )
        return report

    try:
        remote_main = github.main_sha()
        if remote_main != sha:
            report.add_fail(
                "main-equals-candidate",
                f"GitHub main is {remote_main}, expected candidate {sha}",
            )
        else:
            report.add_pass("main-equals-candidate", sha)
    except CheckError as exc:
        report.add_fail("main-equals-candidate", str(exc))

    try:
        remote_tag = github.tag_ref(version)
        if remote_tag is not None:
            report.add_fail("tag-still-absent", f"tag {version!r} already exists on GitHub")
        else:
            report.add_pass("tag-still-absent", f"tag {version!r} not yet created")
    except CheckError as exc:
        report.add_fail("tag-still-absent", str(exc))

    try:
        conclusions = github.check_conclusions(sha)
        failing = {name: c for name, c in conclusions.items() if c not in ("success", "neutral", "skipped")}
        if not conclusions:
            report.add_warn("required-checks-green", f"no check runs found for {sha}")
        elif failing:
            report.add_fail("required-checks-green", f"non-passing checks: {failing}")
        else:
            report.add_pass("required-checks-green", f"{len(conclusions)} check(s) all passing")
    except CheckError as exc:
        report.add_fail("required-checks-green", str(exc))

    if previous_version:
        try:
            delta = compute_release_delta(github, previous_version, sha)
            skip_only = all(
                SKIP_LABEL in pr["labels"] and not (set(pr["labels"]) & set(RELEASE_IMPACT_LABELS))
                for pr in delta["prs"].values()
            ) if delta["prs"] else True
            report.add_pass(
                "commit-accounting-since-previous-tag",
                f"{len(delta['commits'])} commit(s), {len(delta['prs'])} PR(s), "
                f"{len(delta['unaccounted_commits'])} unaccounted",
            )
            if delta["unaccounted_commits"]:
                report.add_warn(
                    "commit-accounting-since-previous-tag",
                    f"direct-to-main commits present: {delta['unaccounted_commits']}",
                )
        except CheckError as exc:
            report.add_fail("commit-accounting-since-previous-tag", str(exc))

    return report


# ---------------------------------------------------------------------------
# Mode: verify-published
# ---------------------------------------------------------------------------

def run_verify_published(version, sha, github, offline, allow_missing_release_note=False):
    report = Report()

    try:
        version = validate_version(version)
        sha = validate_sha(sha)  # normalizes case -- comparisons below assume lowercase
        report.add_pass("input-format", f"version={version!r} sha={sha!r}")
    except CheckError as exc:
        report.add_fail("input-format", str(exc))
        return report

    try:
        actual_readme_version = parse_readme_version()
        if actual_readme_version != version:
            report.add_fail(
                "readme-version-matches",
                f"README.md says {actual_readme_version!r}, expected {version!r}",
            )
        else:
            report.add_pass("readme-version-matches", version)
    except CheckError as exc:
        report.add_fail("readme-version-matches", str(exc))

    changelog_text = CHANGELOG_PATH.read_text(encoding="utf-8")
    section_match = None
    for candidate in CHANGELOG_DATED_SECTION_RE.finditer(changelog_text):
        if candidate.group("version") == version:
            section_match = candidate
            break
    if section_match is None:
        report.add_fail("changelog-section-exists", f"no dated section found for {version}")
    else:
        report.add_pass(
            "changelog-section-exists",
            f"## [{version}] compare/{section_match.group('prev')}...{version} "
            f"({section_match.group('date')})",
        )

    try:
        counts = load_current_counts()
        report.add_pass("profile-define-counts", str(counts))
    except CheckError as exc:
        report.add_fail("profile-define-counts", str(exc))

    check_result = _run(["python3", str(ROOT / "scripts" / "build_profiles.py"), "check"])
    if check_result.returncode != 0:
        report.add_fail(
            "profile-generation-sync",
            f"build_profiles.py check failed: {check_result.stderr.strip() or check_result.stdout.strip()}",
        )
    else:
        report.add_pass("profile-generation-sync", "build_profiles.py check passed")

    formatter_check = _run(["python3", str(ROOT / "scripts" / "build_formatter.py"), "--check"])
    if formatter_check.returncode != 0:
        report.add_fail(
            "formatter-generation-sync",
            f"build_formatter.py --check failed: "
            f"{formatter_check.stderr.strip() or formatter_check.stdout.strip()}",
        )
    else:
        report.add_pass("formatter-generation-sync", "build_formatter.py --check passed")

    release_note_path = RELEASE_DIR / f"{version}.md"
    if release_note_path.exists():
        report.add_pass("release-note-artifact", f"{release_note_path} present")
    elif allow_missing_release_note and version == LEGACY_RELEASE_NOTE_EXCEPTION_VERSION:
        report.add_warn(
            "release-note-artifact",
            f"{release_note_path} absent -- explicitly allowed via legacy exception "
            f"(only {LEGACY_RELEASE_NOTE_EXCEPTION_VERSION} predates the "
            ".release/<version>.md system)",
        )
    elif allow_missing_release_note:
        report.add_fail(
            "release-note-artifact",
            f"{release_note_path} not found and --allow-missing-release-note does not apply: "
            f"the legacy exception is scoped to {LEGACY_RELEASE_NOTE_EXCEPTION_VERSION} only, "
            f"not {version!r}. Every release after {LEGACY_RELEASE_NOTE_EXCEPTION_VERSION} must "
            "carry a real .release/<version>.md",
        )
    else:
        report.add_fail(
            "release-note-artifact",
            f"{release_note_path} not found (--allow-missing-release-note only applies to "
            f"the documented legacy release, {LEGACY_RELEASE_NOTE_EXCEPTION_VERSION})",
        )

    try:
        local_type = git_tag_object_type(version) if git_tag_exists_locally(version) else None
    except CheckError:
        local_type = None

    if offline:
        report.add_warn(
            "github-dependent-checks",
            "offline mode requested; tag/release publication checks against GitHub were "
            "SKIPPED (not silently)",
        )
        if local_type is not None:
            if local_type != "commit":
                report.add_fail(
                    "tag-directness-local",
                    f"local tag object type is {local_type!r}, expected 'commit' "
                    "(annotated tags are rejected by project convention)",
                )
            else:
                report.add_pass("tag-directness-local", "commit (direct/lightweight)")
            local_target = git_tag_target_sha(version)
            if local_target != sha:
                report.add_fail(
                    "tag-sha-matches-local",
                    f"local tag {version} resolves to {local_target}, expected {sha}",
                )
            else:
                report.add_pass("tag-sha-matches-local", sha)
        return report

    try:
        tag_ref = github.tag_ref(version)
        if tag_ref is None:
            report.add_fail("tag-exists", f"tag {version!r} not found on GitHub")
        else:
            report.add_pass("tag-exists", f"{tag_ref}")
            if tag_ref["type"] != "commit":
                report.add_fail(
                    "tag-directness",
                    f"tag object type is {tag_ref['type']!r}, expected 'commit' "
                    "(annotated tags are rejected by project convention)",
                )
            else:
                report.add_pass("tag-directness", "commit (direct/lightweight)")

            if tag_ref["sha"] != sha:
                report.add_fail(
                    "tag-sha-matches",
                    f"tag {version} resolves to {tag_ref['sha']}, expected {sha}",
                )
            else:
                report.add_pass("tag-sha-matches", sha)
    except CheckError as exc:
        report.add_fail("tag-exists", str(exc))

    try:
        release = github.release(version)
        if release is None:
            report.add_fail("release-exists", f"no GitHub release found for tag {version!r}")
        else:
            report.add_pass("release-exists", f"tag_name={release['tag_name']}")

            if release["draft"]:
                report.add_fail("release-draft-false", "release is still a draft")
            else:
                report.add_pass("release-draft-false", "draft=false")

            if release["prerelease"]:
                report.add_fail("release-prerelease-false", "release is marked prerelease")
            else:
                report.add_pass("release-prerelease-false", "prerelease=false")

            if not release["published_at"]:
                report.add_fail("release-published-timestamp", "no published_at timestamp")
            else:
                report.add_pass("release-published-timestamp", release["published_at"])

            if release["target_commitish"] != sha:
                report.add_warn(
                    "release-target-commitish",
                    f"target_commitish={release['target_commitish']!r} != {sha!r} -- "
                    "not authoritative once a direct tag exists, but worth a manual look",
                )
            else:
                report.add_pass("release-target-commitish", sha)

            if release_note_path.exists():
                # .release/<version>.md is authoritative under the approved
                # architecture: a mismatch here is drift, not an expected
                # hand-edit-at-publish-time variation -- hard fail, never a
                # warning. Legacy releases with no artifact at all (6.0.1,
                # via --allow-missing-release-note) are handled separately
                # above and are unaffected by this branch.
                expected_body = release_note_path.read_text(encoding="utf-8").strip()
                if release["body"].strip() != expected_body:
                    report.add_fail(
                        "release-body-matches-artifact",
                        f"published body text differs from {release_note_path} -- "
                        ".release/<version>.md is the authoritative release body; "
                        "post-merge drift is not allowed",
                    )
                else:
                    report.add_pass("release-body-matches-artifact", "byte-identical")
    except CheckError as exc:
        report.add_fail("release-exists", str(exc))

    try:
        run = github.latest_workflow_run("notify-release-discord.yml", event="release")
        if run is None:
            report.add_warn("notify-workflow-conclusion", "no release-triggered notify run found")
        elif run["conclusion"] != "success":
            report.add_warn(
                "notify-workflow-conclusion",
                f"latest notify run conclusion={run['conclusion']!r} ({run['html_url']})",
            )
        else:
            report.add_pass("notify-workflow-conclusion", "success")
    except CheckError as exc:
        report.add_warn("notify-workflow-conclusion", str(exc))

    return report


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="mode", required=True)

    prepare = subparsers.add_parser("prepare")
    prepare.add_argument("--version", required=True)
    prepare.add_argument("--offline", action="store_true")
    prepare.set_defaults(func=cmd_prepare)

    candidate = subparsers.add_parser("candidate")
    candidate.add_argument("--version", required=True)
    candidate.add_argument("--sha", required=True)
    candidate.add_argument("--offline", action="store_true")
    candidate.set_defaults(func=cmd_candidate)

    verify_published = subparsers.add_parser("verify-published")
    verify_published.add_argument("--version", required=True)
    verify_published.add_argument("--sha", required=True)
    verify_published.add_argument("--offline", action="store_true")
    verify_published.add_argument("--allow-missing-release-note", action="store_true")
    verify_published.set_defaults(func=cmd_verify_published)

    return parser


def cmd_prepare(args):
    github = None if args.offline else GhCliAdapter()
    return run_prepare(args.version, github, args.offline)


def cmd_candidate(args):
    github = None if args.offline else GhCliAdapter()
    return run_candidate(args.version, args.sha, github, args.offline)


def cmd_verify_published(args):
    github = None if args.offline else GhCliAdapter()
    return run_verify_published(
        args.version, args.sha, github, args.offline, args.allow_missing_release_note
    )


def main():
    parser = build_parser()
    args = parser.parse_args()
    try:
        report = args.func(args)
    except GithubUnavailableError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print(report.render())
    return 0 if report.passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
