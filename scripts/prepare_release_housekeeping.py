#!/usr/bin/env python3
"""
Deterministic release-housekeeping generation primitives (Phase 1).

Pure mutation logic, kept separate from check_release.py's read-only
validation: given a target version, previous version, date, and the
prepared-from SHA, produce the exact README/CHANGELOG edits and the two
`.release/<version>.*` artifacts a future Prepare Release workflow would
commit. This module does not touch git, does not write files by default
(see write_release_preparation), and never calls out to an LLM/network
service -- release-note generation is a fixed template, not prose synthesis.
"""

import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from check_release import (  # noqa: E402
    CheckError,
    DRAFT_HEADING_MARKER,
    DRAFT_NOTICE,
    NOTICE_LINE,
    README_PATH,
    README_VERSION_RE,
    CHANGELOG_PATH,
    CHANGELOG_UNRELEASED_RE,
    RELEASE_DIR,
    ROOT,
    changelog_section_body,
    load_compatibility_baseline,
    load_current_counts,
    resolve_repo_slug,
    validate_version,
)


PROVENANCE_SCHEMA_VERSION = 1


# ---------------------------------------------------------------------------
# README
# ---------------------------------------------------------------------------

def update_readme_version(text, new_version):
    validate_version(new_version)
    matches = list(README_VERSION_RE.finditer(text))
    if len(matches) == 0:
        raise CheckError("README.md version anchor not found")
    if len(matches) > 1:
        raise CheckError(
            f"README.md version anchor matched {len(matches)} times, expected exactly 1"
        )
    match = matches[0]
    new_line = f"{match.group(1)}{new_version}{match.group(3)}"
    return text[: match.start()] + new_line + text[match.end():]


# ---------------------------------------------------------------------------
# CHANGELOG
# ---------------------------------------------------------------------------

def rollover_changelog(text, new_version, previous_version, date_str, repo_slug):
    validate_version(new_version)
    validate_version(previous_version)
    if not re.match(r"^\d{4}-\d{2}-\d{2}$", date_str):
        raise CheckError(f"invalid date {date_str!r}: must be YYYY-MM-DD")

    headers = list(CHANGELOG_UNRELEASED_RE.finditer(text))
    if len(headers) != 1:
        raise CheckError(
            f"expected exactly one Unreleased header, found {len(headers)}"
        )
    header = headers[0]

    if header.group("prev") != previous_version:
        raise CheckError(
            f"Unreleased compare link previous version is "
            f"{header.group('prev')!r}, expected {previous_version!r}"
        )

    if re.search(rf"^## \[{re.escape(new_version)}\]\(", text, re.MULTILINE):
        raise CheckError(f"CHANGELOG.md already contains a section for {new_version}")

    after_header = text[header.end():]
    notice_re = re.compile(r"^\n\n" + re.escape(NOTICE_LINE) + r"\n", re.MULTILINE)
    notice_match = notice_re.match(after_header)
    if not notice_match:
        raise CheckError(
            "Unreleased notice paragraph not found immediately after header"
        )

    body_region_start = header.end() + notice_match.end()
    next_section_match = re.search(r"\n## \[", text[body_region_start:])
    body_end = (
        body_region_start + next_section_match.start()
        if next_section_match
        else len(text)
    )

    body_content = text[body_region_start:body_end].strip("\n")

    new_unreleased_block = (
        f"## [Unreleased](https://github.com/{repo_slug}/compare/{new_version}...HEAD)\n"
        f"\n"
        f"{NOTICE_LINE}\n"
    )
    new_dated_block = (
        f"\n"
        f"## [{new_version}](https://github.com/{repo_slug}/compare/"
        f"{previous_version}...{new_version}) ({date_str})\n"
    )
    if body_content:
        new_dated_block += "\n" + body_content + "\n"

    prefix = text[: header.start()]
    suffix = text[body_end:]

    new_text = prefix + new_unreleased_block + new_dated_block + suffix
    return new_text, body_content


# ---------------------------------------------------------------------------
# .release/<version>.json -- preparation provenance
# ---------------------------------------------------------------------------

def build_provenance(version, previous_version, prepared_from_sha, prepared_at_date, compatibility, counts):
    validate_version(version)
    validate_version(previous_version)
    if not re.match(r"^[0-9a-f]{40}$", prepared_from_sha):
        raise CheckError(f"invalid prepared_from_sha {prepared_from_sha!r}: must be 40 lowercase hex chars")
    if not re.match(r"^\d{4}-\d{2}-\d{2}$", prepared_at_date):
        raise CheckError(f"invalid prepared_at_date {prepared_at_date!r}: must be YYYY-MM-DD")

    return {
        "schema_version": PROVENANCE_SCHEMA_VERSION,
        "version": version,
        "previous_version": previous_version,
        "prepared_from_sha": prepared_from_sha,
        "prepared_at_date": prepared_at_date,
        "compatibility": {
            "streamnzb_version": compatibility["streamnzb_version"],
            "streamnzb_sha": compatibility["streamnzb_sha"],
            "jhin_version": compatibility["jhin_version"],
        },
        "expected_counts": {
            "samsung": counts["samsung"],
            "neutral": counts["neutral"],
            "core": counts["core"],
            "presentation": counts["presentation"],
            "device": counts["device"],
            "defines": counts["defines"],
        },
    }


