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
    # Vodes is a real group only when Vidhin explicitly matches Vodes.
    # Do not derive it from the separate "Not-Vodes" group.
    has_positive_vodes = (
        r"\[Vodes\]" in classifier
        or r"(?<!Not)-Vodes\b" in classifier       
    )

    if has_positive_vodes:
        out.add("Vodes")
    return dedupe_casefold(out)

def dedupe_casefold(tokens):
    """Deduplicate case-insensitively with deterministic canonical spelling."""
    seen={}
    # Sort first so callers may safely pass sets without causing casing churn
    # between Python processes/runs.
    for token in sorted(tokens, key=lambda x:(x.casefold(),x)):
        key=token.casefold()
        if key not in seen:
            seen[key]=token
    return list(seen.values())

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

def raw_regex_pattern(pattern):
    """
    Preserve a Vidhin regex for direct use by StreamNZB.

    The surrounding JavaScript-style /.../flags delimiters are removed.
    Case-insensitive upstream regexes are represented with an inline (?i)
    prefix so StreamNZB receives the same matching semantics.
    """
    body, flags = js_regex_parts(pattern)

    unsupported = set(flags) - {"i"}
    if unsupported:
        raise ValueError(
            f"Unsupported raw regex flags "
            f"{''.join(sorted(unsupported))!r}: {pattern}"
        )

    if "i" in flags:
        body = "(?i)" + body

    return body


def raw_release_name_pattern(pattern):
    """Backward-compatible wrapper for existing Anime LQ generation."""
    return raw_regex_pattern(pattern)

def target_cfgs(mapping):
    tg=mapping.get("targets")
    if not isinstance(tg,dict): raise KeyError("mapping.targets missing")
    # Backward-compatible list schema
    if tg and isinstance(next(iter(tg.values())),list):
        return {k:{"sources":v,"scope":"unknown","field":"group"} for k,v in tg.items()}
    return tg

def validate_anime_upstream_structure(upstream):
    """
    Guard against unexpected structural changes in Vidhin's live Anime tiers.

    Expected upstream hierarchy:
      - Anime Web T1-T6
      - Anime BD  T1-T8

    Multiple records with the same source name are allowed because Vidhin
    may use supplemental regexes for a tier.

    This validates source/tier structure only. Token collision validation is
    performed after resolve(), using the same semantic parser as generation.
    """
    expected = {
        "Web": {f"Anime Web T{i}" for i in range(1, 7)},
        "BD": {f"Anime BD T{i}" for i in range(1, 9)},
    }

    found = {
        "Web": set(),
        "BD": set(),
    }

    anime_source_re = re.compile(
        r"^Anime (?P<quality>Web|BD) T(?P<tier>\d+)$"
    )

    for rec in rows(upstream):
        name = n(rec)

        if not isinstance(name, str):
            continue

        match = anime_source_re.fullmatch(name)

        if not match:
            continue

        quality = match.group("quality")
        found[quality].add(name)

    problems = []

    for quality in ("Web", "BD"):
        missing = sorted(
            expected[quality] - found[quality]
        )

        unexpected = sorted(
            found[quality] - expected[quality]
        )

        if missing:
            problems.append(
                f"Missing Anime {quality} source(s):\n  - "
                + "\n  - ".join(missing)
            )

        if unexpected:
            problems.append(
                f"Unexpected Anime {quality} source(s):\n  - "
                + "\n  - ".join(unexpected)
            )

    if problems:
        raise RuntimeError(
            "Vidhin upstream Anime tier structure changed.\n\n"
            + "\n\n".join(problems)
            + "\n\nManual review and mapping update required."
        )
    
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
                elif mode=="raw_release_name":
                    recs.append({
                        "source":src,
                        "pattern":pat,
                        "tokens":[],
                        "raw_release_name_pattern":raw_release_name_pattern(pat),
                    })
                elif mode=="raw_regex":
                    recs.append({
                        "source":src,
                        "pattern":pat,
                        "tokens":[],
                        "raw_regex_pattern":raw_regex_pattern(pat),
                    })
                else:
                    recs.append({
                        "source":src,
                        "pattern":pat,
                        "tokens":semantic_tokens(pat)
                    })
        toks=set()
        for r in recs: toks.update(r["tokens"])
        toks.update(cfg.get("add_tokens",[]))
        toks.difference_update(cfg.get("remove_tokens",[]))
        out[target]={
            "sources":cfg["sources"],"scope":cfg["scope"],"field":cfg["field"],
            "mode":mode,"records":recs,"tokens":dedupe_casefold(toks)
        }

        for field in ("tier_family","tier_report_family","tier"):
            if field in cfg:
                out[target][field]=cfg[field]
        if mode=="lq":
            out[target]["extra_exact_groups"]=cfg.get("extra_exact_groups",[])
            out[target]["release_name_fallbacks"]=cfg.get("release_name_fallbacks",[])
    if missing: raise RuntimeError("Mapped upstream rule(s) missing:\n- "+"\n- ".join(missing))
    return out

