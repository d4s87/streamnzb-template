#!/usr/bin/env python3

import argparse
import json
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from discord_common import (  # noqa: E402
    truncate_with_ellipsis,
    wrap_discord_content,
    write_discord_payload,
)


MAX_COMMIT_SUBJECT_LENGTH = 240

PUBLISHED_FORMATTERS = (
    "formatter.txt",
    "formatter-debug.txt",
)

DISPLAY_NAMES = {
    "formatter.txt": "DraCuLa",
    "formatter-debug.txt": "DraCuLa Debug",
}


def git_output(args, cwd=None):
    result = subprocess.run(
        ["git", *args],
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )

    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise ValueError(
            f"git {' '.join(args)} failed: {detail}"
        )

    return result.stdout


def validate_revision(value, label):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} revision must be non-empty")

    value = value.strip()

    if set(value) == {"0"}:
        raise ValueError(
            f"{label} revision cannot be the all-zero Git SHA"
        )

    return value


def formatter_change_commit(before, after, cwd=None):
    before = validate_revision(before, "before")
    after = validate_revision(after, "after")

    output = git_output(
        [
            "log",
            "-1",
            "--format=%H%x00%s",
            f"{before}..{after}",
            "--",
            *PUBLISHED_FORMATTERS,
        ],
        cwd=cwd,
    ).rstrip("\n")

    if not output:
        return None

    try:
        commit_sha, subject = output.split("\x00", 1)
    except ValueError as exc:
        raise ValueError(
            "unexpected formatter git log output"
        ) from exc

    commit_sha = validate_revision(
        commit_sha,
        "formatter change commit",
    )

    if not subject.strip():
        raise ValueError(
            "formatter change commit subject must be non-empty"
        )

    return {
        "commit_sha": commit_sha,
        "commit_subject": subject.strip(),
    }


def changed_formatters(before, after, cwd=None):
    before = validate_revision(before, "before")
    after = validate_revision(after, "after")

    output = git_output(
        [
            "diff",
            "--name-only",
            "--diff-filter=ACMRT",
            before,
            after,
            "--",
            *PUBLISHED_FORMATTERS,
        ],
        cwd=cwd,
    )

    changed = {
        line.strip()
        for line in output.splitlines()
        if line.strip()
    }

    unexpected = changed.difference(PUBLISHED_FORMATTERS)

    if unexpected:
        raise ValueError(
            "unexpected formatter path(s): "
            + ", ".join(sorted(unexpected))
        )

    return [
        path
        for path in PUBLISHED_FORMATTERS
        if path in changed
    ]


def normalize_files(value):
    if not isinstance(value, list):
        raise ValueError(
            "formatter files must be a JSON list"
        )

    files = []

    for item in value:
        if item not in PUBLISHED_FORMATTERS:
            raise ValueError(
                f"unsupported published formatter path: {item!r}"
            )

        if item not in files:
            files.append(item)

    return [
        path
        for path in PUBLISHED_FORMATTERS
        if path in files
    ]


def normalize_commit_subject(value):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(
            "commit subject must be non-empty"
        )

    value = " ".join(value.split())

    return truncate_with_ellipsis(value, MAX_COMMIT_SUBJECT_LENGTH)


def build_message(
    files,
    repository,
    server_url,
    commit_sha,
    commit_subject,
):
    files = normalize_files(files)

    if not files:
        raise ValueError(
            "at least one published formatter must have changed"
        )

    if not isinstance(repository, str) or "/" not in repository:
        raise ValueError(
            "repository must use owner/name form"
        )

    if not isinstance(server_url, str) or not server_url.strip():
        raise ValueError(
            "server URL must be non-empty"
        )

    commit_sha = validate_revision(
        commit_sha,
        "commit",
    )
    commit_subject = normalize_commit_subject(
        commit_subject
    )

    names = [
        f"**{DISPLAY_NAMES[path]}**"
        for path in files
    ]

    if len(names) == 1:
        updated = names[0]
    else:
        updated = " and ".join(names)

    short_sha = commit_sha[:7]
    commit_url = (
        f"{server_url.rstrip('/')}/"
        f"{repository.strip()}/commit/{commit_sha}"
    )

    lines = [
        "🎨 **DraCuLa formatter updated**",
        "",
        f"Published formatter updated: {updated}.",
        "",
        f"Change: {commit_subject}",
        "",
        (
            "Users with a linked formatter import can refresh it "
            "to receive the latest presentation changes."
        ),
        "",
        f"Commit: [`{short_sha}`]({commit_url})",
    ]

    return "\n".join(lines)


def build_payload(
    files,
    repository,
    server_url,
    commit_sha,
    commit_subject,
):
    return wrap_discord_content(
        build_message(
            files,
            repository,
            server_url,
            commit_sha,
            commit_subject,
        )
    )


def write_github_output(path, values):
    output = Path(path)

    with output.open(
        "a",
        encoding="utf-8",
    ) as handle:
        for key, value in values.items():
            handle.write(f"{key}={value}\n")


def command_detect(args):
    files = changed_formatters(
        args.before,
        args.after,
    )

    analysis = json.dumps(
        files,
        ensure_ascii=False,
        separators=(",", ":"),
    )

    change_commit = (
        formatter_change_commit(
            args.before,
            args.after,
        )
        if files
        else None
    )

    if files and change_commit is None:
        raise ValueError(
            "formatter files changed but no formatter-changing "
            "commit could be resolved"
        )

    outputs = {
        "notify": "true" if files else "false",
        "files": analysis,
    }

    if change_commit is not None:
        outputs.update(
            {
                "commit_sha": change_commit["commit_sha"],
                "commit_subject": (
                    change_commit["commit_subject"]
                ),
            }
        )

    write_github_output(
        args.github_output,
        outputs,
    )

    if files:
        print(
            "Published formatter changes detected: "
            + ", ".join(files)
        )
        print(
            "Formatter-changing commit: "
            + change_commit["commit_sha"]
            + " "
            + change_commit["commit_subject"]
        )
    else:
        print(
            "No published formatter changes detected."
        )


def command_payload(args):
    try:
        files = json.loads(args.files)
    except json.JSONDecodeError as exc:
        raise ValueError(
            "formatter files must be valid JSON"
        ) from exc

    payload = build_payload(
        files,
        args.repository,
        args.server_url,
        args.commit_sha,
        args.commit_subject,
    )

    content = write_discord_payload(args.output, payload)

    print(content)


def build_parser():
    parser = argparse.ArgumentParser()

    subparsers = parser.add_subparsers(
        dest="command",
        required=True,
    )

    detect = subparsers.add_parser("detect")
    detect.add_argument("--before", required=True)
    detect.add_argument("--after", required=True)
    detect.add_argument(
        "--github-output",
        required=True,
    )
    detect.set_defaults(func=command_detect)

    payload = subparsers.add_parser("payload")
    payload.add_argument("--files", required=True)
    payload.add_argument("--repository", required=True)
    payload.add_argument("--server-url", required=True)
    payload.add_argument("--commit-sha", required=True)
    payload.add_argument(
        "--commit-subject",
        required=True,
    )
    payload.add_argument(
        "--output",
        default="discord-formatter-payload.json",
    )
    payload.set_defaults(func=command_payload)

    return parser


def main():
    parser = build_parser()
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
