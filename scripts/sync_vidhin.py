#!/usr/bin/env python3
"""Track Vidhin release-group regexes used by StreamNZB Define rules.

V1 is intentionally conservative: it snapshots the exact upstream regex records
that feed each StreamNZB Define. It does not rewrite profile.txt.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import sys
import urllib.request
from collections import defaultdict
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MAPPING = ROOT / "scripts" / "vidhin_mapping.json"
DEFAULT_SNAPSHOT = ROOT / "generated" / "vidhin-defines.json"
DEFAULT_REPORT = ROOT / "generated" / "vidhin-sync-report.md"


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def fetch_json(url: str) -> Any:
    req = urllib.request.Request(url, headers={"User-Agent": "streamnzb-vidhin-sync/1"})
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def index_upstream(records: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    by_name: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for record in records:
        name = record.get("name")
        pattern = record.get("pattern")
        if isinstance(name, str) and isinstance(pattern, str):
            by_name[name].append({"pattern": pattern, "score": record.get("score")})
    return dict(by_name)


def build_snapshot(mapping: dict[str, Any], upstream: list[dict[str, Any]]) -> dict[str, Any]:
    indexed = index_upstream(upstream)
    targets: dict[str, Any] = {}
    missing: list[str] = []

    for target, source_names in mapping["targets"].items():
        source_records = []
        for source_name in source_names:
            records = indexed.get(source_name, [])
            if not records:
                missing.append(source_name)
                continue
            # Keep duplicates: Vidhin intentionally uses several records with the same name.
            source_records.append({"name": source_name, "records": records})

        canonical = json.dumps(source_records, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
        targets[target] = {
            "sources": source_records,
            "sha256": sha256_text(canonical),
        }

    if missing:
        unique = sorted(set(missing))
        raise RuntimeError("Mapped upstream rule(s) missing: " + ", ".join(unique))

    return {
        "schema_version": 1,
        "mapping_schema_version": mapping.get("schema_version", 1),
        "upstream_url": mapping["upstream_url"],
        "generated_at_utc": dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat(),
        "targets": targets,
    }


def compare(old: dict[str, Any] | None, new: dict[str, Any]) -> list[tuple[str, str]]:
    if old is None:
        return [(name, "baseline") for name in new["targets"]]

    changes: list[tuple[str, str]] = []
    old_targets = old.get("targets", {})
    new_targets = new.get("targets", {})
    for name in sorted(set(old_targets) | set(new_targets)):
        if name not in old_targets:
            changes.append((name, "added target"))
        elif name not in new_targets:
            changes.append((name, "removed target"))
        elif old_targets[name].get("sha256") != new_targets[name].get("sha256"):
            changes.append((name, "upstream regex changed"))
    return changes


def source_pattern_diff(old_target: dict[str, Any], new_target: dict[str, Any]) -> list[str]:
    def flatten(target: dict[str, Any]) -> dict[str, list[str]]:
        out: dict[str, list[str]] = {}
        for source in target.get("sources", []):
            out[source["name"]] = [r["pattern"] for r in source.get("records", [])]
        return out

    old = flatten(old_target)
    new = flatten(new_target)
    lines: list[str] = []
    for source in sorted(set(old) | set(new)):
        if old.get(source) == new.get(source):
            continue
        lines.append(f"- `{source}`: {len(old.get(source, []))} pattern(s) → {len(new.get(source, []))} pattern(s)")
    return lines


def render_report(old: dict[str, Any] | None, new: dict[str, Any], changes: list[tuple[str, str]]) -> str:
    lines = [
        "# Vidhin sync report",
        "",
        f"Upstream: `{new['upstream_url']}`",
        f"Checked: `{new['generated_at_utc']}`",
        "",
    ]
    if old is None:
        lines += [
            "Initial baseline created.",
            "",
            f"Tracked StreamNZB Defines: **{len(new['targets'])}**",
            "",
            "No profile changes are made by this v1 sync.",
        ]
        return "\n".join(lines) + "\n"

    if not changes:
        lines += ["No mapped Vidhin regex changes detected.", ""]
        return "\n".join(lines)

    lines += [f"Changed Define mappings: **{len(changes)}**", ""]
    for target, reason in changes:
        lines.append(f"## {target}")
        lines.append("")
        lines.append(f"Status: **{reason}**")
        if target in old.get("targets", {}) and target in new.get("targets", {}):
            lines.extend(source_pattern_diff(old["targets"][target], new["targets"][target]))
        lines.append("")
    lines += [
        "## Review required",
        "",
        "This workflow only tracks upstream changes. Review the changed Vidhin patterns before updating StreamNZB Define memberships.",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mapping", type=Path, default=DEFAULT_MAPPING)
    parser.add_argument("--snapshot", type=Path, default=DEFAULT_SNAPSHOT)
    parser.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    parser.add_argument("--source-file", type=Path, help="Use local regexes.json instead of downloading upstream")
    parser.add_argument("--dry-run", action="store_true", help="Do not update snapshot/report files")
    args = parser.parse_args()

    mapping = load_json(args.mapping)
    upstream = load_json(args.source_file) if args.source_file else fetch_json(mapping["upstream_url"])
    if not isinstance(upstream, list):
        raise RuntimeError("Upstream regexes.json must contain a JSON array")

    old = load_json(args.snapshot) if args.snapshot.exists() else None
    new = build_snapshot(mapping, upstream)
    changes = compare(old, new)
    report = render_report(old, new, changes)

    # Do not rewrite generated files when nothing changed; otherwise the
    # timestamp alone would create a pointless GitHub PR every scheduled run.
    should_write = old is None or bool(changes)
    if not args.dry_run and should_write:
        args.snapshot.parent.mkdir(parents=True, exist_ok=True)
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.snapshot.write_text(json.dumps(new, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
        args.report.write_text(report, encoding="utf-8")

    if old is None:
        print(f"Created baseline for {len(new['targets'])} Define mappings.")
    elif changes:
        print(f"Detected changes in {len(changes)} Define mapping(s).")
    else:
        print("No mapped Vidhin changes detected.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1)
