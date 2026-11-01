#!/usr/bin/env python3
"""
Regression tests for scripts/validate_vidhin_sync_pr.py and
.github/workflows/validate-vidhin-sync-pr.yml.

Each case builds a throwaway git repository whose base commit holds the
committed mapping, generated files and profiles, and whose head commit is a
sync produced by the real generator from an edited upstream. Engine results
are synthetic, as in tests/test_vidhin_sync_assessment.py; the real engine
runs in the workflow itself.
"""

import importlib.util
import json
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("validate_pr", ROOT / "scripts/validate_vidhin_sync_pr.py")
v = importlib.util.module_from_spec(spec)
spec.loader.exec_module(v)
assess = v.assess

TMP = Path(tempfile.mkdtemp(prefix="vidhin-pr-validation-test-"))
GIT = ["-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false",
       "-c", "core.hooksPath=/dev/null"]
REPO = "d4s87/streamnzb-template"
PIN = re.search(r'STREAMNZB_REF:-([0-9a-f]{40})', (ROOT / v.COMPAT_SCRIPT).read_text()).group(1)
PASSING_LOG = f"==> StreamNZB compatibility harness\n==> Using StreamNZB {PIN}\n{v.COMPAT_PASSED}\n"
REMUX_T1 = "(BLURANiUM|BMF|FraMeSToR|PmP)"
counter = 0


def fresh(name):
    global counter
    counter += 1
    d = TMP / f"{name}-{counter}"
    d.mkdir()
    return d


def git(repo, *args):
    return subprocess.run(["git", "-C", str(repo), *GIT, *args], check=True, capture_output=True,
                          text=True).stdout.strip()


def upstream_records():
    defines = json.loads((ROOT / v.BASELINE).read_text(encoding="utf-8"))["defines"]
    out = []
    for entry in defines.values():
        for rec in entry["records"]:
            item = {"name": rec["source"], "pattern": rec["pattern"], "score": 0}
            if item not in out:
                out.append(item)
    return out


def edit_remux(new_alternation):
    def edit(up):
        rec = next(r for r in up if r["name"] == "Sonarr Remux T1" and REMUX_T1 in r["pattern"])
        rec["pattern"] = rec["pattern"].replace(REMUX_T1, new_alternation)
    return edit


SAFE_EDIT = edit_remux("(BMF|BLURANiUM|FraMeSToR|PmP)")  # upstream text only
REVIEW_EDIT = edit_remux("(BLURANiUM|BMF|FraMeSToR|NEWGRP|PmP)")  # new Remux T1 group


def make_pr(edit=SAFE_EDIT, tamper=None, advance_base=False):
    repo = fresh("repo")
    git(repo, "init", "-q", "-b", "main")
    for path in (v.MAPPING, v.COMPAT_SCRIPT, *v.GENERATED, *(p for _, p in v.PROFILES)):
        (repo / path).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / path, repo / path)
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", "base")
    base = git(repo, "rev-parse", "HEAD")
    git(repo, "switch", "-q", "-c", v.SYNC_BRANCH)
    up = upstream_records()
    if edit:
        edit(up)
    up_path = fresh("upstream") / "up.json"
    up_path.write_text(json.dumps(up), encoding="utf-8")
    subprocess.run(["python3", str(ROOT / "scripts/sync_vidhin.py"), "--mapping", str(repo / v.MAPPING),
                    "--baseline", str(repo / v.BASELINE), "--report", str(repo / v.REPORT),
                    "--library", str(repo / v.LIBRARY), "--upstream-file", str(up_path)],
                   check=True, capture_output=True)
    if tamper:
        tamper(repo)
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "--allow-empty", "-m", "chore: sync Vidhin release-group rules")
    head = git(repo, "rev-parse", "HEAD")
    if advance_base:
        git(repo, "switch", "-q", "main")
        (repo / "README.md").write_text("main moved\n", encoding="utf-8")
        git(repo, "add", "-A")
        git(repo, "commit", "-q", "-m", "unrelated main change")
        base = git(repo, "rev-parse", "HEAD")
    git(repo, "checkout", "-q", "--detach", base)
    return repo, base, head


def same_outcome(probe, library):
    return {"found": True, "fetch": True, "rank": 100, "matched": {}, "rejections": []}


