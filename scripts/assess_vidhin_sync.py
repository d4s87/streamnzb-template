#!/usr/bin/env python3
"""
Advisory semantic assessment of a Vidhin synchronization change.

Compares the accepted (old) and proposed (new) Vidhin baseline and Define
Library, maps each change to the published-profile rules that consume it,
and emits a deterministic SAFE / REVIEW_REQUIRED / BLOCKED decision with
machine-readable reasons. Policy: docs/VIDHIN_SYNC_ASSESSMENT.md.

Two subcommands share the same diff:

  plan    writes the real-engine probe plan consumed by
          tests/streamnzb_compat/vidhin_sync_probe_test.go
  assess  combines the diff with compatibility-suite and probe results

This tool is advisory. It never merges, approves or labels anything.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import sync_vidhin  # noqa: E402
from build_profiles import decode_share_code  # noqa: E402

ASSESSMENT_SCHEMA = 1
PLAN_SCHEMA = 1

SAFE = "SAFE"
REVIEW = "REVIEW_REQUIRED"
BLOCKED = "BLOCKED"
DECISION_RANK = {SAFE: 0, REVIEW: 1, BLOCKED: 2}

# Every reason code and the decision it forces. A code missing from this
# table is a programming error and is treated as BLOCKED.
REASONS = {
    # Allowlisted, behavior-preserving.
    "no_semantic_change": SAFE,
    "metadata_only": SAFE,
    "upstream_pattern_text_only": SAFE,
    "upstream_records_regrouped": SAFE,
    # Valid but behavior-changing: a human must review.
    "tier_group_added": REVIEW,
    "tier_group_removed": REVIEW,
    "tier_movement": REVIEW,
    "tier_precedence_changed": REVIEW,
    "classification_group_added": REVIEW,
    "classification_group_removed": REVIEW,
    "generated_hdr_group_changed": REVIEW,
    "trusted_group_added": REVIEW,
    "trusted_group_removed": REVIEW,
    "content_kind_scope_changed": REVIEW,
    "define_metadata_changed": REVIEW,
    "regex_or_source_gate_changed": REVIEW,
    "indirect_policy_effect": REVIEW,
    "expected_change_not_observed": REVIEW,
    # Invalid output, missing evidence or unexpected structure.
    "input_unreadable": BLOCKED,
    "baseline_schema_changed": BLOCKED,
    "unexpected_baseline_field": BLOCKED,
    "define_set_changed": BLOCKED,
    "mapping_mismatch": BLOCKED,
    "unknown_mode": BLOCKED,
    "unexpected_define_field": BLOCKED,
    "define_emptied": BLOCKED,
    "library_unparseable": BLOCKED,
    "library_define_set_mismatch": BLOCKED,
    "library_scope_mismatch": BLOCKED,
    "trusted_define_mismatch": BLOCKED,
    "library_not_reproducible": BLOCKED,
    "unexplained_library_change": BLOCKED,
    "consumer_define_missing": BLOCKED,
    "real_engine_regressions_failed": BLOCKED,
    "real_engine_regressions_not_run": BLOCKED,
    "engine_evidence_missing": BLOCKED,
    "engine_evidence_stale": BLOCKED,
    "engine_evidence_incomplete": BLOCKED,
    "unexpected_behavior_delta": BLOCKED,
}

BASELINE_KEYS = {
    "schema_version",
    "mapping_schema_version",
    "upstream_url",
    "generated_at_utc",
    "defines",
}
BASELINE_IDENTITY_KEYS = ("schema_version", "mapping_schema_version", "upstream_url")
KNOWN_MODES = {
    "standard",
    "lq",
    "lq_release_title",
    "raw_release_name",
    "dubs_only",
    "raw_regex",
    "obfuscated",
    "anime_bd",
    "generated_dynamic_hdr_groups",
}
ENTRY_KEYS = {
    "sources",
    "scope",
    "field",
    "mode",
    "records",
    "tokens",
    "tier_family",
    "tier_report_family",
    "tier",
    "extra_exact_groups",
    "release_name_fallbacks",
}
RECORD_KEYS = {
    "source",
    "pattern",
    "tokens",
    "case_sensitive",
    "case_insensitive",
    "conditions",
    "group_tokens",
    "raw_release_name_pattern",
    "raw_regex_pattern",
    "obfuscated_regex_pattern",
    "lq_release_title",
}
METADATA_FIELDS = (
    "sources",
    "scope",
    "field",
    "mode",
    "tier_family",
    "tier_report_family",
    "tier",
    "extra_exact_groups",
    "release_name_fallbacks",
)
# Modes whose rendered Define depends only on the union of record tokens.
TOKEN_UNION_MODES = {"standard", "generated_dynamic_hdr_groups"}
TOKENLESS_MODES = {"raw_release_name", "dubs_only", "raw_regex", "obfuscated", "lq_release_title"}

TRUSTED = sync_vidhin.TRUSTED_RELEASE_GROUPS_DEFINE
DEFAULT_PROFILES = (
    ("samsung", ROOT / "profile.txt"),
    ("neutral", ROOT / "profile-neutral.txt"),
)

# Readable names for the indirect effects reviewers asked to see called out.
EFFECT_LABELS = {
    "Reject suspicious 4K upscale": "4K-upscale exemption",
    "Adaptive HD x265": "HD x265 exemption",
    "Unknown resolution": "unknown-resolution exemption",
    "Anime Unknown Resolution": "Anime unknown-resolution exemption",
    "Reject bad 4K BluRay": "4K BluRay exemption",
    "Reject bad 4K Anime": "4K Anime exemption",
    "Generated Dynamic HDR Penalty": "generated HDR/DV penalty",
    "Adaptive Low-Score Filtering": "low-score pruning",
}

MATCHED_RE = re.compile(r'matched\("([^"]+)"\)')
PROBE_TOKEN_RE = re.compile(r"^[A-Za-z0-9]+$")
SAFE_PROBE_SAMPLE = 3

KIND_REQUESTS = {
    "movie": {"kind": "movie", "title": "Example Movie", "stem": "Example.Movie.2020"},
    "series": {"kind": "series", "title": "Example Show", "season": 1, "episode": 1, "stem": "Example.Show.S01E01"},
    "anime_movie": {"kind": "anime_movie", "title": "Example Anime Movie", "anime": True, "stem": "Example.Anime.Movie.2020"},
    "anime_show": {"kind": "anime_show", "title": "Example Anime", "anime": True, "season": 1, "episode": 1, "stem": "Example.Anime.S01E01"},
}
SIBLING_KIND = {"movie": "series", "series": "movie", "anime_movie": "anime_show", "anime_show": "anime_movie"}
SCOPE_KINDS = {
    None: ("movie", "series", "anime_show"),
    "movie": ("movie",),
    "series": ("series",),
    "anime_movie": ("anime_movie",),
    "anime_show": ("anime_show",),
}
# Release-name tails per tier family: 1080p single, 2160p upscale, 1080p x265.
FAMILY_TAILS = {
    "WEB": ("1080p.AMZN.WEB-DL.DDP5.1.H264", "2160p.WEB-DL.DDP5.1.H265", "1080p.WEB-DL.DDP5.1.x265"),
    "UHD BluRay": ("2160p.UHD.BluRay.DTS-HD.MA.5.1.HDR.x265", "2160p.UHD.BluRay.DTS-HD.MA.5.1.HDR.x265", "1080p.BluRay.DTS.5.1.x265"),
    "HD BluRay": ("1080p.BluRay.DTS.5.1.x264", "2160p.UHD.BluRay.DTS-HD.MA.5.1.x265", "1080p.BluRay.DTS.5.1.x265"),
    "BluRay": ("1080p.BluRay.DTS.5.1.x264", "2160p.UHD.BluRay.DTS-HD.MA.5.1.x265", "1080p.BluRay.DTS.5.1.x265"),
    "Remux": ("1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1", "2160p.UHD.BluRay.DTS-HD.MA.5.1.x265", "1080p.BluRay.DTS.5.1.x265"),
}
GENERIC_TAIL = "1080p.AMZN.WEB-DL.DDP5.1.H264"
# Extra single-candidate tails for classifications gated on other facts.
DEFINE_EXTRA_TAILS = {
    "Movies Generated Dynamic HDR Groups": (
        "2160p.UHD.BluRay.REMUX.DV.HDR.HEVC",
        "2160p.WEB-DL.DDP5.1.HDR10Plus.H265",
        "2160p.UHD.BluRay.REMUX.HDR10.HEVC",
    ),
    "Atmos Exclude Groups": ("1080p.BluRay.REMUX.AVC.TrueHD.7.1.Atmos",),
    "TrueHD Exclude Groups": ("1080p.BluRay.REMUX.AVC.TrueHD.7.1",),
}
UPSCALE_ANCHOR_TAIL = "1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1"
X265_PEER_TAIL = "1080p.WEB-DL.DDP5.1.H264"
X265_PEERS = 7


# --------------------------------------------------------------------------
# Inputs


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical_json(value) -> str:
    return json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n"


def parse_library(text: str):
    """Parse 'Name [scope]: define if cond' lines exactly as the Go harness does."""
    out = {}
    errors = []
    for number, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        label, sep, cond = line.partition(": define if ")
        label = label.strip()
        cond = cond.strip()
        if not sep or not label or not cond:
            errors.append(f"line {number}: {raw[:120]!r}")
            continue
        name, scope = label, None
        if label.endswith("]"):
            name, _, rest = label.rpartition(" [")
            scope = rest[:-1].strip()
            name = name.strip()
            if not name:
                errors.append(f"line {number}: invalid scope label")
                continue
        if name in out:
            errors.append(f"line {number}: duplicate Define {name!r}")
            continue
        out[name] = {"scope": scope, "when": cond}
    if not out and not errors:
        errors.append("library contains no Defines")
    return out, errors


def profile_consumers(profiles):
    """Map Define name -> [(variant, rule, action, points)] from published profiles."""
    consumers = {}
    for variant, payload in profiles.items():
        for rule in payload.get("rules", []):
            for define in sorted(set(MATCHED_RE.findall(rule.get("when", "")))):
                action = rule.get("action") or "score"
                consumers.setdefault(define, []).append(
                    {
                        "variant": variant,
                        "rule": rule.get("name"),
                        "action": action,
                        "points": rule.get("points") if action == "score" else None,
                    }
                )
    return consumers


def tokens_of(entry) -> set[str]:
    return set(entry.get("tokens", []))


def fold(tokens) -> dict[str, str]:
    return {t.casefold(): t for t in sorted(tokens)}


def records_without_pattern(entry):
    return [{k: v for k, v in r.items() if k != "pattern"} for r in entry.get("records", [])]


# --------------------------------------------------------------------------
# Semantic diff


class Assessment:
    def __init__(self):
        self.reasons = []

    def add(self, code, detail, define=None):
        self.reasons.append(
            {
                "code": code,
                "decision": REASONS.get(code, BLOCKED),
                "define": define,
                "detail": detail,
            }
        )

    def decision(self):
        if not self.reasons:
            return BLOCKED
        return max((r["decision"] for r in self.reasons), key=DECISION_RANK.get)

    def sorted_reasons(self):
        return sorted(
            self.reasons,
            key=lambda r: (-DECISION_RANK[r["decision"]], r["code"], r["define"] or "", r["detail"]),
        )


def is_tier(name):
    return sync_vidhin.is_trusted_release_group_name(name)


def tier_family(name):
    core = name[: -len(" Groups")]
    for prefix in ("Anime Movies ", "Anime Shows ", "Movies ", "Shows "):
        if core.startswith(prefix):
            rest = core[len(prefix):]
            for family in ("UHD BluRay", "HD BluRay", "BluRay", "WEB", "Remux"):
                if rest.startswith(family + " T"):
                    return family
    return None


def trusted_membership(defines):
    """(scope, casefolded token) for every tier-classified group."""
    out = {}
    for name, entry in defines.items():
        if is_tier(name):
            for token in tokens_of(entry):
                out.setdefault((entry.get("scope"), token.casefold()), token)
    return out


def expected_entry_metadata(cfg):
    want = {
        "sources": cfg["sources"],
        "scope": cfg["scope"],
        "field": cfg["field"],
        "mode": cfg.get("mode", "standard"),
    }
    for field in ("tier_family", "tier_report_family", "tier"):
        if field in cfg:
            want[field] = cfg[field]
    if want["mode"] == "lq":
        want["extra_exact_groups"] = cfg.get("extra_exact_groups", [])
        want["release_name_fallbacks"] = cfg.get("release_name_fallbacks", [])
    return want


def diff(inputs):
    """Return (Assessment, changes) for old/new baseline + library inputs."""
    a = Assessment()
    old_base, new_base = inputs["old_baseline"], inputs["new_baseline"]
    old_lib_text, new_lib_text = inputs["old_library"], inputs["new_library"]
    mapping, profiles = inputs["mapping"], inputs["profiles"]
    changes = {
        "defines": [],
        "trusted": {"added": [], "removed": []},
        "tier_movements": [],
        "effects": [],
        "token_changes": {},
        "condition_changes": [],
        "safe_candidates": [],
    }

    for label, base in (("old", old_base), ("new", new_base)):
        extra = sorted(set(base) - BASELINE_KEYS)
        if extra:
            a.add("unexpected_baseline_field", f"{label} baseline has {extra}")
        if not isinstance(base.get("defines"), dict):
            a.add("input_unreadable", f"{label} baseline has no defines object")
            return a, changes
    for key in BASELINE_IDENTITY_KEYS:
        if old_base.get(key) != new_base.get(key):
            a.add("baseline_schema_changed", f"{key}: {old_base.get(key)!r} -> {new_base.get(key)!r}")
    if mapping.get("schema_version") != new_base.get("mapping_schema_version"):
        a.add("mapping_mismatch", "baseline mapping_schema_version does not match the mapping")
    if mapping.get("upstream_url") != new_base.get("upstream_url"):
        a.add("mapping_mismatch", "baseline upstream_url does not match the mapping")
    metadata_only = old_base.get("generated_at_utc") != new_base.get("generated_at_utc")

    old, new = old_base["defines"], new_base["defines"]
    targets = sync_vidhin.target_cfgs(mapping)
    if set(old) != set(new):
        a.add(
            "define_set_changed",
            f"added {sorted(set(new) - set(old))}, removed {sorted(set(old) - set(new))}",
        )
    if set(new) != set(targets):
        a.add("mapping_mismatch", "baseline Define names differ from mapping targets")

    for name in sorted(new):
        entry = new[name]
        extra = sorted(set(entry) - ENTRY_KEYS)
        if extra:
            a.add("unexpected_define_field", f"entry fields {extra}", name)
        rec_extra = sorted({k for r in entry.get("records", []) for k in r} - RECORD_KEYS)
        if rec_extra:
            a.add("unexpected_define_field", f"record fields {rec_extra}", name)
        if entry.get("mode") not in KNOWN_MODES:
            a.add("unknown_mode", f"mode {entry.get('mode')!r}", name)
        if name in targets:
            want = expected_entry_metadata(targets[name])
            got = {k: entry.get(k) for k in want}
            if got != want or any(k in entry for k in set(METADATA_FIELDS) - set(want)):
                a.add("mapping_mismatch", "generated metadata differs from mapping target", name)
        if entry.get("mode") not in TOKENLESS_MODES and not entry.get("tokens"):
            a.add("define_emptied", "no release-group tokens", name)
    if set(old) != set(targets) or set(new) != set(targets):
        return a, changes  # every later step assumes the mapped Define set

    old_lib, old_errors = parse_library(old_lib_text)
    new_lib, new_errors = parse_library(new_lib_text)
    for label, errors in (("old", old_errors), ("new", new_errors)):
        for error in errors:
            a.add("library_unparseable", f"{label} library {error}")
    if old_errors or new_errors:
        return a, changes

    for label, lib, defines in (("old", old_lib, old), ("new", new_lib, new)):
        if set(lib) != set(defines) | {TRUSTED}:
            a.add(
                "library_define_set_mismatch",
                f"{label} library vs baseline: extra {sorted(set(lib) - set(defines) - {TRUSTED})}, "
                f"missing {sorted((set(defines) | {TRUSTED}) - set(lib))}",
            )
            continue
        for name, entry in defines.items():
            if lib[name]["scope"] != entry.get("scope"):
                a.add("library_scope_mismatch", f"{label} library scope {lib[name]['scope']!r}", name)
        want_trusted = " or ".join(f'matched("{n}")' for n in sorted(n for n in defines if is_tier(n)))
        if lib[TRUSTED]["when"] != want_trusted or lib[TRUSTED]["scope"] is not None:
            a.add("trusted_define_mismatch", f"{label} {TRUSTED} is not the OR of every tier Define")

    for label, defines, text in (("old", old, old_lib_text), ("new", new, new_lib_text)):
        try:
            rendered = sync_vidhin.render(defines, mapping)
        except Exception as exc:  # malformed baseline: report, never crash
            a.add("library_not_reproducible", f"{label} baseline cannot be rendered: {exc}")
            continue
        if rendered != text:
            a.add("library_not_reproducible", f"{label} library does not match its baseline")

    consumers = profile_consumers(profiles)
    for define in sorted(consumers):
        if define not in new_lib:
            rules = sorted({c["rule"] for c in consumers[define]})
            a.add("consumer_define_missing", f"referenced by {rules}", define)

    movements = sync_vidhin.detect_tier_movements(old, new, mapping)
    for m in movements:
        changes["tier_movements"].append(m)
        a.add("tier_movement", f'{m["family"]}: {m["token"]} T{m["old_tier"]} -> T{m["new_tier"]}')

    old_trusted, new_trusted = trusted_membership(old), trusted_membership(new)
    for key in sorted(set(new_trusted) - set(old_trusted), key=lambda k: (str(k[0]), k[1])):
        changes["trusted"]["added"].append({"scope": key[0], "token": new_trusted[key]})
        a.add("trusted_group_added", f"{new_trusted[key]} [{key[0]}]")
    for key in sorted(set(old_trusted) - set(new_trusted), key=lambda k: (str(k[0]), k[1])):
        changes["trusted"]["removed"].append({"scope": key[0], "token": old_trusted[key]})
        a.add("trusted_group_removed", f"{old_trusted[key]} [{key[0]}]")
    trusted_changed = bool(changes["trusted"]["added"] or changes["trusted"]["removed"])

    old_eff = sync_vidhin.apply_web_precedence(old, mapping)
    new_eff = sync_vidhin.apply_web_precedence(new, mapping)

    any_change = False
    for name in sorted(set(old) & set(new)):
        o, n = old[name], new[name]
        line_changed = old_lib[name] != new_lib[name]
        added = sorted(tokens_of(n) - tokens_of(o), key=str.casefold)
        removed = sorted(tokens_of(o) - tokens_of(n), key=str.casefold)
        eff_o, eff_n = set(old_eff[name]["effective_tokens"]), set(new_eff[name]["effective_tokens"])
        eff_added = sorted(eff_n - eff_o, key=str.casefold)
        eff_removed = sorted(eff_o - eff_n, key=str.casefold)
        meta = [(f, o.get(f), n.get(f)) for f in METADATA_FIELDS if o.get(f) != n.get(f)]
        regrouped = False
        if o.get("mode") in TOKEN_UNION_MODES and n.get("mode") in TOKEN_UNION_MODES:
            recs = o.get("records", []) + n.get("records", [])
            structure_changed = (
                {r.get("source") for r in o.get("records", [])} != {r.get("source") for r in n.get("records", [])}
                or any(set(r) - {"source", "pattern", "tokens"} for r in recs)
            )
            regrouped = [r.get("tokens") for r in o.get("records", [])] != [
                r.get("tokens") for r in n.get("records", [])
            ]
        else:
            structure_changed = records_without_pattern(o) != records_without_pattern(n)
        pattern_changed = [r.get("pattern") for r in o.get("records", [])] != [
            r.get("pattern") for r in n.get("records", [])
        ]
        if not (line_changed or added or removed or eff_added or eff_removed or meta
                or structure_changed or pattern_changed or regrouped):
            continue
        any_change = True
        record = {
            "define": name,
            "scope": n.get("scope"),
            "mode": n.get("mode"),
            "added": added,
            "removed": removed,
            "metadata": [{"field": f, "old": ov, "new": nv} for f, ov, nv in meta],
            "library_condition_changed": line_changed,
            "classification": "unchanged",
        }

        for field, ov, nv in meta:
            code = "content_kind_scope_changed" if field == "scope" else "define_metadata_changed"
            a.add(code, f"{field}: {json.dumps(ov)} -> {json.dumps(nv)}", name)

        if added or removed:
            record["classification"] = "tokens"
            if n.get("mode") == "generated_dynamic_hdr_groups":
                for t in added:
                    a.add("generated_hdr_group_changed", f"+{t}", name)
                for t in removed:
                    a.add("generated_hdr_group_changed", f"-{t}", name)
            else:
                prefix = "tier_group" if is_tier(name) else "classification_group"
                for t in added:
                    a.add(f"{prefix}_added", f"+{t}", name)
                for t in removed:
                    a.add(f"{prefix}_removed", f"-{t}", name)
            changes["token_changes"][name] = {"added": added, "removed": removed}
        elif eff_added or eff_removed:
            # A token moved into a higher tier of the same WEB family.
            record["classification"] = "precedence"
            record["added"], record["removed"] = eff_added, eff_removed
            detail = ", ".join([f"+{t}" for t in eff_added] + [f"-{t}" for t in eff_removed])
            a.add("tier_precedence_changed", f"effective tokens {detail}", name)
            changes["token_changes"][name] = {"added": eff_added, "removed": eff_removed}
        elif structure_changed:
            record["classification"] = "condition"
            a.add("regex_or_source_gate_changed", "upstream record semantics changed", name)
            changes["condition_changes"].append(name)
        elif line_changed and not meta:
            record["classification"] = "unexplained"
            a.add("unexplained_library_change", "library condition changed without a baseline change", name)
            changes["condition_changes"].append(name)
        elif meta:
            record["classification"] = "metadata"
            changes["condition_changes"].append(name)
        elif regrouped:
            record["classification"] = "records_regrouped"
            a.add(
                "upstream_records_regrouped",
                "upstream split/merged records; token union and rendered Define are byte-identical",
                name,
            )
            changes["safe_candidates"].append(name)
        else:
            record["classification"] = "pattern_text_only"
            a.add("upstream_pattern_text_only", "upstream regex text changed; rendered Define is byte-identical", name)
            changes["safe_candidates"].append(name)

        changes["defines"].append(record)

        if record["classification"] not in ("pattern_text_only", "records_regrouped"):
            touched = [name] + ([TRUSTED] if trusted_changed and is_tier(name) else [])
            for define in touched:
                for c in consumers.get(define, []):
                    effect = {"define": name, "via": define, **c, "label": EFFECT_LABELS.get(c["rule"])}
                    changes["effects"].append(effect)

    seen = set()
    for e in sorted(changes["effects"], key=lambda e: (e["via"], e["rule"], e["variant"])):
        key = (e["via"], e["rule"])
        if key in seen:
            continue
        seen.add(key)
        variants = sorted({x["variant"] for x in changes["effects"] if (x["via"], x["rule"]) == key})
        what = f'{e["action"]} {e["points"]:+d}' if e["action"] == "score" else e["action"]
        label = f'; {e["label"]}' if e["label"] else ""
        a.add("indirect_policy_effect", f'{e["rule"]} ({what}{label}) in {",".join(variants)}', e["via"])

    if not any_change:
        a.add("metadata_only" if metadata_only else "no_semantic_change", "no Define semantics changed")
    return a, changes


# --------------------------------------------------------------------------
# Real-engine probe plan


def probe_kinds(scope):
    return SCOPE_KINDS.get(scope, ())


def applies(define_scope, kind):
    if define_scope is None:
        return True
    return kind == define_scope or {"movie": "anime_movie", "series": "anime_show"}.get(define_scope) == kind


def build_plan(inputs, changes):
    new = inputs["new_baseline"].get("defines", {})
    old = inputs["old_baseline"].get("defines", {})
    every = {**old, **new}
    changed_tokens = {}  # kind -> casefolded tokens that changed in an applicable Define
    condition_kinds = set()
    for name, delta in changes["token_changes"].items():
        scope = every[name].get("scope")
        for kind in KIND_REQUESTS:
            if applies(scope, kind):
                changed_tokens.setdefault(kind, set()).update(t.casefold() for t in delta["added"] + delta["removed"])
    for name in changes["condition_changes"]:
        for kind in KIND_REQUESTS:
            if any(applies(d[name].get("scope"), kind) for d in (old, new) if name in d):
                condition_kinds.add(kind)
    all_tokens = {t.casefold() for e in every.values() for t in e.get("tokens", [])}

    probes = {}

    def expect(kind, group):
        return kind in condition_kinds or group.casefold() in changed_tokens.get(kind, set())

    def add(define, token, scenario, kind, target, pool):
        req = {k: v for k, v in KIND_REQUESTS[kind].items() if k != "stem"}
        pid = f"{scenario}|{kind}|{target}"
        if pid in probes:  # the same release shape probed via another Define
            return
        probes[pid] = {
            "id": pid,
            "define": define,
            "token": token,
            "scenario": scenario,
            "request": req,
            "pool": pool,
            "target": target,
            "expect_change": expect(kind, target.rsplit("-", 1)[-1]),
        }

    def title(kind, tail, group):
        return f'{KIND_REQUESTS[kind]["stem"]}.{tail}-{group}'

    def token_probes(name, token):
        entry = every[name]
        family = tier_family(name) if is_tier(name) else None
        tails = FAMILY_TAILS.get(family, (GENERIC_TAIL, None, None))
        for kind in probe_kinds(entry.get("scope")):
            single = title(kind, tails[0], token)
            add(name, token, "single", kind, single, [single])
            for i, extra in enumerate(DEFINE_EXTRA_TAILS.get(name, ())):
                t = title(kind, extra, token)
                add(name, token, f"single-{i + 1}", kind, t, [t])
            sibling = SIBLING_KIND[kind]
            cross = title(sibling, tails[0], token)
            add(name, token, "cross-kind", sibling, cross, [cross])
            near = token + "X"
            if near.casefold() not in all_tokens:
                miss = title(kind, tails[0], near)
                add(name, token, "near-miss", kind, miss, [miss])
            if family and kind in ("movie", "series"):
                anchor = title(kind, UPSCALE_ANCHOR_TAIL, "ANCHOR")
                upscale = title(kind, tails[1], token)
                add(name, token, "4k-upscale", kind, upscale, [anchor, upscale])
                peers = [title(kind, X265_PEER_TAIL, f"PEER{i}") for i in range(X265_PEERS)]
                x265 = title(kind, tails[2], token)
                add(name, token, "hd-x265", kind, x265, peers + [x265])

    unprobeable = []
    for name, delta in sorted(changes["token_changes"].items()):
        for token in delta["added"] + delta["removed"]:
            if PROBE_TOKEN_RE.match(token):
                token_probes(name, token)
            else:
                unprobeable.append({"define": name, "token": token})
    for name in changes["safe_candidates"]:
        sample = [t for t in sorted(new[name].get("tokens", []), key=str.casefold) if PROBE_TOKEN_RE.match(t)]
        for token in sample[:SAFE_PROBE_SAMPLE]:
            token_probes(name, token)

    return {
        "schema_version": PLAN_SCHEMA,
        "old_library_sha256": sha256_bytes(inputs["old_library"].encode("utf-8")),
        "new_library_sha256": sha256_bytes(inputs["new_library"].encode("utf-8")),
        "variants": sorted(inputs["profile_paths"]),
        "profiles": {k: str(Path(v).resolve()) for k, v in sorted(inputs["profile_paths"].items())},
        "probes": [probes[k] for k in sorted(probes)],
        "unprobeable": unprobeable,
    }


def plan_paths(plan, old_library_path, new_library_path):
    out = dict(plan)
    out["old_library"] = str(Path(old_library_path).resolve())
    out["new_library"] = str(Path(new_library_path).resolve())
    return out


def outcome_key(r):
    return (r.get("found"), r.get("fetch"), r.get("rank"), r.get("matched") or {}, sorted(r.get("rejections") or []))


def describe_delta(old, new):
    parts = []
    def state(r):
        return "kept" if r.get("fetch") else "rejected"

    if old.get("fetch") != new.get("fetch"):
        why = sorted(set(new.get("rejections") or []) - set(old.get("rejections") or []))
        parts.append(f"{state(old)} -> {state(new)}" + (f" ({'; '.join(why)})" if why else ""))
    if old.get("rank") != new.get("rank"):
        parts.append(f"rank {old.get('rank')} -> {new.get('rank')}")
    om, nm = old.get("matched") or {}, new.get("matched") or {}
    for rule in sorted(set(om) | set(nm)):
        if om.get(rule) != nm.get(rule):
            if rule not in om:
                parts.append(f"+{rule} ({nm[rule]:+d})")
            elif rule not in nm:
                parts.append(f"-{rule} ({om[rule]:+d})")
            else:
                parts.append(f"{rule} {om[rule]:+d} -> {nm[rule]:+d}")
    return "; ".join(parts) or "outcome changed"


def check_engine(a, plan, plan_bytes, results, compat_result):
    evidence = {
        "compat_suite": compat_result,
        "probes_planned": len(plan["probes"]),
        "probes_completed": 0,
        "deltas": [],
        "unprobeable": plan["unprobeable"],
    }
    if compat_result == "failed":
        a.add("real_engine_regressions_failed", "scripts/test_streamnzb_compat.sh failed")
    elif compat_result != "passed":
        a.add("real_engine_regressions_not_run", "no compatibility-suite result supplied")
    if not plan["probes"]:
        return evidence
    if results is None:
        a.add("engine_evidence_missing", f'{len(plan["probes"])} planned probes have no results')
        return evidence
    if (
        results.get("plan_sha256") != sha256_bytes(plan_bytes)
        or results.get("old_library_sha256") != plan["old_library_sha256"]
        or results.get("new_library_sha256") != plan["new_library_sha256"]
    ):
        a.add("engine_evidence_stale", "probe results were produced for different inputs")
        return evidence

    by = {}
    for r in results.get("results", []):
        by[(r.get("id"), r.get("variant"), r.get("library"))] = r
    missing = 0
    probes = {p["id"]: p for p in plan["probes"]}
    for pid, probe in probes.items():
        for variant in plan["variants"]:
            o, n = by.get((pid, variant, "old")), by.get((pid, variant, "new"))
            if o is None or n is None or not o.get("found") or not n.get("found"):
                missing += 1
                continue
            evidence["probes_completed"] += 1
            if outcome_key(o) != outcome_key(n):
                evidence["deltas"].append(
                    {
                        "id": pid,
                        "variant": variant,
                        "scenario": probe["scenario"],
                        "kind": probe["request"]["kind"],
                        "target": probe["target"],
                        "expected": probe["expect_change"],
                        "delta": describe_delta(o, n),
                    }
                )
    if missing:
        a.add("engine_evidence_incomplete", f"{missing} probe/variant result(s) missing or target not found")

    for d in evidence["deltas"]:
        if not d["expected"]:
            a.add(
                "unexpected_behavior_delta",
                f'{d["variant"]} {d["kind"]} {d["scenario"]} {d["target"]}: {d["delta"]}',
                probes[d["id"]]["define"],
            )
    observed = {(probes[d["id"]]["define"], probes[d["id"]]["token"]) for d in evidence["deltas"]}
    for pid, probe in probes.items():
        if probe["scenario"] == "single" and probe["expect_change"]:
            key = (probe["define"], probe["token"])
            if key not in observed:
                observed.add(key)
                a.add(
                    "expected_change_not_observed",
                    f'no probe for {probe["token"]} changed behavior',
                    probe["define"],
                )
    evidence["deltas"].sort(key=lambda d: (not d["expected"], d["id"], d["variant"]))
    return evidence


# --------------------------------------------------------------------------
# Reports


def render_markdown(result, max_deltas=40):
    lines = [
        "## Vidhin sync assessment (advisory)",
        "",
        f"**Decision: `{result['decision']}`**",
        "",
        "> Advisory only. This assessment never merges, approves or labels a PR. "
        "Release impact remains a human decision. "
        "Policy: `docs/VIDHIN_SYNC_ASSESSMENT.md`.",
        "",
        "### Reasons",
        "",
        "| Decision | Code | Define | Detail |",
        "|---|---|---|---|",
    ]
    for r in result["reasons"]:
        detail = r["detail"].replace("|", "\\|")
        lines.append(f"| {r['decision']} | `{r['code']}` | {r['define'] or ''} | {detail} |")
    lines.append("")

    changes = result["changes"]
    if changes["defines"]:
        lines += ["### Classification changes", ""]
        for d in changes["defines"]:
            bits = [f"+{t}" for t in d["added"]] + [f"-{t}" for t in d["removed"]]
            bits += [f'{m["field"]} changed' for m in d["metadata"]]
            if not bits:
                bits = [d["classification"].replace("_", " ")]
            lines.append(f"- **{d['define']}** [{d['scope']}]: {', '.join(f'`{b}`' for b in bits)}")
        lines.append("")
    if changes["trusted"]["added"] or changes["trusted"]["removed"]:
        lines += ["### Trusted Release Groups membership", ""]
        lines += [f"- `+ {x['token']}` [{x['scope']}]" for x in changes["trusted"]["added"]]
        lines += [f"- `- {x['token']}` [{x['scope']}]" for x in changes["trusted"]["removed"]]
        lines.append("")

    ev = result["evidence"]
    lines += [
        "### Real-engine evidence",
        "",
        f"- Compatibility suite (`scripts/test_streamnzb_compat.sh`): **{ev['compat_suite']}**",
        f"- Old-vs-new library probes: {ev['probes_completed']} of {ev['probes_planned'] * len(result['inputs']['profiles'])} "
        f"probe/profile runs completed, {len(ev['deltas'])} changed behavior "
        f"({sum(not d['expected'] for d in ev['deltas'])} unexpected)",
    ]
    if ev["unprobeable"]:
        names = ", ".join(f"`{u['token']}`" for u in ev["unprobeable"])
        lines.append(f"- Not probeable as a plain group name (review manually): {names}")
    lines.append("")
    if ev["deltas"]:
        lines += ["<details><summary>Behavior deltas (old → new library)</summary>", ""]
        for d in ev["deltas"][:max_deltas]:
            flag = "" if d["expected"] else " **UNEXPECTED**"
            lines.append(f"- {d['variant']} · {d['kind']} · {d['scenario']} · `{d['target']}`: {d['delta']}{flag}")
        if len(ev["deltas"]) > max_deltas:
            lines.append(f"- … {len(ev['deltas']) - max_deltas} more in the assessment JSON artifact")
        lines += ["", "</details>", ""]

    lines += ["### Inputs", ""]
    for key, value in result["inputs"].items():
        if key != "profiles":
            lines.append(f"- `{key}`: `{value}`")
    for variant, digest in result["inputs"]["profiles"].items():
        lines.append(f"- profile `{variant}`: `{digest}`")
    summary = {"decision": result["decision"], "reason_codes": sorted({r["code"] for r in result["reasons"]})}
    lines += ["", f"<!-- vidhin-sync-assessment {json.dumps(summary, sort_keys=True)} -->", ""]
    return "\n".join(lines)


# --------------------------------------------------------------------------
# CLI


def load_inputs(args, a):
    def read(path, label):
        try:
            return Path(path).read_text(encoding="utf-8")
        except OSError as exc:
            a.add("input_unreadable", f"{label}: {exc}")
            return None

    raw = {
        "old_baseline": read(args.old_baseline, "old baseline"),
        "new_baseline": read(args.new_baseline, "new baseline"),
        "old_library": read(args.old_library, "old library"),
        "new_library": read(args.new_library, "new library"),
        "mapping": read(args.mapping, "mapping"),
    }
    profile_paths = dict(args.profile) if args.profile else {k: v for k, v in DEFAULT_PROFILES}
    profile_text = {k: read(v, f"{k} profile") for k, v in profile_paths.items()}
    if any(v is None for v in raw.values()) or any(v is None for v in profile_text.values()):
        return None

    inputs = {"old_library": raw["old_library"], "new_library": raw["new_library"], "profile_paths": profile_paths}
    for key in ("old_baseline", "new_baseline", "mapping"):
        try:
            inputs[key] = json.loads(raw[key])
        except json.JSONDecodeError as exc:
            a.add("input_unreadable", f"{key}: {exc}")
            return None
    try:
        inputs["profiles"] = {k: decode_share_code(v) for k, v in profile_text.items()}
    except Exception as exc:
        a.add("input_unreadable", f"profile: {exc}")
        return None
    inputs["digests"] = {
        "old_baseline": sha256_bytes(raw["old_baseline"].encode("utf-8")),
        "new_baseline": sha256_bytes(raw["new_baseline"].encode("utf-8")),
        "old_library": sha256_bytes(raw["old_library"].encode("utf-8")),
        "new_library": sha256_bytes(raw["new_library"].encode("utf-8")),
        "mapping": sha256_bytes(raw["mapping"].encode("utf-8")),
        "profiles": {k: sha256_bytes(v.encode("utf-8")) for k, v in sorted(profile_text.items())},
    }
    return inputs


def profile_arg(value):
    variant, sep, path = value.partition("=")
    if not sep or not variant or not path:
        raise argparse.ArgumentTypeError("expected VARIANT=PATH")
    return variant, Path(path)


def build_parser():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="command", required=True)
    for name in ("plan", "assess"):
        p = sub.add_parser(name)
        p.add_argument("--old-baseline", type=Path, required=True)
        p.add_argument("--new-baseline", type=Path, default=sync_vidhin.BASELINE)
        p.add_argument("--old-library", type=Path, required=True)
        p.add_argument("--new-library", type=Path, default=sync_vidhin.LIBRARY)
        p.add_argument("--mapping", type=Path, default=sync_vidhin.MAPPING)
        p.add_argument("--profile", type=profile_arg, action="append", help="VARIANT=PATH (default: samsung, neutral)")
        p.add_argument("--plan", type=Path, required=(name == "assess"), help="probe plan path")
    assess = sub.choices["assess"]
    assess.add_argument("--compat-result", choices=("passed", "failed", "not_run"), default="not_run")
    assess.add_argument("--engine-results", type=Path)
    assess.add_argument("--json-out", type=Path, required=True)
    assess.add_argument("--markdown-out", type=Path)
    return ap


def run(args):
    a = Assessment()
    inputs = load_inputs(args, a)
    changes = {"defines": [], "trusted": {"added": [], "removed": []}, "tier_movements": [], "effects": [],
               "token_changes": {}, "condition_changes": [], "safe_candidates": []}
    plan = {"schema_version": PLAN_SCHEMA, "old_library_sha256": None, "new_library_sha256": None,
            "variants": [], "profiles": {}, "probes": [], "unprobeable": []}
    if inputs is not None:
        try:
            a, changes = diff(inputs)
        except Exception as exc:  # fail closed on any unforeseen input shape
            a.add("input_unreadable", f"semantic diff failed: {type(exc).__name__}: {exc}")
        try:
            plan = build_plan(inputs, changes)
        except Exception as exc:  # malformed baseline shape: keep the empty plan, fail closed
            a.add("input_unreadable", f"probe planning failed: {type(exc).__name__}: {exc}")
    plan = plan_paths(plan, args.old_library, args.new_library)
    plan_bytes = canonical_json(plan).encode("utf-8")
    return a, inputs, changes, plan, plan_bytes


def main(argv=None):
    args = build_parser().parse_args(argv)
    a, inputs, changes, plan, plan_bytes = run(args)

    if args.command == "plan":
        args.plan.parent.mkdir(parents=True, exist_ok=True)
        args.plan.write_bytes(plan_bytes)
        print(f"Wrote {len(plan['probes'])} probe(s) to {args.plan}")
        return 0

    if args.plan.exists() and args.plan.read_bytes() != plan_bytes:
        a.add("engine_evidence_stale", "probe plan on disk differs from the plan for these inputs")
    results = None
    if args.engine_results and args.engine_results.exists():
        try:
            results = json.loads(args.engine_results.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            a.add("engine_evidence_missing", f"probe results unreadable: {exc}")
    evidence = check_engine(a, plan, plan_bytes, results, args.compat_result)

    digests = inputs["digests"] if inputs else {"profiles": {}}
    result = {
        "schema_version": ASSESSMENT_SCHEMA,
        "decision": a.decision(),
        "advisory": True,
        "reasons": a.sorted_reasons(),
        "changes": {k: changes[k] for k in ("defines", "trusted", "tier_movements")},
        "effects": sorted(
            changes["effects"], key=lambda e: (e["define"], e["via"], e["rule"], e["variant"])
        ),
        "evidence": evidence,
        "inputs": {**{k: v for k, v in digests.items() if k != "profiles"}, "profiles": digests["profiles"]},
    }
    args.json_out.parent.mkdir(parents=True, exist_ok=True)
    args.json_out.write_text(canonical_json(result), encoding="utf-8")
    if args.markdown_out:
        args.markdown_out.parent.mkdir(parents=True, exist_ok=True)
        args.markdown_out.write_text(render_markdown(result), encoding="utf-8")
    print(f"Vidhin sync assessment: {result['decision']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
