#!/usr/bin/env python3
import importlib.util, json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("sync",ROOT/"scripts/sync_vidhin.py")
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)

mapping=json.loads((ROOT/"scripts/vidhin_mapping.json").read_text())
baseline=json.loads((ROOT/"generated/vidhin-defines.json").read_text())
golden=json.loads((ROOT/"tests/golden_defines.json").read_text())

# Existing compatibility test if the old raw-pattern baseline is available.
if "targets" in baseline:
    up=[]; seen=set()
    for _,entry in baseline["targets"].items():
        for src in entry.get("sources",[]):
            name=src["name"]
            for rec in src.get("records",[]):
                key=(name,rec["pattern"])
                if key not in seen:
                    seen.add(key); up.append({"name":name,"pattern":rec["pattern"],"score":0})
    # Only resolve mappings whose source exists in this historical fixture.
    available={x["name"] for x in up}
    reduced=json.loads(json.dumps(mapping))
    reduced["targets"]={
        k:v for k,v in reduced["targets"].items()
        if all(s in available for s in v["sources"])
    }
    current=m.resolve(reduced,up)
    for name,expect in golden.items():
        assert current[name]["field"]==expect["field"], (name,current[name]["field"],expect["field"])
        vals=set(current[name]["tokens"])
        missing=set(expect["must_include"])-vals
        assert not missing, f"{name}: missing {sorted(missing)}"

    library=m.render(current,reduced)
    assert 'Movies HD BluRay T1 Groups [movie]: define if releaseName matches ' in library
    assert 'Movies Remux T1 Groups [movie]: define if group matches ' in library
    assert m.clean_token("Not-Vodes") is None
    assert "Not-Vodes" not in library
    assert "?<!Not" not in library
    assert "?!-raws" not in library
    assert "?<=remux" not in library
    assert "?!-" not in library
    assert "|Remux|" not in library
else:
    print("Historical raw-pattern compatibility test skipped (baseline already schema v3).")

# Case-insensitive duplicate normalization.
deduped=m.dedupe_casefold(["SiGMA","SIGMA","SbR","sbR","playWEB"])
assert len([x for x in deduped if x.casefold()=="sigma"]) == 1
assert len([x for x in deduped if x.casefold()=="sbr"]) == 1
assert "playWEB" in deduped

# Deterministic LQ fixture from current Vidhin patterns.
lq_upstream=[
    {
        "name":"LQ (Radarr)",
        "pattern":r"/\b(beAst|COLLECTiVE|EPiC|iVy|KiNGDOM|LUCY|Scene|SUNSCREEN|SyncUP)\b/",
        "score":0
    },
    {
        "name":"LQ (Radarr)",
        "pattern":r"/\b(24xHD|41RGB|EVO|Feranki1980|GalaxyRG|jennaortega(UHD)?|VISIONPLUSHDR(-X|1000)?|YIFY|YTS(.(MX|LT|AG))?|Zero00)\b|Pahe(\.(ph|in))?\b|\bx265-E/i",
        "score":0
    },
    {
        "name":"LQ (Sonarr)",
        "pattern":r"/\b(iVy)\b/",
        "score":0
    },
    {
        "name":"LQ (Sonarr)",
        "pattern":r"/\b(BRiNK|BTM|CHX|Feranki1980|MeGusta|PSA|Zero00)\b|Pahe(\.(ph|in))?\b/i",
        "score":0
    }
]
lq_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Movies LQ Groups":mapping["targets"]["Movies LQ Groups"],
        "Shows LQ Groups":mapping["targets"]["Shows LQ Groups"]
    }
}
lq=m.resolve(lq_mapping,lq_upstream)
lib=m.render(lq,lq_mapping)

assert "Movies LQ Groups [movie]: define if " in lib
assert 'group == "E"' in lib
assert 'releaseName matches "(?i)(?:^|[-._ ])24xHD$"' in lib
assert 'releaseName matches "(?i)(?:^|[-._ ])E$"' in lib
assert "jennaortega(?:UHD)?" in lib
assert r"YTS(?:\.(?:MX|LT|AG))?" in lib
assert r"Pahe(?:\.(?:ph|in))?" in lib
assert "x265-E" in lib

