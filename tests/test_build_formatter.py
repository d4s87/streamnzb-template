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
    # Compare raw bytes, not normalised text: a CRLF-terminated share code must fail.
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

    # Canonical output is UTF-8, LF-terminated, exactly one trailing newline.
    jellyfin_output = tmp / EXPECTED_PAIRS[2][1]
    good = jellyfin_output.read_bytes()
    assert good.endswith(b"\n") and not good.endswith(b"\n\n") and b"\r" not in good

    # Newline drift decodes to the same payload but is NOT canonical: --check and
    # --check --all reject it (naming the byte difference), and the rebuild repairs it
    # to the canonical LF bytes; a second run is a no-op.
    for label, drifted in (
        ("CRLF terminator", good.rstrip(b"\n") + b"\r\n"),
        ("missing final newline", good.rstrip(b"\n")),
        ("extra blank line", good + b"\n"),
    ):
        jellyfin_output.write_bytes(drifted)
        result = run(tmp, "--check", "--all")
        assert result.returncode == 1, f"{label}: --check --all accepted byte drift"
        assert "not byte-identical" in result.stderr and "formatter-jellyfin" in result.stderr, result.stderr
        result = run(tmp, "--source", EXPECTED_PAIRS[2][0], "--output", EXPECTED_PAIRS[2][1], "--check")
        assert result.returncode == 1, f"{label}: explicit --check accepted byte drift"
        result = run(tmp, "--all")
        assert result.returncode == 0 and "Generated" in result.stdout, f"{label}: {result.stdout}{result.stderr}"
        assert jellyfin_output.read_bytes() == good, f"{label}: the rebuild did not restore the canonical bytes"
        assert "already identical" in run(tmp, "--all").stdout
        assert run(tmp, "--check", "--all").returncode == 0

    # CRLF in the production share code is caught by the bare --check too, and repaired.
    production_output = tmp / EXPECTED_PAIRS[0][1]
    production_good = production_output.read_bytes()
    production_output.write_bytes(production_good.rstrip(b"\n") + b"\r\n")
    assert run(tmp, "--check").returncode == 1
    assert run(tmp, "--all").returncode == 0 and production_output.read_bytes() == production_good

    # Repeated generation from a missing file is deterministic.
    jellyfin_output.unlink()
    assert run(tmp, "--all").returncode == 0 and jellyfin_output.read_bytes() == good
    jellyfin_output.unlink()
    assert run(tmp, "--all").returncode == 0 and jellyfin_output.read_bytes() == good

    # A desynchronised debug share code is caught too (it was not checked at release time before).
    debug_output = tmp / EXPECTED_PAIRS[1][1]
    debug_output.write_text((tmp / EXPECTED_PAIRS[0][1]).read_text(encoding="utf-8"), encoding="utf-8")
    result = run(tmp, "--check", "--all")
    assert result.returncode == 1 and "formatter-debug" in result.stderr, result.stderr
finally:
    shutil.rmtree(tmp)

