#!/usr/bin/env python3
"""
Required PR-head validation for generated Vidhin synchronization PRs.

Runs from a checkout of the trusted base commit. The PR head is data only:
its three generated files are read as git blobs and never executed.

  prepare  classify the PR, check its author, repository, ancestry and file
           scope, overlay the head's generated files on the base checkout,
           and reproduce them with the trusted generator
  decide   verify the assessment evidence against the base, the head and
           the pinned StreamNZB engine, re-run the semantic assessment, and
           write the final decision

Policy: docs/VIDHIN_SYNC_ASSESSMENT.md ("PR-head validation"). This tool
never merges, approves or labels anything.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import assess_vidhin_sync as assess  # noqa: E402

SCHEMA = 1
NOT_APPLICABLE = "NOT_APPLICABLE"
# Must match sync-vidhin.yml's create-pull-request branch and add-paths, and
# the GitHub App that opens the PR (tests/test_validate_vidhin_sync_pr.py).
SYNC_BRANCH = "chore/vidhin-sync"
SYNC_AUTHOR = "streamnzb-template-prepare-release[bot]"
BASELINE = "generated/vidhin-defines.json"
REPORT = "generated/vidhin-sync-report.md"
LIBRARY = "generated/streamnzb-defines.txt"
GENERATED = (BASELINE, REPORT, LIBRARY)
MAPPING = "scripts/vidhin_mapping.json"
PROFILES = (("neutral", "profile-neutral.txt"), ("samsung", "profile.txt"))
COMPAT_SCRIPT = "scripts/test_streamnzb_compat.sh"
COMPAT_PASSED = "StreamNZB compatibility tests passed."
SHA_RE = re.compile(r"[0-9a-f]{40}")


def git(repo, *args):
    return subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True).stdout


def blob(repo, rev, path):
    return git(repo, "show", f"{rev}:{path}")


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def applicable(head_ref):
    return head_ref == SYNC_BRANCH


class Reasons(list):
    def add(self, code, detail):
        self.append({"code": code, "detail": detail})


# --------------------------------------------------------------------------
# prepare


def reproduce(repo, base_sha, head):
    """Re-run the trusted generator on the head baseline's own upstream records."""
    new = json.loads(head[BASELINE])
    upstream = []
    for entry in new["defines"].values():
        for rec in entry["records"]:
            item = {"name": rec["source"], "pattern": rec["pattern"], "score": 0}
            if item not in upstream:
                upstream.append(item)
    with tempfile.TemporaryDirectory() as tmp:
        d = Path(tmp)
        (d / "up.json").write_text(json.dumps(upstream), encoding="utf-8")
        (d / "mapping.json").write_bytes(blob(repo, base_sha, MAPPING))
        (d / "b.json").write_bytes(blob(repo, base_sha, BASELINE))
        (d / "r.md").write_bytes(blob(repo, base_sha, REPORT))
        proc = subprocess.run(
            [sys.executable, str(ROOT / "scripts/sync_vidhin.py"), "--mapping", str(d / "mapping.json"),
             "--baseline", str(d / "b.json"), "--report", str(d / "r.md"), "--library", str(d / "l.txt"),
             "--upstream-file", str(d / "up.json")],
            capture_output=True, text=True,
        )
        if proc.returncode:
            raise RuntimeError(((proc.stderr or proc.stdout).strip().splitlines() or ["sync_vidhin.py failed"])[-1])
        out = {BASELINE: (d / "b.json").read_bytes(), REPORT: (d / "r.md").read_bytes(),
               LIBRARY: (d / "l.txt").read_bytes()}

    problems = []
    want = json.loads(out[BASELINE])
    stamp = new.pop("generated_at_utc", None)
    want.pop("generated_at_utc", None)
    try:
        dt.datetime.fromisoformat(stamp)
    except (TypeError, ValueError):
        problems.append(f"{BASELINE}: generated_at_utc {stamp!r} is not an ISO timestamp")
    if new != want:
        problems.append(f"{BASELINE} differs from the trusted generator output")
    for path in (REPORT, LIBRARY):
        if head[path] != out[path]:
            problems.append(f"{path} differs from the trusted generator output")
    return problems