assert "Shows LQ Groups [series]: define if " in lib
assert 'group == "iVy"' in lib
assert 'releaseName matches "(?i)(?:^|[-._ ])Feranki1980$"' in lib
assert len(lq)==2

# Bad Dual Groups must preserve Vidhin's raw group-regex semantics.
#
# Unlike the tier regexes, these upstream patterns are simple group regexes
# rather than lookahead classifiers. They must therefore remain raw regexes
# instead of being flattened through semantic_tokens().
bad_dual_upstream=[
    {
        "name":"Radarr Bad Dual Groups",
        "pattern":(
            r"/\b(alfaHD.*|BAT|C\.A\.A|MGE|ONLYMOViE|TM|TvR|ZNM)\b/i"
        ),
        "score":0
    },
    {
        "name":"Sonarr Bad Dual Groups",
        "pattern":(
            r"/\b(alfaHD.*|BAT|BiOMA|C\.A\.A|ZNM)\b/i"
        ),
        "score":0
    }
]

bad_dual_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Movies Bad Dual Groups":{
            "sources":["Radarr Bad Dual Groups"],
            "scope":"movie",
            "field":"group",
            "mode":"raw_regex",
        },
        "Shows Bad Dual Groups":{
            "sources":["Sonarr Bad Dual Groups"],
            "scope":"series",
            "field":"group",
            "mode":"raw_regex",
        },
    }
}

bad_dual=m.resolve(
    bad_dual_mapping,
    bad_dual_upstream,
)

assert len(bad_dual)==2

movie_bad_dual=bad_dual["Movies Bad Dual Groups"]
show_bad_dual=bad_dual["Shows Bad Dual Groups"]

assert movie_bad_dual["scope"]=="movie"
assert movie_bad_dual["field"]=="group"
assert movie_bad_dual["mode"]=="raw_regex"
assert movie_bad_dual["tokens"]==[]

assert show_bad_dual["scope"]=="series"
assert show_bad_dual["field"]=="group"
assert show_bad_dual["mode"]=="raw_regex"
assert show_bad_dual["tokens"]==[]

movie_pattern=movie_bad_dual["records"][0]["raw_regex_pattern"]
show_pattern=show_bad_dual["records"][0]["raw_regex_pattern"]

assert movie_pattern.startswith(r"(?i)\b(")
assert show_pattern.startswith(r"(?i)\b(")

# Preserve regex semantics rather than flattening group names.
assert "alfaHD.*" in movie_pattern
assert r"C\.A\.A" in movie_pattern
assert "alfaHD.*" in show_pattern
assert r"C\.A\.A" in show_pattern

# Radarr/Sonarr differences must remain intact.
assert "MGE" in movie_pattern
assert "ONLYMOViE" in movie_pattern
assert "TM" in movie_pattern
assert "TvR" in movie_pattern
assert "BiOMA" not in movie_pattern

assert "BiOMA" in show_pattern
assert "MGE" not in show_pattern
assert "ONLYMOViE" not in show_pattern
assert "TM" not in show_pattern
assert "TvR" not in show_pattern

bad_dual_library=m.render(
    bad_dual,
    bad_dual_mapping,
)

assert (
    'Movies Bad Dual Groups [movie]: define if group matches '
    in bad_dual_library
)
assert (
    'Shows Bad Dual Groups [series]: define if group matches '
    in bad_dual_library
)

assert r"(?i)\b(alfaHD.*|BAT" in bad_dual_library
assert r"C\.A\.A" in bad_dual_library

# Missing upstream sources must fail closed.
missing_bad_dual_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Missing Bad Dual Groups":{
            "sources":["Missing Bad Dual Source"],
            "scope":"movie",
            "field":"group",
            "mode":"raw_regex",
        }
    }
}

try:
    m.resolve(
        missing_bad_dual_mapping,
        bad_dual_upstream,
    )
except RuntimeError as exc:
    assert "Missing Bad Dual Source" in str(exc)
else:
    raise AssertionError(
        "Missing Bad Dual upstream source was not rejected"
    )