def validate_anime_tier_collisions(current):
    """
    Detect release-group tokens appearing in multiple tiers of the same
    live Anime quality hierarchy.

    Movie and Show targets intentionally mirror the same upstream sources,
    so the canonical Anime Movies targets are sufficient for this check.

    WEB <-> BluRay overlap is allowed.
    """
    families = (
        ("WEB", 6),
        ("BluRay", 8),
    )

    problems = []

    for quality, max_tier in families:
        seen = {}

        for tier in range(1, max_tier + 1):
            target = f"Anime Movies {quality} T{tier} Groups"

            if target not in current:
                raise RuntimeError(
                    f"Expected resolved Anime target missing: {target}"
                )

            tokens = current[target].get("tokens", [])

            if not tokens:
                raise RuntimeError(
                    f"Resolved Anime target contains no tokens: {target}"
                )

            local_seen = set()

            for token in tokens:
                key = token.casefold()

                if key in local_seen:
                    raise RuntimeError(
                        f"Duplicate token {token!r} inside {target}"
                    )

                local_seen.add(key)

                if key in seen:
                    previous_tier, previous_token = seen[key]

                    problems.append(
                        f"{token!r}: "
                        f"T{previous_tier} ({previous_token!r}) "
                        f"and T{tier}"
                    )
                else:
                    seen[key] = (tier, token)

        if problems:
            break

    if problems:
        raise RuntimeError(
            f"Vidhin upstream Anime {quality} tier collision detected:\n\n  - "
            + "\n  - ".join(problems)
            + "\n\nAutomatic synchronization stopped."
        )

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

def render_raw_release_name_condition(entry):
    conditions=[]

    for rec in entry.get("records",[]):
        pattern=rec.get("raw_release_name_pattern")

        if not pattern:
            continue

        escaped=pattern.replace('"','\\"')
        conditions.append(
            f'releaseName matches "{escaped}"'
        )

    if not conditions:
        raise ValueError(
            "raw_release_name Define contains no usable regex records"
        )

    return " or ".join(conditions)

def render_raw_regex_condition(entry):
    conditions=[]

    field=entry.get("field")

    if field not in {"group","releaseName"}:
        raise ValueError(
            f"raw_regex Define uses unsupported field: {field!r}"
        )

    for rec in entry.get("records",[]):
        pattern=rec.get("raw_regex_pattern")

        if not pattern:
            continue

        escaped=pattern.replace('"','\\\"')
        conditions.append(
            f'{field} matches "{escaped}"'
        )

    if not conditions:
        raise ValueError(
            "raw_regex Define contains no usable regex records"
        )

    return " or ".join(conditions)


