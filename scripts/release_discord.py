#!/usr/bin/env python3

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from discord_common import (  # noqa: E402
    truncate_with_ellipsis,
    wrap_discord_content,
    write_discord_payload,
)


MAX_RELEASE_NAME_LENGTH = 240


def load_event(path):
    value = json.loads(
        Path(path).read_text(encoding="utf-8")
    )

    if not isinstance(value, dict):
        raise ValueError("GitHub event payload must be an object")

    return value


def release_from_event(event):
    if event.get("action") != "published":
        raise ValueError(
            "GitHub release event action must be 'published'"
        )

    release = event.get("release")

    if not isinstance(release, dict):
        raise ValueError(
            "GitHub event must contain a release object"
        )

    if release.get("draft") is True:
        raise ValueError(
            "refusing to notify for a draft release"
        )

    tag_name = release.get("tag_name")

    if not isinstance(tag_name, str) or not tag_name.strip():
        raise ValueError(
            "published release must contain a tag_name"
        )

    html_url = release.get("html_url")

    if not isinstance(html_url, str) or not html_url.strip():
        raise ValueError(
            "published release must contain an html_url"
        )

    name = release.get("name")

    if not isinstance(name, str) or not name.strip():
        name = tag_name

    name = truncate_with_ellipsis(
        name.strip(), MAX_RELEASE_NAME_LENGTH
    )

    prerelease = release.get("prerelease", False)

    if not isinstance(prerelease, bool):
        raise ValueError(
            "release prerelease field must be boolean"
        )

    return {
        "name": name,
        "tag_name": tag_name.strip(),
        "html_url": html_url.strip(),
        "prerelease": prerelease,
    }


def build_message(release):
    if release["prerelease"]:
        heading = (
            "🧪 **DraCuLa StreamNZB Template "
            "pre-release published**"
        )
    else:
        heading = (
            "🚀 **DraCuLa StreamNZB Template release published**"
        )

    lines = [
        heading,
        "",
        f"**{release['name']}** (`{release['tag_name']}`)",
        "",
        "A new DraCuLa StreamNZB Template release is available.",
        "",
        "For linked installs, refresh the **Define Library first**, "
        "then review profile/formatter updates as applicable.",
        "",
        f"Release: {release['html_url']}",
    ]

    return "\n".join(lines)


def build_payload(event):
    release = release_from_event(event)

    return wrap_discord_content(build_message(release))


def command_payload(args):
    event = load_event(args.event)
    payload = build_payload(event)

    content = write_discord_payload(args.output, payload)

    print(content)


def build_parser():
    parser = argparse.ArgumentParser()

    subparsers = parser.add_subparsers(
        dest="command",
        required=True,
    )

    payload = subparsers.add_parser("payload")
    payload.add_argument("--event", required=True)
    payload.add_argument(
        "--output",
        default="discord-release-payload.json",
    )
    payload.set_defaults(func=command_payload)

    return parser


def main():
    parser = build_parser()
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
