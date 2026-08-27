#!/usr/bin/env python3
from __future__ import annotations
import argparse, datetime as dt, json, re, urllib.request
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
MAPPING=ROOT/"scripts"/"vidhin_mapping.json"
BASELINE=ROOT/"generated"/"vidhin-defines.json"
REPORT=ROOT/"generated"/"vidhin-sync-report.md"
LIBRARY=ROOT/"generated"/"streamnzb-defines.txt"

def jload(p): return json.loads(Path(p).read_text(encoding="utf-8"))
def fetch(url):
    req=urllib.request.Request(url,headers={"User-Agent":"streamnzb-template-vidhin-sync/2.4"})
    with urllib.request.urlopen(req,timeout=30) as r: return json.load(r)
def rows(data):
    if isinstance(data,list): return data
    for k in ("regexes","data","items"):
        if isinstance(data,dict) and isinstance(data.get(k),list): return data[k]
    raise ValueError("Unsupported upstream JSON structure")

def n(rec):
    for k in ("name","title","label"):
        if isinstance(rec.get(k),str): return rec[k]
def p(rec):
    for k in ("pattern","regex","expression"):
        if isinstance(rec.get(k),str): return rec[k]

def lookaheads(pattern):
    out=[]; i=0
    while True:
        s=pattern.find("(?=",i)
        if s<0: break
        depth=1; j=s+3; esc=False; cls=False
        while j<len(pattern) and depth:
            c=pattern[j]
            if esc: esc=False
            elif c=="\\": esc=True
            elif cls:
                if c=="]": cls=False
            elif c=="[": cls=True
            elif c=="(": depth+=1
            elif c==")": depth-=1
            j+=1
        if depth==0: out.append(pattern[s+3:j-1]); i=j
        else: break
    return out

QUALITY_TERMS={
"BluRay","Blu-Ray","HD-?DVD","BDMux","BD(?!$)","UHD","4K","WEB[-_. ]DL(?:mux)?",
"WEBDL","AmazonHD","AmazonSD","iTunesHD","MaxdomeHD","NetflixU?HD","WebHD",
"HBOMaxHD","DisneyHD","WebRip","Web-Rip","WEBMux"
}

def innermost_groups(text):
    out=[]; stack=[]; esc=False; cls=False
    for i,c in enumerate(text):
        if esc: esc=False; continue
        if c=="\\": esc=True; continue
        if cls:
            if c=="]": cls=False
            continue
        if c=="[": cls=True; continue
        if c=="(": stack.append(i)
        elif c==")" and stack:
            s=stack.pop(); body=text[s+1:i]
            if "(" not in body and ")" not in body:
                out.append(re.sub(r"^\?:","",body))
    return out

def clean_token(t):
    t=t.strip()
    if not t: return None
    # Strip common regex boundary/context fragments around a group name.
    t=re.sub(r"^\?:","",t)
    t=re.sub(r"^\^|\$$","",t)
    # Vidhin lookaround/context fragments are not release-group names.
    if t.startswith("?"):
        return None
    # Vidhin uses Not-Vodes as contextual exclusion syntax; it is not a release group.
    if t == "Not-Vodes":
        return None
    # "Remux" can appear as contextual regex structure in Anime rules, not as a group.
    if t == "Remux":
        return None
    # Reject obvious structural wrapper fragments.
    if t.startswith("\\[") or t.startswith("-") or "\\b" in t:
        return None
    if t in QUALITY_TERMS: return None
    if any(x in t for x in ("(?:720|1080|2160)","[xh][ .]?26","DDP?","AMZN|NF|DP")): return None
    return t

