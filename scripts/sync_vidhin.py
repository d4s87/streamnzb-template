#!/usr/bin/env python3
from __future__ import annotations
import argparse, datetime as dt, json, re, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MAPPING = ROOT / "scripts" / "vidhin_mapping.json"
DEFAULT_BASELINE = ROOT / "generated" / "vidhin-defines.json"
DEFAULT_REPORT = ROOT / "generated" / "vidhin-sync-report.md"
DEFAULT_LIBRARY = ROOT / "generated" / "streamnzb-defines.txt"

def load_json(path): return json.loads(Path(path).read_text(encoding="utf-8"))
def load_json_url(url):
    req=urllib.request.Request(url,headers={"User-Agent":"streamnzb-template-vidhin-sync/2.2"})
    with urllib.request.urlopen(req,timeout=30) as r: return json.load(r)

def records(data):
    if isinstance(data,list): return data
    for k in ("regexes","data","items"):
        if isinstance(data,dict) and isinstance(data.get(k),list): return data[k]
    raise ValueError("Unsupported upstream JSON structure")

def name_of(rec):
    for k in ("name","title","label"):
        if isinstance(rec.get(k),str): return rec[k]
def pattern_of(rec):
    for k in ("pattern","regex","expression"):
        if isinstance(rec.get(k),str): return rec[k]
def mapping_targets(mapping):
    for k in ("targets","defines"):
        if isinstance(mapping.get(k),dict): return mapping[k]
    raise KeyError("Mapping must contain either 'targets' or 'defines'")

def balanced_positive_lookaheads(pattern):
    out=[]; i=0
    while True:
        start=pattern.find("(?=",i)
        if start<0: break
        depth=1; j=start+3; esc=False; cls=False
        while j<len(pattern) and depth:
            ch=pattern[j]
            if esc: esc=False
            elif ch=="\\": esc=True
            elif cls:
                if ch=="]": cls=False
            elif ch=="[": cls=True
            elif ch=="(": depth+=1
            elif ch==")": depth-=1
            j+=1
        if depth==0:
            out.append(pattern[start+3:j-1]); i=j
        else: break
    return out

def innermost_alt_groups(text):
    groups=[]; stack=[]; esc=False; cls=False
    for idx,ch in enumerate(text):
        if esc: esc=False; continue
        if ch=="\\": esc=True; continue
        if cls:
            if ch=="]": cls=False
            continue
        if ch=="[": cls=True; continue
        if ch=="(": stack.append(idx)
        elif ch==")" and stack:
            start=stack.pop()
            body=text[start+1:idx]
            if "(" not in body and ")" not in body and "|" in body:
                groups.append(re.sub(r"^\?:","",body))
    return groups

def looks_like_group_token(token):
    bad=("BluRay","Blu-Ray","WEB","WEBDL","WebRip","Remux","720","1080","2160","BDMux","Amazon","Netflix")
    return not any(x in token for x in bad)

def semantic_tokens(pattern):
    las=balanced_positive_lookaheads(pattern)
    if not las: return []
    classifier=las[-1]; tokens=set()
    for body in innermost_alt_groups(classifier):
        parts=[token.strip() for token in body.split("|") if token.strip()]
        # Skip structural wrapper alternations such as \[sam\]|-sam\b.
        if any("\\[" in token or "\\b" in token or token.startswith("-") for token in parts):
            continue
        for token in parts:
            if looks_like_group_token(token):
                tokens.add(token)
    for m in re.finditer(r"\\\[([A-Za-z0-9][A-Za-z0-9_.+-]*)\\\]",classifier): tokens.add(m.group(1))
    for m in re.finditer(r"(?<![A-Za-z0-9])-([A-Za-z0-9][A-Za-z0-9_.+-]*)\\b",classifier): tokens.add(m.group(1))
    for m in re.finditer(r"\\b\(\?:?([A-Za-z0-9][A-Za-z0-9_.+-]*)\)\\b",classifier): tokens.add(m.group(1))
    return sorted(tokens,key=lambda x:(x.casefold(),x))

def resolve(mapping,upstream):
    by={}
    for rec in records(upstream):
        n=name_of(rec); p=pattern_of(rec)
        if n and p: by.setdefault(n,[]).append(p)
    result={}; missing=[]
    for target,sources in mapping_targets(mapping).items():
        recs=[]
        for src in sources:
            pats=by.get(src,[])
            if not pats: missing.append(f"{target}: {src}")
            for p in pats: recs.append({"source":src,"pattern":p,"tokens":semantic_tokens(p)})
        result[target]={"sources":sources,"records":recs}
    if missing: raise RuntimeError("Mapped upstream rule(s) missing:\n- "+"\n- ".join(missing))
    return result

def convert_v1_target(entry):
    recs=[]; names=[]
    for src in entry.get("sources",[]):
        n=src.get("name")
        if not isinstance(n,str): continue
        names.append(n)
        for rec in src.get("records",[]):
            p=rec.get("pattern")
            if isinstance(p,str): recs.append({"source":n,"pattern":p,"tokens":semantic_tokens(p)})
    return {"sources":names,"records":recs}