# Unsupported JavaScript regex flags must fail closed.
try:
    m.raw_regex_pattern(r"/\bBAT\b/g")
except ValueError as exc:
    assert "Unsupported raw regex flags" in str(exc)
else:
    raise AssertionError(
        "Unsupported raw-regex flag was not rejected"
    )

# Existing raw_release_name behavior remains backward-compatible.
assert (
    m.raw_release_name_pattern(r"/\bExample\b/i")
    ==
    m.raw_regex_pattern(r"/\bExample\b/i")
)


# raw_regex reporting must distinguish additions, changes and removals.
raw_regex_report_mapping={
    "targets":{
        "Bad Dual":{
            "sources":["Radarr Bad Dual Groups"],
            "scope":"movie",
            "field":"group",
            "mode":"raw_regex",
        }
    }
}

raw_regex_v1={
    "Bad Dual":{
        "sources":["Radarr Bad Dual Groups"],
        "scope":"movie",
        "field":"group",
        "mode":"raw_regex",
        "records":[{
            "source":"Radarr Bad Dual Groups",
            "pattern":r"/\b(BAT|ZNM)\b/i",
            "tokens":[],
            "raw_regex_pattern":r"(?i)\b(BAT|ZNM)\b",
        }],
        "tokens":[],
    }
}

raw_regex_v2={
    "Bad Dual":{
        "sources":["Radarr Bad Dual Groups"],
        "scope":"movie",
        "field":"group",
        "mode":"raw_regex",
        "records":[{
            "source":"Radarr Bad Dual Groups",
            "pattern":r"/\b(BAT|ZNM|MGE)\b/i",
            "tokens":[],
            "raw_regex_pattern":r"(?i)\b(BAT|ZNM|MGE)\b",
        }],
        "tokens":[],
    }
}

added_report,added_changed=m.report(
    {},
    raw_regex_v1,
    raw_regex_report_mapping,
)

assert added_changed==1
assert "**Raw regex added**" in added_report
assert "Raw upstream regex changed, but the extracted release-group set did not." not in added_report

changed_report,changed_changed=m.report(
    raw_regex_v1,
    raw_regex_v2,
    raw_regex_report_mapping,
)

assert changed_changed==1
assert "**Raw regex changed**" in changed_report
assert "Raw upstream regex changed, but the extracted release-group set did not." not in changed_report

removed_report,removed_changed=m.report(
    raw_regex_v1,
    {},
    raw_regex_report_mapping,
)

assert removed_changed==1
assert "**Raw regex removed**" in removed_report



# Obfuscated must preserve Vidhin's Radarr/Sonarr marker families while
# translating only the two PCRE lookbehinds that Go regexp cannot compile.
obfuscated_upstream=[
    {
        "name":"Obfuscated (Radarr)",
        "pattern":(
            r"/-4P\b|-4Planet\b|-AsRequested\b|-BUYMORE\b|"
            r"-Chamele0n\b|-GEROV\b|-iNC0GNiTO\b|-NZBGeek\b|"
            r"-Obfuscated\b|-postbot\b|-Rakuv\b|"
            r"(?<=\b[12]\d{3}\b).*(Scrambled)\b|"
            r"-WhiteRev\b|-xpost\b|-WRTEAM\b|-CAPTCHA\b|_nzb\b/i"
        ),
        "score":0,
    },
    {
        "name":"Obfuscated (Sonarr)",
        "pattern":(
            r"/-4P\b|-4Planet\b|-AsRequested\b|-BUYMORE\b|"
            r"-Chamele0n\b|-GEROV\b|-iNC0GNiTO\b|-NZBGeek\b|"
            r"-Obfuscated\b|-postbot\b|-Rakuv\b|"
            r"(?<=\bS\d+\b).*(Scrambled)\b|"
            r"-WhiteRev\b|-xpost\b|-WRTEAM\b|-CAPTCHA\b|_nzb\b/i"
        ),
        "score":0,
    },
]

obfuscated_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Movies Obfuscated":{
            "sources":["Obfuscated (Radarr)"],
            "scope":None,
            "field":"releaseName",
            "mode":"obfuscated",
        },
        "Shows Obfuscated":{
            "sources":["Obfuscated (Sonarr)"],
            "scope":None,
            "field":"releaseName",
            "mode":"obfuscated",
        },
    },
}