# ---------------------------------------------------------------------------
# Source protection: no operation may replace a formatter source with a share code.
# Aliases are detected by file identity, not by comparing path text, and the guard
# runs in --check as well as in a build. Every rejected operation must leave the
# source (and the alias) byte-identical.
# ---------------------------------------------------------------------------
JELLYFIN_SOURCE_REL, JELLYFIN_OUTPUT_REL = EXPECTED_PAIRS[2]
tmp = make_tree()
try:
    source = tmp / JELLYFIN_SOURCE_REL
    original = source.read_bytes()
    streamnzb = tmp / "tests" / "streamnzb_compat"
    aliases = {
        "identical path": JELLYFIN_SOURCE_REL,
        "absolute alias of the relative path": str(source),
        "path containing ..": "tests/streamnzb_compat/../streamnzb_compat/formatter-jellyfin.source.json",
        "path containing . and //": "./tests//streamnzb_compat/./formatter-jellyfin.source.json",
    }
    try:
        (streamnzb / "link.json").symlink_to(source.name)
        aliases["symbolic link"] = "tests/streamnzb_compat/link.json"
        (tmp / "dir-link").symlink_to(streamnzb, target_is_directory=True)
        aliases["symbolic link to the parent directory"] = "dir-link/formatter-jellyfin.source.json"
    except (OSError, NotImplementedError):
        print("NOTE: symbolic links unavailable; symlink alias cases skipped")
    try:
        import os

        os.link(source, streamnzb / "hard.json")
        aliases["hard link"] = "tests/streamnzb_compat/hard.json"
    except (OSError, NotImplementedError):
        print("NOTE: hard links unavailable; hard-link alias case skipped")

    for label, output in aliases.items():
        for mode in ((), ("--check",)):
            result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", output, *mode)
            assert result.returncode == 1, f"{label} {mode}: rc={result.returncode}\n{result.stdout}{result.stderr}"
            assert "same file" in result.stderr, f"{label} {mode}: {result.stderr}"
            assert "Traceback" not in result.stderr
            assert source.read_bytes() == original, f"{label} {mode}: the source was modified"
    for name in ("link.json", "hard.json"):
        if (streamnzb / name).exists() or (streamnzb / name).is_symlink():
            assert (streamnzb / name).read_bytes() == original, f"{name} was modified"
    if (streamnzb / "link.json").exists():
        assert (streamnzb / "link.json").is_symlink(), "the symlink alias was replaced"

    # Explicit-pair mode with the default production pair and the source of another pair.
    result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", EXPECTED_PAIRS[0][0])
    assert result.returncode == 1 and "refusing to overwrite" in result.stderr, result.stderr
    assert (tmp / EXPECTED_PAIRS[0][0]).read_bytes() == (ROOT / EXPECTED_PAIRS[0][0]).read_bytes()
    garbage = tmp / "notes.txt"
    garbage.write_text("not a share code\n", encoding="utf-8")
    for mode in ((), ("--check",)):
        result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "notes.txt", *mode)
        assert result.returncode == 1 and "refusing to overwrite" in result.stderr, result.stderr
        assert garbage.read_text(encoding="utf-8") == "not a share code\n"

    # A missing output directory is a controlled failure that leaves the source alone.
    result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "no-such-dir/jf.txt")
    assert result.returncode == 1 and "Traceback" not in result.stderr and "FileNotFoundError" in result.stderr, result.stderr
    assert source.read_bytes() == original and not (tmp / "no-such-dir").exists()

    # Normal, distinct paths still work: a missing output is created atomically, an
    # existing generated output is left alone, and --check agrees.
    created = tmp / "elsewhere" / "jf.txt"
    created.parent.mkdir()
    result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "elsewhere/jf.txt", "--check")
    assert result.returncode == 1 and "not synchronized" in result.stderr and not created.exists()
    result = run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "elsewhere/jf.txt")
    assert result.returncode == 0 and "Generated" in result.stdout, result.stderr
    assert created.read_bytes() == (ROOT / JELLYFIN_OUTPUT_REL).read_bytes()
    assert [p.name for p in created.parent.iterdir()] == ["jf.txt"], "temporary file left behind"
    assert "already identical" in run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "elsewhere/jf.txt").stdout
    assert run(tmp, "--source", JELLYFIN_SOURCE_REL, "--output", "elsewhere/jf.txt", "--check").returncode == 0
    assert source.read_bytes() == original
finally:
    shutil.rmtree(tmp)

# ---------------------------------------------------------------------------
# Aggregate processing: one failing pair is reported (with its path) and never
# stops the others; the overall status is nonzero; expected file and format
# failures are diagnostics, not tracebacks; programming errors still propagate.
# Faults are injected in-process, so the outcome does not depend on OS permissions.
# ---------------------------------------------------------------------------
import contextlib  # noqa: E402
import io  # noqa: E402
import os  # noqa: E402