def prepare(repo, *, base_sha, head_sha, head_ref, head_repo, base_repo, author, author_type):
    repo = Path(repo)
    reasons = Reasons()
    state = {"schema_version": SCHEMA, "applicable": applicable(head_ref), "base_sha": base_sha,
             "head_sha": head_sha, "head_ref": head_ref, "head_repo": head_repo, "author": author,
             "reasons": reasons}
    if not state["applicable"]:
        return state

    if author != SYNC_AUTHOR or author_type != "Bot":
        reasons.add("unexpected_author", f"{author} ({author_type}); expected {SYNC_AUTHOR}")
    if head_repo != base_repo:
        reasons.add("fork_head", f"head repository {head_repo}, base repository {base_repo}")
    for label, value in (("base", base_sha), ("head", head_sha)):
        if not SHA_RE.fullmatch(value or ""):
            reasons.add("commit_missing", f"{label} {value!r} is not a full commit SHA")
        elif subprocess.run(["git", "-C", str(repo), "cat-file", "-e", f"{value}^{{commit}}"],
                            capture_output=True).returncode:
            reasons.add("commit_missing", f"{label} commit {value} is not available")
    if reasons:
        return state
    if git(repo, "rev-parse", "HEAD").decode().strip() != base_sha:
        reasons.add("checkout_not_base", "the validation checkout is not the PR base commit")
    if subprocess.run(["git", "-C", str(repo), "merge-base", "--is-ancestor", base_sha, head_sha]).returncode:
        reasons.add("head_not_up_to_date", f"base {base_sha} is not an ancestor of head {head_sha}")

    # The direct base..head tree diff, so a stale head also shows reverted base changes.
    raw = git(repo, "diff", "--raw", "--no-renames", "--no-abbrev", "-z", base_sha, head_sha).decode().split("\0")
    changed = {}
    for meta, path in zip(raw[0::2], raw[1::2]):
        _, new_mode, _, _, status = meta.lstrip(":").split()
        changed[path] = (status, new_mode)
    for path, (status, mode) in sorted(changed.items()):
        if path not in GENERATED or status != "M" or mode != "100644":
            reasons.add("unauthorized_change", f"{status} {mode} {path}")
    if not changed:
        reasons.add("empty_sync", "the head does not change any generated file")
    if reasons:
        return state

    head = {path: blob(repo, head_sha, path) for path in GENERATED}
    for path, data in head.items():
        (repo / path).write_bytes(data)
    try:
        for problem in reproduce(repo, base_sha, head):
            reasons.add("generator_mismatch", problem)
    except Exception as exc:  # any generator failure on head data fails closed
        reasons.add("generator_failed", f"{type(exc).__name__}: {exc}")
    return state


# --------------------------------------------------------------------------
# decide


def assessment_args(repo, adir):
    """The exact inputs scripts/assess_vidhin_sync.sh uses for this checkout."""
    repo, adir = Path(repo), Path(adir)
    args = ["--old-baseline", str(adir / "old-baseline.json"), "--old-library", str(adir / "old-library.txt"),
            "--new-baseline", str(repo / BASELINE), "--new-library", str(repo / LIBRARY),
            "--mapping", str(repo / MAPPING), "--plan", str(adir / "probe-plan.json")]
    for variant, path in PROFILES:
        args += ["--profile", f"{variant}={repo / path}"]
    return args


def compat_result(log, pin):
    used = re.findall(r"^==> Using StreamNZB ([0-9a-f]{40})$", log, flags=re.MULTILINE)
    if used != [pin]:
        return None
    return "passed" if COMPAT_PASSED in log.splitlines() else "failed"


