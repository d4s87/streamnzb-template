#!/usr/bin/env python3

import argparse
import json
import subprocess
from pathlib import Path


BASELINE_PATH = Path("generated/vidhin-defines.json")
LIBRARY_PATH = Path("generated/streamnzb-defines.txt")


def load_json_text(text):
    value = json.loads(text)

    if not isinstance(value, dict):
        raise ValueError("Vidhin baseline must be a JSON object")

    defines = value.get("defines")

    if not isinstance(defines, dict):
        raise ValueError(
            "Vidhin baseline must contain a 'defines' object"
        )

    return value


def semantic_diff(old_document, new_document):
    old_defines = old_document["defines"]
    new_defines = new_document["defines"]

    old_names = set(old_defines)
    new_names = set(new_defines)

    added = sorted(new_names - old_names)
    removed = sorted(old_names - new_names)
    changed = sorted(
        name
        for name in old_names & new_names
        if old_defines[name] != new_defines[name]
    )

    return {
        "added": added,
        "changed": changed,
        "removed": removed,
    }


def has_named_changes(changes):
    return any(
        changes[key]
        for key in ("added", "changed", "removed")
    )


def analyze(
    old_document,
    new_document,
    old_library,
    new_library,
):
    changes = semantic_diff(old_document, new_document)
    library_changed = old_library != new_library

    return {
        **changes,
        "library_changed": library_changed,
        "notify": (
            has_named_changes(changes)
            or library_changed
        ),
    }


def build_message(
    analysis,
    *,
    repository,
    server_url,
    commit_sha,
    max_visible=15,
):
    entries = (
        [f"➕ {name}" for name in analysis["added"]]
        + [f"🔄 {name}" for name in analysis["changed"]]
        + [f"➖ {name}" for name in analysis["removed"]]
    )

    visible = entries[:max_visible]
    remaining = len(entries) - len(visible)

    lines = [
        "🧛 **DraCuLa Define Library updated**",
        "",
        "The linked DraCuLa StreamNZB Define Library has changed.",
    ]

    if visible:
        lines.extend(
            [
                "",
                "**Vidhin classification changes:**",
                *visible,
            ]
        )

        if remaining:
            lines.append(f"…and {remaining} more.")

    if analysis["library_changed"] and not entries:
        lines.extend(
            [
                "",
                "The published StreamNZB Define Library changed "
                "without a mapped classification-name change.",
            ]
        )

    commit_url = (
        f"{server_url.rstrip('/')}/"
        f"{repository}/commit/{commit_sha}"
    )

    lines.extend(
        [
            "",
            "Open **StreamNZB → Define Libraries → "
            "DraCuLa → Refresh** and review/apply the update.",
            "",
            "Refresh the **Define Library first** before any "
            "profile update that may depend on new classifications.",
            "",
            f"Commit: {commit_url}",
        ]
    )

    message = "\n".join(lines)

    if len(message) > 1950:
        message = (
            message[:1900].rstrip()
            + "\n\nSee the commit above for the complete change set."
        )

    if len(message) > 2000:
        raise ValueError(
            "Discord message exceeds 2000 characters"
        )

    return message


def git_show(revision, path):
    result = subprocess.run(
        ["git", "show", f"{revision}:{path.as_posix()}"],
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout


def analyze_revisions(before_sha, after_sha):
    if not before_sha or set(before_sha) == {"0"}:
        return {
            "notify": False,
            "reason": "no-usable-before-sha",
            "added": [],
            "changed": [],
            "removed": [],
            "library_changed": False,
        }

    try:
        old_json_text = git_show(before_sha, BASELINE_PATH)
        old_library = git_show(before_sha, LIBRARY_PATH)
    except subprocess.CalledProcessError:
        return {
            "notify": False,
            "reason": "previous-baseline-unavailable",
            "added": [],
            "changed": [],
            "removed": [],
            "library_changed": False,
        }

    if after_sha:
        try:
            new_json_text = git_show(after_sha, BASELINE_PATH)
            new_library = git_show(after_sha, LIBRARY_PATH)
        except subprocess.CalledProcessError as exc:
            raise RuntimeError(
                "Current generated Vidhin artifacts are unavailable "
                f"at revision {after_sha}"
            ) from exc
    else:
        new_json_text = BASELINE_PATH.read_text(
            encoding="utf-8"
        )
        new_library = LIBRARY_PATH.read_text(
            encoding="utf-8"
        )

    old_document = load_json_text(old_json_text)
    new_document = load_json_text(new_json_text)

    analysis = analyze(
        old_document,
        new_document,
        old_library,
        new_library,
    )

    analysis["reason"] = (
        "semantic-change"
        if analysis["notify"]
        else "no-semantic-change"
    )

    return analysis


def write_github_output(path, analysis):
    with Path(path).open("a", encoding="utf-8") as output:
        output.write(
            f"notify={'true' if analysis['notify'] else 'false'}\n"
        )

        output.write("analysis<<EOF\n")
        output.write(
            json.dumps(
                analysis,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        output.write("\nEOF\n")


def command_detect(args):
    analysis = analyze_revisions(
        args.before,
        args.after,
    )

    print(
        json.dumps(
            analysis,
            ensure_ascii=False,
            indent=2,
        )
    )

    if args.github_output:
        write_github_output(
            args.github_output,
            analysis,
        )


def command_payload(args):
    analysis = json.loads(args.analysis)

    if not analysis.get("notify"):
        raise SystemExit(
            "ERROR: refusing to build Discord payload "
            "for notify=false"
        )

    message = build_message(
        analysis,
        repository=args.repository,
        server_url=args.server_url,
        commit_sha=args.commit_sha,
    )

    payload = {
        "content": message,
        "allowed_mentions": {
            "parse": [],
        },
    }

    Path(args.output).write_text(
        json.dumps(
            payload,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    print(message)


def build_parser():
    parser = argparse.ArgumentParser()

    subparsers = parser.add_subparsers(
        dest="command",
        required=True,
    )

    detect = subparsers.add_parser("detect")
    detect.add_argument("--before", required=True)
    detect.add_argument("--after", required=True)
    detect.add_argument("--github-output")
    detect.set_defaults(func=command_detect)

    payload = subparsers.add_parser("payload")
    payload.add_argument("--analysis", required=True)
    payload.add_argument("--repository", required=True)
    payload.add_argument("--server-url", required=True)
    payload.add_argument("--commit-sha", required=True)
    payload.add_argument(
        "--output",
        default="discord-payload.json",
    )
    payload.set_defaults(func=command_payload)

    return parser


def main():
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