def run_main(root, *args):
    """Run build_formatter.main() in-process against `root`; returns (rc, stdout, stderr)."""
    out, err = io.StringIO(), io.StringIO()
    real_file, real_argv = build_formatter.__file__, sys.argv
    build_formatter.__file__ = str(root / "scripts" / "build_formatter.py")
    sys.argv = ["build_formatter.py", *args]
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            rc = build_formatter.main()
    finally:
        build_formatter.__file__, sys.argv = real_file, real_argv
    return rc, out.getvalue(), err.getvalue()


@contextlib.contextmanager
def deny_reads(name):
    """Make every read of a file called `name` raise PermissionError."""
    real_bytes, real_text = Path.read_bytes, Path.read_text

    def guard(real):
        def wrapper(self, *a, **k):
            if self.name == name:
                raise PermissionError(13, "Permission denied (injected)", str(self))
            return real(self, *a, **k)
        return wrapper

    Path.read_bytes, Path.read_text = guard(real_bytes), guard(real_text)
    try:
        yield
    finally:
        Path.read_bytes, Path.read_text = real_bytes, real_text


def corrupt_pair(root, label, index):
    source, output = (root / EXPECTED_PAIRS[index][0]), (root / EXPECTED_PAIRS[index][1])
    if label == "missing output":
        output.unlink()
    elif label == "malformed output":
        output.write_text("SNZBF1:%%%not-base64%%%\n", encoding="utf-8")
    elif label == "output that is not a share code":
        output.write_text("hello\n", encoding="utf-8")
    elif label == "truncated gzip payload":
        output.write_text(output.read_text(encoding="utf-8").strip()[:-30] + "\n", encoding="utf-8")
    elif label == "malformed source":
        source.write_text("{not json", encoding="utf-8")
    elif label == "source with the wrong schema":
        source.write_text(json.dumps({"streamnzb_format_profile": 1}), encoding="utf-8")
    elif label == "missing source":
        source.unlink()
    elif label == "unsynchronized output":
        data = json.loads(source.read_text(encoding="utf-8"))
        data["name"] += " changed"
        source.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


FAULTS = (
    "missing output", "malformed output", "output that is not a share code", "truncated gzip payload",
    "malformed source", "source with the wrong schema", "missing source", "unsynchronized output",
)
for index in range(len(EXPECTED_PAIRS)):          # every position in the --all order
    for label in FAULTS:
        for mode in (("--check", "--all"), ("--all",)):
            if mode == ("--all",) and label == "unsynchronized output":
                continue                           # a build legitimately repairs this one
            if mode == ("--all",) and label == "missing output":
                continue                           # a build legitimately creates this one
            tmp = make_tree()
            try:
                corrupt_pair(tmp, label, index)
                snapshot = {rel: (tmp / rel).read_bytes() for pair in EXPECTED_PAIRS for rel in pair if (tmp / rel).exists()}
                rc, out, err = run_main(tmp, *mode)
                where = f"{label} at pair {index} {mode}"
                assert rc == 1, f"{where}: rc={rc}\n{out}{err}"
                assert "Traceback" not in err and "ERROR" in err, f"{where}: {err}"
                affected = [Path(EXPECTED_PAIRS[index][0]).name, EXPECTED_PAIRS[index][1]]
                assert any(name in err for name in affected), f"{where}: the diagnostic does not name the pair: {err}"
                healthy = len(EXPECTED_PAIRS) - 1
                marker = "PASS:" if "--check" in mode else "No "
                assert out.count(marker) == healthy, f"{where}: other pairs were not processed ({out.count(marker)} of {healthy})\n{out}"
                # A rejected pair never alters any source, and healthy artifacts stay byte-identical.
                for rel, data in snapshot.items():
                    if (tmp / rel).exists() and (rel.endswith(".source.json") or rel not in EXPECTED_PAIRS[index]):
                        assert (tmp / rel).read_bytes() == data, f"{where}: {rel} was modified"
            finally:
                shutil.rmtree(tmp)