obfuscated=m.resolve(
    obfuscated_mapping,
    obfuscated_upstream,
)

assert len(obfuscated)==2

movie_obfuscated=obfuscated["Movies Obfuscated"]
show_obfuscated=obfuscated["Shows Obfuscated"]

movie_pattern=(
    movie_obfuscated["records"][0]["obfuscated_regex_pattern"]
)
show_pattern=(
    show_obfuscated["records"][0]["obfuscated_regex_pattern"]
)

assert movie_pattern.startswith("(?i)")
assert show_pattern.startswith("(?i)")

assert "(?<=" not in movie_pattern
assert "(?<=" not in show_pattern
assert "(?<!" not in movie_pattern
assert "(?<!" not in show_pattern

assert r"\b[12]\d{3}\b.*Scrambled\b" in movie_pattern
assert r"\bS\d+\b.*Scrambled\b" in show_pattern

for marker in (
    "-4P",
    "-4Planet",
    "-AsRequested",
    "-BUYMORE",
    "-Chamele0n",
    "-GEROV",
    "-iNC0GNiTO",
    "-NZBGeek",
    "-Obfuscated",
    "-postbot",
    "-Rakuv",
    "-WhiteRev",
    "-xpost",
    "-WRTEAM",
    "-CAPTCHA",
    "_nzb",
):
    assert marker in movie_pattern
    assert marker in show_pattern

obfuscated_library=m.render(
    obfuscated,
    obfuscated_mapping,
)

assert "Movies Obfuscated: define if releaseName matches " in obfuscated_library
assert "Shows Obfuscated: define if releaseName matches " in obfuscated_library
assert "(?<=" not in obfuscated_library
assert "(?<!" not in obfuscated_library

# Upstream translation must fail closed if the expected lookbehind changes.
changed_obfuscated_upstream=[
    {
        "name":"Obfuscated (Radarr)",
        "pattern":r"/-Obfuscated\b|Scrambled\b/i",
        "score":0,
    }
]

try:
    m.resolve(
        {
            "schema_version":3,
            "upstream_url":"fixture",
            "targets":{
                "Movies Obfuscated":{
                    "sources":["Obfuscated (Radarr)"],
                    "scope":None,
                    "field":"releaseName",
                    "mode":"obfuscated",
                }
            },
        },
        changed_obfuscated_upstream,
    )
except ValueError as exc:
    assert "manual review" in str(exc).lower()
else:
    raise AssertionError(
        "changed Obfuscated upstream regex was not rejected"
    )


# Anime LQ must preserve Vidhin's full release-name regex semantics.
anime_lq_upstream=[
    {
        "name":"Anime LQ Groups",
        "pattern":(
            r"/\b(Anime[ .-]?(Chap|Land|Time)|"
            r"(Baked|Dead|Space)Fish|"
            r"Mini(Freeza|MTBB|sCuba|Theatre))\b|"
            r"\[224\]|-224\b|"
            r"\[(Cerberus|Daddy(Subs)?)\]|"
            r"-(Cerberus|Daddy(Subs)?)\b|"
            r"^\[Ari\]|-Ari$/i"
        ),
        "score":0
    }
]

anime_lq_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Anime LQ Groups":{
            "sources":["Anime LQ Groups"],
            "scope":None,
            "field":"releaseName",
            "mode":"raw_release_name",
            "anime_lq":True,
        }
    }
}

anime_lq=m.resolve(
    anime_lq_mapping,
    anime_lq_upstream,
)

assert len(anime_lq)==1

anime_lq_entry=anime_lq["Anime LQ Groups"]

assert anime_lq_entry["mode"]=="raw_release_name"
assert anime_lq_entry["field"]=="releaseName"
assert anime_lq_entry["tokens"]==[]

anime_lq_library=m.render(
    anime_lq,
    anime_lq_mapping,
)

