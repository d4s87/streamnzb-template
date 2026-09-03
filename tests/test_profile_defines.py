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


def validate_profile_rule_names(rules: list[dict]) -> None:
    """
    Validate the production rule-name contract required by Jhin v0.6.

    The current StreamNZB profile schema has no enabled/disabled field,
    so every published rule is active. Jhin v0.6 forbids duplicate
    enabled rule names.
    """
    seen: dict[str, int] = {}

    for index, rule in enumerate(rules, start=1):
        if not isinstance(rule, dict):
            raise AssertionError(
                f"Profile rule #{index} is not an object"
            )

        name = rule.get("name")

        if not isinstance(name, str) or not name.strip():
            raise AssertionError(
                f"Profile rule #{index} has an empty or invalid name"
            )

        previous = seen.get(name)

        if previous is not None:
            raise AssertionError(
                "Duplicate production rule name "
                f"{name!r}: rules #{previous} and #{index}. "
                "Jhin v0.6 forbids duplicate enabled rule names."
            )

        seen[name] = index


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


def validate_anime_bluray_tier_scores(
    rules: list[dict],
) -> None:
    """
    Protect the Anime Movie and Anime Show release-group hierarchy
    from cumulative portable scoring preferences.

    Both Anime kinds use the same tier-score contract.

    WEB:
        T1 500
        T2 400
        T3 300
        T4 200
        T5 100
        T6  20

    BluRay:
        T1 560
        T2 480
        T3 400
        T4 320
        T5 240
        T6 160
        T7  80
        T8   0

    The complete pinned StreamNZB/Jhin ranking regression establishes
    +77 as the largest currently supported effective portable Anime
    stack (Anime Show WEB). Every adjacent Anime tier must therefore
    retain at least an 80-point release-group gap.

    Tier conditions may contain intentional classification logic such
    as the LazyRemux / UltraRemux exception, so this validation does
    not simplify or replace their expressions.
    """

    expected_web = {
        1: 500,
        2: 400,
        3: 300,
        4: 200,
        5: 100,
        6: 20,
    }

    expected_bluray = {
        1: 560,
        2: 480,
        3: 400,
        4: 320,
        5: 240,
        6: 160,
        7: 80,
        8: 0,
    }

    families = {
        "WEB": expected_web,
        "BluRay": expected_bluray,
    }

    for media in (
        "Anime Movies",
        "Anime Shows",
    ):
        for family, expected in families.items():
            for tier, expected_points in expected.items():
                name = f"{media} {family} T{tier}"

                matches = [
                    rule
                    for rule in rules
                    if rule.get("name") == name
                ]

                if len(matches) != 1:
                    raise AssertionError(
                        f"Expected exactly one {name!r} rule, "
                        f"found {len(matches)}"
                    )

                rule = matches[0]

                if rule.get("points") != expected_points:
                    raise AssertionError(
                        f"{name} must score "
                        f"{expected_points:+d}; found "
                        f"{rule.get('points')!r}"
                    )

                when = rule.get("when")

                if not isinstance(when, str) or not when.strip():
                    raise AssertionError(
                        f"{name} must have a valid condition"
                    )

                required_match = (
                    f'matched("{media} {family} '
                    f'T{tier} Groups")'
                )

                if required_match not in when:
                    raise AssertionError(
                        f"{name} must reference "
                        f"{required_match!r}"
                    )

    min_anime_tier_gap = 80
    max_effective_anime_stack = 77

    for family, expected in families.items():
        ordered = [
            expected[tier]
            for tier in sorted(expected)
        ]

        gaps = [
            higher - lower
            for higher, lower in zip(
                ordered,
                ordered[1:],
            )
        ]

        if min(gaps) < min_anime_tier_gap:
            raise AssertionError(
                f"Anime {family} minimum adjacent tier gap "
                f"must be >= {min_anime_tier_gap}; found {gaps}"
            )

        for gap in gaps:
            if gap <= max_effective_anime_stack:
                raise AssertionError(
                    f"Anime {family} tier gap {gap} does not "
                    f"dominate the +{max_effective_anime_stack} "
                    "effective portable scoring ceiling"
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


def validate_bad_dual(
    rules: list[dict],
    defines: dict[str, dict],
) -> None:
    """
    Validate Bad Dual classification and production scoring scopes.

    The generated Vidhin classifications and the profile scoring rules
    must both remain explicitly content-scoped. A missing profile scope
    appears as "All Content" in StreamNZB and is a regression.
    """
    expected = {
        "Movie Bad Dual Penalty": {
            "scope": "movie",
            "define": "Movies Bad Dual Groups",
            "define_scope": "movie",
            "when": 'matched("Movies Bad Dual Groups")',
        },
        "Show Bad Dual Penalty": {
            "scope": "series",
            "define": "Shows Bad Dual Groups",
            "define_scope": "series",
            "when": (
                'not isAnime and '
                'matched("Shows Bad Dual Groups")'
            ),
        },
    }

    for rule_name, spec in expected.items():
        define_name = spec["define"]

        if define_name not in defines:
            raise AssertionError(
                f"Required {define_name} Define is missing"
            )

        actual_define_scope = defines[define_name]["scope"]

        if actual_define_scope != spec["define_scope"]:
            raise AssertionError(
                f"{define_name} must use "
                f"[{spec['define_scope']}] scope; "
                f"found [{actual_define_scope}]"
            )

        condition = defines[define_name]["condition"]

        if not condition.startswith('group matches "'):
            raise AssertionError(
                f"{define_name} must match against parsed group"
            )

        matches = [
            rule
            for rule in rules
            if rule.get("name") == rule_name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {rule_name} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]

        actual_scope = rule.get("scope")

        if actual_scope != spec["scope"]:
            raise AssertionError(
                f"{rule_name} must use scope "
                f"{spec['scope']!r}; found "
                f"{actual_scope!r}. Missing scope would "
                "appear as All Content in StreamNZB."
            )

        if rule.get("points") != -10000:
            raise AssertionError(
                f"{rule_name} must score -10000"
            )

        if rule.get("when") != spec["when"]:
            raise AssertionError(
                f"{rule_name} condition drifted: "
                f"{rule.get('when')!r}"
            )

def validate_adaptive_hd_x265(
    rules: list[dict],
) -> None:
    name = "Adaptive HD x265"

    matches = [
        rule
        for rule in rules
        if rule.get("name") == name
    ]

    if len(matches) != 1:
        raise AssertionError(
            f"Expected exactly one {name!r} rule, "
            f"found {len(matches)}"
        )

    rule = matches[0]

    if rule.get("action") != "reject":
        raise AssertionError(
            f"{name} must use action=reject"
        )

    if "points" in rule:
        raise AssertionError(
            f"{name} Reject rule must not define points"
        )

    if rule.get("scope"):
        raise AssertionError(
            f"{name} must not define an explicit scope"
        )

    when = rule.get("when")

    if not isinstance(when, str) or not when.strip():
        raise AssertionError(
            f"{name} has no valid condition"
        )

    required_fragments = (
        'not isAnime',
        'parsed.codec == "hevc"',
        'not ("remux" in traits)',
        'not any(hdr, # == "HDR" or # == "HDR10+" or # == "DV")',
        'not library',
        'resolution == "1080p"',
        'resolution == "720p"',
        'parsed.codec == "avc"',
        '"remux" in traits',
        '"bluray" in traits',
        '"webdl" in traits',
    )

    missing = [
        fragment
        for fragment in required_fragments
        if fragment not in when
    ]

    if missing:
        raise AssertionError(
            f"{name} is missing expected policy fragment(s): "
            + ", ".join(repr(fragment) for fragment in missing)
        )

    # Both resolution branches must retain the tested > 6
    # availability threshold.
    if when.count(") > 6") != 2:
        raise AssertionError(
            f"{name} must contain exactly two > 6 "
            "aggregate thresholds"
        )


def validate_1080p_remux_preference(rules):
    matches = [
        rule
        for rule in rules
        if rule.get("name") == "1080p Remux Preference"
    ]

    if len(matches) != 1:
        raise AssertionError(
            "Expected exactly one '1080p Remux Preference' rule, "
            f"found {len(matches)}"
        )

    rule = matches[0]

    if rule.get("points") != 50:
        raise AssertionError(
            "'1080p Remux Preference' must score exactly +50"
        )

    if "action" in rule:
        raise AssertionError(
            "'1080p Remux Preference' must remain a score rule "
            "without an explicit action"
        )

    if "scope" in rule:
        raise AssertionError(
            "'1080p Remux Preference' must not have an explicit scope"
        )

    when = rule.get("when")

    if not isinstance(when, str) or not when.strip():
        raise AssertionError(
            "'1080p Remux Preference' must have a non-empty condition"
        )

    required = [
        '(kind == "movie" or kind == "series")',
        "not isAnime",
        'resolution == "1080p"',
        '"remux" in traits',
        "not library",
        'resolution == "2160p"',
        '"webdl" in traits',
        '# == "HDR"',
        '# == "HDR10+"',
    ]

    missing = [
        fragment
        for fragment in required
        if fragment not in when
    ]

    if missing:
        raise AssertionError(
            "'1080p Remux Preference' condition is missing: "
            + ", ".join(repr(item) for item in missing)
        )

    if when.count("count(") != 2:
        raise AssertionError(
            "'1080p Remux Preference' must contain exactly "
            "two aggregate count() predicates"
        )

    if ") > 0" not in when:
        raise AssertionError(
            "'1080p Remux Preference' must require at least "
            "one eligible 2160p WEB-DL"
        )

    if ") == 0" not in when:
        raise AssertionError(
            "'1080p Remux Preference' must suppress the bonus "
            "when HDR/HDR10+ 2160p WEB-DL is available"
        )

    forbidden = [
        "seadex.best",
        "seadex.alternative",
        '# == "DV"',
    ]

    present = [
        fragment
        for fragment in forbidden
        if fragment in when
    ]

    if present:
        raise AssertionError(
            "'1080p Remux Preference' unexpectedly contains: "
            + ", ".join(repr(item) for item in present)
        )



def validate_season_pack_limits(rules):
    """Validate the independent episode/non-pack and season-pack R/Q ceilings."""

    general_name = "Best 3 per R/Q"
    pack_name = "Best 1 Season Pack per R/Q"

    expected_group = 'resolution + " " + quality'
    pack_when = (
        '(kind == "series" or kind == "anime_show") '
        'and seasonPack'
    )
    general_when = f"not ({pack_when})"

    expected = {
        general_name: {
            "action": "limit",
            "count": 3,
            "group_by": expected_group,
            "when": general_when,
        },
        pack_name: {
            "action": "limit",
            "count": 1,
            "group_by": expected_group,
            "when": pack_when,
        },
    }

    resolved = {}

    for name, spec in expected.items():
        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]
        resolved[name] = rule

        for field, wanted in spec.items():
            actual = rule.get(field)

            if actual != wanted:
                raise AssertionError(
                    f"{name} {field} drifted: "
                    f"{actual!r}; expected {wanted!r}"
                )

        if "points" in rule:
            raise AssertionError(
                f"{name} must remain a limit rule "
                "without score points"
            )

        if "scope" in rule:
            raise AssertionError(
                f"{name} must use its explicit kind/seasonPack "
                "condition rather than profile scope"
            )

    if (
        resolved[general_name]["group_by"]
        != resolved[pack_name]["group_by"]
    ):
        raise AssertionError(
            "Episode/non-pack and season-pack ceilings must "
            "use the same resolution + quality grouping"
        )

    if resolved[general_name]["count"] <= resolved[pack_name]["count"]:
        raise AssertionError(
            "General R/Q ceiling must remain larger than "
            "the season-pack ceiling"
        )

    if "seasonPack" not in resolved[pack_name]["when"]:
        raise AssertionError(
            "Season-pack ceiling must explicitly require seasonPack"
        )

    if not resolved[general_name]["when"].startswith("not ("):
        raise AssertionError(
            "General R/Q ceiling must explicitly exclude "
            "the episodic season-pack partition"
        )