# An unreadable artifact (injected PermissionError) at each position, in both modes.
for index in range(len(EXPECTED_PAIRS)):
    for mode in (("--check", "--all"), ("--all",)):
        tmp = make_tree()
        try:
            with deny_reads(EXPECTED_PAIRS[index][1]):
                rc, out, err = run_main(tmp, *mode)
            assert rc == 1 and "Traceback" not in err, f"unreadable at {index} {mode}: {rc} {err}"
            assert "PermissionError" in err and EXPECTED_PAIRS[index][1] in err, err
            assert out.count("PASS:" if "--check" in mode else "No ") == len(EXPECTED_PAIRS) - 1, out
        finally:
            shutil.rmtree(tmp)
if os.name == "posix" and hasattr(os, "geteuid") and os.geteuid() != 0:
    tmp = make_tree()
    try:
        (tmp / "formatter.txt").chmod(0)
        result = run(tmp, "--check", "--all")
        assert result.returncode == 1 and "Traceback" not in result.stderr and "PermissionError" in result.stderr
        assert result.stdout.count("PASS:") == 2, result.stdout
    finally:
        (tmp / "formatter.txt").chmod(0o644)
        shutil.rmtree(tmp)

# Programming errors are not swallowed by the per-pair handler.
real_encode = build_formatter.encode_payload
build_formatter.encode_payload = lambda payload: (_ for _ in ()).throw(RuntimeError("injected programming error"))
tmp = make_tree()
try:
    try:
        run_main(tmp, "--check", "--all")
    except RuntimeError as exc:
        assert "injected" in str(exc)
    else:
        raise AssertionError("a RuntimeError was swallowed by the per-pair handler")
finally:
    build_formatter.encode_payload = real_encode
    shutil.rmtree(tmp)
rc, out, err = run_main(ROOT, "--check", "--all")
assert rc == 0 and out.count("PASS:") == len(EXPECTED_PAIRS) and not err, (rc, out, err)

# ---------------------------------------------------------------------------
# Newline determinism. Windows is emulated: every text-mode write that does not
# opt out of translation (newline omitted OR newline=None) turns "\n" into "\r\n";
# binary writes and an explicit newline="\n" or "" are untouched. This is a
# platform-independent emulation; no Windows runtime was used.
# ---------------------------------------------------------------------------
import builtins  # noqa: E402


@contextlib.contextmanager
def windows_newline_translation():
    real_fdopen, real_open, real_io_open = os.fdopen, builtins.open, io.open

    def translating(real):
        # newline is a named parameter so that it is seen however the caller passes it
        # (omitted, None, keyword or positional, as pathlib does).
        def wrapper(file, mode="r", buffering=-1, encoding=None, errors=None, newline=None, *args, **kwargs):
            if any(flag in mode for flag in "wax+") and "b" not in mode and newline is None:
                newline = "\r\n"
            return real(file, mode, buffering, encoding, errors, newline, *args, **kwargs)
        return wrapper

    os.fdopen, builtins.open, io.open = translating(real_fdopen), translating(real_open), translating(real_io_open)
    try:
        yield
    finally:
        os.fdopen, builtins.open, io.open = real_fdopen, real_open, real_io_open


def writer_fdopen_default(path, text):          # the previously shipped defect
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(text)


def writer_fdopen_none(path, text):              # explicit default
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC)
    with os.fdopen(fd, "w", encoding="utf-8", newline=None) as handle:
        handle.write(text)


def writer_open_default(path, text):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(text)


def writer_write_text(path, text):
    Path(path).write_text(text, encoding="utf-8")


def writer_binary(path, text):
    with open(path, "wb") as handle:
        handle.write(text.encode("utf-8"))