def render(current,mapping):
    current=apply_web_precedence(current,mapping)
    lines=["# Generated from Vidhin05/Releases-Regex.",
           "# Review artifact only; profile.txt is not modified.",""]
    for name in sorted(current):
        e=current[name]
        if e.get("mode")=="lq":
            cond=render_lq_condition(e)
        elif e.get("mode")=="raw_release_name":
            cond=render_raw_release_name_condition(e)
        elif e.get("mode")=="raw_regex":
            cond=render_raw_regex_condition(e)
        else:
            body="|".join(e["effective_tokens"]).replace('"','\\"')
            field=e["field"]
            if field=="releaseName":
                cond=f'releaseName matches "(?i)(?:^|[-._ ])(?:{body})$"'
            else:
                cond=f'group matches "(?i)^({body})$"'
        scope=e.get("scope")
        scope_text=f" [{scope}]" if scope else ""
        lines.append(
            f'{name}{scope_text}: define if {cond}'
        )
    return "\n".join(lines)+"\n"

def tier_locations(data,mapping):
    """
    Return the tier location of each release-group token.

    Tokens are keyed case-insensitively. Multiple StreamNZB targets may
    intentionally mirror the same upstream tier (for example Anime Movies
    and Anime Shows), so identical tier locations are deduplicated.
    """
    locations={}

    for name,cfg in target_cfgs(mapping).items():
        family=cfg.get("tier_report_family",cfg.get("tier_family"))
        tier=cfg.get("tier")

        if not family or tier is None or name not in data:
            continue

        for token in toks(data[name]):
            key=(family,token.casefold())

            locations.setdefault(key,{
                "tiers":set(),
                "tokens":set(),
            })

            locations[key]["tiers"].add(tier)
            locations[key]["tokens"].add(token)

    return locations


def detect_tier_movements(old,new,mapping):
    """
    Detect unambiguous release-group movements inside a tier family.

    New/removed groups, case-only spelling changes, cross-family changes,
    and ambiguous multi-tier locations are not reported as movements.
    """
    old_locations=tier_locations(old,mapping)
    new_locations=tier_locations(new,mapping)

    movements=[]

    for key in sorted(
        set(old_locations) & set(new_locations),
        key=lambda x:(x[0].casefold(),x[1]),
    ):
        family,_=key
        old_location=old_locations[key]
        new_location=new_locations[key]

        if len(old_location["tiers"])!=1:
            continue

        if len(new_location["tiers"])!=1:
            continue

        old_tier=next(iter(old_location["tiers"]))
        new_tier=next(iter(new_location["tiers"]))

        if old_tier==new_tier:
            continue

        token=sorted(
            new_location["tokens"],
            key=lambda x:(x.casefold(),x),
        )[0]

        movements.append({
            "family":family,
            "token":token,
            "old_tier":old_tier,
            "new_tier":new_tier,
        })

    return movements

