#!/usr/bin/env python3

import base64
import gzip
import json
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

SAMSUNG_PATH = ROOT / "profile.txt"
NEUTRAL_PATH = ROOT / "profile-neutral.txt"
RULES_PATH = ROOT / "profiles" / "rules.json"
VARIANTS_PATH = ROOT / "profiles" / "variants.json"

PREFIX = "SNZBP1:"

DEVICE_RULES = [
    "DV without HDR fallback",
    "Reduce Atmos",
    "Reduce TrueHD bonus",
    "Reduce DTS Lossless bonus",
]

PRESENTATION_RULES = {
    "10bit",
    "B-Global",
    "Bilibili",
    "HIDIVE",
}


def decode(path: Path):
    text = path.read_text(encoding="utf-8").strip()

    if not text.startswith(PREFIX):
        raise AssertionError(
            f"{path.name} does not start with {PREFIX!r}"
        )

    encoded = text[len(PREFIX):]
    encoded += "=" * (-len(encoded) % 4)

    return json.loads(
        gzip.decompress(
            base64.urlsafe_b64decode(encoded)
        ).decode("utf-8")
    )


rules_source = json.loads(
    RULES_PATH.read_text(encoding="utf-8")
)

variants_source = json.loads(
    VARIANTS_PATH.read_text(encoding="utf-8")
)

assert rules_source["schema_version"] == 1
assert rules_source["streamnzb_profile"] == 1
assert len(rules_source["rules"]) == 111

entries = rules_source["rules"]

owners = {}

for entry in entries:
    owners[entry["owner"]] = owners.get(entry["owner"], 0) + 1

assert owners == {
    "core": 103,
    "presentation": 4,
    "device:samsung-qn90a": 4,
}

device_names = [
    entry["rule"]["name"]
    for entry in entries
    if entry["owner"] == "device:samsung-qn90a"
]

assert device_names == DEVICE_RULES

presentation_names = {
    entry["rule"]["name"]
    for entry in entries
    if entry["owner"] == "presentation"
}

assert presentation_names == PRESENTATION_RULES

reject_3d_entries = [
    entry
    for entry in entries
    if entry["rule"]["name"] == "Reject 3D"
]

assert len(reject_3d_entries) == 1
assert reject_3d_entries[0]["owner"] == "core"

variants = {
    variant["artifact"]: variant
    for variant in variants_source["variants"]
}

assert set(variants) == {
    "profile.txt",
    "profile-neutral.txt",
}

assert variants["profile.txt"] == {
    "artifact": "profile.txt",
    "name": "Samsung QN90A",
    "preset": "4k",
    "owners": [
        "core",
        "presentation",
        "device:samsung-qn90a",
    ],
    "expected_rules": 111,
}

assert variants["profile-neutral.txt"] == {
    "artifact": "profile-neutral.txt",
    "name": "DraCuLa Neutral",
    "preset": "4k",
    "owners": [
        "core",
        "presentation",
    ],
    "expected_rules": 107,
}

samsung_before = SAMSUNG_PATH.read_bytes()
neutral_before = NEUTRAL_PATH.read_bytes()

result = subprocess.run(
    [
        "python3",
        str(ROOT / "scripts" / "build_profiles.py"),
        "build",
    ],
    check=False,
)

assert result.returncode == 0

samsung_after = SAMSUNG_PATH.read_bytes()
neutral_after = NEUTRAL_PATH.read_bytes()

assert samsung_after == samsung_before
assert neutral_after == neutral_before

samsung = decode(SAMSUNG_PATH)
neutral = decode(NEUTRAL_PATH)

assert samsung["name"] == "Samsung QN90A"
assert neutral["name"] == "DraCuLa Neutral"

assert samsung["preset"] == "4k"
assert neutral["preset"] == "4k"

assert samsung["streamnzb_profile"] == 1
assert neutral["streamnzb_profile"] == 1

samsung_rules = samsung["rules"]
neutral_rules = neutral["rules"]

assert len(samsung_rules) == 111
assert len(neutral_rules) == 107

samsung_names = [
    rule["name"]
    for rule in samsung_rules
]

neutral_names = [
    rule["name"]
    for rule in neutral_rules
]

device_set = set(DEVICE_RULES)

expected_neutral_names = [
    name
    for name in samsung_names
    if name not in device_set
]

assert neutral_names == expected_neutral_names

removed = [
    name
    for name in samsung_names
    if name not in set(neutral_names)
]

assert removed == DEVICE_RULES

assert "Reject 3D" in samsung_names
assert "Reject 3D" in neutral_names

samsung_by_name = {
    rule["name"]: rule
    for rule in samsung_rules
}

neutral_by_name = {
    rule["name"]: rule
    for rule in neutral_rules
}

for name in neutral_names:
    assert neutral_by_name[name] == samsung_by_name[name]

print("PASS: profile variant generation tests")
