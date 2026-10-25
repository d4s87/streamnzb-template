#!/usr/bin/env python3
"""
Regression tests for scripts/assess_vidhin_sync.py.

Real historical sync commits (SumVision, monkee/CMRG, HHWEB) must never be
SAFE; synthetic cases cover every decision class. Engine results here are
synthetic so the decision logic is tested in isolation; real-engine evidence
comes from tests/streamnzb_compat/vidhin_sync_probe_test.go.

Needs full git history for the historical fixtures (validate.yml checks out
with fetch-depth: 0).
"""

import copy
import hashlib
import importlib.util
import json
import re
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("assess", ROOT / "scripts/assess_vidhin_sync.py")
assess = importlib.util.module_from_spec(spec)
spec.loader.exec_module(assess)
sv = assess.sync_vidhin

MAPPING = json.loads((ROOT / "scripts/vidhin_mapping.json").read_text(encoding="utf-8"))
BASELINE = json.loads((ROOT / "generated/vidhin-defines.json").read_text(encoding="utf-8"))
LIBRARY = (ROOT / "generated/streamnzb-defines.txt").read_text(encoding="utf-8")

# (label, merge commit) of syncs that changed scoring/filtering behavior.
HISTORICAL = {
    "SumVision generated HDR (PR #80)": "4d1ddb0",
    "monkee/CMRG WEB T2 (PR #81)": "2f72419",
    "HHWEB removal + 126811 LQ (PR #65)": "2b625cd",
}

TMP = Path(tempfile.mkdtemp(prefix="vidhin-assessment-test-"))
counter = 0


def git_show(rev, path):
    return subprocess.run(
        ["git", "-C", str(ROOT), "show", f"{rev}:{path}"], check=True, capture_output=True, text=True
    ).stdout


def sha(data):
    return hashlib.sha256(data).hexdigest()


def same_outcome(probe, library):
    return {"found": True, "fetch": True, "rank": 100, "matched": {}, "rejections": []}


def expected_outcome(probe, library):
    out = same_outcome(probe, library)
    if library == "new" and probe["expect_change"]:
        out["rank"] = 400
    return out


def write_results(plan_path, outcome, *, stale=False, drop=0):
    plan_bytes = plan_path.read_bytes()
    plan = json.loads(plan_bytes)
    results = [
        {"id": p["id"], "variant": v, "library": lib, **outcome(p, lib)}
        for p in plan["probes"]
        for v in plan["variants"]
        for lib in ("old", "new")
    ]
    path = plan_path.with_name("results.json")
    path.write_text(
        json.dumps(
            {
                "schema_version": 1,
                "plan_sha256": "0" * 64 if stale else sha(plan_bytes),
                "old_library_sha256": plan["old_library_sha256"],
                "new_library_sha256": plan["new_library_sha256"],
                "results": results[drop:],
            }
        ),
        encoding="utf-8",
    )
    return path


def run(old_base, old_lib, new_base, new_lib, mapping=MAPPING, *, compat="passed", outcome=same_outcome,
        engine=True, **result_kwargs):
    global counter
    counter += 1
    d = TMP / str(counter)
    d.mkdir()
    files = {
        "old-b.json": json.dumps(old_base) if isinstance(old_base, dict) else old_base,
        "new-b.json": json.dumps(new_base) if isinstance(new_base, dict) else new_base,
        "old-l.txt": old_lib,
        "new-l.txt": new_lib,
        "map.json": json.dumps(mapping) if isinstance(mapping, dict) else mapping,
    }
    for name, text in files.items():
        (d / name).write_text(text, encoding="utf-8")
    args = [
        "--old-baseline", str(d / "old-b.json"), "--new-baseline", str(d / "new-b.json"),
        "--old-library", str(d / "old-l.txt"), "--new-library", str(d / "new-l.txt"),
        "--mapping", str(d / "map.json"), "--plan", str(d / "plan.json"),
    ]
    assert assess.main(["plan", *args]) == 0
    extra = []
    if engine:
        extra = ["--engine-results", str(write_results(d / "plan.json", outcome, **result_kwargs))]
    assert assess.main(["assess", *args, "--compat-result", compat, *extra,
                        "--json-out", str(d / "a.json"), "--markdown-out", str(d / "a.md")]) == 0
    result = json.loads((d / "a.json").read_text(encoding="utf-8"))
    result["_markdown"] = (d / "a.md").read_text(encoding="utf-8")
    result["_codes"] = {r["code"] for r in result["reasons"]}
    return result


