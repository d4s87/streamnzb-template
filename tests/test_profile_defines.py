#!/usr/bin/env python3

import base64
import gzip
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

PROFILE_PATH = ROOT / "profile.txt"
DEFINES_PATH = ROOT / "generated" / "streamnzb-defines.txt"

PROFILE_PREFIX = "SNZBP1:"


def decode_share_code(text: str, prefix: str) -> dict:
    """
    Decode a StreamNZB share code:

        PREFIX + base64url(gzip(JSON))

    The repository artifact is expected to contain exactly one canonical
    share code with no surrounding prose.
    """
    code = text.strip()

    if not code:
        raise AssertionError("Share code file is empty")

    if "\n" in code or "\r" in code:
        raise AssertionError(
            "profile.txt must contain exactly one StreamNZB share code"
        )

    if not code.startswith(prefix):
        raise AssertionError(
            f"profile.txt must start with {prefix!r}"
        )

    encoded = code[len(prefix):]

    if not encoded:
        raise AssertionError(
            f"{prefix} share code contains no payload"
        )

    # StreamNZB removes Base64URL padding during export.
    encoded += "=" * (-len(encoded) % 4)

    try:
        compressed = base64.urlsafe_b64decode(encoded)
    except Exception as exc:
        raise AssertionError(
            "profile.txt contains invalid Base64URL data"
        ) from exc

    try:
        raw = gzip.decompress(compressed)
    except Exception as exc:
        raise AssertionError(
            "profile.txt contains invalid gzip data"
        ) from exc

    try:
        payload = json.loads(raw.decode("utf-8"))
    except Exception as exc:
        raise AssertionError(
            "profile.txt does not contain valid UTF-8 JSON"
        ) from exc

    if not isinstance(payload, dict):
        raise AssertionError(
            "Decoded StreamNZB profile must be a JSON object"
        )

    return payload


def parse_define_library(text: str) -> dict[str, dict]:
    """
    Parse StreamNZB Define Library text.

    Expected line format:

        Define Name [scope]: define if condition
    """
    defines = {}

    line_re = re.compile(
        r"^(?P<name>.+?)"
        r"(?: \[(?P<scope>[^\]]+)\])?: "
        r"define if "
        r"(?P<condition>.+)$"
    )

    for line_number, raw_line in enumerate(text.splitlines(), start=1):
        line = raw_line.strip()

        if not line or line.startswith("#"):
            continue

        match = line_re.match(line)

        if not match:
            raise AssertionError(
                f"Invalid Define syntax on line {line_number}: {raw_line!r}"
            )

        name = match.group("name")
        scope = match.group("scope")
        condition = match.group("condition")

        if name in defines:
            raise AssertionError(
                f"Duplicate Define name: {name!r}"
            )

        defines[name] = {
            "scope": scope,
            "condition": condition,
            "line": line_number,
        }

    return defines


def extract_matched_dependencies(profile: dict) -> dict[str, list[str]]:
    """
    Return:

        {
            "Define Name": ["Rule A", "Rule B"],
            ...
        }

    for every matched("...") reference used by profile rules.
    """
    rules = profile.get("rules")

    if not isinstance(rules, list):
        raise AssertionError(
            'Decoded profile must contain a "rules" array'
        )

    if not rules:
        raise AssertionError(
            "Decoded profile contains no rules"
        )

    if len(rules) > 500:
        raise AssertionError(
            f"Decoded profile contains {len(rules)} rules; "
            "StreamNZB supports at most 500"
        )

    # Supports both:
    #
    #   matched("Name")
    #   matched('Name')
    #
    # even though the current profile normally uses double quotes.
    matched_re = re.compile(
        r"""matched\(\s*(["'])(.*?)\1\s*\)"""
    )

    dependencies: dict[str, list[str]] = {}

    for index, rule in enumerate(rules, start=1):
        if not isinstance(rule, dict):
            raise AssertionError(
                f"Profile rule #{index} is not an object"
            )

        name = rule.get("name", "")
        when = rule.get("when")

        if not isinstance(when, str) or not when.strip():
            raise AssertionError(
                f"Profile rule #{index} ({name!r}) "
                'has an empty or invalid "when" expression'
            )

        if len(when) > 10_000:
            raise AssertionError(
                f"Profile rule #{index} ({name!r}) "
                'has a "when" expression longer than 10,000 characters'
            )

        for match in matched_re.finditer(when):
            define_name = match.group(2)

            dependencies.setdefault(
                define_name,
                [],
            ).append(name or f"rule #{index}")

    return dependencies