assert "Anime LQ Groups: define if " in anime_lq_library
assert "Anime LQ Groups [" not in anime_lq_library
assert 'releaseName matches "(?i)' in anime_lq_library
assert r"Anime[ .-]?(Chap|Land|Time)" in anime_lq_library
assert r"(Baked|Dead|Space)Fish" in anime_lq_library
assert r"Mini(Freeza|MTBB|sCuba|Theatre)" in anime_lq_library
assert r"\[224\]" in anime_lq_library
assert r"-224\b" in anime_lq_library
assert r"\[(Cerberus|Daddy(Subs)?)\]" in anime_lq_library
assert r"^\[Ari\]" in anime_lq_library
assert r"-Ari$" in anime_lq_library

# Anime Vodes regression tests.
#
# Vidhin treats Vodes and Not-Vodes as separate release groups:
#   Anime Web T1 -> Vodes
#   Anime Web T2 -> Not-Vodes
#
# Not-Vodes must never cause a Vodes token to leak into T2.
anime_vodes_upstream=[
    {
        "name":"Anime Web T1",
        "pattern":(
            r"/^(?=.*(WEB-DL))"
            r"(?=.*(\[(Arid|smol|SoM|Vodes)\]|"
            r"-(Arid|smol|SoM)\b|"
            r"\b(Arg0|LostYears|SCY|ZeroBuild)\b|"
            r"(?<!Not)-Vodes\b)).*/i"
        ),
        "score":0
    },
    {
        "name":"Anime Web T2",
        "pattern":(
            r"/^(?=.*(WEB-DL))"
            r"(?=.*(\[(Asakura|Cyan|Not-Vodes|Pizza)\]|"
            r"-(Asakura|Cyan|Not-Vodes|Pizza)\b|"
            r"\b(BlackRose|MTBB|Okay-Subs)\b)).*/i"
        ),
        "score":0
    }
]

anime_vodes_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Anime Shows WEB T1 Groups":{
            "sources":["Anime Web T1"],
            "scope":"anime_show",
            "field":"releaseName",
            "anime":True
        },
        "Anime Shows WEB T2 Groups":{
            "sources":["Anime Web T2"],
            "scope":"anime_show",
            "field":"releaseName",
            "anime":True
        }
    }
}

anime_vodes=m.resolve(anime_vodes_mapping,anime_vodes_upstream)

t1=set(anime_vodes["Anime Shows WEB T1 Groups"]["tokens"])
t2=set(anime_vodes["Anime Shows WEB T2 Groups"]["tokens"])

assert "Vodes" in t1
assert "Vodes" not in t2
assert "Not-Vodes" not in t1
assert "Not-Vodes" not in t2

anime_vodes_library=m.render(anime_vodes,anime_vodes_mapping)

assert "Vodes" in anime_vodes_library
assert "Not-Vodes" not in anime_vodes_library

# Canonical casing must be deterministic even when input is an unordered set.
assert m.dedupe_casefold({"SiGMA","SIGMA"}) == ["SIGMA"]
assert m.dedupe_casefold({"sbR","SbR"}) == ["SbR"]

# ---------------------------------------------------------------------------
# Live Anime upstream structure guard
# ---------------------------------------------------------------------------

valid_structure = []

for tier in range(1, 7):
    valid_structure.append({
        "name": f"Anime Web T{tier}",
        "pattern": "/(?=(?:.*))(?=(?:Group))/i",
    })

for tier in range(1, 9):
    valid_structure.append({
        "name": f"Anime BD T{tier}",
        "pattern": "/(?=(?:.*))(?=(?:Group))/i",
    })

# Expected structure must pass.
m.validate_anime_upstream_structure(valid_structure)


# Missing expected tier must fail.
missing_tier = [
    rec for rec in valid_structure
    if rec["name"] != "Anime Web T6"
]

try:
    m.validate_anime_upstream_structure(missing_tier)
except RuntimeError as exc:
    assert "Anime Web T6" in str(exc)
else:
    raise AssertionError(
        "Missing Anime Web T6 was not detected"
    )


# Unexpected new tier must fail.
new_tier = valid_structure + [{
    "name": "Anime Web T7",
    "pattern": "/(?=(?:.*))(?=(?:Group))/i",
}]