def semantic_tokens(pattern):
    la=lookaheads(pattern)
    if not la: return []
    classifier=la[-1]
    out=set()
    # Every innermost alternation/list in the final classifier can contain release groups.
    for body in innermost_groups(classifier):
        if "|" in body:
            for x in body.split("|"):
                x=clean_token(x)
                if x: out.add(x)
        else:
            x=clean_token(body)
            if x: out.add(x)
    # Explicit bracket/dash singles.
    for m in re.finditer(r"\\\[([A-Za-z0-9][A-Za-z0-9_.+-]*)\\\]",classifier):
        out.add(m.group(1))
    for m in re.finditer(r"(?<![A-Za-z0-9])-([A-Za-z0-9][A-Za-z0-9_.+-]*)\\b",classifier):
        out.add(m.group(1))
    # Contextual Vodes should normalize to Vodes, never Not-Vodes.
    if "Vodes" in classifier: out.add("Vodes")
    return dedupe_casefold(out)

def dedupe_casefold(tokens):
    """Deduplicate tokens case-insensitively while preserving first spelling."""
    seen={}
    for token in tokens:
        key=token.casefold()
        if key not in seen:
            seen[key]=token
    return sorted(seen.values(), key=lambda x:(x.casefold(),x))

def split_top_level(text, sep="|"):
    out=[]; buf=[]; depth=0; cls=False; esc=False
    for ch in text:
        if esc:
            buf.append(ch); esc=False; continue
        if ch=="\\":
            buf.append(ch); esc=True; continue
        if cls:
            buf.append(ch)
            if ch=="]": cls=False
            continue
        if ch=="[":
            buf.append(ch); cls=True; continue
        if ch=="(":
            depth+=1; buf.append(ch); continue
        if ch==")":
            depth=max(0,depth-1); buf.append(ch); continue
        if ch==sep and depth==0:
            out.append("".join(buf)); buf=[]; continue
        buf.append(ch)
    out.append("".join(buf))
    return [x for x in out if x]

def js_regex_parts(pattern):
    if not pattern.startswith("/"):
        return pattern, ""
    esc=False; cls=False; last=None
    for i in range(1,len(pattern)):
        ch=pattern[i]
        if esc:
            esc=False; continue
        if ch=="\\":
            esc=True; continue
        if cls:
            if ch=="]": cls=False
            continue
        if ch=="[":
            cls=True; continue
        if ch=="/":
            last=i
    if last is None:
        return pattern, ""
    return pattern[1:last], pattern[last+1:]

def matching_paren(text, open_idx):
    depth=0; esc=False; cls=False
    for i in range(open_idx,len(text)):
        ch=text[i]
        if esc:
            esc=False; continue
        if ch=="\\":
            esc=True; continue
        if cls:
            if ch=="]": cls=False
            continue
        if ch=="[":
            cls=True; continue
        if ch=="(":
            depth+=1
        elif ch==")":
            depth-=1
            if depth==0: return i
    return None

def normalize_lq_term(term):
    term=term.strip()
    term=re.sub(r"^\\b","",term)
    term=re.sub(r"\\b$","",term)
    # Keep the tested StreamNZB forms deterministic/readable.
    term=term.replace("jennaortega(UHD)?","jennaortega(?:UHD)?")
    term=term.replace("VISIONPLUSHDR(-X|1000)?","VISIONPLUSHDR(?:-X|1000)?")
    term=term.replace("YTS(.(MX|LT|AG))?","YTS(?:\\.(?:MX|LT|AG))?")
    term=term.replace("Pahe(\\.(ph|in))?","Pahe(?:\\.(?:ph|in))?")
    return term

def lq_pattern_terms(pattern):
    """Extract the group alternatives from Vidhin's simple Radarr/Sonarr LQ regexes."""
    body,flags=js_regex_parts(pattern)
    case_sensitive="i" not in flags
    terms=[]
    for branch in split_top_level(body):
        branch=branch.strip()
        if branch.startswith(r"\b("):
            open_idx=branch.find("(")
            close_idx=matching_paren(branch,open_idx)
            if close_idx is None:
                raise ValueError(f"Unbalanced LQ source regex: {pattern}")
            inner=branch[open_idx+1:close_idx]
            terms.extend(normalize_lq_term(x) for x in split_top_level(inner))
        else:
            term=normalize_lq_term(branch)
            if term:
                terms.append(term)
    return dedupe_casefold(terms),case_sensitive