def validate_required_anime_structure(defines: dict[str, dict]) -> None:
    expected = {}

    for media, scope in (
        ("Anime Movies", "anime_movie"),
        ("Anime Shows", "anime_show"),
    ):
        for tier in range(1, 7):
            expected[
                f"{media} WEB T{tier} Groups"
            ] = scope

        for tier in range(1, 9):
            expected[
                f"{media} BluRay T{tier} Groups"
            ] = scope

    missing = sorted(
        name
        for name in expected
        if name not in defines
    )

    if missing:
        raise AssertionError(
            "Required Anime Define(s) missing:\n  - "
            + "\n  - ".join(missing)
        )

    wrong_scope = []

    for name, expected_scope in expected.items():
        actual_scope = defines[name]["scope"]

        if actual_scope != expected_scope:
            wrong_scope.append(
                f"{name}: expected [{expected_scope}], "
                f"found [{actual_scope}]"
            )

    if wrong_scope:
        raise AssertionError(
            "Anime Define scope mismatch(es):\n  - "
            + "\n  - ".join(wrong_scope)
        )


def validate_anime_lq(
    rules: list[dict],
    defines: dict[str, dict],
) -> None:
    """
    Validate the Anime LQ Define and profile scoring policy.

    Vidhin's Anime LQ classification is applied to Anime releases with a
    -10000 penalty, except when the release is a SeaDex Best or SeaDex
    Alternative recommendation.
    """
    define_name = "Anime LQ Groups"

    if define_name not in defines:
        raise AssertionError(
            "Required Anime LQ Groups Define is missing"
        )

    if defines[define_name]["scope"] is not None:
        raise AssertionError(
            "Anime LQ Groups Define must use All Content "
            "(no explicit Define scope)"
        )

    condition = defines[define_name]["condition"]

    if not condition.startswith('releaseName matches "'):
        raise AssertionError(
            "Anime LQ Groups must match against releaseName"
        )

    anime_lq_rules = [
        rule
        for rule in rules
        if rule.get("name") == "Anime LQ Penalty"
    ]

    if len(anime_lq_rules) != 1:
        raise AssertionError(
            "Expected exactly one Anime LQ Penalty rule, "
            f"found {len(anime_lq_rules)}"
        )

    anime_lq = anime_lq_rules[0]

    if anime_lq.get("points") != -10000:
        raise AssertionError(
            "Anime LQ Penalty must score -10000"
        )

    # This is intentionally one Anime-wide rule rather than separate
    # movie/series rules.
    if "scope" in anime_lq:
        raise AssertionError(
            "Anime LQ Penalty must not use movie/series scope; "
            "it must rely on isAnime"
        )

    when = anime_lq.get("when")

    if not isinstance(when, str) or not when.strip():
        raise AssertionError(
            "Anime LQ Penalty has no valid when expression"
        )

    required_conditions = (
        "isAnime",
        'matched("Anime LQ Groups")',
        "seadex.best",
        "seadex.alternative",
    )

    for required in required_conditions:
        if required not in when:
            raise AssertionError(
                "Anime LQ Penalty missing required condition: "
                f"{required}"
            )

    if "not (seadex.best or seadex.alternative)" not in when:
        raise AssertionError(
            "Anime LQ Penalty must exempt both SeaDex Best "
            "and SeaDex Alternative releases"
        )

    if "seadex.known" in when:
        raise AssertionError(
            "Anime LQ Penalty must not use seadex.known; "
            "SeaDex title availability is not the same as a "
            "Best/Alternative recommendation"
        )