def expect(result, decision, *codes, label):
    assert result["decision"] == decision, (label, result["decision"], sorted(result["_codes"]))
    missing = set(codes) - result["_codes"]
    assert not missing, (label, f"missing reason codes {sorted(missing)}", sorted(result["_codes"]))
    print(f"ok - {label}: {decision}")


def with_defines(defines, base=BASELINE):
    out = copy.deepcopy(base)
    out["defines"] = defines
    return out


def rendered(defines, mapping=MAPPING):
    return sv.render(defines, mapping)


# Every reason code maps to a known decision.
for code, decision in assess.REASONS.items():
    assert decision in assess.DECISION_RANK, code

# ---------------------------------------------------------------- historical

for label, commit in HISTORICAL.items():
    old_base = json.loads(git_show(f"{commit}^1", "generated/vidhin-defines.json"))
    new_base = json.loads(git_show(commit, "generated/vidhin-defines.json"))
    old_lib = git_show(f"{commit}^1", "generated/streamnzb-defines.txt")
    new_lib = git_show(commit, "generated/streamnzb-defines.txt")
    mapping = json.loads(git_show(commit, "scripts/vidhin_mapping.json"))

    # Even if the engine showed no observable change at all, never SAFE.
    for outcome in (same_outcome, expected_outcome):
        r = run(old_base, old_lib, new_base, new_lib, mapping, outcome=outcome)
        assert r["decision"] == assess.REVIEW, (label, outcome.__name__, r["decision"], sorted(r["_codes"]))
    r = run(old_base, old_lib, new_base, new_lib, mapping, compat="not_run", engine=False)
    expect(r, assess.BLOCKED, "real_engine_regressions_not_run", "engine_evidence_missing",
           label=f"{label} without evidence")

    r = run(old_base, old_lib, new_base, new_lib, mapping, outcome=expected_outcome)
    if commit == "4d1ddb0":
        expect(r, assess.REVIEW, "generated_hdr_group_changed", "indirect_policy_effect", label=label)
        assert any(e["rule"] == "Generated Dynamic HDR Penalty" for e in r["effects"]), label
        assert any("SumVision" in rr["detail"] for rr in r["reasons"]), label
    elif commit == "2f72419":
        expect(r, assess.REVIEW, "tier_group_added", "trusted_group_added", "upstream_records_regrouped",
               "indirect_policy_effect", label=label)
        added = {(x["scope"], x["token"]) for x in r["changes"]["trusted"]["added"]}
        assert added == {("movie", "monkee"), ("series", "CMRG")}, added
        rules = {e["rule"] for e in r["effects"]}
        for rule in ("Movies WEB T2", "Shows WEB T2", "Reject suspicious 4K upscale", "Adaptive HD x265",
                     "Unknown resolution"):
            assert rule in rules, (label, rule)
        assert "4K-upscale exemption" in r["_markdown"] and "HD x265 exemption" in r["_markdown"]
        plan = json.loads((TMP / str(counter) / "plan.json").read_text(encoding="utf-8"))
        scenarios = {p["scenario"] for p in plan["probes"] if p["token"] == "monkee"}
        assert {"single", "cross-kind", "near-miss", "4k-upscale", "hd-x265"} <= scenarios, scenarios
    else:
        expect(r, assess.REVIEW, "tier_group_removed", "trusted_group_removed", "classification_group_added",
               label=label)
        assert any(e["rule"] == "Adaptive Low-Score Filtering" for e in r["effects"]), label

# ---------------------------------------------------------------- SAFE

defines = BASELINE["defines"]