def target_cfgs(mapping):
    tg=mapping.get("targets")
    if not isinstance(tg,dict): raise KeyError("mapping.targets missing")
    # Backward-compatible list schema
    if tg and isinstance(next(iter(tg.values())),list):
        return {k:{"sources":v,"scope":"unknown","field":"group"} for k,v in tg.items()}
    return tg

def resolve(mapping,upstream):
    by={}
    for rec in rows(upstream):
        name=n(rec); pat=p(rec)
        if name and pat: by.setdefault(name,[]).append(pat)
    out={}; missing=[]
    for target,cfg in target_cfgs(mapping).items():
        recs=[]
        mode=cfg.get("mode","standard")
        for src in cfg["sources"]:
            pats=by.get(src,[])
            if not pats: missing.append(f"{target}: {src}")
            for pat in pats:
                if mode=="lq":
                    tt,case_sensitive=lq_pattern_terms(pat)
                    recs.append({
                        "source":src,"pattern":pat,"tokens":tt,
                        "case_sensitive":case_sensitive
                    })
                else:
                    recs.append({"source":src,"pattern":pat,"tokens":semantic_tokens(pat)})
        toks=set()
        for r in recs: toks.update(r["tokens"])
        toks.update(cfg.get("add_tokens",[]))
        toks.difference_update(cfg.get("remove_tokens",[]))
        out[target]={
            "sources":cfg["sources"],"scope":cfg["scope"],"field":cfg["field"],
            "mode":mode,"records":recs,"tokens":dedupe_casefold(toks)
        }
        if mode=="lq":
            out[target]["extra_exact_groups"]=cfg.get("extra_exact_groups",[])
            out[target]["release_name_fallbacks"]=cfg.get("release_name_fallbacks",[])
    if missing: raise RuntimeError("Mapped upstream rule(s) missing:\n- "+"\n- ".join(missing))
    return out

def read_old(path):
    path=Path(path)
    if not path.exists(): return {}
    old=jload(path)
    if isinstance(old.get("defines"),dict): return old["defines"]
    # v1 shape: parse raw records again with current parser
    if isinstance(old.get("targets"),dict):
        conv={}
        for target,entry in old["targets"].items():
            recs=[]; toks=set()
            for src in entry.get("sources",[]):
                sn=src.get("name")
                for r in src.get("records",[]):
                    pat=r.get("pattern")
                    if pat:
                        tt=semantic_tokens(pat); toks.update(tt)
                        recs.append({"source":sn,"pattern":pat,"tokens":tt})
            conv[target]={"records":recs,"tokens":dedupe_casefold(toks)}
        return conv
    return {}

def toks(entry):
    if isinstance(entry.get("tokens"),list): return set(entry["tokens"])
    o=set()
    for r in entry.get("records",[]): o.update(r.get("tokens",[]))
    return o

def apply_web_precedence(current,mapping):
    current=json.loads(json.dumps(current))
    fams={}
    for name,cfg in target_cfgs(mapping).items():
        fam=cfg.get("tier_family"); tier=cfg.get("tier")
        if fam and tier: fams.setdefault(fam,[]).append((tier,name))
    for fam,items in fams.items():
        seen=set()
        for _,name in sorted(items):
            eff=set(current[name]["tokens"])-seen
            current[name]["effective_tokens"]=dedupe_casefold(eff)
            seen.update(eff)
    for name,e in current.items():
        e.setdefault("effective_tokens",list(e["tokens"]))
    return current