def writer_newline_lf(path, text):
    with open(path, "w", encoding="utf-8", newline="\n") as handle:
        handle.write(text)


def writer_newline_empty(path, text):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        handle.write(text)


translating_writers = (writer_fdopen_default, writer_fdopen_none, writer_open_default, writer_write_text)
canonical_writers = (writer_binary, writer_newline_lf, writer_newline_empty)
sample = "SNZBF1:abc\n"
tmp = Path(tempfile.mkdtemp(prefix="build-formatter-writers-"))
try:
    probe = tmp / "probe.txt"
    with windows_newline_translation():
        for writer in translating_writers:           # the emulation must catch each defect
            writer(probe, sample)
            assert probe.read_bytes() == b"SNZBF1:abc\r\n", f"the emulation missed {writer.__name__}"
        for writer in canonical_writers:             # and must not flag canonical writers
            writer(probe, sample)
            assert probe.read_bytes() == sample.encode(), f"{writer.__name__} should write canonical bytes"
    for writer in translating_writers:               # outside the emulation they write LF
        writer(probe, sample)
        assert probe.read_bytes() == sample.encode()
finally:
    shutil.rmtree(tmp)

# The real builder under the emulation: LF, CRLF, missing and extra final newline all
# regenerate to the exact canonical bytes, repeatedly and deterministically; the
# previously defective writers do not, and --check then rejects their output.
canonical = (ROOT / JELLYFIN_OUTPUT_REL).read_bytes()
drifts = (
    ("canonical LF", canonical),
    ("CRLF terminator", canonical.rstrip(b"\n") + b"\r\n"),
    ("missing final newline", canonical.rstrip(b"\n")),
    ("extra final newline", canonical + b"\n"),
)
for label, content in drifts:
    tmp = make_tree()
    try:
        output = tmp / JELLYFIN_OUTPUT_REL
        output.write_bytes(content)
        with windows_newline_translation():
            first = run_main(tmp, "--all")
            assert first[0] == 0, f"{label}: {first}"
            assert output.read_bytes() == canonical, f"{label}: the rebuild did not restore the canonical bytes"
            output.unlink()
            run_main(tmp, "--all")
            assert output.read_bytes() == canonical, f"{label}: regeneration is not deterministic"
        assert run_main(tmp, "--check", "--all")[0] == 0
        assert (b"\r" in content) == (label == "CRLF terminator")
    finally:
        shutil.rmtree(tmp)

real_atomic_write = build_formatter.atomic_write
for defective in (writer_fdopen_default, writer_fdopen_none, writer_open_default, writer_write_text):
    tmp = make_tree()
    try:
        output = tmp / JELLYFIN_OUTPUT_REL
        output.unlink()
        build_formatter.atomic_write = lambda path, text, writer=defective: writer(path, text)
        try:
            with windows_newline_translation():
                run_main(tmp, "--all")
        finally:
            build_formatter.atomic_write = real_atomic_write
        assert b"\r\n" in output.read_bytes(), f"{defective.__name__}: the defective writer was not exposed"
        assert run_main(tmp, "--check", "--all")[0] == 1, f"{defective.__name__}: --check accepted CRLF output"
    finally:
        shutil.rmtree(tmp)

tmp = Path(tempfile.mkdtemp(prefix="build-formatter-writer-"))
try:
    target = tmp / "share-code.txt"
    expected = build_formatter.encode_payload(build_formatter.load_source(ROOT / JELLYFIN_SOURCE_REL))
    with windows_newline_translation():
        build_formatter.atomic_write(target, expected)
    assert target.read_bytes() == expected.encode("utf-8") and b"\r" not in target.read_bytes()
    assert [p.name for p in tmp.iterdir()] == ["share-code.txt"], "temporary file left behind"
finally:
    shutil.rmtree(tmp)