def validate_repack_proper_preferences(rules):
    """Validate the corrected-release tie-breaker policy."""

    expected = {
        "Repack/Proper Preference": {
            "points": -15,
            "effective_points": 5,
            "when": (
                '(proper or repack) and not '
                '(releaseName matches '
                '"(?i)(?:^|[. _\\\\[\\\\]-])REPACK'
                '[. _-]?(?:2|3)(?:$|[. _\\\\[\\\\]-])")'
            ),
        },
        "Repack2 Preference": {
            "points": -14,
            "effective_points": 6,
            "when": (
                'repack and releaseName matches '
                '"(?i)(?:^|[. _\\\\[\\\\]-])REPACK'
                '[. _-]?2(?:$|[. _\\\\[\\\\]-])"'
            ),
        },
        "Repack3 Preference": {
            "points": -13,
            "effective_points": 7,
            "when": (
                'repack and releaseName matches '
                '"(?i)(?:^|[. _\\\\[\\\\]-])REPACK'
                '[. _-]?3(?:$|[. _\\\\[\\\\]-])"'
            ),
        },
    }

    for name, spec in expected.items():
        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]

        if rule.get("points") != spec["points"]:
            raise AssertionError(
                f"{name} rule-layer points drifted: "
                f"{rule.get('points')!r}; expected "
                f"{spec['points']:+d} to compensate Jhin native +20 "
                f"and preserve effective {spec['effective_points']:+d}"
            )

        if spec["points"] + 20 != spec["effective_points"]:
            raise AssertionError(
                f"{name} compensation contract is internally invalid: "
                f"{spec['points']:+d} + native +20 != "
                f"{spec['effective_points']:+d}"
            )

        if rule.get("when") != spec["when"]:
            raise AssertionError(
                f"{name} condition drifted: "
                f"{rule.get('when')!r}"
            )

        if "action" in rule:
            raise AssertionError(
                f"{name} must remain a score rule "
                "without an explicit action"
            )

        if "scope" in rule:
            raise AssertionError(
                f"{name} must remain global "
                "without an explicit profile scope"
            )

        when = rule["when"]

        if 'matched("' in when or "matched('" in when:
            raise AssertionError(
                f"{name} must use native StreamNZB parser traits "
                "and releaseName matching, not Define dependencies"
            )

        if "seadex" in when.lower():
            raise AssertionError(
                f"{name} must remain a global tie-breaker "
                "without SeaDex-specific predicates"
            )

    base_when = expected["Repack/Proper Preference"]["when"]

    if "not (releaseName matches" not in base_when:
        raise AssertionError(
            "Base REPACK/PROPER rule must negate the numbered "
            "REPACK matcher as a parenthesized expression"
        )

    if "(?:2|3)" not in base_when:
        raise AssertionError(
            "Base REPACK/PROPER rule must exclude REPACK2/REPACK3"
        )