def verify_evidence(repo, base_sha, head_sha, adir, reasons):
    """Return the assessment decision, or None when the evidence is not trustworthy."""
    if git(repo, "rev-parse", "HEAD").decode().strip() != base_sha:
        reasons.add("checkout_modified", "HEAD moved away from the base commit")
    files = {
        repo / BASELINE: blob(repo, head_sha, BASELINE),
        repo / LIBRARY: blob(repo, head_sha, LIBRARY),
        repo / REPORT: blob(repo, head_sha, REPORT),
        adir / "old-baseline.json": blob(repo, base_sha, BASELINE),
        adir / "old-library.txt": blob(repo, base_sha, LIBRARY),
        repo / MAPPING: blob(repo, base_sha, MAPPING),
        repo / COMPAT_SCRIPT: blob(repo, base_sha, COMPAT_SCRIPT),
        **{repo / path: blob(repo, base_sha, path) for _, path in PROFILES},
    }
    for path, want in files.items():
        if not path.is_file() or path.read_bytes() != want:
            reasons.add("checkout_modified", f"{path.name} does not match its PR base/head blob")
    allowed = {f" M {path}" for path in GENERATED}
    status = [line for line in git(repo, "status", "--porcelain", "--untracked-files=all").decode().splitlines()
              if line not in allowed]
    hooks = [p.name for p in (repo / ".git/hooks").glob("*") if p.is_file() and not p.name.endswith(".sample")]
    if status or hooks:
        reasons.add("checkout_modified", f"unexpected checkout changes: {status + hooks}")

    pin = re.search(r'^STREAMNZB_REF="\$\{STREAMNZB_REF:-([0-9a-f]{40})\}"$',
                    blob(repo, base_sha, COMPAT_SCRIPT).decode(), flags=re.MULTILINE)
    log_path = adir / "compat.log"
    compat = None
    if not pin:
        reasons.add("engine_revision_mismatch", f"no pinned StreamNZB revision in {COMPAT_SCRIPT}")
    elif not log_path.is_file():
        reasons.add("compat_evidence_missing", "compat.log is missing")
    else:
        compat = compat_result(log_path.read_text(encoding="utf-8", errors="replace"), pin.group(1))
        if compat is None:
            reasons.add("engine_revision_mismatch", f"compat.log does not show exactly StreamNZB {pin.group(1)}")

    try:
        recorded = (adir / "assessment.json").read_bytes()
        decision = json.loads(recorded)["decision"]
    except (OSError, ValueError, KeyError, TypeError) as exc:
        reasons.add("assessment_missing", f"assessment.json unreadable: {type(exc).__name__}")
        return None
    if reasons:
        return None

    # Re-run the semantic assessment on the verified inputs and engine results;
    # any edit to assessment.json, or a compat result the log does not support,
    # makes the outputs differ.
    with tempfile.TemporaryDirectory() as tmp:
        out = Path(tmp) / "assessment.json"
        rc = assess.main(["assess", *assessment_args(repo, adir), "--compat-result", compat,
                          "--engine-results", str(adir / "probe-results.json"), "--json-out", str(out)])
        if rc != 0 or out.read_bytes() != recorded:
            reasons.add("assessment_contradictory", "assessment.json does not match a re-run on the verified inputs")
            return None
    if decision not in assess.DECISION_RANK:
        reasons.add("assessment_contradictory", f"unknown decision {decision!r}")
        return None
    return decision