def expected_outcome(probe, library):
    out = same_outcome(probe, library)
    if library == "new" and probe["expect_change"]:
        out["rank"] = 400
    return out


def always_changes(probe, library):
    out = same_outcome(probe, library)
    out["rank"] = 100 if library == "old" else 999
    return out


def run_assessment(repo, base, adir, outcome, compat, log):
    """What scripts/assess_vidhin_sync.sh produces, with synthetic engine output."""
    (adir / "old-baseline.json").write_bytes(v.blob(repo, base, v.BASELINE))
    (adir / "old-library.txt").write_bytes(v.blob(repo, base, v.LIBRARY))
    args = v.assessment_args(repo, adir)
    assert assess.main(["plan", *args]) == 0
    plan_bytes = (adir / "probe-plan.json").read_bytes()
    plan = json.loads(plan_bytes)
    (adir / "probe-results.json").write_text(json.dumps({
        "schema_version": 1,
        "plan_sha256": v.sha256(plan_bytes),
        "old_library_sha256": plan["old_library_sha256"],
        "new_library_sha256": plan["new_library_sha256"],
        "results": [{"id": p["id"], "variant": var, "library": lib, **outcome(p, lib)}
                    for p in plan["probes"] for var in plan["variants"] for lib in ("old", "new")],
    }), encoding="utf-8")
    (adir / "compat.log").write_text(log, encoding="utf-8")
    assert assess.main(["assess", *args, "--compat-result", compat, "--engine-results",
                        str(adir / "probe-results.json"), "--json-out", str(adir / "assessment.json")]) == 0


def validate(pr, *, head_ref=v.SYNC_BRANCH, author=v.SYNC_AUTHOR, author_type="Bot", head_repo=REPO,
             outcome=same_outcome, compat="passed", log=PASSING_LOG, live_head=None, live_base=None,
             after_assess=None):
    repo, base, head = pr
    state = v.prepare(repo, base_sha=base, head_sha=head, head_ref=head_ref, head_repo=head_repo,
                      base_repo=REPO, author=author, author_type=author_type)
    prep = fresh("evidence")
    (prep / "prepare.json").write_text(assess.canonical_json(state), encoding="utf-8")
    adir = prep / "assessment"
    adir.mkdir()
    if state["applicable"] and not state["reasons"]:
        run_assessment(repo, base, adir, outcome, compat, log)
    if after_assess:
        after_assess(repo, adir)
    result = v.decide(repo, head_ref=head_ref, pr_number="7", prepare_path=prep / "prepare.json",
                      assessment_dir=adir, live_head_sha=head if live_head is None else live_head,
                      live_base_sha=base if live_base is None else live_base)
    git(repo, "checkout", "-q", "--", ".")  # later cases reuse the repository
    git(repo, "clean", "-fdq")
    result["_codes"] = {r["code"] for r in result["reasons"]}
    result["_state"] = state
    return result


def expect(result, decision, *codes, label):
    assert result["decision"] == decision, (label, result["decision"], result["reasons"])
    missing = set(codes) - result["_codes"]
    assert not missing, (label, f"missing {sorted(missing)}", result["reasons"])
    print(f"ok - {label}: {decision}")


# ---------------------------------------------------------------- valid syncs

safe_pr = make_pr(SAFE_EDIT)
r = validate(safe_pr)
expect(r, assess.SAFE, label="valid SAFE sync")
assert r["assessment_decision"] == assess.SAFE and not r["reasons"]
assert not r["_state"]["reasons"]

review_pr = make_pr(REVIEW_EDIT)
r = validate(review_pr, outcome=expected_outcome)
expect(r, assess.REVIEW, label="REVIEW_REQUIRED new tier group")
assert not r["reasons"], r["reasons"]

# The GHOSTS local override survives reproduction unchanged ...
repo, _, head = review_pr
assert 'group matches "(?i)^(GHOSTS)$"' in v.blob(repo, head, v.LIBRARY).decode()
# ... and a head that drops it is not what the trusted generator produces.
def drop_ghosts_override(repo):
    lib = repo / v.LIBRARY
    lib.write_text(lib.read_text(encoding="utf-8").replace("(?i)^(GHOSTS)$", "^(GHOSTS)$"), encoding="utf-8")
expect(validate(make_pr(REVIEW_EDIT, tamper=drop_ghosts_override)), assess.BLOCKED, "generator_mismatch",
       label="GHOSTS override removed by hand")

