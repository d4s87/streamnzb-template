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
)

assert metadata_changed==1, (
    "Mapping-only metadata change was not detected"
)

assert "**Generated metadata changed**" in metadata_report

assert "`scope`: `\"anime\"` → `null`" in metadata_report

assert "**Raw release-name regex changed**" not in metadata_report

print("All v2.4 compatibility, LQ and Anime tier tests passed.")