def decide(repo, *, head_ref, pr_number, prepare_path, assessment_dir, live_head_sha, live_base_sha):
    repo, adir = Path(repo), Path(assessment_dir)
    result = {"schema_version": SCHEMA, "decision": NOT_APPLICABLE, "pr": {"number": pr_number, "head_ref": head_ref},
              "reasons": [], "assessment_decision": None}
    if not applicable(head_ref):
        return result

    reasons = Reasons()
    try:
        state = json.loads(Path(prepare_path).read_text(encoding="utf-8"))
        base_sha, head_sha = state["base_sha"], state["head_sha"]
        if state.get("applicable") is not True or state.get("head_ref") != head_ref:
            reasons.add("evidence_contradictory", "prepare evidence is for a different PR")
        for r in state["reasons"]:
            reasons.add(r["code"], r["detail"])
    except (OSError, ValueError, KeyError, TypeError) as exc:
        reasons.add("evidence_missing", f"prepare evidence unreadable: {type(exc).__name__}")
        state, base_sha, head_sha = {}, None, None
    result["pr"].update({"base_sha": base_sha, "head_sha": head_sha, "author": state.get("author")})

    decision = None
    if not reasons:
        decision = verify_evidence(repo, base_sha, head_sha, adir, reasons)
        result["assessment_decision"] = decision
    if head_sha and live_head_sha != head_sha:
        reasons.add("stale_pr_head", f"PR head is now {live_head_sha or 'unknown'}")
    if base_sha and live_base_sha != base_sha:
        reasons.add("stale_base", f"base branch is now {live_base_sha or 'unknown'}")
    result["reasons"] = sorted(reasons, key=lambda r: (r["code"], r["detail"]))
    result["decision"] = assess.BLOCKED if reasons or decision is None else decision
    return result


def render_summary(result):
    lines = ["## Vidhin sync PR validation", "", f"**Decision: `{result['decision']}`**", ""]
    pr = result["pr"]
    if result["decision"] == NOT_APPLICABLE:
        lines.append(f"Not a `{SYNC_BRANCH}` PR; nothing to validate.")
        return "\n".join(lines) + "\n"
    lines.append(f"PR head `{pr.get('head_sha')}` against base `{pr.get('base_sha')}`.")
    lines.append("")
    if result["reasons"]:
        lines += ["**Validation failures**", *[f"- `{r['code']}`: {r['detail']}" for r in result["reasons"]], ""]
    if result["assessment_decision"]:
        lines.append(f"Re-verified assessment: `{result['assessment_decision']}` (details in the evidence artifact).")
        lines.append("")
    lines.append("This check never approves or merges. REVIEW_REQUIRED passes the check but needs human review.")
    return "\n".join(lines) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", type=Path, default=ROOT)
    sub = ap.add_subparsers(dest="command", required=True)
    p = sub.add_parser("prepare")
    for name in ("base-sha", "head-sha", "head-ref", "head-repo", "base-repo", "author", "author-type"):
        p.add_argument(f"--{name}", default="")
    p.add_argument("--out", type=Path, required=True)
    d = sub.add_parser("decide")
    for name in ("head-ref", "pr-number", "live-head-sha", "live-base-sha"):
        d.add_argument(f"--{name}", default="")
    d.add_argument("--prepare", type=Path, required=True)
    d.add_argument("--assessment-dir", type=Path, required=True)
    d.add_argument("--json-out", type=Path, required=True)
    d.add_argument("--summary-out", type=Path)
    args = ap.parse_args(argv)

    if args.command == "prepare":
        state = prepare(args.repo, base_sha=args.base_sha, head_sha=args.head_sha, head_ref=args.head_ref,
                        head_repo=args.head_repo, base_repo=args.base_repo, author=args.author,
                        author_type=args.author_type)
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(assess.canonical_json(state), encoding="utf-8")
        run_engine = state["applicable"] and not state["reasons"]
        if os.environ.get("GITHUB_OUTPUT"):
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as fh:
                fh.write(f"run_engine={str(run_engine).lower()}\n")
        print(f"Vidhin sync PR prepare: applicable={state['applicable']} reasons={len(state['reasons'])}")
        return 0

    result = decide(args.repo, head_ref=args.head_ref, pr_number=args.pr_number, prepare_path=args.prepare,
                    assessment_dir=args.assessment_dir, live_head_sha=args.live_head_sha,
                    live_base_sha=args.live_base_sha)
    args.json_out.parent.mkdir(parents=True, exist_ok=True)
    args.json_out.write_text(assess.canonical_json(result), encoding="utf-8")
    summary = render_summary(result)
    if args.summary_out:
        with open(args.summary_out, "a", encoding="utf-8") as fh:
            fh.write(summary)
    print(summary)
    return 1 if result["decision"] == assess.BLOCKED else 0


if __name__ == "__main__":
    sys.exit(main())