# ---------------------------------------------------------------- BLOCKED by the assessment

r = validate(safe_pr, outcome=always_changes)
expect(r, assess.BLOCKED, label="behavior delta behind byte-identical Defines")
assert r["assessment_decision"] == assess.BLOCKED and not r["reasons"], r

r = validate(safe_pr, compat="failed", log=PASSING_LOG.replace(v.COMPAT_PASSED, "ERROR: tests failed"))
expect(r, assess.BLOCKED, label="compatibility suite failed")
assert r["assessment_decision"] == assess.BLOCKED

expect(validate(safe_pr, log=PASSING_LOG.replace(v.COMPAT_PASSED, "ERROR: tests failed")), assess.BLOCKED,
       "assessment_contradictory", label="SAFE claimed while the compat log shows failure")

# ---------------------------------------------------------------- scope, identity, ancestry

def change_mapping(repo):
    path = repo / v.MAPPING
    path.write_text(path.read_text(encoding="utf-8").replace('"GHOSTS"', '"GHOSTS2"'), encoding="utf-8")
expect(validate(make_pr(tamper=change_mapping)), assess.BLOCKED, "unauthorized_change", label="mapping changed")

def add_workflow(repo):
    (repo / ".github/workflows").mkdir(parents=True)
    (repo / ".github/workflows/evil.yml").write_text("on: push\n", encoding="utf-8")
r = validate(make_pr(tamper=add_workflow))
expect(r, assess.BLOCKED, "unauthorized_change", label="workflow file added")
assert any(".github/workflows/evil.yml" in x["detail"] for x in r["reasons"])

def chmod_library(repo):
    (repo / v.LIBRARY).chmod(0o755)
expect(validate(make_pr(tamper=chmod_library)), assess.BLOCKED, "unauthorized_change", label="file mode changed")

expect(validate(make_pr(edit=None)), assess.BLOCKED, "empty_sync", label="sync without generated changes")

r = validate(make_pr(advance_base=True))
expect(r, assess.BLOCKED, "head_not_up_to_date", "unauthorized_change", label="head behind base")

r = validate(safe_pr, head_repo="someone/streamnzb-template")
expect(r, assess.BLOCKED, "fork_head", label="fork PR on chore/vidhin-sync")
assert r["assessment_decision"] is None
expect(validate(safe_pr, author="d4s87", author_type="User"), assess.BLOCKED, "unexpected_author",
       label="human-authored chore/vidhin-sync PR")
expect(validate(safe_pr, author="dependabot[bot]"), assess.BLOCKED, "unexpected_author",
       label="other bot on chore/vidhin-sync")
expect(validate((safe_pr[0], safe_pr[1], "0" * 40)), assess.BLOCKED, "commit_missing",
       label="head commit not fetched")

# ---------------------------------------------------------------- generator mismatch

def edit_library(repo):
    lib = repo / v.LIBRARY
    lib.write_text(lib.read_text(encoding="utf-8").replace("(?i)^(BLURANiUM", "(?i)^(EVIL|BLURANiUM", 1),
                   encoding="utf-8")
expect(validate(make_pr(tamper=edit_library)), assess.BLOCKED, "generator_mismatch", label="hand-edited library")

def edit_report(repo):
    with open(repo / v.REPORT, "a", encoding="utf-8") as fh:
        fh.write("\nLooks SAFE to me.\n")
expect(validate(make_pr(tamper=edit_report)), assess.BLOCKED, "generator_mismatch", label="hand-edited report")

def edit_stamp(repo):
    data = json.loads((repo / v.BASELINE).read_text(encoding="utf-8"))
    data["generated_at_utc"] = "yesterday"
    (repo / v.BASELINE).write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
expect(validate(make_pr(tamper=edit_stamp)), assess.BLOCKED, "generator_mismatch", label="malformed timestamp")

def edit_baseline_token(repo):
    data = json.loads((repo / v.BASELINE).read_text(encoding="utf-8"))
    data["defines"]["Shows Remux T1 Groups"]["tokens"].append("EVIL")
    (repo / v.BASELINE).write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
expect(validate(make_pr(tamper=edit_baseline_token)), assess.BLOCKED, "generator_mismatch",
       label="baseline token without upstream record")

# ---------------------------------------------------------------- stale or modified evidence