try:
    m.validate_anime_upstream_structure(new_tier)
except RuntimeError as exc:
    assert "Anime Web T7" in str(exc)
else:
    raise AssertionError(
        "Unexpected Anime Web T7 was not detected"
    )

# ---------------------------------------------------------------------------
# Generated metadata change detection
#
# Mapping-only changes must trigger regeneration even when the upstream
# regex and extracted tokens are unchanged.
# ---------------------------------------------------------------------------

metadata_old={
    "Anime LQ Groups":{
        "sources":["Anime LQ Groups"],
        "scope":"anime",
        "field":"releaseName",
        "mode":"raw_release_name",
        "records":[{
            "source":"Anime LQ Groups",
            "pattern":r"/\bExample\b/i",
            "tokens":[],
            "raw_release_name_pattern":r"(?i)\bExample\b",
        }],
        "tokens":[],
    }
}

metadata_new={
    "Anime LQ Groups":{
        "sources":["Anime LQ Groups"],
        "scope":None,
        "field":"releaseName",
        "mode":"raw_release_name",
        "records":[{
            "source":"Anime LQ Groups",
            "pattern":r"/\bExample\b/i",
            "tokens":[],
            "raw_release_name_pattern":r"(?i)\bExample\b",
        }],
        "tokens":[],
    }
}

metadata_report,metadata_changed=m.report(
    metadata_old,
    metadata_new,
    mapping,
)

assert metadata_changed==1, (
    "Mapping-only metadata change was not detected"
)

assert "**Generated metadata changed**" in metadata_report

assert "`scope`: `\"anime\"` → `null`" in metadata_report

assert "**Raw release-name regex changed**" not in metadata_report

# ---------------------------------------------------------------------------
# Tier movement reporting
# ---------------------------------------------------------------------------

def tier_entry(tokens):
    return {
        "records": [],
        "tokens": tokens,
    }


# Promotion: T2 -> T1.
movement_mapping={
    "targets":{
        "T1":{
            "sources":["T1"],
            "scope":"anime_movie",
            "field":"releaseName",
            "tier_family":"Anime Movies WEB",
            "tier_report_family":"Anime WEB",
            "tier":1,
        },
        "T2":{
            "sources":["T2"],
            "scope":"anime_movie",
            "field":"releaseName",
            "tier_family":"Anime Movies WEB",
            "tier_report_family":"Anime WEB",
            "tier":2,
        },
    }
}

old={
    "T1":tier_entry([]),
    "T2":tier_entry(["MTBB"]),
}

new={
    "T1":tier_entry(["MTBB"]),
    "T2":tier_entry([]),
}

movements=m.detect_tier_movements(old,new,movement_mapping)

assert len(movements)==1
assert movements[0]["family"]=="Anime WEB"
assert movements[0]["token"]=="MTBB"
assert movements[0]["old_tier"]==2
assert movements[0]["new_tier"]==1

movement_report,_=m.report(old,new,movement_mapping)

assert "## Tier movements" in movement_report
assert "### Anime WEB" in movement_report
assert "`MTBB`: T2 → T1" in movement_report


# Demotion: T1 -> T3.
demotion_mapping={
    "targets":{
        "T1":{
            "sources":["T1"],
            "scope":"movie",
            "field":"group",
            "tier_family":"Movies WEB",
            "tier":1,
        },
        "T3":{
            "sources":["T3"],
            "scope":"movie",
            "field":"group",
            "tier_family":"Movies WEB",
            "tier":3,
        },
    }
}

old={
    "T1":tier_entry(["Example"]),
    "T3":tier_entry([]),
}

new={
    "T1":tier_entry([]),
    "T3":tier_entry(["Example"]),
}

movements=m.detect_tier_movements(old,new,demotion_mapping)

assert len(movements)==1
assert movements[0]["old_tier"]==1
assert movements[0]["new_tier"]==3


# A newly added token is not a tier movement.
old={
    "T1":tier_entry([]),
    "T2":tier_entry([]),
}

new={
    "T1":tier_entry(["NewGroup"]),
    "T2":tier_entry([]),
}