r = run(BASELINE, LIBRARY, BASELINE, LIBRARY)
expect(r, assess.SAFE, "no_semantic_change", label="identical sync")
assert "<!-- vidhin-sync-assessment " in r["_markdown"]

stamped = copy.deepcopy(BASELINE)
stamped["generated_at_utc"] = "2099-01-01T00:00:00+00:00"
expect(run(BASELINE, LIBRARY, stamped, LIBRARY), assess.SAFE, "metadata_only", label="timestamp only")

pattern_only = copy.deepcopy(defines)
record = pattern_only["Movies WEB T1 Groups"]["records"][0]
record["pattern"] = record["pattern"].replace("/i", "/i ", 1)
r = run(BASELINE, LIBRARY, with_defines(pattern_only), rendered(pattern_only))
expect(r, assess.SAFE, "upstream_pattern_text_only", label="upstream regex text only")
assert r["evidence"]["probes_planned"] > 0 and not r["evidence"]["deltas"]

# ---------------------------------------------------------------- REVIEW_REQUIRED

added = copy.deepcopy(defines)
added["Movies WEB T3 Groups"]["records"][0]["tokens"].append("NEWGRP")
added["Movies WEB T3 Groups"]["tokens"] = sv.dedupe_casefold(added["Movies WEB T3 Groups"]["tokens"] + ["NEWGRP"])
r = run(BASELINE, LIBRARY, with_defines(added), rendered(added), outcome=expected_outcome)
expect(r, assess.REVIEW, "tier_group_added", "trusted_group_added", label="new trusted tier group")

lq = copy.deepcopy(defines)
lq["Movies LQ Groups"]["records"][0]["tokens"].append("NEWLQ")
lq["Movies LQ Groups"]["tokens"] = sorted(lq["Movies LQ Groups"]["tokens"] + ["NEWLQ"], key=str.casefold)
r = run(BASELINE, LIBRARY, with_defines(lq), rendered(lq), outcome=expected_outcome)
expect(r, assess.REVIEW, "classification_group_added", label="new LQ group")

promoted = copy.deepcopy(defines)
t2 = promoted["Movies WEB T2 Groups"]["tokens"][0]
promoted["Movies WEB T1 Groups"]["records"][0]["tokens"].append(t2)
promoted["Movies WEB T1 Groups"]["tokens"] = sv.dedupe_casefold(promoted["Movies WEB T1 Groups"]["tokens"] + [t2])
r = run(BASELINE, LIBRARY, with_defines(promoted), rendered(promoted), outcome=expected_outcome)
expect(r, assess.REVIEW, "tier_group_added", "tier_precedence_changed", label="WEB T2 group promoted to T1")
assert "unexplained_library_change" not in r["_codes"]

scoped_map = copy.deepcopy(MAPPING)
scoped_map["targets"]["Atmos Exclude Groups"]["scope"] = "movie"
scoped = copy.deepcopy(defines)
scoped["Atmos Exclude Groups"]["scope"] = "movie"
r = run(BASELINE, LIBRARY, with_defines(scoped), rendered(scoped, scoped_map), scoped_map)
expect(r, assess.REVIEW, "content_kind_scope_changed", label="content-kind scope change")

regex = copy.deepcopy(defines)
regex["Retag Markers"]["records"][0]["raw_regex_pattern"] += "|NEWMARKER"
r = run(BASELINE, LIBRARY, with_defines(regex), rendered(regex))
expect(r, assess.REVIEW, "regex_or_source_gate_changed", label="raw regex change")

gate = copy.deepcopy(defines)
gate["Anime Shows BluRay T1 Groups"]["records"][0]["case_insensitive"] = not gate[
    "Anime Shows BluRay T1 Groups"]["records"][0]["case_insensitive"]
r = run(BASELINE, LIBRARY, with_defines(gate), rendered(gate))
expect(r, assess.REVIEW, "regex_or_source_gate_changed", label="Anime BD source-gate case flag")

# ---------------------------------------------------------------- BLOCKED

removed = copy.deepcopy(defines)
del removed["Atmos Exclude Groups"]
expect(run(BASELINE, LIBRARY, with_defines(removed), LIBRARY), assess.BLOCKED, "define_set_changed",
       label="Define removed")