def validate_retag_soft_penalty(
    rules: list[dict],
) -> None:
    """Validate the global Retag metadata tie-breaker."""

    name = 'Retag Soft Penalty'
    expected_when = 'releaseName matches "(?i)(?:[.]heb\\b|\\[eztvx?(?:[ ._-]?(?:io|re|to))?\\]|\\[(?:rarbg|rartv|TGx)\\])"'

    matches = [
        rule
        for rule in rules
        if rule.get("name") == name
    ]

    if len(matches) != 1:
        raise AssertionError(
            f"Expected exactly one {name!r} rule, "
            f"found {len(matches)}"
        )

    rule = matches[0]

    if rule.get("points") != -1:
        raise AssertionError(
            f"{name} must score exactly -1; "
            f"found {rule.get('points')!r}"
        )

    if rule.get("when") != expected_when:
        raise AssertionError(
            f"{name} condition drifted: "
            f"{rule.get('when')!r}"
        )

    if "action" in rule:
        raise AssertionError(
            f"{name} must remain a score rule "
            "without an explicit action"
        )

    if "scope" in rule:
        raise AssertionError(
            f"{name} must remain global and must not "
            "define an explicit content scope"
        )

    if 'matched("' in rule["when"] or "matched('" in rule["when"]:
        raise AssertionError(
            f"{name} must use releaseName matching directly "
            "without Define dependencies"
        )

    if "seadex" in rule["when"].lower():
        raise AssertionError(
            f"{name} must not contain SeaDex-specific predicates"
        )