expect(validate(safe_pr, live_head="1" * 40), assess.BLOCKED, "stale_pr_head", label="PR head pushed after assessment")
expect(validate(safe_pr, live_base="2" * 40), assess.BLOCKED, "stale_base", label="base moved after assessment")
expect(validate(safe_pr, live_head=""), assess.BLOCKED, "stale_pr_head", label="live PR head unavailable")

def modify_checkout(repo, adir):
    with open(repo / v.LIBRARY, "a", encoding="utf-8") as fh:
        fh.write("Extra [movie]: define if group == \"X\"\n")
expect(validate(safe_pr, after_assess=modify_checkout), assess.BLOCKED, "checkout_modified",
       label="checkout modified after assessment")

def modify_profile(repo, adir):
    (repo / "profile.txt").write_text("SNZBP1:tampered\n", encoding="utf-8")
expect(validate(safe_pr, after_assess=modify_profile), assess.BLOCKED, "checkout_modified",
       label="profile modified after assessment")

def add_untracked(repo, adir):
    (repo / "evil.sh").write_text("#!/bin/sh\n", encoding="utf-8")
expect(validate(safe_pr, after_assess=add_untracked), assess.BLOCKED, "checkout_modified",
       label="untracked file left by the engine run")

def plant_hook(repo, adir):
    hook = repo / ".git/hooks/post-checkout"
    hook.write_text("#!/bin/sh\n", encoding="utf-8")
    hook.chmod(0o755)
r = validate(safe_pr, after_assess=plant_hook)
(safe_pr[0] / ".git/hooks/post-checkout").unlink()  # hooks are outside the work tree the helper resets
expect(r, assess.BLOCKED, "checkout_modified", label="git hook planted during the engine run")
assert any("post-checkout" in x["detail"] for x in r["reasons"]), r["reasons"]

expect(validate(safe_pr, live_base=""), assess.BLOCKED, "stale_base", label="live base unavailable")


def remove(name):
    return lambda repo, adir: (adir / name).unlink()
expect(validate(safe_pr, after_assess=remove("assessment.json")), assess.BLOCKED, "assessment_missing",
       label="missing assessment")
expect(validate(safe_pr, after_assess=remove("compat.log")), assess.BLOCKED, "compat_evidence_missing",
       label="missing compat log")
expect(validate(review_pr, outcome=expected_outcome, after_assess=remove("probe-results.json")), assess.BLOCKED,
       "assessment_contradictory", label="probe results removed after assessment")

def falsify_safe(repo, adir):
    data = json.loads((adir / "assessment.json").read_text(encoding="utf-8"))
    data["decision"] = assess.SAFE
    data["reasons"] = [{"code": "no_semantic_change", "decision": assess.SAFE, "define": None, "detail": "x"}]
    (adir / "assessment.json").write_text(assess.canonical_json(data), encoding="utf-8")
expect(validate(review_pr, outcome=expected_outcome, after_assess=falsify_safe), assess.BLOCKED,
       "assessment_contradictory", label="falsified SAFE assessment")

def falsify_results(repo, adir):
    data = json.loads((adir / "probe-results.json").read_text(encoding="utf-8"))
    for item in data["results"]:
        item["rank"] = 100
    (adir / "probe-results.json").write_text(json.dumps(data), encoding="utf-8")
expect(validate(review_pr, outcome=expected_outcome, after_assess=falsify_results), assess.BLOCKED,
       "assessment_contradictory", label="probe results edited after assessment")

def falsify_old_baseline(repo, adir):
    (adir / "old-baseline.json").write_bytes((repo / v.BASELINE).read_bytes())
expect(validate(safe_pr, after_assess=falsify_old_baseline), assess.BLOCKED, "checkout_modified",
       label="assessment compared against the wrong base")

expect(validate(safe_pr, log=PASSING_LOG.replace(PIN, "f" * 40)), assess.BLOCKED, "engine_revision_mismatch",
       label="different StreamNZB revision")
expect(validate(safe_pr, log=PASSING_LOG + f"==> Using StreamNZB {'e' * 40}\n"), assess.BLOCKED,
       "engine_revision_mismatch", label="two StreamNZB revisions in one log")

def corrupt(path_name, text):
    return lambda repo, adir: (adir / path_name).write_text(text, encoding="utf-8")
