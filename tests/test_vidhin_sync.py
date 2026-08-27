#!/usr/bin/env python3
import importlib.util
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("sync",ROOT/"scripts/sync_vidhin.py")
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)

def check(pattern,expected,label):
    got=set(m.semantic_tokens(pattern)); exp=set(expected)
    assert got==exp, f"{label}: expected {sorted(exp)}, got {sorted(got)}"

check(r'/^(?=.*(?:[_. ]|\d{4}p-|\bHybrid-).*Remux\b)(?=.*\b(3L|ATELiER|BMF)\b).*/i',
      ["3L","ATELiER","BMF"],"remux")
check(r'/^(?=.*(WEB[-_. ]DL|WEBDL|AmazonHD|NetflixU?HD|WebRip))(?=.*\b(?:BYNDR|CMRG|TEPES)\b).*/i',
      ["BYNDR","CMRG","TEPES"],"web")
check(r'/^(?=.*(BluRay|BDMux))(?=.*(\[(Moxie|smol|SoM)\]|-(Moxie|smol|SoM)\b|\b(DemiHuman|FLE|Flugel|LYS1TH3A)\b)).*/i',
      ["Moxie","smol","SoM","DemiHuman","FLE","Flugel","LYS1TH3A"],"anime")
check(r'/^(?=.*(BluRay|BDMux))(?=.*(\[sam\]|-sam\b)).*/',["sam"],"sam")

old={"Movies Remux T1 Groups":{"records":[{"source":"x","pattern":"a","tokens":["ATELiER","BMF"]}]},
     "Movies Remux T2 Groups":{"records":[{"source":"y","pattern":"b","tokens":["NCmt"]}]}}
new={"Movies Remux T1 Groups":{"records":[{"source":"x","pattern":"c","tokens":["BMF"]}]},
     "Movies Remux T2 Groups":{"records":[{"source":"y","pattern":"d","tokens":["ATELiER","NCmt"]}]}}
report,changed=m.report_diff(old,new)
assert changed==2 and "`- ATELiER`" in report and "`+ ATELiER`" in report
print("All v2.2 semantic tests passed.")