def render_lq_condition(entry):
    conditions=[]
    for exact in entry.get("extra_exact_groups",[]):
        conditions.append(f'group == "{exact}"')
    for rec in entry.get("records",[]):
        rt=rec.get("tokens",[])
        if not rt: continue
        body="|".join(rt).replace('"','\\"')
        prefix="" if rec.get("case_sensitive",False) else "(?i)"
        if rec.get("case_sensitive",False) and len(rt)==1 and re.fullmatch(r"[A-Za-z0-9_.+-]+",rt[0]):
            conditions.append(f'group == "{rt[0]}"')
        else:
            conditions.append(f'group matches "{prefix}^({body})$"')
    for fallback in entry.get("release_name_fallbacks",[]):
        esc=re.escape(fallback).replace(r"\.","\\.")
        conditions.append(f'releaseName matches "(?i)(?:^|[-._ ]){esc}$"')
    return " or ".join(conditions)

def render(current,mapping):
    current=apply_web_precedence(current,mapping)
    lines=["# Generated from Vidhin05/Releases-Regex.",
           "# Review artifact only; profile.txt is not modified.",""]
    for name in sorted(current):
        e=current[name]
        if e.get("mode")=="lq":
            cond=render_lq_condition(e)
        else:
            body="|".join(e["effective_tokens"]).replace('"','\\"')
            field=e["field"]
            if field=="releaseName":
                cond=f'releaseName matches "(?i)(?:^|[-._ ])(?:{body})$"'
            else:
                cond=f'group matches "(?i)^({body})$"'
        lines.append(f'{name} [{e["scope"]}]: define if {cond}')
    return "\n".join(lines)+"\n"

def report(old,new):
    lines=["# Vidhin sync report",""]; changed=0
    for name in sorted(set(old)|set(new)):
        a=toks(old.get(name,{})); b=toks(new.get(name,{}))
        add=sorted(b-a,key=str.casefold); rem=sorted(a-b,key=str.casefold)
        oldraw={(r.get("source"),r.get("pattern")) for r in old.get(name,{}).get("records",[])}
        newraw={(r.get("source"),r.get("pattern")) for r in new.get(name,{}).get("records",[])}
        raw=oldraw!=newraw
        if not add and not rem and not raw: continue
        changed+=1; lines += [f"## {name}",""]
        if add: lines += ["**Added release-group tokens**"]+[f"- `+ {x}`" for x in add]+[""]
        if rem: lines += ["**Removed release-group tokens**"]+[f"- `- {x}`" for x in rem]+[""]
        if raw and not(add or rem):
            lines += ["Raw upstream regex changed, but the extracted release-group set did not.",""]
    if not changed: lines += ["No mapped Vidhin changes detected.",""]
    lines += ["---",f"Tracked StreamNZB Defines: **{len(new)}**","",
              "> `profile.txt` is not modified. Generated Defines require review.",""]
    return "\n".join(lines),changed

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--mapping",type=Path,default=MAPPING)
    ap.add_argument("--baseline",type=Path,default=BASELINE)
    ap.add_argument("--report",type=Path,default=REPORT)
    ap.add_argument("--library",type=Path,default=LIBRARY)
    ap.add_argument("--upstream-file",type=Path)
    a=ap.parse_args()
    mapping=jload(a.mapping)
    upstream=jload(a.upstream_file) if a.upstream_file else fetch(mapping["upstream_url"])
    cur=resolve(mapping,upstream); old=read_old(a.baseline)
    text,changed=report(old,cur)
    a.library.parent.mkdir(parents=True,exist_ok=True)
    a.library.write_text(render(cur,mapping),encoding="utf-8")
    if changed or not a.baseline.exists():
        payload={"schema_version":3,"mapping_schema_version":mapping["schema_version"],
                 "upstream_url":mapping["upstream_url"],
                 "generated_at_utc":dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat(),
                 "defines":cur}
        a.baseline.parent.mkdir(parents=True,exist_ok=True)
        a.baseline.write_text(json.dumps(payload,indent=2,ensure_ascii=False)+"\n",encoding="utf-8")
        a.report.write_text(text,encoding="utf-8")
        print(f"Detected changes in {changed} mapped Define(s).")
    else:
        print("No mapped Vidhin changes detected.")

if __name__=="__main__": main()
