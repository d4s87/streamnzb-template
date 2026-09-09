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
]

PRESENTATION_RULES = {
    "10bit",
    "B-Global",
    "Bilibili",
    "HIDIVE",
    "Peacock",
    "Paramount+",
    "Criterion Channel",
    "Roku",
    "Syfy",
    "DC Universe",
    "Coupang",
    "DMM TV",
    "FOD",
    "Hotstar",
    "KOCOWA",
    "U-NEXT",
    "Viki",
    "Wavve",
    "WeTV",
    "Youku",
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

assert rules_source["schema_version"] == 2
assert variants_source["schema_version"] == 1
assert rules_source["streamnzb_profile"] == 2

EXPECTED_SCORING = {
    "movie": {
        "size_target_gb": 20,
        "size_weight": 500,
    },
    "anime_movie": {
        "size_target_gb": 20,
        "size_weight": 500,
    },
    "series": {
        "size_target_gb": 6,
        "size_weight": 500,
    },
    "anime_show": {
        "size_target_gb": 6,
        "size_weight": 500,
    },
}

assert rules_source["scoring"] == EXPECTED_SCORING
assert len(rules_source["rules"]) == 144

entries = rules_source["rules"]

owners = {}

for entry in entries:
    owners[entry["owner"]] = owners.get(entry["owner"], 0) + 1

assert owners == {
    "core": 123,
    "presentation": 20,
    "device:samsung-qn90a": 1,
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
    "expected_rules": 144,
}

assert variants["profile-neutral.txt"] == {
    "artifact": "profile-neutral.txt",
    "name": "DraCuLa Neutral",
    "preset": "4k",
    "owners": [
        "core",
        "presentation",
    ],
    "expected_rules": 143,
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

assert samsung["scoring"] == EXPECTED_SCORING
assert neutral["scoring"] == EXPECTED_SCORING
assert samsung["scoring"] == neutral["scoring"]

assert samsung["streamnzb_profile"] == 2
assert neutral["streamnzb_profile"] == 2

samsung_rules = samsung["rules"]
neutral_rules = neutral["rules"]

assert len(samsung_rules) == 144
assert len(neutral_rules) == 143

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

# ---------------------------------------------------------------------------
# Audio neutralization scoping (post scoring-ceiling audit)
#
# Prove build_profiles.py's validate_audio_neutralization_scoping() actually
# fires on drift, not just that the current committed source happens to
# pass. Universal "Neutralize X" rules must never gain an isAnime
# condition; non-Anime "Prefer X" residual bonuses must never lose their
# "not isAnime" scoping; Anime-only neutralizers for previously-untouched
# codecs (AAC/DTS Lossy/Dolby Digital) must stay isAnime-scoped.
# ---------------------------------------------------------------------------

import importlib.util as _importlib_util

_spec = _importlib_util.spec_from_file_location(
    "build_profiles", ROOT / "scripts" / "build_profiles.py"
)
build_profiles = _importlib_util.module_from_spec(_spec)
_spec.loader.exec_module(build_profiles)

_good_entries = (
    [
        {"owner": "core", "rule": {"name": name, "when": '"truehd" in traits'}}
        for name in build_profiles.EXPECTED_UNIVERSAL_AUDIO_NEUTRALIZERS
    ]
    + [
        {
            "owner": "core",
            "rule": {"name": name, "when": 'not isAnime and "truehd" in traits'},
        }
        for name in build_profiles.EXPECTED_NON_ANIME_AUDIO_PREFERENCES
    ]
    + [
        {
            "owner": "core",
            "rule": {"name": name, "when": 'isAnime and "aac" in traits'},
        }
        for name in build_profiles.EXPECTED_ANIME_ONLY_AUDIO_NEUTRALIZERS
    ]
)

# Valid scoping must pass.
build_profiles.validate_audio_neutralization_scoping(_good_entries)


def _with_when(entries, name, when):
    out = []
    for entry in entries:
        if entry["rule"]["name"] == name:
            entry = {
                "owner": entry["owner"],
                "rule": {**entry["rule"], "when": when},
            }
        out.append(entry)
    return out


# A universal neutralizer accidentally gaining an isAnime condition
# (either direction) must fail closed.
_drifted = _with_when(
    _good_entries, "Neutralize TrueHD", 'not isAnime and "truehd" in traits'
)
try:
    build_profiles.validate_audio_neutralization_scoping(_drifted)
except ValueError as exc:
    assert "Neutralize TrueHD" in str(exc)
    assert "universal" in str(exc)
else:
    raise AssertionError(
        "universal neutralizer gaining isAnime was not detected"
    )

# A non-Anime residual preference losing "not isAnime" must fail closed.
_drifted = _with_when(_good_entries, "Prefer Atmos", '"atmos" in traits')
try:
    build_profiles.validate_audio_neutralization_scoping(_drifted)
except ValueError as exc:
    assert "Prefer Atmos" in str(exc)
    assert "non-Anime" in str(exc)
else:
    raise AssertionError(
        "residual preference losing not-isAnime scoping was not detected"
    )

# An Anime-only neutralizer losing its isAnime scoping must fail closed.
_drifted = _with_when(_good_entries, "Neutralize Anime AAC", '"aac" in traits')
try:
    build_profiles.validate_audio_neutralization_scoping(_drifted)
except ValueError as exc:
    assert "Neutralize Anime AAC" in str(exc)
    assert "Anime only" in str(exc)
else:
    raise AssertionError(
        "Anime-only neutralizer losing isAnime scoping was not detected"
    )

# An Anime-only neutralizer accidentally becoming universal (isAnime
# replaced by "not isAnime", i.e. inverted rather than dropped) must
# also fail closed.
_drifted = _with_when(
    _good_entries, "Neutralize Anime AAC", 'not isAnime and "aac" in traits'
)
try:
    build_profiles.validate_audio_neutralization_scoping(_drifted)
except ValueError as exc:
    assert "Neutralize Anime AAC" in str(exc)
else:
    raise AssertionError(
        "Anime-only neutralizer inverted to non-Anime was not detected"
    )

# A missing expected rule must fail closed rather than silently validating.
_incomplete = [
    e for e in _good_entries if e["rule"]["name"] != "Prefer Dolby Digital Plus"
]
try:
    build_profiles.validate_audio_neutralization_scoping(_incomplete)
except ValueError as exc:
    assert "Prefer Dolby Digital Plus" in str(exc)
else:
    raise AssertionError(
        "missing audio neutralization rule was not detected"
    )

# ---------------------------------------------------------------------------
# Dolby Digital ordering-integrity fix (post-V5.2). Prove
# validate_dolby_digital_ordering() actually fires on drift, not just that
# the current committed source happens to pass: "Neutralize Dolby Digital"
# must exist exactly as specified (points -50, exact when clause, no
# scope), "Prefer Dolby Digital Plus" must keep its +25/non-Anime shape,
# and neither "Neutralize Anime Dolby Digital" (subsumed) nor a "Prefer
# Dolby Digital" residual (never intended) may exist.
# ---------------------------------------------------------------------------

_dd_good_entries = [
    {
        "owner": "core",
        "rule": {
            "name": "Neutralize Dolby Digital",
            "points": -50,
            "when": '"dolby_digital" in traits',
        },
    },
    {
        "owner": "core",
        "rule": {
            "name": "Prefer Dolby Digital Plus",
            "points": 25,
            "when": 'not isAnime and "dolby_digital_plus" in traits',
        },
    },
]

# Valid shape must pass.
build_profiles.validate_dolby_digital_ordering(_dd_good_entries)

# Missing "Neutralize Dolby Digital" must fail closed.
_dd_missing = [
    e for e in _dd_good_entries if e["rule"]["name"] != "Neutralize Dolby Digital"
]
try:
    build_profiles.validate_dolby_digital_ordering(_dd_missing)
except ValueError as exc:
    assert "Neutralize Dolby Digital" in str(exc)
else:
    raise AssertionError("missing 'Neutralize Dolby Digital' was not detected")

# Points drift on the neutralizer must fail closed.
_dd_drifted_points = [
    {
        "owner": e["owner"],
        "rule": {**e["rule"], "points": -49} if e["rule"]["name"] == "Neutralize Dolby Digital" else e["rule"],
    }
    for e in _dd_good_entries
]
try:
    build_profiles.validate_dolby_digital_ordering(_dd_drifted_points)
except ValueError as exc:
    assert "points drifted" in str(exc)
else:
    raise AssertionError("'Neutralize Dolby Digital' points drift was not detected")

# Condition drift on the neutralizer must fail closed.
_dd_drifted_when = _with_when(
    _dd_good_entries, "Neutralize Dolby Digital", '"dolby_digital_plus" in traits'
)
try:
    build_profiles.validate_dolby_digital_ordering(_dd_drifted_when)
except ValueError as exc:
    assert "when clause drifted" in str(exc)
else:
    raise AssertionError("'Neutralize Dolby Digital' condition drift was not detected")

# A scope appearing on the universal neutralizer must fail closed.
_dd_scoped = [
    {
        "owner": e["owner"],
        "rule": {**e["rule"], "scope": "movie"} if e["rule"]["name"] == "Neutralize Dolby Digital" else e["rule"],
    }
    for e in _dd_good_entries
]
try:
    build_profiles.validate_dolby_digital_ordering(_dd_scoped)
except ValueError as exc:
    assert "unscoped" in str(exc)
else:
    raise AssertionError("scope appearing on 'Neutralize Dolby Digital' was not detected")

# Prefer Dolby Digital Plus losing its non-Anime scoping must fail closed.
_dd_ddp_universal = _with_when(
    _dd_good_entries, "Prefer Dolby Digital Plus", '"dolby_digital_plus" in traits'
)
try:
    build_profiles.validate_dolby_digital_ordering(_dd_ddp_universal)
except ValueError as exc:
    assert "Prefer Dolby Digital Plus" in str(exc)
else:
    raise AssertionError(
        "'Prefer Dolby Digital Plus' losing non-Anime scoping was not detected"
    )

# The subsumed Anime-only rule reappearing must fail closed.
_dd_resurrected = _dd_good_entries + [
    {
        "owner": "core",
        "rule": {
            "name": "Neutralize Anime Dolby Digital",
            "points": -50,
            "when": 'isAnime and "dolby_digital" in traits',
        },
    }
]
try:
    build_profiles.validate_dolby_digital_ordering(_dd_resurrected)
except ValueError as exc:
    assert "Neutralize Anime Dolby Digital" in str(exc)
else:
    raise AssertionError(
        "resurrected 'Neutralize Anime Dolby Digital' was not detected"
    )

# A never-intended "Prefer Dolby Digital" residual must fail closed.
_dd_prefer_dd = _dd_good_entries + [
    {
        "owner": "core",
        "rule": {
            "name": "Prefer Dolby Digital",
            "points": 10,
            "when": 'not isAnime and "dolby_digital" in traits',
        },
    }
]
try:
    build_profiles.validate_dolby_digital_ordering(_dd_prefer_dd)
except ValueError as exc:
    assert "Prefer Dolby Digital" in str(exc)
else:
    raise AssertionError("unexpected 'Prefer Dolby Digital' residual was not detected")

print("PASS: Dolby Digital ordering-integrity structural guards")

# ---------------------------------------------------------------------------
# Scoring share-code round-trip (schema v2). StreamNZB v5.18.0 / Jhin 0.6.2
# closed Gaisberg/streamnzb#267: SNZBP1 share codes now carry the
# profile-level `scoring` map, marked by `streamnzb_profile: 2`. This proves
# DraCuLa's own encode_payload/decode_share_code round-trip preserves the
# exact four-entry scoring map byte-for-byte through the real code path, and
# that no stray content kind (e.g. a "default" entry) or stray field sneaks
# into either direction.
# ---------------------------------------------------------------------------

_roundtrip_payload = {
    "name": "Round-trip fixture",
    "preset": "4k",
    "scoring": EXPECTED_SCORING,
    "rules": [],
    "streamnzb_profile": 2,
}

_roundtrip_encoded = build_profiles.encode_payload(_roundtrip_payload)
_roundtrip_decoded = build_profiles.decode_share_code(_roundtrip_encoded)

assert _roundtrip_decoded["streamnzb_profile"] == 2
assert _roundtrip_decoded["scoring"] == EXPECTED_SCORING
assert set(_roundtrip_decoded["scoring"].keys()) == {
    "movie",
    "anime_movie",
    "series",
    "anime_show",
}

for _kind, _expected_kind_scoring in EXPECTED_SCORING.items():
    _decoded_kind_scoring = _roundtrip_decoded["scoring"][_kind]

    assert _decoded_kind_scoring == _expected_kind_scoring, (
        f"scoring[{_kind!r}] round-trip mismatch: "
        f"{_decoded_kind_scoring!r} != {_expected_kind_scoring!r}"
    )
    assert set(_decoded_kind_scoring.keys()) == {
        "size_target_gb",
        "size_weight",
    }, f"scoring[{_kind!r}] carries a stray field: {_decoded_kind_scoring!r}"

print("PASS: scoring share-code round-trip (schema v2, all 4 content kinds)")

print("PASS: profile variant generation tests")