def load_previous_baseline(path):
    path=Path(path)
    if not path.exists(): return {}
    prev=load_json(path)
    if isinstance(prev.get("defines"),dict): return prev["defines"]
    if isinstance(prev.get("targets"),dict): return {k:convert_v1_target(v) for k,v in prev["targets"].items()}
    return prev if isinstance(prev,dict) else {}

def token_union(entry):
    out=set()
    for r in entry.get("records",[]): out.update(r.get("tokens",[]))
    return out
def raw_patterns(entry):
    return {(r.get("source"),r.get("pattern")) for r in entry.get("records",[]) if r.get("source") is not None and r.get("pattern") is not None}

def scope_for_target(name):
    if name.startswith("Anime Movies "): return "anime_movie"
    if name.startswith("Anime Shows "): return "anime_show"
    if name.startswith("Movies "): return "movie"
    if name.startswith("Shows "): return "series"
    raise ValueError(f"Cannot infer scope: {name}")

def add_effective_tokens(defines):
    for name,e in defines.items(): e["effective_tokens"]=sorted(token_union(e),key=lambda x:(x.casefold(),x))
    for prefix in ("Movies WEB","Shows WEB"):
        seen=set()
        for i in (1,2,3):
            name=f"{prefix} T{i} Groups"
            if name not in defines: continue
            cur=set(defines[name]["effective_tokens"]); kept=cur-seen
            defines[name]["effective_tokens"]=sorted(kept,key=lambda x:(x.casefold(),x))
            seen.update(kept)

def render_streamnzb_library(defines):
    work=json.loads(json.dumps(defines)); add_effective_tokens(work)
    lines=[
      "# Generated from Vidhin05/Releases-Regex.",
      "# Review artifact only; profile.txt is not modified.",
      ""
    ]
    for name in sorted(work):
        toks=work[name]["effective_tokens"]
        if not toks:
            lines.append(f"# WARNING: no semantic tokens extracted for {name}"); continue
        body="|".join(toks).replace('"','\\"')
        lines.append(f'{name} [{scope_for_target(name)}]: define if group matches "(?i)^({body})$"')
    return "\n".join(lines)+"\n"

def report_diff(old,new):
    lines=["# Vidhin sync report",""]; changed=0
    for target in sorted(set(old)|set(new)):
        o=old.get(target,{"records":[]}); n=new.get(target,{"records":[]})
        ot,nt=token_union(o),token_union(n)
        add=sorted(nt-ot,key=str.casefold); rem=sorted(ot-nt,key=str.casefold)
        raw=raw_patterns(o)!=raw_patterns(n)
        if not add and not rem and not raw: continue
        changed+=1; lines += [f"## {target}",""]
        if add:
            lines += ["**Added release-group tokens**"]+[f"- `+ {x}`" for x in add]+[""]
        if rem:
            lines += ["**Removed release-group tokens**"]+[f"- `- {x}`" for x in rem]+[""]
        if raw and not(add or rem): lines += ["Raw upstream regex changed, but the extracted release-group set did not.",""]
    if not changed: lines += ["No mapped Vidhin changes detected.",""]
    lines += ["---",f"Tracked StreamNZB Defines: **{len(new)}**","",
              "> `profile.txt` is not modified. `streamnzb-defines.txt` is a generated review artifact.",""]
    return "\n".join(lines),changed

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--mapping",type=Path,default=DEFAULT_MAPPING)
    ap.add_argument("--baseline",type=Path,default=DEFAULT_BASELINE)
    ap.add_argument("--report",type=Path,default=DEFAULT_REPORT)
    ap.add_argument("--library",type=Path,default=DEFAULT_LIBRARY)
    ap.add_argument("--upstream-file",type=Path)
    a=ap.parse_args()
    mapping=load_json(a.mapping)
    upstream=load_json(a.upstream_file) if a.upstream_file else load_json_url(mapping["upstream_url"])
    current=resolve(mapping,upstream); old=load_previous_baseline(a.baseline)
    report,changed=report_diff(old,current)
    a.library.parent.mkdir(parents=True,exist_ok=True)
    a.library.write_text(render_streamnzb_library(current),encoding="utf-8")
    if changed or not a.baseline.exists():
        payload={"schema_version":2,"mapping_schema_version":mapping.get("schema_version",1),
                 "upstream_url":mapping["upstream_url"],
                 "generated_at_utc":dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat(),
                 "defines":current}
        a.baseline.parent.mkdir(parents=True,exist_ok=True); a.report.parent.mkdir(parents=True,exist_ok=True)
        a.baseline.write_text(json.dumps(payload,indent=2,ensure_ascii=False)+"\n",encoding="utf-8")
        a.report.write_text(report,encoding="utf-8")
        print(f"Detected changes in {changed} mapped Define(s).")
    else:
        print("No mapped Vidhin changes detected.")

if __name__=="__main__": main()


if __name__ == "__main__":
    main()