expect(validate(safe_pr, after_assess=corrupt("assessment.json", "{not json")), assess.BLOCKED,
       "assessment_missing", label="malformed assessment")

r = validate(safe_pr)
pr_dir = fresh("missing-prepare")
r = v.decide(safe_pr[0], head_ref=v.SYNC_BRANCH, pr_number="7", prepare_path=pr_dir / "prepare.json",
             assessment_dir=pr_dir, live_head_sha=safe_pr[2], live_base_sha=safe_pr[1])
assert r["decision"] == assess.BLOCKED and r["reasons"][0]["code"] == "evidence_missing", r
print("ok - missing prepare evidence: BLOCKED")

# ---------------------------------------------------------------- unrelated PRs

for head_ref, author in (("docs/prepare-release-6-4-7", v.SYNC_AUTHOR), ("fix/some-change", "d4s87"),
                         ("chore/vidhin-sync-extra", v.SYNC_AUTHOR), ("", "")):
    r = validate(make_pr(tamper=change_mapping), head_ref=head_ref, author=author)
    expect(r, v.NOT_APPLICABLE, label=f"unrelated PR {head_ref or '(no ref)'}")
    assert r["_state"] == {**r["_state"], "applicable": False, "reasons": []}

# ---------------------------------------------------------------- CLI, determinism

cli_dir = fresh("cli")
repo, base, head = make_pr(SAFE_EDIT)
outputs = cli_dir / "github_output"
import os  # noqa: E402
os.environ["GITHUB_OUTPUT"] = str(outputs)
common = ["--repo", str(repo)]
assert v.main([*common, "prepare", "--base-sha", base, "--head-sha", head, "--head-ref", v.SYNC_BRANCH,
               "--head-repo", REPO, "--base-repo", REPO, "--author", v.SYNC_AUTHOR, "--author-type", "Bot",
               "--out", str(cli_dir / "prepare.json")]) == 0
assert outputs.read_text() == "run_engine=true\n"
run_assessment(repo, base, cli_dir, same_outcome, "passed", PASSING_LOG)
decide_args = [*common, "decide", "--head-ref", v.SYNC_BRANCH, "--pr-number", "7", "--prepare",
               str(cli_dir / "prepare.json"), "--assessment-dir", str(cli_dir), "--live-head-sha", head,
               "--live-base-sha", base, "--json-out", str(cli_dir / "decision.json"),
               "--summary-out", str(cli_dir / "summary.md")]
assert v.main(decide_args) == 0
first = (cli_dir / "decision.json").read_bytes()
assert v.main(decide_args) == 0
assert (cli_dir / "decision.json").read_bytes() == first, "decision is not deterministic"
decision = json.loads(first)
assert decision["decision"] == assess.SAFE and decision["pr"]["head_sha"] == head and decision["pr"]["base_sha"] == base
assert "**Decision: `SAFE`**" in (cli_dir / "summary.md").read_text()
stale_args = [a if a != head else "1" * 40 for a in decide_args]
assert v.main(stale_args) == 1, "BLOCKED must fail the check"

blocked_out = cli_dir / "blocked_output"
os.environ["GITHUB_OUTPUT"] = str(blocked_out)
assert v.main([*common, "prepare", "--base-sha", base, "--head-sha", head, "--head-ref", v.SYNC_BRANCH,
               "--head-repo", "fork/x", "--base-repo", REPO, "--author", v.SYNC_AUTHOR, "--author-type", "Bot",
               "--out", str(cli_dir / "prepare-fork.json")]) == 0
assert blocked_out.read_text() == "run_engine=false\n", "the engine must not run for a rejected PR"
assert v.main([*common, "decide", "--head-ref", "feat/x", "--prepare", str(cli_dir / "none.json"),
               "--assessment-dir", str(cli_dir), "--json-out", str(cli_dir / "na.json")]) == 0
del os.environ["GITHUB_OUTPUT"]
print("ok - CLI exit codes, outputs and determinism")

# ---------------------------------------------------------------- workflow contract