def validate_regressions(defines: dict[str, dict]) -> None:
    full_library = DEFINES_PATH.read_text(encoding="utf-8")

    if "Not-Vodes" in full_library:
        raise AssertionError(
            "Regression: Not-Vodes leaked into generated Define Library"
        )

    web_t1_names = (
        "Anime Movies WEB T1 Groups",
        "Anime Shows WEB T1 Groups",
    )

    web_t2_names = (
        "Anime Movies WEB T2 Groups",
        "Anime Shows WEB T2 Groups",
    )

    for name in web_t1_names:
        condition = defines[name]["condition"]

        if "Vodes" not in condition:
            raise AssertionError(
                f"{name} must contain Vodes"
            )

    for name in web_t2_names:
        condition = defines[name]["condition"]

        if "Vodes" in condition:
            raise AssertionError(
                f"{name} must not contain Vodes"
            )

    for media in ("Anime Movies", "Anime Shows"):
        t4 = defines[
            f"{media} BluRay T4 Groups"
        ]["condition"]

        t5 = defines[
            f"{media} BluRay T5 Groups"
        ]["condition"]

        if "LazyRemux" not in t4:
            raise AssertionError(
                f"{media} BluRay T4 must contain LazyRemux"
            )

        if "LazyRemux" in t5:
            raise AssertionError(
                f"{media} BluRay T5 must not contain LazyRemux"
            )

        if "UltraRemux" not in t5:
            raise AssertionError(
                f"{media} BluRay T5 must contain UltraRemux"
            )

        if "UltraRemux" in t4:
            raise AssertionError(
                f"{media} BluRay T4 must not contain UltraRemux"
            )


profile_text = PROFILE_PATH.read_text(encoding="utf-8")
defines_text = DEFINES_PATH.read_text(encoding="utf-8")

profile = decode_share_code(
    profile_text,
    PROFILE_PREFIX,
)

if profile.get("streamnzb_profile") != 1:
    raise AssertionError(
        "Decoded profile is not a StreamNZB profile "
        '(expected "streamnzb_profile": 1)'
    )

profile_name = profile.get("name")

if not isinstance(profile_name, str) or not profile_name.strip():
    raise AssertionError(
        "Decoded StreamNZB profile has no valid name"
    )

# Keep the decoded rules available to all profile-level validations.
rules = profile.get("rules")

if not isinstance(rules, list):
    raise AssertionError(
        'Decoded profile must contain a "rules" array'
    )

if not rules:
    raise AssertionError(
        "Decoded profile contains no rules"
    )

if len(rules) != 86:
    raise AssertionError(
        f"Expected 86 profile rules, found {len(rules)}"
    )

defines = parse_define_library(defines_text)

if len(defines) != 50:
    raise AssertionError(
        f"Expected 50 generated Defines, found {len(defines)}"
    )

dependencies = extract_matched_dependencies(profile)

missing_dependencies = sorted(
    name
    for name in dependencies
    if name not in defines
)

if missing_dependencies:
    details = []

    for name in missing_dependencies:
        used_by = ", ".join(dependencies[name])

        details.append(
            f'{name!r} used by: {used_by}'
        )

    raise AssertionError(
        "Profile references missing Define(s):\n  - "
        + "\n  - ".join(details)
    )

validate_required_anime_structure(defines)
validate_anime_lq(rules, defines)
validate_regressions(defines)

print(
    "Profile/Define validation passed: "
    f"{len(rules)} profile rules, "
    f"{len(dependencies)} referenced Defines, "
    f"{len(defines)} available Defines."
)