def validate_audio_preferences(
    rules: list[dict],
) -> None:
    """
    Validate the non-stacking audio preference policy.

    StreamNZB's 4k preset applies a native -1000 score to the
    dubbed / Dual-Multi Audio trait. Each shared +1010 profile rule
    compensates for that native baseline and leaves the intended
    effective +10 preference after the complete ranking pipeline.

    Non-Anime Movies and Shows share the parsed ``dubbed`` rule;
    Anime remains isolated on its dedicated Dual/Multi release-name
    rule. Independent legacy audio rules must not exist.
    """

    expected = {
        "Non-Anime Dubbed/Dual/Multi Audio Preference": {
            "points": 1010,
            "when": 'not isAnime and "dubbed" in traits',
        },
        "Anime Dual/Multi Audio Preference": {
            "points": 1010,
            "when": (
                'isAnime and releaseName matches '
                '"(?i)\\\\b(?:Dual|Multi)[. _-]?Audio\\\\b"'
            ),
        },
    }

    resolved = {}

    for name, spec in expected.items():
        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]
        resolved[name] = rule

        if rule.get("points") != spec["points"]:
            raise AssertionError(
                f"{name} must score exactly "
                f"{spec['points']:+d}; "
                f"found {rule.get('points')!r}"
            )

        if rule.get("when") != spec["when"]:
            raise AssertionError(
                f"{name} condition drifted: "
                f"{rule.get('when')!r}"
            )

        if "action" in rule:
            raise AssertionError(
                f"{name} must remain a score rule "
                "without an explicit action"
            )

        if "scope" in rule:
            raise AssertionError(
                f"{name} must use isAnime/not isAnime "
                "rather than an explicit profile scope"
            )

        when = rule["when"]

        if 'matched("' in when or "matched('" in when:
            raise AssertionError(
                f"{name} must not depend on a Define rule"
            )

        if "seadex" in when.lower():
            raise AssertionError(
                f"{name} must not contain "
                "SeaDex-specific predicates"
            )

    non_anime = resolved[
        "Non-Anime Dubbed/Dual/Multi Audio Preference"
    ]["when"]

    anime = resolved[
        "Anime Dual/Multi Audio Preference"
    ]["when"]

    if non_anime != 'not isAnime and "dubbed" in traits':
        raise AssertionError(
            "Non-Anime audio preference must use StreamNZB's "
            "parsed dubbed trait"
        )

    if not anime.startswith("isAnime and "):
        raise AssertionError(
            "Anime Dual/Multi Audio Preference "
            "must remain Anime-only"
        )

    if "(?:Dual|Multi)" not in anime:
        raise AssertionError(
            "Anime Dual/Multi Audio Preference must "
            "combine Dual and Multi matching in one rule"
        )

    # These historical independent rules produced reachable +700
    # and +900 stacks because StreamNZB also marks Dual/Multi
    # releases with the dubbed trait.
    forbidden_names = {
        "Dubbed bonus",
        "Dual audio",
        "Multi audio",
        "Non-Anime Dubbed bonus",
        "Non-Anime Dual audio",
        "Non-Anime Multi audio",
        "Anime Dual Audio Preference",
        "Anime Multi Audio Preference",
    }

    found_forbidden = sorted(
        rule.get("name")
        for rule in rules
        if rule.get("name") in forbidden_names
    )

    if found_forbidden:
        raise AssertionError(
            "Audio preferences must remain shared and "
            "non-stacking; found forbidden rule(s): "
            + ", ".join(found_forbidden)
        )


