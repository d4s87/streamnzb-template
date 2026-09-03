#!/usr/bin/env python3

import importlib.util
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

spec = importlib.util.spec_from_file_location(
    "formatter_discord",
    ROOT / "scripts/formatter_discord.py",
)

module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def run_git(repo, *args):
    result = subprocess.run(
        ["git", *args],
        cwd=repo,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )

    if result.returncode != 0:
        raise AssertionError(
            "git command failed: "
            + " ".join(args)
            + "\n"
            + result.stderr
        )

    return result.stdout.strip()


payload = module.build_payload(
    ["formatter.txt"],
    "d4s87/streamnzb-template",
    "https://github.com",
    "e4141f9ec3195d04c69e71efbeb3c2d67bad75cd",
)

assert payload["allowed_mentions"] == {
    "parse": [],
}

message = payload["content"]

assert "DraCuLa formatter updated" in message
assert "**DraCuLa**" in message
assert "DraCuLa Debug" not in message
assert "linked formatter import" in message
assert "e4141f9" in message
assert (
    "https://github.com/d4s87/streamnzb-template/"
    "commit/e4141f9ec3195d04c69e71efbeb3c2d67bad75cd"
    in message
)
assert len(message) <= 2000


payload = module.build_payload(
    [
        "formatter-debug.txt",
        "formatter.txt",
        "formatter.txt",
    ],
    "d4s87/streamnzb-template",
    "https://github.com/",
    "1234567890abcdef",
)

message = payload["content"]

assert "**DraCuLa** and **DraCuLa Debug**" in message


try:
    module.build_payload(
        ["tests/streamnzb_compat/formatter.source.json"],
        "d4s87/streamnzb-template",
        "https://github.com",
        "1234567",
    )
except ValueError as exc:
    assert "unsupported" in str(exc)
else:
    raise AssertionError(
        "non-published formatter path unexpectedly accepted"
    )


try:
    module.build_payload(
        [],
        "d4s87/streamnzb-template",
        "https://github.com",
        "1234567",
    )
except ValueError as exc:
    assert "at least one" in str(exc)
else:
    raise AssertionError(
        "empty formatter update unexpectedly accepted"
    )


with tempfile.TemporaryDirectory() as temp:
    repo = Path(temp)

    run_git(repo, "init")
    run_git(repo, "config", "user.email", "test@example.com")
    run_git(repo, "config", "user.name", "Formatter Test")
    run_git(repo, "config", "commit.gpgsign", "false")
    run_git(repo, "config", "tag.gpgsign", "false")

    (repo / "formatter.txt").write_text(
        "normal-v1\n",
        encoding="utf-8",
    )
    (repo / "formatter-debug.txt").write_text(
        "debug-v1\n",
        encoding="utf-8",
    )
    (repo / "formatter.source.json").write_text(
        '{"name":"source-v1"}\n',
        encoding="utf-8",
    )

    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base = run_git(repo, "rev-parse", "HEAD")

    (repo / "formatter.source.json").write_text(
        '{"name":"source-v2"}\n',
        encoding="utf-8",
    )

    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "source only")
    source_only = run_git(repo, "rev-parse", "HEAD")

    assert module.changed_formatters(
        base,
        source_only,
        cwd=repo,
    ) == []

    (repo / "formatter.txt").write_text(
        "normal-v2\n",
        encoding="utf-8",
    )

    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "normal formatter")
    normal = run_git(repo, "rev-parse", "HEAD")

    assert module.changed_formatters(
        source_only,
        normal,
        cwd=repo,
    ) == [
        "formatter.txt",
    ]

    (repo / "formatter-debug.txt").write_text(
        "debug-v2\n",
        encoding="utf-8",
    )

    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "debug formatter")
    both = run_git(repo, "rev-parse", "HEAD")

    assert module.changed_formatters(
        source_only,
        both,
        cwd=repo,
    ) == [
        "formatter.txt",
        "formatter-debug.txt",
    ]


print("PASS: formatter Discord notification tests")
