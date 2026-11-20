#!/usr/bin/env python3
"""Tests for scripts/build_formatter.py and the published formatter artifacts.

Covers the formatter registry (production, debug, Jellyfin), `--all`,
deterministic byte-for-byte generation, source/share-code round trips, drift
detection, and the rule that only the Jellyfin formatter may have an empty
description. The real-engine rendering regressions live in
tests/streamnzb_compat (scripts/test_formatter.sh).
"""

import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import build_formatter  # noqa: E402

EXPECTED_PAIRS = (
    ("tests/streamnzb_compat/formatter.source.json", "formatter.txt"),
    ("tests/streamnzb_compat/formatter-debug.source.json", "formatter-debug.txt"),
    ("tests/streamnzb_compat/formatter-jellyfin.source.json", "formatter-jellyfin.txt"),
)


def run(root, *args):
    return subprocess.run(
        [sys.executable, str(root / "scripts" / "build_formatter.py"), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def make_tree():
    """A minimal copy of the repository: the script, sources and share codes."""
    tmp = Path(tempfile.mkdtemp(prefix="build-formatter-test-"))
    (tmp / "scripts").mkdir()
    shutil.copy(ROOT / "scripts" / "build_formatter.py", tmp / "scripts")
    for source, output in EXPECTED_PAIRS:
        (tmp / source).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy(ROOT / source, tmp / source)
        shutil.copy(ROOT / output, tmp / output)
    return tmp


# The registry lists exactly the published formatters, and every file exists.
assert build_formatter.FORMATTERS == EXPECTED_PAIRS, build_formatter.FORMATTERS
assert len({output for _, output in EXPECTED_PAIRS}) == len(EXPECTED_PAIRS)
for source, output in EXPECTED_PAIRS:
    assert (ROOT / source).is_file(), source
    assert (ROOT / output).is_file(), output

# Every committed share code is byte-for-byte what the tool generates from its
# source (deterministic gzip, mtime 0), and decodes back to that source.
for source, output in EXPECTED_PAIRS:
    payload = build_formatter.load_source(ROOT / source)
    generated = build_formatter.encode_payload(payload)
    # Compare raw bytes, not normalised text: a CRLF-terminated share code must fail here
    # even though the semantic `--check` (which release tooling runs) would accept it.
    assert generated.encode("utf-8") == (ROOT / output).read_bytes(), (
        f"{output} is not byte-for-byte what build_formatter.py generates from {source}"
    )
    assert build_formatter.encode_payload(payload) == generated, "non-deterministic encoding"
    assert build_formatter.decode_share_code(ROOT / output) == payload, f"{output} round trip"
    assert generated.startswith("SNZBF1:") and generated.count("\n") == 1

# `--all --check` passes on the repository; the bare `--check` still checks only
# the production formatter, exactly as before.
result = run(ROOT, "--check", "--all")
assert result.returncode == 0, result.stderr
assert result.stdout.count("PASS:") == len(EXPECTED_PAIRS), result.stdout
result = run(ROOT, "--check")
assert result.returncode == 0 and result.stdout.count("PASS:") == 1, result.stdout

# --all cannot be mixed with an explicit pair.
result = run(ROOT, "--all", "--source", EXPECTED_PAIRS[0][0])
assert result.returncode != 0 and "--all cannot be combined" in result.stderr

# Drift in the Jellyfin pair fails `--all --check` (what release tooling runs),
# names the drifted file, and does not hide the formatters that are in sync.
tmp = make_tree()
try:
    jellyfin_source = tmp / EXPECTED_PAIRS[2][0]
    payload = json.loads(jellyfin_source.read_text(encoding="utf-8"))
    payload["result_name_template"] += " "
    jellyfin_source.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    result = run(tmp, "--check", "--all")
    assert result.returncode == 1, result.stdout
    assert "formatter-jellyfin" in result.stderr, result.stderr
    assert result.stdout.count("PASS:") == 2, result.stdout
    assert run(tmp, "--check").returncode == 0, "bare --check only covers the production pair"

    # Rebuilding fixes the Jellyfin pair and leaves the other share codes byte-identical.
    before = {output: (tmp / output).read_bytes() for _, output in EXPECTED_PAIRS[:2]}
    result = run(tmp, "--all")
    assert result.returncode == 0, result.stderr
    assert {output: (tmp / output).read_bytes() for _, output in EXPECTED_PAIRS[:2]} == before
    assert run(tmp, "--check", "--all").returncode == 0

    # A desynchronised debug share code is caught too (it was not checked at release time before).
    debug_output = tmp / EXPECTED_PAIRS[1][1]
    debug_output.write_text((tmp / EXPECTED_PAIRS[0][1]).read_text(encoding="utf-8"), encoding="utf-8")
    result = run(tmp, "--check", "--all")
    assert result.returncode == 1 and "formatter-debug" in result.stderr, result.stderr
finally:
    shutil.rmtree(tmp)

# Only the Jellyfin formatter may have an empty description (StreamNZB's Jellyfin
# endpoint renders the name only); production and debug must keep theirs.
payloads = {out: build_formatter.load_source(ROOT / src) for src, out in EXPECTED_PAIRS}
assert payloads["formatter.txt"]["result_description_template"].strip()
assert payloads["formatter-debug.txt"]["result_description_template"].strip()
jellyfin = payloads["formatter-jellyfin.txt"]
assert jellyfin["name"] == "DraCuLa Jellyfin"
assert jellyfin["result_description_template"] == "", "the Jellyfin formatter must not copy a description"
assert jellyfin["result_name_template"].strip()

# Release tooling checks every formatter pair. Behavioural test: run the real
# document checks shared by `candidate` and `prep-pr`, and `verify-published`, with
# the build_formatter subprocess replaced by a failing fake, and require the
# finding and the `--all` command. (`prepare` does not run generation checks for
# any artifact, profiles included, and is intentionally not covered.)
import subprocess  # noqa: E402

import check_release  # noqa: E402

captured = []
real_run = check_release._run


def fake_run(args, cwd=check_release.ROOT):
    if any("build_formatter.py" in str(arg) for arg in args):
        captured.append([str(arg) for arg in args])
        return subprocess.CompletedProcess(args, 1, "", "ERROR: formatter-jellyfin is not synchronized")
    return real_run(args, cwd)


check_release._run = fake_run
try:
    for label, call in (
        ("candidate/prep-pr documents", lambda: (lambda r: (check_release._check_release_documents(r, "6.5.1"), r)[1])(check_release.Report())),
        ("verify-published", lambda: check_release.run_verify_published("6.5.1", "9efb02f19c90bf9d923140f8cc10bbe2666f40eb", None, True)),
    ):
        captured.clear()
        report = call()
        findings = [f for f in report.findings if f.check == "formatter-generation-sync"]
        assert len(findings) == 1 and not findings[0].ok, f"{label}: {findings}"
        assert "--check" in captured[0] and "--all" in captured[0], f"{label}: {captured}"
        assert "formatter-jellyfin" in findings[0].detail, f"{label}: {findings[0].detail}"
finally:
    check_release._run = real_run

print("PASS: build_formatter registry, --all, determinism, drift detection and description policy.")
