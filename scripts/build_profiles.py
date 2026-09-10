#!/usr/bin/env python3

import argparse
import base64
import gzip
import io
import json
import os
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

RULES_PATH = ROOT / "profiles" / "rules.json"
VARIANTS_PATH = ROOT / "profiles" / "variants.json"

PROFILE_PREFIX = "SNZBP1:"
EXPECTED_RULES_SOURCE_SCHEMA = 2
EXPECTED_VARIANTS_SOURCE_SCHEMA = 1
EXPECTED_STREAMNZB_SCHEMA = 2

EXPECTED_OWNERS = {
    "core",
    "presentation",
    "device:samsung-qn90a",
}

EXPECTED_DEVICE_RULES = [
    "DV without HDR fallback",
]

EXPECTED_PRESENTATION_RULES = {
    "10bit",
    "B-Global",
    "Bilibili",
    "HIDIVE",
    # Non-Anime streaming-service formatter fallback badges (audited
    # 2026-09-09): zero-point, presentation-only, scoped `not isAnime`,
    # covering only the 16 services proven to have no Jhin v0.6.2 native
    # `.Network` coverage and no PCRE-lookaround-dependent Vidhin regex.
    # Do not add any of the 11 deferred/ambiguous services (bare Max,
    # Movies Anywhere/MA, Google Play, iTunes, Showtime, Stan, Fandango,
    # Comedy Central, TVING, Viu, iQIYI) to this set without a dedicated
    # RE2 boolean-translation + fixture pass first.
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

# High-impact audio normalization contract (post scoring-ceiling audit):
# the universal "Neutralize X" half of each pair must compensate the
# native Jhin score for every content kind, including Anime. The "Prefer
# X" half is the deliberate small residual bonus and must remain
# non-Anime-only, exactly mirroring the existing Neutralize/Prefer HDR10
# Plus split. If either side drifts, Anime's 80-point minimum tier gap
# (already proven razor-thin) can silently be violated again.
EXPECTED_UNIVERSAL_AUDIO_NEUTRALIZERS = {
    "Neutralize TrueHD",
    "Neutralize DTS Lossless",
    "Neutralize Atmos",
    "Neutralize Dolby Digital Plus",
    # Dolby Digital is a scoring-integrity-only universal neutralizer: it
    # has no paired "Prefer Dolby Digital" residual (see
    # EXPECTED_DOLBY_DIGITAL_NEUTRALIZER below). Native +50 previously beat
    # DDP's compensated effective +25 for Movies/Shows; this closes that
    # inversion without adding any new positive score.
    "Neutralize Dolby Digital",
}

EXPECTED_NON_ANIME_AUDIO_PREFERENCES = {
    "Prefer TrueHD",
    "Prefer DTS Lossless",
    "Prefer Atmos",
    "Prefer Dolby Digital Plus",
}

# Codecs Jhin scores natively without any DraCuLa compensation for
# Movies/Shows (their 200-point tier gap safely absorbs +100/+100).
# Anime's 80-point gap cannot, so these two are neutralized for Anime
# only; Movies/Shows must remain untouched. Dolby Digital was formerly a
# third member of this set, but it is now neutralized universally (see
# EXPECTED_DOLBY_DIGITAL_NEUTRALIZER) because its native +50 outranked
# DTS Lossless Plus's compensated effective +25 for Movies/Shows too —
# "Neutralize Anime Dolby Digital" is subsumed and must no longer exist.
EXPECTED_ANIME_ONLY_AUDIO_NEUTRALIZERS = {
    "Neutralize Anime AAC",
    "Neutralize Anime DTS Lossy",
}

# Dolby Digital ordering-integrity fix: native +50 must be neutralized to 0
# for every content kind, universally, with no non-Anime residual "Prefer
# Dolby Digital" rule (unlike the Neutralize/Prefer pairs above, this is a
# pure correctness fix, not a new preference layer) so that Dolby Digital
# Plus's existing effective +25 remains strictly preferred over plain
# Dolby Digital everywhere.
EXPECTED_DOLBY_DIGITAL_NEUTRALIZER = {
    "name": "Neutralize Dolby Digital",
    "points": -50,
    "when": '"dolby_digital" in traits',
}

FORBIDDEN_AUDIO_RULE_NAMES = {
    "Neutralize Anime Dolby Digital",
    "Prefer Dolby Digital",
}

# Video-codec neutralization contract (codec-scoring tier-authority audit):
# StreamNZB's own streaming preset gives AVC/HEVC/AV1 different native ranks
# (+300/+700/+700) regardless of content kind, a +400 swing that overturned
# 7 of 11 production tier families. Unlike the audio split above, the
# tightest actual non-Anime margin measured anywhere in the system (+3, the
# HDR10+/lossless-audio physical-media combo) leaves no room for any bounded
# residual preference, so all three are neutralized to exactly 0 universally
# with no Anime/non-Anime split and no residual "Prefer" counterpart.
EXPECTED_VIDEO_CODEC_NEUTRALIZERS = {
    "Neutralize AVC": (-300, "avc"),
    "Neutralize HEVC": (-700, "hevc"),
    "Neutralize AV1": (-700, "av1"),
}


def load_json(path: Path):
    return json.loads(
        path.read_text(encoding="utf-8")
    )


def atomic_write(path: Path, text: str):
    path.parent.mkdir(parents=True, exist_ok=True)

    fd, temporary = tempfile.mkstemp(
        dir=path.parent,
        prefix=f".{path.name}.",
        text=True,
    )

    try:
        with os.fdopen(
            fd,
            "w",
            encoding="utf-8",
        ) as handle:
            handle.write(text)

        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def encode_payload(payload: dict) -> str:
    raw = json.dumps(
        payload,
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode("utf-8")

    buffer = io.BytesIO()

    with gzip.GzipFile(
        fileobj=buffer,
        mode="wb",
        compresslevel=9,
        mtime=0,
    ) as gz:
        gz.write(raw)

    encoded = base64.urlsafe_b64encode(
        buffer.getvalue()
    ).decode("ascii").rstrip("=")

    return PROFILE_PREFIX + encoded + "\n"


def decode_share_code(text: str) -> dict:
    code = text.strip()

    if "\n" in code or "\r" in code:
        raise ValueError(
            "profile artifact must contain exactly one share code"
        )

    if not code.startswith(PROFILE_PREFIX):
        raise ValueError(
            f"profile artifact does not start with {PROFILE_PREFIX!r}"
        )

    encoded = code[len(PROFILE_PREFIX):]

    if not encoded:
        raise ValueError("profile share code has no payload")

    encoded += "=" * (-len(encoded) % 4)

    compressed = base64.urlsafe_b64decode(encoded)
    raw = gzip.decompress(compressed)
    payload = json.loads(raw.decode("utf-8"))

    if not isinstance(payload, dict):
        raise ValueError(
            "decoded profile payload must be an object"
        )

    return payload


def validate_registry(payload: dict):
    if set(payload) != {
        "schema_version",
        "streamnzb_profile",
        "scoring",
        "rules",
    }:
        raise ValueError(
            "rules source keys differ from expected schema"
        )

    if payload["schema_version"] != EXPECTED_RULES_SOURCE_SCHEMA:
        raise ValueError(
            "unsupported rules source schema"
        )

    expected_scoring = {
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

    if payload["scoring"] != expected_scoring:
        raise ValueError(
            "rules source scoring policy differs from expected "
            "bounded size policy"
        )

    if payload["streamnzb_profile"] != EXPECTED_STREAMNZB_SCHEMA:
        raise ValueError(
            "unsupported StreamNZB profile schema"
        )

    entries = payload["rules"]

    if not isinstance(entries, list):
        raise ValueError("rules source must contain a rules array")

    if len(entries) != 146:
        raise ValueError(
            f"expected 146 source rules, found {len(entries)}"
        )

    names = []
    owner_counts = {}

    for index, entry in enumerate(entries, start=1):
        if not isinstance(entry, dict):
            raise ValueError(
                f"source entry #{index} is not an object"
            )

        if set(entry) != {"owner", "rule"}:
            raise ValueError(
                f"source entry #{index} has unexpected keys"
            )

        owner = entry["owner"]
        rule = entry["rule"]

        if owner not in EXPECTED_OWNERS:
            raise ValueError(
                f"source entry #{index} has unknown owner {owner!r}"
            )

        if not isinstance(rule, dict):
            raise ValueError(
                f"source entry #{index} rule is not an object"
            )

        name = rule.get("name")

        if not isinstance(name, str) or not name.strip():
            raise ValueError(
                f"source entry #{index} has invalid rule name"
            )

        names.append(name)
        owner_counts[owner] = owner_counts.get(owner, 0) + 1

    if len(set(names)) != len(names):
        raise ValueError("source contains duplicate rule names")

    expected_counts = {
    "core": 125,
    "presentation": 20,
    "device:samsung-qn90a": 1,
}

    if owner_counts != expected_counts:
        raise ValueError(
            f"unexpected ownership counts: {owner_counts!r}"
        )

    device_names = [
        entry["rule"]["name"]
        for entry in entries
        if entry["owner"] == "device:samsung-qn90a"
    ]

    if device_names != EXPECTED_DEVICE_RULES:
        raise ValueError(
            "Samsung device-rule membership/order differs"
        )

    presentation_names = {
        entry["rule"]["name"]
        for entry in entries
        if entry["owner"] == "presentation"
    }

    if presentation_names != EXPECTED_PRESENTATION_RULES:
        raise ValueError(
            "presentation-rule membership differs"
        )

    reject_3d = [
        entry
        for entry in entries
        if entry["rule"]["name"] == "Reject 3D"
    ]

    if len(reject_3d) != 1:
        raise ValueError(
            f"expected one Reject 3D rule, found {len(reject_3d)}"
        )

    if reject_3d[0]["owner"] != "core":
        raise ValueError(
            "Reject 3D must remain owned by core"
        )

    validate_audio_neutralization_scoping(entries)
    validate_dolby_digital_ordering(entries)
    validate_video_codec_neutralization_scoping(entries)
    validate_retag_rules(entries)
    validate_edition_neutralization_scoping(entries)

    return entries


def validate_audio_neutralization_scoping(entries):
    by_name = {
        entry["rule"]["name"]: entry["rule"]
        for entry in entries
    }

    expected_names = (
        EXPECTED_UNIVERSAL_AUDIO_NEUTRALIZERS
        | EXPECTED_NON_ANIME_AUDIO_PREFERENCES
        | EXPECTED_ANIME_ONLY_AUDIO_NEUTRALIZERS
    )

    missing = expected_names - set(by_name)

    if missing:
        raise ValueError(
            "expected audio neutralization rule(s) missing: "
            + ", ".join(sorted(missing))
        )

    for name in EXPECTED_UNIVERSAL_AUDIO_NEUTRALIZERS:
        when = by_name[name]["when"]

        if "isAnime" in when:
            raise ValueError(
                f"{name!r} must remain universal (compensate the native "
                "score for every content kind, including Anime); found "
                f"an isAnime condition in its when clause: {when!r}"
            )

    for name in EXPECTED_NON_ANIME_AUDIO_PREFERENCES:
        when = by_name[name]["when"]

        if "not isAnime" not in when:
            raise ValueError(
                f"{name!r} must remain scoped to non-Anime content "
                "(the deliberate residual bonus must not reach Anime's "
                f"80-point minimum tier gap); when clause: {when!r}"
            )

    for name in EXPECTED_ANIME_ONLY_AUDIO_NEUTRALIZERS:
        when = by_name[name]["when"]

        if "not isAnime" in when or "isAnime" not in when:
            raise ValueError(
                f"{name!r} must remain scoped to Anime only (Movies/Shows "
                "keep this native codec score untouched); "
                f"when clause: {when!r}"
            )


def validate_dolby_digital_ordering(entries):
    by_name = {
        entry["rule"]["name"]: entry["rule"]
        for entry in entries
    }

    dd_rule = by_name.get("Neutralize Dolby Digital")

    if dd_rule is None:
        raise ValueError("expected rule missing: 'Neutralize Dolby Digital'")

    if dd_rule["points"] != EXPECTED_DOLBY_DIGITAL_NEUTRALIZER["points"]:
        raise ValueError(
            "'Neutralize Dolby Digital' points drifted: expected "
            f"{EXPECTED_DOLBY_DIGITAL_NEUTRALIZER['points']}, "
            f"found {dd_rule['points']}"
        )

    if dd_rule["when"] != EXPECTED_DOLBY_DIGITAL_NEUTRALIZER["when"]:
        raise ValueError(
            "'Neutralize Dolby Digital' when clause drifted: expected "
            f"{EXPECTED_DOLBY_DIGITAL_NEUTRALIZER['when']!r}, "
            f"found {dd_rule['when']!r}"
        )

    if "scope" in dd_rule:
        raise ValueError(
            "'Neutralize Dolby Digital' must remain unscoped (universal); "
            f"found scope {dd_rule['scope']!r}"
        )

    ddp_prefer = by_name.get("Prefer Dolby Digital Plus")

    if ddp_prefer is None:
        raise ValueError("expected rule missing: 'Prefer Dolby Digital Plus'")

    if ddp_prefer["points"] != 25:
        raise ValueError(
            "'Prefer Dolby Digital Plus' points drifted: expected 25, "
            f"found {ddp_prefer['points']}"
        )

    if "not isAnime" not in ddp_prefer["when"]:
        raise ValueError(
            "'Prefer Dolby Digital Plus' must remain scoped to non-Anime "
            f"content; when clause: {ddp_prefer['when']!r}"
        )

    present_forbidden = FORBIDDEN_AUDIO_RULE_NAMES & set(by_name)

    if present_forbidden:
        raise ValueError(
            "Dolby Digital ordering fix subsumed these rule(s); they must "
            "no longer exist: " + ", ".join(sorted(present_forbidden))
        )


def validate_video_codec_neutralization_scoping(entries):
    by_name = {}

    for entry in entries:
        name = entry["rule"]["name"]

        if name not in EXPECTED_VIDEO_CODEC_NEUTRALIZERS:
            continue

        if name in by_name:
            raise ValueError(
                f"{name!r} must appear exactly once, found more than one"
            )

        by_name[name] = entry["rule"]

    missing = set(EXPECTED_VIDEO_CODEC_NEUTRALIZERS) - set(by_name)

    if missing:
        raise ValueError(
            "expected video-codec neutralization rule(s) missing: "
            + ", ".join(sorted(missing))
        )

    for name, (
        expected_points,
        codec_value,
    ) in EXPECTED_VIDEO_CODEC_NEUTRALIZERS.items():
        rule = by_name[name]
        when = rule["when"]
        points = rule["points"]

        if points != expected_points:
            raise ValueError(
                f"{name!r} points drifted: "
                f"expected {expected_points}, found {points}"
            )

        if "isAnime" in when:
            raise ValueError(
                f"{name!r} must remain universal (no Anime/non-Anime "
                f"condition); when clause: {when!r}"
            )

        if "kind" in when:
            raise ValueError(
                f"{name!r} must remain universal (no content-kind "
                f"condition); when clause: {when!r}"
            )

        expected_condition = f'parsed.codec == "{codec_value}"'

        if when != expected_condition:
            raise ValueError(
                f"{name!r} must condition on parsed codec identity only "
                f"({expected_condition!r}); found: {when!r}"
            )

    all_names = {
        entry["rule"]["name"]
        for entry in entries
    }

    forbidden_residuals = {
        "Prefer AVC",
        "Prefer HEVC",
        "Prefer AV1",
    }

    unexpected_residuals = forbidden_residuals & all_names

    if unexpected_residuals:
        raise ValueError(
            "unexpected video-codec residual preference rule(s) found "
            "without a deliberate reviewed change: "
            + ", ".join(sorted(unexpected_residuals))
        )


# Retag audit contract: the pre-existing "Retag Soft Penalty" rule matches
# known redistribution-site markers (.heb, EZTV, RARBG, RARTV, TGx) and must
# stay exactly as-is. "Literal RETAG Soft Penalty" is a separate, later rule
# for a standalone scene RETAG token — a different semantic signal that
# happens to share the same -1 magnitude, kept as an independent rule
# deliberately so each stays independently testable/evolvable.
EXPECTED_REDISTRIBUTION_RETAG_RULE = {
    "name": "Retag Soft Penalty",
    "when": 'matched("Retag Markers")',
    "points": -1,
}

EXPECTED_LITERAL_RETAG_RULE = {
    "name": "Literal RETAG Soft Penalty",
    "when": 'releaseName matches "(?i)(?:^|[. _\\[-])RETAG(?:$|[. _\\]-])"',
    "points": -1,
}


def validate_retag_rules(entries):
    by_name = {}

    for entry in entries:
        name = entry["rule"]["name"]

        if name not in (
            EXPECTED_REDISTRIBUTION_RETAG_RULE["name"],
            EXPECTED_LITERAL_RETAG_RULE["name"],
        ):
            continue

        if name in by_name:
            raise ValueError(
                f"{name!r} must appear exactly once, found more than one"
            )

        by_name[name] = entry

    for expected in (
        EXPECTED_REDISTRIBUTION_RETAG_RULE,
        EXPECTED_LITERAL_RETAG_RULE,
    ):
        name = expected["name"]

        if name not in by_name:
            raise ValueError(
                f"expected retag rule missing: {name!r}"
            )

        entry = by_name[name]
        rule = entry["rule"]

        if entry["owner"] != "core":
            raise ValueError(
                f"{name!r} must remain owned by core; "
                f"found owner {entry['owner']!r}"
            )

        if "scope" in rule:
            raise ValueError(
                f"{name!r} must remain universal (no scope key); "
                f"found scope {rule['scope']!r}"
            )

        if rule.get("points") != expected["points"]:
            raise ValueError(
                f"{name!r} points drifted: expected {expected['points']}, "
                f"found {rule.get('points')!r}"
            )

        if rule.get("when") != expected["when"]:
            raise ValueError(
                f"{name!r} condition drifted from its audited form:\n"
                f"  expected: {expected['when']!r}\n"
                f"  found:    {rule.get('when')!r}"
            )

        when = rule.get("when", "")

        if name == EXPECTED_LITERAL_RETAG_RULE["name"]:
            if "releaseName matches" not in when:
                raise ValueError(
                    f"{name!r} must remain a releaseName-based condition"
                )
        elif name == EXPECTED_REDISTRIBUTION_RETAG_RULE["name"]:
            if 'matched("Retag Markers")' != when:
                raise ValueError(
                    f"{name!r} must remain sourced from the Vidhin-synced "
                    '"Retag Markers" Define, not a hand-written condition'
                )


# Edition audit contract: Jhin's scalar Edition field grants a generic
# native AttrEdition rank of +100 for any non-empty parsed value, with zero
# differentiation between the 10 canonical values. "Neutralize Edition" is a
# universal Core rule that removes that native authority unconditionally, so
# every parsed Edition is effective 0 unless an explicit, reviewed DraCuLa
# residual preference independently applies. "Movie Edition Preference"
# restores exactly +25 for Directors Cut/Extended Edition on Movies only;
# every other canonical value (Anniversary, Ultimate, Collectors, Theatrical,
# Uncut, IMAX, Diamond, Remastered) must stay at effective 0 from this rule.
EXPECTED_EDITION_NEUTRALIZER_RULE = {
    "name": "Neutralize Edition",
    "when": 'edition != ""',
    "points": -100,
}

EXPECTED_MOVIE_EDITION_PREFERENCE_RULE = {
    "name": "Movie Edition Preference",
    "when": 'edition == "Directors Cut" or edition == "Extended Edition"',
    "points": 25,
    "scope": "movie",
}


def validate_edition_neutralization_scoping(entries):
    by_name = {}

    for entry in entries:
        name = entry["rule"]["name"]

        if name not in (
            EXPECTED_EDITION_NEUTRALIZER_RULE["name"],
            EXPECTED_MOVIE_EDITION_PREFERENCE_RULE["name"],
        ):
            continue

        if name in by_name:
            raise ValueError(
                f"{name!r} must appear exactly once, found more than one"
            )

        by_name[name] = entry

    neutralizer_name = EXPECTED_EDITION_NEUTRALIZER_RULE["name"]

    if neutralizer_name not in by_name:
        raise ValueError(f"expected edition rule missing: {neutralizer_name!r}")

    neutralizer_entry = by_name[neutralizer_name]
    neutralizer_rule = neutralizer_entry["rule"]

    if neutralizer_entry["owner"] != "core":
        raise ValueError(
            f"{neutralizer_name!r} must remain owned by core; "
            f"found owner {neutralizer_entry['owner']!r}"
        )

    if "scope" in neutralizer_rule:
        raise ValueError(
            f"{neutralizer_name!r} must remain universal (no scope key, "
            "must neutralize the native Edition score for every content "
            f"kind); found scope {neutralizer_rule['scope']!r}"
        )

    if neutralizer_rule.get("points") != EXPECTED_EDITION_NEUTRALIZER_RULE["points"]:
        raise ValueError(
            f"{neutralizer_name!r} points drifted: expected "
            f"{EXPECTED_EDITION_NEUTRALIZER_RULE['points']}, "
            f"found {neutralizer_rule.get('points')!r}"
        )

    if neutralizer_rule.get("when") != EXPECTED_EDITION_NEUTRALIZER_RULE["when"]:
        raise ValueError(
            f"{neutralizer_name!r} condition drifted from its audited "
            f"form:\n  expected: {EXPECTED_EDITION_NEUTRALIZER_RULE['when']!r}\n"
            f"  found:    {neutralizer_rule.get('when')!r}"
        )

    preference_name = EXPECTED_MOVIE_EDITION_PREFERENCE_RULE["name"]

    if preference_name not in by_name:
        raise ValueError(f"expected edition rule missing: {preference_name!r}")

    preference_entry = by_name[preference_name]
    preference_rule = preference_entry["rule"]

    if preference_entry["owner"] != "core":
        raise ValueError(
            f"{preference_name!r} must remain owned by core; "
            f"found owner {preference_entry['owner']!r}"
        )

    if preference_rule.get("scope") != EXPECTED_MOVIE_EDITION_PREFERENCE_RULE["scope"]:
        raise ValueError(
            f"{preference_name!r} must remain scoped to movie only; "
            f"found scope {preference_rule.get('scope')!r}"
        )

    if (
        preference_rule.get("points")
        != EXPECTED_MOVIE_EDITION_PREFERENCE_RULE["points"]
    ):
        raise ValueError(
            f"{preference_name!r} points drifted: expected "
            f"{EXPECTED_MOVIE_EDITION_PREFERENCE_RULE['points']}, "
            f"found {preference_rule.get('points')!r}"
        )

    if (
        preference_rule.get("when")
        != EXPECTED_MOVIE_EDITION_PREFERENCE_RULE["when"]
    ):
        raise ValueError(
            f"{preference_name!r} condition drifted from its audited "
            f"form:\n  expected: "
            f"{EXPECTED_MOVIE_EDITION_PREFERENCE_RULE['when']!r}\n"
            f"  found:    {preference_rule.get('when')!r}"
        )

    all_names = {
        entry["rule"]["name"]
        for entry in entries
    }

    if "IMAX" not in all_names:
        raise ValueError(
            "expected the pre-existing IMAX rule to remain present"
        )

    if "Open matte" not in all_names:
        raise ValueError(
            "expected the pre-existing Open matte rule to remain present"
        )

    forbidden_residuals = {
        "Anniversary Edition Preference",
        "Ultimate Edition Preference",
        "Collectors Edition Preference",
        "Theatrical Preference",
        "Uncut Preference",
        "Diamond Edition Preference",
        "Remastered Preference",
    }

    unexpected_residuals = forbidden_residuals & all_names

    if unexpected_residuals:
        raise ValueError(
            "unexpected edition residual preference rule(s) found without "
            "a deliberate reviewed change: "
            + ", ".join(sorted(unexpected_residuals))
        )


def validate_variants(payload: dict):
    if set(payload) != {"schema_version", "variants"}:
        raise ValueError(
            "variants source keys differ from expected schema"
        )

    if payload["schema_version"] != EXPECTED_VARIANTS_SOURCE_SCHEMA:
        raise ValueError(
            "unsupported variants source schema"
        )

    variants = payload["variants"]

    if not isinstance(variants, list) or len(variants) != 2:
        raise ValueError(
            "expected exactly two profile variants"
        )

    expected = {
        "profile.txt": {
            "name": "Samsung QN90A",
            "preset": "4k",
            "owners": [
                "core",
                "presentation",
                "device:samsung-qn90a",
            ],
            "expected_rules": 146,
        },
        "profile-neutral.txt": {
            "name": "DraCuLa Neutral",
            "preset": "4k",
            "owners": [
                "core",
                "presentation",
            ],
            "expected_rules": 145,
        },
    }

    seen = set()

    for index, variant in enumerate(variants, start=1):
        if not isinstance(variant, dict):
            raise ValueError(
                f"variant #{index} is not an object"
            )

        if set(variant) != {
            "artifact",
            "name",
            "preset",
            "owners",
            "expected_rules",
        }:
            raise ValueError(
                f"variant #{index} has unexpected keys"
            )

        artifact = variant["artifact"]

        if artifact not in expected:
            raise ValueError(
                f"unexpected variant artifact {artifact!r}"
            )

        if artifact in seen:
            raise ValueError(
                f"duplicate variant artifact {artifact!r}"
            )

        seen.add(artifact)

        wanted = expected[artifact]

        for key, value in wanted.items():
            if variant[key] != value:
                raise ValueError(
                    f"{artifact} {key} differs: "
                    f"{variant[key]!r} != {value!r}"
                )

    if seen != set(expected):
        raise ValueError(
            "required variant artifact missing"
        )

    return variants


def build_variant(
    *,
    entries,
    variant,
    streamnzb_profile,
    scoring,
):
    owners = set(variant["owners"])

    rules = [
        entry["rule"]
        for entry in entries
        if entry["owner"] in owners
    ]

    if len(rules) != variant["expected_rules"]:
        raise ValueError(
            f"{variant['artifact']} expected "
            f"{variant['expected_rules']} rules, found {len(rules)}"
        )

    payload = {
        "name": variant["name"],
        "preset": variant["preset"],
        "scoring": scoring,
        "rules": rules,
        "streamnzb_profile": streamnzb_profile,
    }

    return payload


def validate_cross_variant(
    samsung: dict,
    neutral: dict,
):
    samsung_rules = samsung["rules"]
    neutral_rules = neutral["rules"]

    samsung_names = [
        rule["name"]
        for rule in samsung_rules
    ]
    neutral_names = [
        rule["name"]
        for rule in neutral_rules
    ]

    device_set = set(EXPECTED_DEVICE_RULES)

    expected_neutral_names = [
        name
        for name in samsung_names
        if name not in device_set
    ]

    if neutral_names != expected_neutral_names:
        raise ValueError(
            "neutral rule order/content membership differs from "
            "Samsung-minus-device expectation"
        )

    samsung_by_name = {
        rule["name"]: rule
        for rule in samsung_rules
    }

    neutral_by_name = {
        rule["name"]: rule
        for rule in neutral_rules
    }

    for name in neutral_names:
        if neutral_by_name[name] != samsung_by_name[name]:
            raise ValueError(
                f"shared rule {name!r} differs between variants"
            )

    if "Reject 3D" not in neutral_by_name:
        raise ValueError(
            "Reject 3D must be present in neutral profile"
        )

    leaked = device_set & set(neutral_names)

    if leaked:
        raise ValueError(
            "device rule leaked into neutral profile: "
            + ", ".join(sorted(leaked))
        )


def build_all():
    rules_source = load_json(RULES_PATH)
    variants_source = load_json(VARIANTS_PATH)

    entries = validate_registry(rules_source)
    variants = validate_variants(variants_source)

    payloads = {}

    for variant in variants:
        payloads[variant["artifact"]] = build_variant(
            entries=entries,
            variant=variant,
            streamnzb_profile=rules_source["streamnzb_profile"],
            scoring=rules_source["scoring"],
        )

    samsung = payloads["profile.txt"]
    neutral = payloads["profile-neutral.txt"]

    validate_cross_variant(
        samsung,
        neutral,
    )

    encoded = {
        artifact: encode_payload(payload)
        for artifact, payload in payloads.items()
    }

    # Decode every generated artifact before any write.
    for artifact, text in encoded.items():
        rebuilt = decode_share_code(text)

        if rebuilt != payloads[artifact]:
            raise ValueError(
                f"{artifact} failed encode/decode round-trip"
            )

    return payloads, encoded


def command_check(_args):
    payloads, encoded = build_all()

    current_samsung = (ROOT / "profile.txt").read_text(
        encoding="utf-8"
    )

    if encoded["profile.txt"] != current_samsung:
        raise SystemExit(
            "ERROR: generated profile.txt differs from current "
            "Samsung behavior-preserving artifact"
        )

    neutral_path = ROOT / "profile-neutral.txt"

    if neutral_path.exists():
        current_neutral = neutral_path.read_text(
            encoding="utf-8"
        )

        if encoded["profile-neutral.txt"] != current_neutral:
            raise SystemExit(
                "ERROR: profile-neutral.txt is not up to date"
            )

    print("PASS: source registry and variants validated")
    print(
        f"PASS: profile.txt canonical output unchanged "
        f"({len(payloads['profile.txt']['rules'])} rules)"
    )

    if neutral_path.exists():
        print(
            "PASS: profile-neutral.txt is current "
            f"({len(payloads['profile-neutral.txt']['rules'])} rules)"
        )
    else:
        print(
            "PASS: neutral variant validates "
            f"({len(payloads['profile-neutral.txt']['rules'])} rules; "
            "artifact not written yet)"
        )

    print("PASS: Reject 3D is present in both variants")
    print("PASS: exactly one Samsung-only device rule")


def command_build(_args):
    payloads, encoded = build_all()

    # Post-V5 policy changes are intentional canonical-source changes.
    # build_all() validates source/schema/ownership/variants and performs
    # encode/decode round-trip validation before any artifact write.

    # All validation above completed before any write.
    atomic_write(
        ROOT / "profile.txt",
        encoded["profile.txt"],
    )

    atomic_write(
        ROOT / "profile-neutral.txt",
        encoded["profile-neutral.txt"],
    )

    print(
        "PASS: profile.txt reproduced byte-for-byte "
        f"({len(payloads['profile.txt']['rules'])} rules)"
    )
    print(
        "PASS: profile-neutral.txt generated "
        f"({len(payloads['profile-neutral.txt']['rules'])} rules)"
    )
    print("PASS: Reject 3D present in both profiles")
    print("PASS: neutral excludes exactly one Samsung device rule")


def build_parser():
    parser = argparse.ArgumentParser(
        description=(
            "Build DraCuLa StreamNZB profile variants "
            "from the canonical ordered rule registry."
        )
    )

    subparsers = parser.add_subparsers(
        dest="command",
        required=True,
    )

    check = subparsers.add_parser(
        "check",
        help="validate sources and generated outputs without writing",
    )
    check.set_defaults(func=command_check)

    build = subparsers.add_parser(
        "build",
        help="validate and write generated profile artifacts",
    )
    build.set_defaults(func=command_build)

    return parser


def main():
    args = build_parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