def validate_anime_version_preferences(
    rules: list[dict],
) -> None:
    """Validate Anime v0-v4 revision tie-breakers."""

    expected_points = {
        0: -1,
        1: 1,
        2: 2,
        3: 3,
        4: 4,
    }

    def version_marker(version: int) -> str:
        return rf"(?i)(?:\b|\d)v{version}\b"

    for version, points in expected_points.items():
        name = f"Anime Version v{version} Preference"

        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]

        exclusions = [
            (
                "not (releaseName matches "
                f'"{version_marker(other)}")'
            )
            for other in range(5)
            if other != version
        ]

        expected_when = (
            "isAnime and releaseName matches "
            f'"{version_marker(version)}" and '
            + " and ".join(exclusions)
        )

        if rule.get("points") != points:
            raise AssertionError(
                f"{name} must score exactly {points:+d}; "
                f"found {rule.get('points')!r}"
            )

        if rule.get("when") != expected_when:
            raise AssertionError(
                f"{name} condition drifted: "
                f"{rule.get('when')!r}"
            )

        if "action" in rule:
            raise AssertionError(
                f"{name} must remain a score rule "
                "without an explicit action"
            )

        if "scope" in rule:
            raise AssertionError(
                f"{name} must use isAnime rather than "
                "an explicit profile scope"
            )

        when = rule["when"]

        if not when.startswith("isAnime and "):
            raise AssertionError(
                f"{name} must remain Anime-only"
            )

        if 'matched("' in when or "matched('" in when:
            raise AssertionError(
                f"{name} must use releaseName matching "
                "without Define dependencies"
            )

        if "seadex" in when.lower():
            raise AssertionError(
                f"{name} must not contain SeaDex-specific "
                "predicates"
            )

        # Each rule must explicitly exclude the four other
        # supported revision markers so supported versions
        # cannot stack.
        exclusion_count = when.count(
            "not (releaseName matches "
        )

        if exclusion_count != 4:
            raise AssertionError(
                f"{name} must contain exactly four "
                "supported-version exclusions"
            )