def report(old,new,mapping):
    lines=["# Vidhin sync report",""]; changed=0

    movements=detect_tier_movements(old,new,mapping)

    if movements:
        lines += ["## Tier movements",""]

        families={}

        for movement in movements:
            families.setdefault(
                movement["family"],[]
            ).append(movement)

        for family in sorted(families,key=str.casefold):
            lines += [f"### {family}",""]

            for movement in sorted(
                families[family],
                key=lambda x:(
                    x["token"].casefold(),
                    x["token"],
                ),
            ):
                lines.append(
                    f'- `{movement["token"]}`: '
                    f'T{movement["old_tier"]} → '
                    f'T{movement["new_tier"]}'
                )

            lines.append("")

    metadata_fields=(
        "sources",
        "scope",
        "field",
        "mode",
        "tier_family",
        "tier_report_family",
        "tier",
    )

    for name in sorted(set(old)|set(new)):
        old_entry=old.get(name,{})
        new_entry=new.get(name,{})

        a=toks(old_entry)
        b=toks(new_entry)

        add=sorted(b-a,key=str.casefold)
        rem=sorted(a-b,key=str.casefold)

        oldraw={
            (r.get("source"),r.get("pattern"))
            for r in old_entry.get("records",[])
        }
        newraw={
            (r.get("source"),r.get("pattern"))
            for r in new_entry.get("records",[])
        }

        raw=oldraw!=newraw

        metadata_changes=[]

        if old_entry and new_entry:
            for field in metadata_fields:
                old_value=old_entry.get(field)
                new_value=new_entry.get(field)

                if old_value != new_value:
                    metadata_changes.append(
                        (field,old_value,new_value)
                    )

        if (
            not add
            and not rem
            and not raw
            and not metadata_changes
        ):
            continue

        changed+=1
        lines += [f"## {name}",""]

        old_mode=old_entry.get("mode")
        new_mode=new_entry.get("mode")

        is_raw_release_name=(
            old_mode=="raw_release_name"
            or new_mode=="raw_release_name"
        )

        is_raw_regex=(
            old_mode=="raw_regex"
            or new_mode=="raw_regex"
        )

        if metadata_changes:
            lines += [
                "**Generated metadata changed**",
            ]

            for field,old_value,new_value in metadata_changes:
                lines.append(
                    f"- `{field}`: "
                    f"`{json.dumps(old_value,ensure_ascii=False)}` "
                    f"→ "
                    f"`{json.dumps(new_value,ensure_ascii=False)}`"
                )

            lines.append("")

        if is_raw_regex:
            if not old_entry and new_entry:
                lines += [
                    "**Raw regex added**",
                    "",
                ]
            elif old_entry and not new_entry:
                lines += [
                    "**Raw regex removed**",
                    "",
                ]
            elif raw:
                lines += [
                    "**Raw regex changed**",
                    "",
                ]

            sources=sorted({
                r.get("source")
                for r in new_entry.get("records",[])
                if r.get("source")
            } | {
                r.get("source")
                for r in old_entry.get("records",[])
                if r.get("source")
            })

            if sources:
                lines.append(
                    "- Source: "
                    + ", ".join(f"`{source}`" for source in sources)
                )
                lines.append("")

            continue

        if is_raw_release_name:
            if not old_entry and new_entry:
                lines += [
                    "**Raw release-name regex added**",
                    "",
                ]
            elif old_entry and not new_entry:
                lines += [
                    "**Raw release-name regex removed**",
                    "",
                ]
            elif raw:
                lines += [
                    "**Raw release-name regex changed**",
                    "",
                ]

            sources=sorted({
                r.get("source")
                for r in new_entry.get("records",[])
                if r.get("source")
            })

            if sources:
                lines += [
                    "**Upstream source(s)**",
                    *[f"- `{source}`" for source in sources],
                    "",
                ]

        else:
            if add:
                lines += [
                    "**Added release-group tokens**",
                    *[f"- `+ {x}`" for x in add],
                    "",
                ]

            if rem:
                lines += [
                    "**Removed release-group tokens**",
                    *[f"- `- {x}`" for x in rem],
                    "",
                ]

            if raw and not(add or rem):
                lines += [
                    "Raw upstream regex changed, but the extracted "
                    "release-group set did not.",
                    "",
                ]

        # A raw-release-name entry whose metadata changed but whose regex did
        # not still benefits from identifying the tracked upstream source.
        if (
            is_raw_release_name
            and metadata_changes
            and not raw
        ):
            sources=sorted({
                r.get("source")
                for r in new_entry.get("records",[])
                if r.get("source")
            })

            if sources:
                lines += [
                    "**Upstream source(s)**",
                    *[f"- `{source}`" for source in sources],
                    "",
                ]

    if not changed:
        lines += [
            "No mapped Vidhin changes detected.",
            "",
        ]

    lines += [
        "---",
        f"Tracked StreamNZB Defines: **{len(new)}**",
        "",
        "> `profile.txt` is not modified. Generated Defines require review.",
        "",
    ]

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
    validate_anime_upstream_structure(upstream)
    cur=resolve(mapping,upstream)
    validate_anime_tier_collisions(cur)
    old=read_old(a.baseline)
    text,changed=report(old,cur,mapping)
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
