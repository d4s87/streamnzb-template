#!/usr/bin/env python3
"""
Render the release-preparation PR body (Phase 2).

Pure formatting over check_release.py's existing commit/PR-delta and
version-suggestion data -- no new validation or accounting logic lives
here; this module only turns already-computed facts into the PR body
Markdown the Prepare Release workflow passes to
peter-evans/create-pull-request via --body-path.
"""

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from check_release import (  # noqa: E402
    GhCliAdapter,
    RELEASE_IMPACT_LABELS,
    SKIP_LABEL,
    compute_release_delta,
    suggest_version_bump,
    validate_sha,
    validate_version,
)


def render_body(version, previous_version, prepared_from_sha, delta, suggestion):
    substantive = []
    bookkeeping = []
    for number, pr in sorted(delta["prs"].items()):
        labels = set(pr["labels"])
        line = f"- #{number} {pr['title']!r} ({', '.join(sorted(labels)) or 'no labels'})"
        # skip-changelog always wins, even alongside an impact label --
        # mirrors suggest_version_bump()'s own precedence exactly, so the
        # rendered body never contradicts the accounting that actually
        # produced the suggested bump above.
        if labels & set(RELEASE_IMPACT_LABELS) and SKIP_LABEL not in labels:
            substantive.append(line)
        else:
            bookkeeping.append(line)

    lines = [
        f"## Release preparation: {version}",
        "",
        f"- Requested version: **{version}**",
        f"- Previous stable: **{previous_version}**",
        f"- Prepared from `main` @ `{prepared_from_sha}`",
        f"- Suggested release impact: **{suggestion['suggested_bump']}**",
        "",
        "### Substantive changes since previous stable",
        "",
        *(substantive or ["_None found._"]),
        "",
        "### Bookkeeping / non-release-driving PRs in this range",
        "",
        *(bookkeeping or ["_None._"]),
        "",
    ]

    if delta["unaccounted_commits"]:
        lines += [
            "### :warning: Direct-to-main commits (not associated with any PR)",
            "",
            *[f"- `{c['sha'][:12]}` {c['subject']}" for c in delta["unaccounted_commits"]],
            "",
        ]

    if suggestion["warnings"]:
        lines += [
            "### Release-impact accounting warnings",
            "",
            *[f"- {w}" for w in suggestion["warnings"]],
            "",
        ]

    lines += [
        "### Generated files",
        "",
        "- `README.md` (current-version line)",
        "- `CHANGELOG.md` (`Unreleased` rolled into a dated section)",
        f"- `.release/{version}.json` (preparation provenance -- machine-owned, never hand-edit)",
        f"- `.release/{version}.md` (release-note **draft seed** -- requires human curation)",
        "",
        "### Required before merge",
        "",
        f"- [ ] Edit `.release/{version}.md` into public release-note tone and remove the "
        "generated draft markers (the `release-note-curated` check hard-fails until you do).",
        "- [ ] Confirm the suggested release impact above matches your intent.",
        "",
        "### Not part of this PR",
        "",
        "- No tag is created; no GitHub release is published, edited, or otherwise mutated.",
        "- Publish Release (Phase 3) is a separate, manually dispatched workflow and is not triggered by this preparation PR.",
        "",
        "### Stale-main rule",
        "",
        f"This PR's release housekeeping was generated from `main` @ `{prepared_from_sha}`. "
        "If `main` advances before this PR merges, the dedicated release-preparation "
        "validation check fails closed (`preparation-freshness`) -- **do not rebase or merge "
        "past that failure**; close this PR and rerun Prepare Release from current `main` instead.",
    ]
    return "\n".join(lines)


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--previous-version", required=True)
    parser.add_argument("--prepared-from-sha", required=True)
    parser.add_argument("--output", required=True)
    return parser


def main():
    args = build_parser().parse_args()
    version = validate_version(args.version)
    previous_version = validate_version(args.previous_version)
    prepared_from_sha = validate_sha(args.prepared_from_sha)

    github = GhCliAdapter()
    delta = compute_release_delta(github, previous_version, prepared_from_sha)
    suggestion = suggest_version_bump(delta)

    body = render_body(version, previous_version, prepared_from_sha, delta, suggestion)
    Path(args.output).write_text(body, encoding="utf-8")
    print(f"Wrote PR body to {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