def validate_movie_edition_preferences(
    rules: list[dict],
) -> None:
    """
    Validate DraCuLa's Movie-version preference policy.

    IMAX is intentionally a strong Movie preference and may outrank
    release-group tiers.

    Open Matte and the parser-backed Director's Cut / Extended Edition
    preference are deliberately small Movie-only tie-breakers.

    Director's Cut and Extended Edition share one +25 rule, preventing
    equivalent alternate-cut labels from stacking with each other.
    """

    expected = {
        "IMAX": {
            "scope": "movie",
            "points": 700,
            "when": 'releaseName matches "(?i)\\\\bIMAX\\\\b"',
        },
        "Open matte": {
            "scope": "movie",
            "points": 25,
            "when": (
                'releaseName matches '
                '"(?i)\\\\bOpen[. _-]?Matte\\\\b"'
            ),
        },
        "Movie Edition Preference": {
            "scope": "movie",
            "points": -75,
            "when": (
                'edition == "Directors Cut" or '
                'edition == "Extended Edition"'
            ),
        },
    }

    resolved = {}

    for name, spec in expected.items():
        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r} rule, "
                f"found {len(matches)}"
            )

        rule = matches[0]
        resolved[name] = rule

        if rule.get("scope") != spec["scope"]:
            raise AssertionError(
                f"{name} must use explicit movie scope; "
                f"found {rule.get('scope')!r}"
            )

        if rule.get("points") != spec["points"]:
            raise AssertionError(
                f"{name} must score exactly "
                f"{spec['points']:+d}; "
                f"found {rule.get('points')!r}"
            )

        if rule.get("when") != spec["when"]:
            raise AssertionError(
                f"{name} condition drifted: "
                f"{rule.get('when')!r}"
            )

        if "action" in rule:
            raise AssertionError(
                f"{name} must remain a score rule "
                "without an explicit action"
            )

        if (
            'matched("' in rule["when"]
            or "matched('" in rule["when"]
        ):
            raise AssertionError(
                f"{name} must not depend on a Define"
            )

    native_edition_points = 100

    imax_effective_points = (
        resolved["IMAX"]["points"] + native_edition_points
    )
    if imax_effective_points != 800:
        raise AssertionError(
            "IMAX effective score must remain +800 after Jhin "
            f"native +100 edition ranking; found {imax_effective_points}"
        )

    movie_edition_effective_points = (
        resolved["Movie Edition Preference"]["points"]
        + native_edition_points
    )
    if movie_edition_effective_points != 25:
        raise AssertionError(
            "Movie Edition Preference effective score must remain +25 "
            "after Jhin native +100 edition ranking; found "
            f"{movie_edition_effective_points}"
        )

    open_matte_effective_points = resolved["Open matte"]["points"]
    if open_matte_effective_points != 25:
        raise AssertionError(
            "Open matte effective score must remain +25; found "
            f"{open_matte_effective_points}"
        )

    movie_tier_gap = 200

    minor_edition_stack = (
        open_matte_effective_points
        + movie_edition_effective_points
    )

    if minor_edition_stack >= movie_tier_gap:
        raise AssertionError(
            "Minor Movie edition preferences must remain below the "
            f"{movie_tier_gap}-point Movie release-group tier gap; "
            f"found reachable +{minor_edition_stack}"
        )

    if resolved["IMAX"]["points"] <= 500:
        raise AssertionError(
            "IMAX must remain an intentional strong Movie-version "
            "preference rather than a minor tie-breaker"
        )


def validate_availability_scoring_policy(
    rules: list[dict],
) -> None:
    """
    Availability is a small tie-breaker, not a replacement for
    release-group quality.

    The formatter still exposes freshness, popularity, backbone and
    confirmation metadata, but positive ranking points are limited to:

        Alive on our backbone +20
        Recently confirmed   +10

    The maximum positive availability stack is therefore +30, safely
    below the 70-point Anime BluRay tier gap.

    The 4k preset already provides the intended native +500 Library
    bonus, so a separate Library hit score rule must not exist.
    """

    forbidden_names = {
        "Library hit",
        "Very fresh NZB",
        "Recent NZB",
        "Popular NZB",
        "Very popular NZB",
        "Highly popular NZB",
    }

    found_forbidden = sorted(
        rule.get("name")
        for rule in rules
        if rule.get("name") in forbidden_names
    )

    if found_forbidden:
        raise AssertionError(
            "Removed availability/library score rules "
            "must not return: "
            + ", ".join(found_forbidden)
        )

    expected = {
        "Alive on our backbone": 20,
        "Recently confirmed": 10,
    }

    for name, expected_points in expected.items():
        matches = [
            rule
            for rule in rules
            if rule.get("name") == name
        ]

        if len(matches) != 1:
            raise AssertionError(
                f"Expected exactly one {name!r}; "
                f"found {len(matches)}"
            )

        rule = matches[0]

        if rule.get("points") != expected_points:
            raise AssertionError(
                f"{name} must score exactly "
                f"{expected_points:+d}; found "
                f"{rule.get('points')!r}"
            )

        if "action" in rule:
            raise AssertionError(
                f"{name} must remain a score rule"
            )

    if sum(expected.values()) >= 70:
        raise AssertionError(
            "Positive availability ceiling must remain "
            "below the 70-point Anime BluRay tier gap"
        )


