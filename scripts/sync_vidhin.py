#!/usr/bin/env python3
"""
Vidhin -> StreamNZB semantic change detector (v2.1, backward-compatible).

- Accepts the existing v1 mapping schema (`targets`) and the newer alias (`defines`).
- Reads the existing v1 baseline schema (`targets`) and upgrades it in memory.
- Does NOT modify profile.txt.
- Keeps Radarr/Sonarr separation exactly as declared in the mapping.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MAPPING = ROOT / "scripts" / "vidhin_mapping.json"
DEFAULT_BASELINE = ROOT / "generated" / "vidhin-defines.json"
DEFAULT_REPORT = ROOT / "generated" / "vidhin-sync-report.md"


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def load_json_url(url: str):
    req = urllib.request.Request(
        url, headers={"User-Agent": "streamnzb-template-vidhin-sync/2.1"}
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def records(data):
    if isinstance(data, list):
        return data
    for key in ("regexes", "data", "items"):
        if isinstance(data, dict) and isinstance(data.get(key), list):
            return data[key]
    raise ValueError("Unsupported upstream JSON structure")


def name_of(rec):
    for k in ("name", "title", "label"):
        if isinstance(rec.get(k), str):
            return rec[k]
    return None


def pattern_of(rec):
    for k in ("pattern", "regex", "expression"):
        if isinstance(rec.get(k), str):
            return rec[k]
    return None


def mapping_targets(mapping):
    targets = mapping.get("targets")
    if isinstance(targets, dict):
        return targets
    defines = mapping.get("defines")
    if isinstance(defines, dict):
        return defines
    raise KeyError("Mapping must contain either 'targets' or 'defines'")


def split_top_level_alternation(s: str):
    out, buf = [], []
    depth = 0
    cls = False
    esc = False

    for ch in s:
        if esc:
            buf.append(ch)
            esc = False
            continue
        if ch == "\\":
            buf.append(ch)
            esc = True
            continue
        if cls:
            buf.append(ch)
            if ch == "]":
                cls = False
            continue
        if ch == "[":
            buf.append(ch)
            cls = True
            continue
        if ch == "(":
            depth += 1
            buf.append(ch)
            continue
        if ch == ")":
            depth = max(0, depth - 1)
            buf.append(ch)
            continue
        if ch == "|" and depth == 0:
            out.append("".join(buf))
            buf = []
            continue
        buf.append(ch)

    out.append("".join(buf))
    return [x for x in out if x]


def candidate_group_body(pattern: str):
    groups = []
    stack = []
    esc = False
    cls = False

    for i, ch in enumerate(pattern):
        if esc:
            esc = False
            continue
        if ch == "\\":
            esc = True
            continue
        if cls:
            if ch == "]":
                cls = False
            continue
        if ch == "[":
            cls = True
            continue
        if ch == "(":
            stack.append(i)
            continue
        if ch == ")" and stack:
            start = stack.pop()
            body = pattern[start + 1 : i]
            if body.startswith(("?<=", "?<!", "?=", "?!")):
                continue
            body2 = re.sub(r"^\?(?:i:|:)", "", body)
            if "|" in body2:
                groups.append(body2)

    if not groups:
        return None

    return max(groups, key=lambda x: len(split_top_level_alternation(x)))


def semantic_tokens(pattern: str):
    body = candidate_group_body(pattern)
    if not body:
        return []

    tokens = []
    for token in split_top_level_alternation(body):
        token = token.strip()
        token = re.sub(r"^\^+", "", token)
        token = re.sub(r"\$+$", "", token)
        token = token.strip()
        if token:
            tokens.append(token)

    return sorted(set(tokens), key=lambda x: (x.casefold(), x))


def resolve(mapping, upstream):
    by_name = {}
    for rec in records(upstream):
        name = name_of(rec)
        pattern = pattern_of(rec)
        if name and pattern:
            by_name.setdefault(name, []).append(pattern)

    result = {}
    missing = []

    for define, sources in mapping_targets(mapping).items():
        raw = []
        for src in sources:
            pats = by_name.get(src, [])
            if not pats:
                missing.append(f"{define}: {src}")
                continue
            for pattern in pats:
                raw.append(
                    {
                        "source": src,
                        "pattern": pattern,
                        "tokens": semantic_tokens(pattern),
                    }
                )

        result[define] = {"sources": sources, "records": raw}

    if missing:
        raise RuntimeError(
            "Mapped upstream rule(s) missing:\n- " + "\n- ".join(missing)
        )

    return result


def convert_v1_target(entry):
    """Convert one v1 target entry to the v2 in-memory shape."""
    records_out = []
    source_names = []

    for source in entry.get("sources", []):
        name = source.get("name")
        if not isinstance(name, str):
            continue
        source_names.append(name)
        for rec in source.get("records", []):
            pattern = rec.get("pattern")
            if isinstance(pattern, str):
                records_out.append(
                    {
                        "source": name,
                        "pattern": pattern,
                        "tokens": semantic_tokens(pattern),
                    }
                )

    return {"sources": source_names, "records": records_out}


def load_previous_baseline(path: Path):
    if not path.exists():
        return {}

    prev = load_json(path)

    if isinstance(prev.get("defines"), dict):
        return prev["defines"]

    if isinstance(prev.get("targets"), dict):
        return {
            name: convert_v1_target(entry)
            for name, entry in prev["targets"].items()
        }

    # Support a bare mapping as a last resort.
    if isinstance(prev, dict):
        return prev

    raise ValueError("Unsupported baseline JSON structure")


def token_union(entry):
    values = set()
    for rec in entry.get("records", []):
        values.update(rec.get("tokens", []))
    return values


def raw_patterns(entry):
    return {
        (rec.get("source"), rec.get("pattern"))
        for rec in entry.get("records", [])
        if rec.get("source") is not None and rec.get("pattern") is not None
    }


def report_diff(old, new):
    lines = ["# Vidhin sync report", ""]
    changed = 0

    for define in sorted(set(old) | set(new)):
        old_entry = old.get(define, {"records": []})
        new_entry = new.get(define, {"records": []})

        old_tokens = token_union(old_entry)
        new_tokens = token_union(new_entry)

        added = sorted(new_tokens - old_tokens, key=str.casefold)
        removed = sorted(old_tokens - new_tokens, key=str.casefold)
        raw_changed = raw_patterns(old_entry) != raw_patterns(new_entry)

        if not added and not removed and not raw_changed:
            continue

        changed += 1
        lines += [f"## {define}", ""]

        if added:
            lines.append("**Added semantic tokens**")
            lines += [f"- `+ {item}`" for item in added]
            lines.append("")

        if removed:
            lines.append("**Removed semantic tokens**")
            lines += [f"- `- {item}`" for item in removed]
            lines.append("")

        if raw_changed and not (added or removed):
            lines += [
                "Raw upstream regex changed, but the conservative semantic token set did not.",
                "",
            ]

    if not changed:
        lines += ["No mapped Vidhin changes detected.", ""]

    lines += [
        "---",
        f"Tracked StreamNZB Defines: **{len(new)}**",
        "",
        "> This workflow does not modify `profile.txt`. Semantic tokens are review aids; raw upstream regex is retained in the baseline.",
        "",
    ]

    return "\n".join(lines), changed


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mapping", type=Path, default=DEFAULT_MAPPING)
    parser.add_argument("--baseline", type=Path, default=DEFAULT_BASELINE)
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    args = parser.parse_args()

    mapping = load_json(args.mapping)
    upstream = load_json_url(mapping["upstream_url"])
    current = resolve(mapping, upstream)
    old = load_previous_baseline(args.baseline)

    report, changed = report_diff(old, current)

    if changed or not args.baseline.exists():
        payload = {
            "schema_version": 2,
            "mapping_schema_version": mapping.get("schema_version", 1),
            "upstream_url": mapping["upstream_url"],
            "generated_at_utc": dt.datetime.now(dt.timezone.utc)
            .replace(microsecond=0)
            .isoformat(),
            "defines": current,
        }

        args.baseline.parent.mkdir(parents=True, exist_ok=True)
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.baseline.write_text(
            json.dumps(payload, indent=2, ensure_ascii=False) + "\n",
            encoding="utf-8",
        )
        args.report.write_text(report, encoding="utf-8")
        print(f"Detected changes in {changed} mapped Define(s).")
    else:
        print("No mapped Vidhin changes detected.")


if __name__ == "__main__":
    main()
