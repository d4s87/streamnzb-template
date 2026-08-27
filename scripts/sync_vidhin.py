#!/usr/bin/env python3
"""
Vidhin -> StreamNZB semantic change detector (v2).

Reads Vidhin regexes.json, resolves the explicitly mapped upstream rules,
and stores both raw patterns and a conservative semantic token view.

Important:
- It does NOT modify profile.txt.
- It never merges Radarr and Sonarr unless mapping explicitly requests it.
- Semantic extraction is conservative. Tokens containing regex operators are
  preserved as regex tokens instead of being "simplified" into guessed names.
"""
from __future__ import annotations
import argparse, datetime as dt, json, re, sys, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MAPPING = ROOT / "scripts" / "vidhin_mapping.json"
DEFAULT_BASELINE = ROOT / "generated" / "vidhin-defines.json"
DEFAULT_REPORT = ROOT / "generated" / "vidhin-sync-report.md"

def load_json_url(url: str):
    req = urllib.request.Request(url, headers={"User-Agent": "streamnzb-template-vidhin-sync/2"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)

def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))

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
    for k in ("regex", "pattern", "expression"):
        if isinstance(rec.get(k), str):
            return rec[k]
    return None

def split_top_level_alternation(s: str):
    out, buf = [], []
    depth = 0
    cls = False
    esc = False
    for ch in s:
        if esc:
            buf.append(ch); esc = False; continue
        if ch == "\\":
            buf.append(ch); esc = True; continue
        if cls:
            buf.append(ch)
            if ch == "]": cls = False
            continue
        if ch == "[":
            buf.append(ch); cls = True; continue
        if ch == "(":
            depth += 1; buf.append(ch); continue
        if ch == ")":
            depth = max(0, depth-1); buf.append(ch); continue
        if ch == "|" and depth == 0:
            out.append("".join(buf)); buf = []; continue
        buf.append(ch)
    out.append("".join(buf))
    return [x for x in out if x]

def candidate_group_body(pattern: str):
    # Find useful alternation-bearing groups. Prefer the innermost/largest
    # non-lookaround group; this avoids treating context guards as group names.
    groups = []
    stack = []
    esc = False
    cls = False
    for i,ch in enumerate(pattern):
        if esc: esc=False; continue
        if ch=="\\": esc=True; continue
        if cls:
            if ch=="]": cls=False
            continue
        if ch=="[": cls=True; continue
        if ch=="(":
            stack.append(i)
        elif ch==")" and stack:
            start=stack.pop()
            body=pattern[start+1:i]
            # Strip common group prefixes.
            body2=re.sub(r'^\?(?:i:|:)', '', body)
            if "|" in body2 and not body.startswith(("?<=", "?<!", "?=", "?!")):
                groups.append(body2)
    if not groups:
        return None
    # Prefer group with most top-level alternatives.
    return max(groups, key=lambda x: len(split_top_level_alternation(x)))

def semantic_tokens(pattern: str):
    body = candidate_group_body(pattern)
    if not body:
        return []
    toks = split_top_level_alternation(body)
    cleaned=[]
    for t in toks:
        t=t.strip()
        # Remove anchors/boundary wrappers only; preserve regex semantics inside token.
        t=re.sub(r'^\^+', '', t)
        t=re.sub(r'\$+$', '', t)
        t=t.strip()
        if t:
            cleaned.append(t)
    # deterministic, case-sensitive preservation
    return sorted(set(cleaned), key=lambda x: (x.casefold(), x))

def resolve(mapping, upstream):
    by_name={}
    for rec in records(upstream):
        n=name_of(rec); p=pattern_of(rec)
        if n and p:
            by_name.setdefault(n, []).append(p)

    result={}
    missing=[]
    for define, sources in mapping["defines"].items():
        raw=[]
        for src in sources:
            pats=by_name.get(src, [])
            if not pats:
                missing.append(f"{define}: {src}")
            raw.extend({"source":src, "pattern":p, "tokens":semantic_tokens(p)} for p in pats)
        result[define]={"sources":sources, "records":raw}
    if missing:
        raise RuntimeError("Mapped upstream rule(s) missing:\n- " + "\n- ".join(missing))
    return result

def token_union(entry):
    vals=set()
    for r in entry.get("records", []):
        vals.update(r.get("tokens", []))
    return vals

def raw_patterns(entry):
    return {(r["source"], r["pattern"]) for r in entry.get("records", [])}

def report_diff(old, new):
    lines=["# Vidhin sync report", ""]
    changed=0
    all_defs=sorted(set(old)|set(new))
    for d in all_defs:
        o=old.get(d, {"records":[]}); n=new.get(d, {"records":[]})
        ot, nt=token_union(o), token_union(n)
        add=sorted(nt-ot, key=str.casefold)
        rem=sorted(ot-nt, key=str.casefold)
        raw_changed=raw_patterns(o)!=raw_patterns(n)
        if not add and not rem and not raw_changed:
            continue
        changed += 1
        lines += [f"## {d}", ""]
        if add or rem:
            if add:
                lines.append("**Added semantic tokens**")
                lines += [f"- `+ {x}`" for x in add]
                lines.append("")
            if rem:
                lines.append("**Removed semantic tokens**")
                lines += [f"- `- {x}`" for x in rem]
                lines.append("")
        if raw_changed and not (add or rem):
            lines += ["Raw upstream regex changed, but the conservative semantic token set did not.", ""]
    if not changed:
        lines += ["No mapped Vidhin changes detected.", ""]
    lines += [
        "---",
        f"Tracked StreamNZB Defines: **{len(new)}**",
        "",
        "> This workflow does not modify `profile.txt`. Semantic tokens are review aids; raw upstream regex is retained in the baseline.",
        ""
    ]
    return "\n".join(lines), changed

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--mapping", type=Path, default=DEFAULT_MAPPING)
    ap.add_argument("--baseline", type=Path, default=DEFAULT_BASELINE)
    ap.add_argument("--report", type=Path, default=DEFAULT_REPORT)
    args=ap.parse_args()

    mapping=load_json(args.mapping)
    upstream=load_json_url(mapping["upstream_url"])
    current=resolve(mapping, upstream)

    old={}
    if args.baseline.exists():
        prev=load_json(args.baseline)
        old=prev.get("defines", prev)

    report, changed=report_diff(old, current)
    args.report.parent.mkdir(parents=True, exist_ok=True)

    if changed or not args.baseline.exists():
        payload={
            "schema_version": 2,
            "upstream_url": mapping["upstream_url"],
            "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "defines": current,
        }
        args.baseline.write_text(json.dumps(payload, indent=2, ensure_ascii=False)+"\n", encoding="utf-8")
        args.report.write_text(report, encoding="utf-8")
        print(f"{changed} mapped Define(s) changed.")
    else:
        print("No mapped Vidhin changes detected.")

if __name__=="__main__":
    main()
