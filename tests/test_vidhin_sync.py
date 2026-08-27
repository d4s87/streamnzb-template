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
print("All v2.4 compatibility and LQ generation tests passed.")

# Canonical casing must be deterministic even when input is an unordered set.
assert m.dedupe_casefold({"SiGMA","SIGMA"}) == ["SIGMA"]
assert m.dedupe_casefold({"sbR","SbR"}) == ["SbR"]