# ---------------------------------------------------------------------------
# .release/<version>.md -- curated release-note seed
#
# DRAFT_NOTICE and DRAFT_HEADING_MARKER (the two generator-owned markers a
# curated note must no longer contain) live in check_release.py, not here,
# so both this generator and check_release.check_release_note_curated() can
# import the same literal strings without a circular import -- this module
# already imports from check_release, so the constants had to live there.
# ---------------------------------------------------------------------------

def generate_release_note(version, previous_version, changelog_body, compatibility, counts, repo_slug):
    validate_version(version)
    validate_version(previous_version)

    body = changelog_body.strip() if changelog_body and changelog_body.strip() else "_No changes recorded._"

    lines = [
        f"# DraCuLa StreamNZB Template {version} {DRAFT_HEADING_MARKER}",
        "",
        DRAFT_NOTICE,
        "",
        "## Changes",
        "",
        body,
        "",
        "## Compatibility",
        "",
        f"- StreamNZB: {compatibility['streamnzb_version']}",
        f"- StreamNZB pinned commit: `{compatibility['streamnzb_sha']}`",
        f"- Jhin: {compatibility['jhin_version']}",
        "",
        "## Current profile counts",
        "",
        f"- Samsung QN90A: **{counts['samsung']}**",
        f"- Hardware Neutral: **{counts['neutral']}**",
        f"- Shared Core: **{counts['core']}**",
        f"- Presentation: **{counts['presentation']}**",
        f"- Samsung device-specific: **{counts['device']}**",
        f"- Define Library: **{counts['defines']}**",
        "",
        f"**Full Changelog**: https://github.com/{repo_slug}/compare/{previous_version}...{version}",
        "",
    ]
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Orchestration: given a target version, run the full generation pipeline
# against real repo state and (optionally) write the results to disk.
# ---------------------------------------------------------------------------

def generate_release_preparation(version, previous_version, date_str, prepared_from_sha, root=ROOT):
    repo_slug = resolve_repo_slug()

    readme_text = (root / "README.md").read_text(encoding="utf-8")
    new_readme_text = update_readme_version(readme_text, version)

    changelog_text = (root / "CHANGELOG.md").read_text(encoding="utf-8")
    new_changelog_text, unreleased_body = rollover_changelog(
        changelog_text, version, previous_version, date_str, repo_slug
    )

    compatibility = load_compatibility_baseline(root=root)
    counts = load_current_counts(root=root)

    provenance = build_provenance(
        version, previous_version, prepared_from_sha, date_str, compatibility, counts
    )

    release_note = generate_release_note(
        version, previous_version, unreleased_body, compatibility, counts, repo_slug
    )

    return {
        "readme_text": new_readme_text,
        "changelog_text": new_changelog_text,
        "provenance": provenance,
        "release_note": release_note,
    }


def write_release_preparation(result, version, root=ROOT):
    (root / "README.md").write_text(result["readme_text"], encoding="utf-8")
    (root / "CHANGELOG.md").write_text(result["changelog_text"], encoding="utf-8")

    release_dir = root / ".release"
    release_dir.mkdir(exist_ok=True)
    (release_dir / f"{version}.json").write_text(
        json.dumps(result["provenance"], indent=2, sort_keys=False) + "\n", encoding="utf-8"
    )
    (release_dir / f"{version}.md").write_text(result["release_note"], encoding="utf-8")


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--previous-version", required=True)
    parser.add_argument("--date", required=True, help="YYYY-MM-DD")
    parser.add_argument("--prepared-from-sha", required=True)
    parser.add_argument(
        "--write", action="store_true",
        help="Write generated files to disk. Without this flag, prints a summary only.",
    )
    return parser


def main():
    args = build_parser().parse_args()
    try:
        result = generate_release_preparation(
            args.version, args.previous_version, args.date, args.prepared_from_sha
        )
    except CheckError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    if args.write:
        write_release_preparation(result, args.version)
        print(f"Wrote README.md, CHANGELOG.md, .release/{args.version}.json, .release/{args.version}.md")
    else:
        print(f"README.md would be updated to version {args.version}")
        print(f"CHANGELOG.md would gain a dated section for {args.version}")
        print(f".release/{args.version}.json:")
        print(json.dumps(result["provenance"], indent=2))
        print(f".release/{args.version}.md:")
        print(result["release_note"])

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