# ---------------------------------------------------------------------------
# Git checkout determinism. `.gitattributes` pins the three published share codes to
# LF. A disposable repository is cloned with core.autocrlf=true (the usual Windows
# Git setting, simulated here on any OS; no Windows runtime was used). Without the
# attributes the checkout holds CRLF and the byte-exact check fails; with them the
# checkout is canonical. The user's Git configuration is neither read nor modified.
# ---------------------------------------------------------------------------
GIT_ENV = dict(
    os.environ,
    GIT_CONFIG_GLOBAL=os.devnull,
    GIT_CONFIG_NOSYSTEM="1",
    GIT_TERMINAL_PROMPT="0",
    GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.invalid",
    GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.invalid",
)


def git(cwd, *args, check=True):
    result = subprocess.run(["git", *args], cwd=cwd, env=GIT_ENV, capture_output=True, text=True, check=False)
    assert not check or result.returncode == 0, f"git {' '.join(args)}: {result.stderr}"
    return result


def make_repo(root, with_attributes):
    repo = root / ("with-attributes" if with_attributes else "without-attributes")
    (repo / "scripts").mkdir(parents=True)
    shutil.copy(ROOT / "scripts" / "build_formatter.py", repo / "scripts")
    for src, out in EXPECTED_PAIRS:
        (repo / src).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy(ROOT / src, repo / src)
        shutil.copy(ROOT / out, repo / out)
    if with_attributes:
        shutil.copy(ROOT / ".gitattributes", repo / ".gitattributes")
    git(repo, "init", "-q", "-b", "main")
    git(repo, "config", "commit.gpgsign", "false")
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", "base")
    return repo


if subprocess.run(["git", "--version"], capture_output=True).returncode == 0:
    tmp = Path(tempfile.mkdtemp(prefix="build-formatter-git-"))
    try:
        with_attrs, without_attrs = make_repo(tmp, True), make_repo(tmp, False)
        # The committed blobs are LF in both repositories (the files are generated LF).
        for repo in (with_attrs, without_attrs):
            for _, out in EXPECTED_PAIRS:
                assert b"\r" not in git(repo, "show", f"HEAD:{out}").stdout.encode()

        clone = tmp / "clone-with-attributes"
        git(tmp, "-c", "core.autocrlf=true", "clone", "-q", str(with_attrs), str(clone))
        for _, out in EXPECTED_PAIRS:
            assert (clone / out).read_bytes() == (ROOT / out).read_bytes(), f"{out}: checkout bytes changed"
        assert run(clone, "--check", "--all").returncode == 0, "byte-exact check failed after an autocrlf checkout"
        eol = git(clone, "ls-files", "--eol", "--", *[out for _, out in EXPECTED_PAIRS]).stdout
        assert eol.count("w/lf") == len(EXPECTED_PAIRS) and "w/crlf" not in eol, eol

        # Negative control: the same configuration without the attributes produces CRLF.
        control = tmp / "clone-without-attributes"
        git(tmp, "-c", "core.autocrlf=true", "clone", "-q", str(without_attrs), str(control))
        assert all(b"\r\n" in (control / out).read_bytes() for _, out in EXPECTED_PAIRS), \
            "the control checkout was not converted; the simulation is not exercising autocrlf"
        assert run(control, "--check", "--all").returncode == 1, "CRLF checkout passed the byte-exact check"
        assert run(control, "--all").returncode == 0, "the rebuild could not repair the CRLF checkout"
        for _, out in EXPECTED_PAIRS:
            assert (control / out).read_bytes() == (ROOT / out).read_bytes()
        assert run(control, "--check", "--all").returncode == 0

        # Intentional corruption in the attributed clone is repaired by a rebuild.
        victim = clone / JELLYFIN_OUTPUT_REL
        victim.write_bytes(victim.read_bytes().rstrip(b"\n") + b"\r\n")
        assert run(clone, "--check", "--all").returncode == 1
        assert run(clone, "--all").returncode == 0 and victim.read_bytes() == (ROOT / JELLYFIN_OUTPUT_REL).read_bytes()
    finally:
        shutil.rmtree(tmp)

    # The attributes apply to exactly the three artifacts and nothing else.
    probe = subprocess.run(
        ["git", "check-attr", "text", "eol", "--", "formatter.txt", "formatter-debug.txt", "formatter-jellyfin.txt",
         "README.md", "profile.txt", "scripts/build_formatter.py", "tests/streamnzb_compat/formatter-jellyfin.source.json"],
        cwd=ROOT, capture_output=True, text=True, check=False,
    )
    if probe.returncode == 0:
        attrs = {}
        for line in probe.stdout.splitlines():
            path, attr, value = line.split(": ", 2)
            attrs[(path, attr)] = value
        for _, out in EXPECTED_PAIRS:
            assert attrs[(out, "text")] == "set" and attrs[(out, "eol")] == "lf", (out, attrs)
        for other in ("README.md", "profile.txt", "scripts/build_formatter.py", "tests/streamnzb_compat/formatter-jellyfin.source.json"):
            assert attrs[(other, "eol")] == "unspecified" and attrs[(other, "text")] == "unspecified", (other, attrs)
        text = (ROOT / ".gitattributes").read_text(encoding="utf-8")
        assert [l for l in text.splitlines() if l and not l.startswith("#")] == [
            f"/{out} text eol=lf" for _, out in EXPECTED_PAIRS
        ], "unexpected .gitattributes content"
