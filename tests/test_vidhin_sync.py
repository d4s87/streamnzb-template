#!/usr/bin/env python3
import importlib.util, json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("sync",ROOT/"scripts/sync_vidhin.py")
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)

mapping=json.loads((ROOT/"scripts/vidhin_mapping.json").read_text())
baseline=json.loads((ROOT/"generated/vidhin-defines.json").read_text())
# Use the committed v1 raw patterns as deterministic fixture.
if "targets" not in baseline:
    print("Golden parser test skipped: baseline already upgraded to v3.")
    raise SystemExit(0)

# Reconstruct an upstream list from baseline records.
up=[]
seen=set()
for target,entry in baseline["targets"].items():
    for src in entry.get("sources",[]):
        name=src["name"]
        for rec in src.get("records",[]):
            key=(name,rec["pattern"])
            if key not in seen:
                seen.add(key); up.append({"name":name,"pattern":rec["pattern"],"score":0})

current=m.resolve(mapping,up)
golden=json.loads((ROOT/"tests/golden_defines.json").read_text())
for name,expect in golden.items():
    assert current[name]["field"]==expect["field"], (name,current[name]["field"],expect["field"])
    vals=set(current[name]["tokens"])
    missing=set(expect["must_include"])-vals
    assert not missing, f"{name}: missing {sorted(missing)}"

library=m.render(current,mapping)
assert 'Movies HD BluRay T1 Groups [movie]: define if releaseName matches ' in library
assert 'Movies Remux T1 Groups [movie]: define if group matches ' in library
assert m.clean_token("Not-Vodes") is None
assert "Not-Vodes" not in library
print("All v2.3 golden compatibility tests passed.")