assert m.detect_tier_movements(
    old,new,movement_mapping
)==[]


# A removed token is not a tier movement.
old={
    "T1":tier_entry(["RemovedGroup"]),
    "T2":tier_entry([]),
}

new={
    "T1":tier_entry([]),
    "T2":tier_entry([]),
}

assert m.detect_tier_movements(
    old,new,movement_mapping
)==[]


# Remaining in the same tier is not a movement.
old={
    "T1":tier_entry(["SameGroup"]),
    "T2":tier_entry([]),
}

new={
    "T1":tier_entry(["SameGroup"]),
    "T2":tier_entry([]),
}

assert m.detect_tier_movements(
    old,new,movement_mapping
)==[]


# Case-only spelling changes are not movements.
old={
    "T1":tier_entry(["MTBB"]),
    "T2":tier_entry([]),
}

new={
    "T1":tier_entry(["mtbb"]),
    "T2":tier_entry([]),
}

assert m.detect_tier_movements(
    old,new,movement_mapping
)==[]


# Anime Movie/Show mirrors must collapse into one reported movement.
mirror_mapping={
    "targets":{
        "Movie T1":{
            "sources":["Anime Web T1"],
            "scope":"anime_movie",
            "field":"releaseName",
            "tier_family":"Anime Movies WEB",
            "tier_report_family":"Anime WEB",
            "tier":1,
        },
        "Movie T2":{
            "sources":["Anime Web T2"],
            "scope":"anime_movie",
            "field":"releaseName",
            "tier_family":"Anime Movies WEB",
            "tier_report_family":"Anime WEB",
            "tier":2,
        },
        "Show T1":{
            "sources":["Anime Web T1"],
            "scope":"anime_show",
            "field":"releaseName",
            "tier_family":"Anime Shows WEB",
            "tier_report_family":"Anime WEB",
            "tier":1,
        },
        "Show T2":{
            "sources":["Anime Web T2"],
            "scope":"anime_show",
            "field":"releaseName",
            "tier_family":"Anime Shows WEB",
            "tier_report_family":"Anime WEB",
            "tier":2,
        },
    }
}

old={
    "Movie T1":tier_entry([]),
    "Movie T2":tier_entry(["MTBB"]),
    "Show T1":tier_entry([]),
    "Show T2":tier_entry(["MTBB"]),
}

new={
    "Movie T1":tier_entry(["MTBB"]),
    "Movie T2":tier_entry([]),
    "Show T1":tier_entry(["MTBB"]),
    "Show T2":tier_entry([]),
}

movements=m.detect_tier_movements(
    old,new,mirror_mapping
)

assert len(movements)==1
assert movements[0]["family"]=="Anime WEB"
assert movements[0]["token"]=="MTBB"
assert movements[0]["old_tier"]==2
assert movements[0]["new_tier"]==1


# Ambiguous multi-tier placement must not be reported as a movement.
old={
    "T1":tier_entry(["Ambiguous"]),
    "T2":tier_entry(["Ambiguous"]),
}

new={
    "T1":tier_entry(["Ambiguous"]),
    "T2":tier_entry([]),
}

assert m.detect_tier_movements(
    old,new,movement_mapping
)==[]

print(
    "All v2.4 compatibility, LQ, Anime tier and "
    "tier-movement tests passed."
)

# Dubs Only raw-release-name regression tests.
#
# Vidhin's Dubs Only classification is a release-name classifier rather
# than a simple release-group list. Preserve the complete upstream regex
# semantics, including explicit dub markers, known groups, and Dual/Multi
# exclusions.
dubs_only_upstream=[
    {
        "name":"Dubs Only",
        "pattern":(
            r"/\b(Golumpa|KamiFS|torenter69)\b|"
            r"\[Yameii\]|-Yameii\b|"
            r"^(?!.*(Dual|Multi)[-_. ]?Audio).*"
            r"((?<!multi-)\b(dub(bed)?)\b|(funi|eng(lish)?)_?dub)|"
            r"^(?!.*(dual[ ._-]?audio|(JA|ZH|KO)\+EN|EN\+(JA|ZH|KO))).*"
            r"\b(KaiDubs|KS)\b/i"
        ),
        "score":0,
    }
]