workflow = (ROOT / ".github/workflows/validate-vidhin-sync-pr.yml").read_text(encoding="utf-8")
assert re.search(r"^on:\n  pull_request:\n", workflow, flags=re.MULTILINE)
assert "pull_request_target" not in workflow and "workflow_run" not in workflow
assert "permissions:\n  contents: read\n  pull-requests: read\n\n" in workflow
assert "secrets." not in workflow, "the validation must not receive secrets"
assert "name: Validate Vidhin sync PR\n    runs-on" in workflow, "stable required-check name"
assert not re.search(r"^    if:", workflow, flags=re.MULTILINE), "a job-level if would report skipped as success"
assert "ref: ${{ github.event.pull_request.base.sha }}" in workflow
assert "persist-credentials: false" in workflow
for value in ("head.ref", "head.sha", "head.repo.full_name", "user.login"):
    uses = re.findall(rf"^.*\$\{{\{{ github\.event\.pull_request\.{re.escape(value)} \}}\}}.*$", workflow, re.MULTILINE)
    assert len(uses) == 1 and re.match(r"^      [A-Z_]+: ", uses[0]), (value, uses)
for step in ("Read live PR head and base", "Decide", "Upload validation evidence"):
    assert re.search(rf"- name: {step}\n(?:        .*\n)*?        if: always\(\)", workflow), step
assert "scripts/validate_vidhin_sync_pr.py prepare" in workflow
assert "./scripts/assess_vidhin_sync.sh \"$BASE_SHA\"" in workflow
assert "STREAMNZB_REF" not in workflow, "the engine revision comes from the trusted base"

assert f'"$HEAD_REF" == "{v.SYNC_BRANCH}"' in workflow
for step in ("Check out trusted base commit", "Set up Python", "Fetch PR head commit",
             "Check identity, scope and generator reproducibility"):
    assert re.search(rf"- name: {step}\n(?:        id: .*\n)?        if: steps\.classify\.outputs\.applicable == 'true'\n", workflow), step

# Run the real Classify block with a fake gh for the live chore/vidhin-sync head.
classify = re.search(r"- name: Classify PR\n(?:        .*\n)*?        run: \|\n((?:          .*\n|\n)+?)\n      - name:",
                     workflow).group(1)
fake_bin = fresh("fake-gh")
(fake_bin / "gh").write_text(
    '#!/usr/bin/env bash\ncase "$FAKE_GH" in\n'
    '  404) echo "gh: Not Found (HTTP 404)" >&2; exit 1;;\n'
    '  error) echo "dial tcp: connection refused" >&2; exit 1;;\n'
    '  *) echo "$FAKE_GH";;\nesac\n', encoding="utf-8")
(fake_bin / "gh").chmod(0o755)


def classify_run(head_ref, fake_gh, head_sha="a" * 40):
    d = fresh("classify")
    env = {**os.environ, "PATH": f"{fake_bin}:{os.environ['PATH']}", "FAKE_GH": fake_gh, "HEAD_REF": head_ref,
           "HEAD_SHA": head_sha, "BASE_REPO": REPO, "RUNNER_TEMP": str(d),
           "GITHUB_OUTPUT": str(d / "out"), "GITHUB_STEP_SUMMARY": str(d / "summary")}
    proc = subprocess.run(["bash", "-e", "-c", classify], env=env, capture_output=True, text=True)
    out = (d / "out").read_text() if (d / "out").exists() else ""
    return proc.returncode, out


assert classify_run(v.SYNC_BRANCH, "error") == (0, "applicable=true\n")
assert classify_run("feat/x", "404") == (0, "applicable=false\n")
assert classify_run("feat/x", "b" * 40) == (0, "applicable=false\n")
assert classify_run("feat/x", "a" * 40)[0] == 1, "a non-sync PR reusing the sync head must fail"
assert classify_run("feat/x", "error")[0] == 1, "an unreadable sync head must fail closed"
print("ok - classify step (sync, unrelated, masking, API failure)")

sync = (ROOT / ".github/workflows/sync-vidhin.yml").read_text(encoding="utf-8")
assert re.search(rf"^\s+branch: {re.escape(v.SYNC_BRANCH)}$", sync, flags=re.MULTILINE)
add_paths = re.search(r"add-paths: \|\n((?:\s{12}\S.*\n)+)", sync).group(1).split()
assert set(add_paths) == set(v.GENERATED), add_paths
assert "Validate Vidhin sync PR" not in sync and "validate_vidhin_sync_pr" not in sync, "sync job is unchanged"
assert "python tests/test_validate_vidhin_sync_pr.py" in (ROOT / ".github/workflows/validate.yml").read_text()
print("ok - workflow contract")

print("PASS: Vidhin sync PR validation")