else:
    print("NOTE: git unavailable; checkout simulation skipped")

# Only the Jellyfin formatter may have an empty description (StreamNZB's Jellyfin
# endpoint renders the name only); production and debug must keep theirs.
payloads = {out: build_formatter.load_source(ROOT / src) for src, out in EXPECTED_PAIRS}
assert payloads["formatter.txt"]["result_description_template"].strip()
assert payloads["formatter-debug.txt"]["result_description_template"].strip()
jellyfin = payloads["formatter-jellyfin.txt"]
assert jellyfin["name"] == "DraCuLa Jellyfin"
assert jellyfin["result_description_template"] == "", "the Jellyfin formatter must not copy a description"
assert jellyfin["result_name_template"].strip()

# Release tooling checks every formatter pair. Behavioural test: run the real entry
# points (`candidate`, `prep-pr`, `verify-published`, all offline) with the
# build_formatter subprocess replaced by a failing fake, and require the finding
# and the `--all` command from each. (`prepare` runs no generation check for any
# artifact, profiles included, and is intentionally not covered.)
import subprocess  # noqa: E402

import check_release  # noqa: E402

captured = []
real_run = check_release._run


def fake_run(args, cwd=check_release.ROOT):
    if any("build_formatter.py" in str(arg) for arg in args):
        captured.append([str(arg) for arg in args])
        return subprocess.CompletedProcess(args, 1, "", "ERROR: formatter-jellyfin is not synchronized")
    return real_run(args, cwd)


SHA = "9efb02f19c90bf9d923140f8cc10bbe2666f40eb"
check_release._run = fake_run
try:
    for label, call in (
        ("candidate", lambda: check_release.run_candidate("6.5.1", SHA, None, True)),
        ("prep-pr", lambda: check_release.run_prep_pr("6.5.1", None, True)),
        ("verify-published", lambda: check_release.run_verify_published("6.5.1", SHA, None, True)),
    ):
        captured.clear()
        report = call()
        findings = [f for f in report.findings if f.check == "formatter-generation-sync"]
        assert len(findings) == 1 and not findings[0].ok, f"{label}: {findings}"
        assert len(captured) == 1 and "--check" in captured[0] and "--all" in captured[0], f"{label}: {captured}"
        assert "formatter-jellyfin" in findings[0].detail, f"{label}: {findings[0].detail}"
finally:
    check_release._run = real_run

print("PASS: build_formatter registry, --all, determinism, drift detection and description policy.")