dubs_only_mapping={
    "schema_version":3,
    "upstream_url":"fixture",
    "targets":{
        "Anime Dubs Only":{
            "sources":["Dubs Only"],
            "scope":None,
            "field":"releaseName",
            "mode":"dubs_only",
        }
    },
}

dubs_only=m.resolve(
    dubs_only_mapping,
    dubs_only_upstream,
)

assert len(dubs_only)==1

dubs_entry=dubs_only["Anime Dubs Only"]

assert dubs_entry["scope"] is None
assert dubs_entry["field"]=="releaseName"
assert dubs_entry["mode"]=="dubs_only"
assert dubs_entry["tokens"]==[]

dubs_pattern=dubs_entry["records"][0]["raw_release_name_pattern"]

assert dubs_pattern.startswith("(?i)")
assert "Golumpa" in dubs_pattern
assert "KamiFS" in dubs_pattern
assert "torenter69" in dubs_pattern
assert "Yameii" in dubs_pattern
assert "KaiDubs" in dubs_pattern
assert "(Dual|Multi)" in dubs_pattern
assert "dual[ ._-]?audio" in dubs_pattern
assert "(JA|ZH|KO)\\+EN" in dubs_pattern
assert "EN\\+(JA|ZH|KO)" in dubs_pattern

dubs_library=m.render(
    dubs_only,
    dubs_only_mapping,
)

assert "Anime Dubs Only: define if " in dubs_library
assert "Anime Dubs Only [" not in dubs_library

# The raw upstream classifier is retained for synchronization...
assert "(?!.*(Dual|Multi)" in dubs_pattern
assert "(?<!multi-)" in dubs_pattern

# ...but unsupported PCRE lookaround must never reach StreamNZB.
assert "(?!" not in dubs_library
assert "(?<!" not in dubs_library

assert 'releaseName matches "(?i)\\b(Golumpa|KamiFS|torenter69)' in dubs_library
assert 'releaseName matches "(?i)\\b(dub(bed)?)' in dubs_library
assert 'not (releaseName matches "(?i)(Dual|Multi)[-_. ]?Audio")' in dubs_library
assert 'not (releaseName matches "(?i)multi-\\bdub(bed)?\\b")' in dubs_library
assert 'releaseName matches "(?i)\\b(KaiDubs|KS)\\b"' in dubs_library
assert 'not (releaseName matches "(?i)dual[ ._-]?audio|' in dubs_library

# Derived local helper: published in the StreamNZB Define Library only.
trusted_name = "Trusted Release Groups"
assert trusted_name not in baseline["defines"]

published_library = (
    ROOT / "generated" / "streamnzb-defines.txt"
).read_text()

trusted_line = next(
    line
    for line in published_library.splitlines()
    if line.startswith(f"{trusted_name}: define if ")
)

def is_trusted_tier_name(name):
    prefixes = ("Movies ", "Shows ", "Anime Movies ", "Anime Shows ")
    families = ("UHD BluRay", "HD BluRay", "BluRay", "WEB", "Remux")

    if not name.endswith(" Groups"):
        return False

    core = name[:-len(" Groups")]
    prefix = next((v for v in prefixes if core.startswith(v)), None)
    if prefix is None:
        return False

    rest = core[len(prefix):]
    family = next((v for v in families if rest.startswith(v + " T")), None)
    if family is None:
        return False

    tier = rest[len(family) + 2:]
    return tier.isdigit()

expected_trusted = sorted(
    name
    for name in baseline["defines"]
    if is_trusted_tier_name(name)
)

assert expected_trusted
for define_name in expected_trusted:
    assert f'matched("{define_name}")' in trusted_line

trusted_refs = []
needle = 'matched("'
pos = 0

while True:
    start = trusted_line.find(needle, pos)
    if start < 0:
        break
    name_start = start + len(needle)
    name_end = trusted_line.find('")', name_start)
    assert name_end >= 0
    trusted_refs.append(trusted_line[name_start:name_end])
    pos = name_end + 2

assert sorted(set(trusted_refs)) == expected_trusted