def validate_unknown_resolution_policy(
    rules: list[dict],
    defines: dict[str, dict],
) -> None:
    """
    Unknown resolution is not inherently bad.

    Weak results are rejected only when more than six
    well-identified alternatives exist. Library, SeaDex,
    known-quality and recognized release-group results
    remain protected. Unknown Quality by itself is not
    rejected.
    """

    name = "Unknown resolution"

    matches = [
        rule
        for rule in rules
        if rule.get("name") == name
    ]

    if len(matches) != 1:
        raise AssertionError(
            f"Expected exactly one {name!r} rule, "
            f"found {len(matches)}"
        )

    rule = matches[0]

    if rule.get("action") != "reject":
        raise AssertionError(
            f"{name} must use action=reject"
        )

    if "points" in rule:
        raise AssertionError(
            f"{name} Reject rule must not define points"
        )

    if rule.get("scope"):
        raise AssertionError(
            f"{name} must remain All Content"
        )

    when = rule.get("when")

    if not isinstance(when, str) or not when.strip():
        raise AssertionError(
            f"{name} has no valid condition"
        )

    required_fragments = (
        'resolution == ""',
        "not library",
        "not seadex.best",
        "not seadex.alternative",
        'quality == ""',
        "count(",
        'resolution != ""',
        'quality != ""',
        ") > 6",
    )

    missing = [
        fragment
        for fragment in required_fragments
        if fragment not in when
    ]

    if missing:
        raise AssertionError(
            f"{name} is missing policy fragment(s): "
            + ", ".join(repr(x) for x in missing)
        )

    if when.count("count(") != 1:
        raise AssertionError(
            f"{name} must contain exactly one "
            "aggregate count()"
        )

    if when.count(") > 6") != 1:
        raise AssertionError(
            f"{name} must use exactly one > 6 threshold"
        )

    tier_pattern = re.compile(
        r"^(?:"
        r"Movies|Shows|Anime Movies|Anime Shows"
        r") "
        r"(?:UHD BluRay|HD BluRay|BluRay|WEB|Remux) "
        r"T\d+ Groups$"
    )

    tier_defines = sorted(
        define_name
        for define_name in defines
        if tier_pattern.fullmatch(define_name)
    )

    if not tier_defines:
        raise AssertionError(
            "No release-group tier Defines found "
            "for Unknown resolution protection"
        )

    helper_name = "Trusted Release Groups"

    if helper_name not in defines:
        raise AssertionError(
            f"Required {helper_name!r} derived Define is missing"
        )

    helper = defines[helper_name]

    if helper["scope"] is not None:
        raise AssertionError(
            f"{helper_name} must remain All Content"
        )

    helper_condition = helper["condition"]

    missing_tiers = [
        define_name
        for define_name in tier_defines
        if f'matched("{define_name}")' not in helper_condition
    ]

    if missing_tiers:
        raise AssertionError(
            f"{helper_name} no longer covers tier Define(s): "
            + ", ".join(missing_tiers)
        )

    helper_refs = []
    needle = 'matched("'
    pos = 0

    while True:
        start = helper_condition.find(needle, pos)
        if start < 0:
            break

        name_start = start + len(needle)
        name_end = helper_condition.find('")', name_start)

        if name_end < 0:
            raise AssertionError(
                f"{helper_name} contains unterminated matched() reference"
            )

        helper_refs.append(
            helper_condition[name_start:name_end]
        )

        pos = name_end + 2

    helper_refs = sorted(set(helper_refs))

    if helper_refs != tier_defines:
        extra = sorted(set(helper_refs) - set(tier_defines))
        missing = sorted(set(tier_defines) - set(helper_refs))
        raise AssertionError(
            f"{helper_name} membership mismatch; "
            f"missing={missing}, extra={extra}"
        )

    if 'matched("Trusted Release Groups")' not in when:
        raise AssertionError(
            f"{name} must protect recognized tier groups through "
            f"{helper_name!r}"
        )

    direct_tier_refs = [
        define_name
        for define_name in tier_defines
        if f'matched("{define_name}")' in when
    ]

    if direct_tier_refs:
        raise AssertionError(
            f"{name} must not directly enumerate tier Defines: "
            + ", ".join(direct_tier_refs)
        )

    # Known Resolution + Unknown Quality must never fall
    # into this rule. The resolution predicate is the
    # outer gate; quality alone is not grounds to reject.
    if not when.lstrip().startswith(
        'resolution == ""'
    ):
        raise AssertionError(
            f"{name} must gate on Unknown Resolution first"
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

if len(rules) != 114:
    raise AssertionError(
        f"Expected 114 profile rules, found {len(rules)}"
    )

defines = parse_define_library(defines_text)

if len(defines) != 56:
    raise AssertionError(
        f"Expected 56 published Defines, found {len(defines)}"
    )

validate_profile_rule_names(rules)

dependencies = extract_matched_dependencies(profile)

missing_dependencies = sorted(
    name
    for name in dependencies
    if name not in defines
)

if missing_dependencies:
    defines_by_casefold: dict[str, list[str]] = {}

    for define_name in defines:
        defines_by_casefold.setdefault(
            define_name.casefold(),
            [],
        ).append(define_name)

    case_mismatches = []
    truly_missing = []

    for name in missing_dependencies:
        candidates = sorted(
            defines_by_casefold.get(name.casefold(), [])
        )

        if candidates:
            case_mismatches.append((name, candidates))
        else:
            truly_missing.append(name)

    sections = []

    if case_mismatches:
        details = []

        for name, candidates in case_mismatches:
            used_by = ", ".join(dependencies[name])
            available = ", ".join(repr(item) for item in candidates)

            details.append(
                f"{name!r} used by: {used_by}; "
                f"case-matching Define(s): {available}"
            )

        sections.append(
            "Profile matched() reference case mismatch(es). "
            "Jhin v0.6 matched() names are case-sensitive:\n  - "
            + "\n  - ".join(details)
        )

    if truly_missing:
        details = []

        for name in truly_missing:
            used_by = ", ".join(dependencies[name])

            details.append(
                f"{name!r} used by: {used_by}"
            )

        sections.append(
            "Profile references missing Define(s):\n  - "
            + "\n  - ".join(details)
        )

    raise AssertionError("\n\n".join(sections))

validate_required_anime_structure(defines)
validate_anime_bluray_tier_scores(rules)
validate_anime_lq(rules, defines)
validate_bad_dual(rules, defines)
validate_adaptive_hd_x265(rules)
validate_1080p_remux_preference(rules)
validate_season_pack_limits(rules)
validate_repack_proper_preferences(rules)
validate_retag_soft_penalty(rules)
validate_audio_preferences(rules)
validate_availability_scoring_policy(rules)
validate_anime_version_preferences(rules)
validate_movie_edition_preferences(rules)
validate_unknown_resolution_policy(rules, defines)
validate_regressions(defines)


# ---------------------------------------------------------------------------
# Anime Dubs Only production policy
# ---------------------------------------------------------------------------

dubs_only_define_name = "Anime Dubs Only"

if dubs_only_define_name not in defines:
    raise AssertionError(
        "Required Anime Dubs Only Define is missing"
    )

dubs_only_define = defines[dubs_only_define_name]

if dubs_only_define["scope"] is not None:
    raise AssertionError(
        "Anime Dubs Only Define must use All Content "
        "(no explicit Define scope)"
    )

dubs_only_condition = dubs_only_define["condition"]

if not isinstance(dubs_only_condition, str) or not dubs_only_condition.strip():
    raise AssertionError(
        "Anime Dubs Only Define has no valid condition"
    )

if "releaseName matches" not in dubs_only_condition:
    raise AssertionError(
        "Anime Dubs Only Define must classify releaseName"
    )

# StreamNZB uses Go/RE2. PCRE lookarounds from the raw upstream
# Vidhin expression must never leak into the generated Define.
for unsupported in ("(?=", "(?!", "(?<=", "(?<!"):
    if unsupported in dubs_only_condition:
        raise AssertionError(
            "Anime Dubs Only Define contains unsupported "
            f"lookaround syntax: {unsupported}"
        )

dubs_only_rules = [
    rule
    for rule in rules
    if rule.get("name") == "Anime Dubs Only Penalty"
]

if len(dubs_only_rules) != 1:
    raise AssertionError(
        "Expected exactly one Anime Dubs Only Penalty rule, "
        f"found {len(dubs_only_rules)}"
    )

dubs_only_rule = dubs_only_rules[0]

expected_dubs_only_when = (
    'isAnime and matched("Anime Dubs Only") '
    'and exists(isAnime and not matched("Anime Dubs Only"))'
)

if dubs_only_rule.get("points") != -10:
    raise AssertionError(
        "Anime Dubs Only Penalty must score exactly -10"
    )

if dubs_only_rule.get("when") != expected_dubs_only_when:
    raise AssertionError(
        "Anime Dubs Only Penalty condition drifted: "
        f"{dubs_only_rule.get('when')!r}"
    )

if "action" in dubs_only_rule:
    raise AssertionError(
        "Anime Dubs Only Penalty must remain a score rule "
        "and must not hard-reject releases"
    )

if "scope" in dubs_only_rule:
    raise AssertionError(
        "Anime Dubs Only Penalty must use isAnime rather than "
        "an explicit profile scope"
    )

if "seadex" in dubs_only_rule.get("when", "").lower():
    raise AssertionError(
        "Anime Dubs Only Penalty must not trigger SeaDex predicates"
    )

if 'exists(isAnime and not matched("Anime Dubs Only"))' not in \
        dubs_only_rule["when"]:
    raise AssertionError(
        "Anime Dubs Only Penalty must preserve scarce dub-only "
        "results by requiring a non-dub Anime alternative"
    )

print(
    "Profile/Define validation passed: "
    f"{len(rules)} profile rules, "
    f"{len(dependencies)} referenced Defines, "
    f"{len(defines)} available Defines."
)