tampered = LIBRARY.replace("(?i)^(", "(?i)^(EVIL|", 1)
expect(run(BASELINE, LIBRARY, BASELINE, tampered), assess.BLOCKED, "library_not_reproducible",
       label="hand-edited library")

expect(run(BASELINE, LIBRARY, BASELINE, LIBRARY + "not a define line\n"), assess.BLOCKED, "library_unparseable",
       label="unparseable library line")

mode = copy.deepcopy(defines)
mode["Atmos Exclude Groups"]["mode"] = "mystery"
expect(run(BASELINE, LIBRARY, with_defines(mode), LIBRARY), assess.BLOCKED, "unknown_mode", "mapping_mismatch",
       label="unknown generator mode")

extra = copy.deepcopy(BASELINE)
extra["surprise"] = True
expect(run(BASELINE, LIBRARY, extra, LIBRARY), assess.BLOCKED, "unexpected_baseline_field",
       label="unexpected baseline field")

emptied = copy.deepcopy(defines)
emptied["Shows Remux T2 Groups"]["tokens"] = []
for rec in emptied["Shows Remux T2 Groups"]["records"]:
    rec["tokens"] = []
expect(run(BASELINE, LIBRARY, with_defines(emptied), rendered(emptied)), assess.BLOCKED, "define_emptied",
       label="tier Define emptied")

listed = copy.deepcopy(BASELINE)
listed["defines"] = []
expect(run(listed, LIBRARY, listed, LIBRARY, engine=False), assess.BLOCKED, "input_unreadable",
       label="non-object defines")

expect(run(BASELINE, LIBRARY, BASELINE, LIBRARY, compat="failed"), assess.BLOCKED,
       "real_engine_regressions_failed", label="failing real-engine regressions")

pattern_args = (BASELINE, LIBRARY, with_defines(pattern_only), rendered(pattern_only))
expect(run(*pattern_args, engine=False), assess.BLOCKED, "engine_evidence_missing", label="probes without results")
expect(run(*pattern_args, stale=True), assess.BLOCKED, "engine_evidence_stale", label="stale probe results")
expect(run(*pattern_args, drop=1), assess.BLOCKED, "engine_evidence_incomplete", label="incomplete probe results")


def always_changes(probe, library):
    out = same_outcome(probe, library)
    out["rank"] = 100 if library == "old" else 999
    return out


expect(run(*pattern_args, outcome=always_changes), assess.BLOCKED, "unexpected_behavior_delta",
       label="behavior delta behind a byte-identical Define")

# ---------------------------------------------------------------- workflow guard

# The post-assessment integrity guard must tolerate the sync job's own log and
# generated-file edits, and nothing else.
workflow = (ROOT / ".github/workflows/sync-vidhin.yml").read_text(encoding="utf-8")
guard = re.search(r"git status --porcelain --untracked-files=all \| grep -v -E '([^']+)'", workflow)
assert guard, "integrity guard pattern not found in sync-vidhin.yml"
status = [
    " M generated/vidhin-defines.json", " M generated/vidhin-sync-report.md",
    " M generated/streamnzb-defines.txt", "?? vidhin-sync.log",
    " M profile.txt", "?? evil.sh", "?? generated/vidhin-sync.log", " M vidhin-sync.log",
]
left = subprocess.run(["grep", "-v", "-E", guard.group(1)], input="\n".join(status) + "\n",
                      capture_output=True, text=True).stdout.splitlines()
assert left == status[4:], left
print("ok - integrity guard allows only generated edits and vidhin-sync.log")

# ---------------------------------------------------------------- determinism

first = run(BASELINE, LIBRARY, with_defines(added), rendered(added), outcome=expected_outcome)
second = run(BASELINE, LIBRARY, with_defines(added), rendered(added), outcome=expected_outcome)
for r in (first, second):
    r.pop("_markdown")
assert first == second, "assessment is not deterministic"
print("ok - deterministic output")

print("PASS: Vidhin sync assessment")
