#!/usr/bin/env python3

import json
import re
from collections import defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

MAPPING_PATH = ROOT / "scripts" / "vidhin_mapping.json"
BASELINE_PATH = ROOT / "generated" / "vidhin-defines.json"


mapping = json.loads(MAPPING_PATH.read_text(encoding="utf-8"))
baseline = json.loads(BASELINE_PATH.read_text(encoding="utf-8"))


assert mapping.get("schema_version") == 3, (
    "Expected mapping schema_version 3"
)

assert baseline.get("schema_version") == 3, (
    "Expected generated baseline schema_version 3"
)

assert baseline.get("mapping_schema_version") == mapping["schema_version"], (
    "generated/vidhin-defines.json was produced with a different "
    "mapping schema version"
)


targets = mapping.get("targets")
defines = baseline.get("defines")

assert isinstance(targets, dict) and targets, (
    "vidhin_mapping.json contains no targets"
)

assert isinstance(defines, dict) and defines, (
    "generated/vidhin-defines.json contains no Defines"
)


# ---------------------------------------------------------------------------
# 1. Mapping <-> committed baseline consistency
# ---------------------------------------------------------------------------

mapped_names = set(targets)
baseline_names = set(defines)

missing_from_baseline = sorted(mapped_names - baseline_names)
unexpected_in_baseline = sorted(baseline_names - mapped_names)

assert not missing_from_baseline, (
    "Mapped target(s) missing from generated baseline:\n  - "
    + "\n  - ".join(missing_from_baseline)
)

assert not unexpected_in_baseline, (
    "Generated baseline contains unmapped target(s):\n  - "
    + "\n  - ".join(unexpected_in_baseline)
)


for name, cfg in targets.items():
    entry = defines[name]

    assert entry.get("scope") == cfg.get("scope"), (
        f"{name}: baseline scope {entry.get('scope')!r} does not match "
        f"mapping scope {cfg.get('scope')!r}"
    )

    assert entry.get("field") == cfg.get("field"), (
        f"{name}: baseline field {entry.get('field')!r} does not match "
        f"mapping field {cfg.get('field')!r}"
    )

    assert entry.get("mode", "standard") == cfg.get("mode", "standard"), (
        f"{name}: baseline mode {entry.get('mode')!r} does not match "
        f"mapping mode {cfg.get('mode', 'standard')!r}"
    )

    assert entry.get("sources") == cfg.get("sources"), (
        f"{name}: baseline sources {entry.get('sources')!r} do not match "
        f"mapping sources {cfg.get('sources')!r}"
    )

ANIME_LQ_TARGET = "Anime LQ Groups"

assert ANIME_LQ_TARGET in targets, (
    "Required Anime LQ mapping target missing"
)

anime_lq_cfg = targets[ANIME_LQ_TARGET]

assert anime_lq_cfg.get("sources") == ["Anime LQ Groups"], (
    "Anime LQ Groups must track Vidhin's Anime LQ Groups source"
)

assert anime_lq_cfg.get("scope") is None, (
    "Anime LQ Groups must use All Content (no explicit scope)"
)

assert anime_lq_cfg.get("field") == "releaseName", (
    "Anime LQ Groups must match releaseName"
)

assert anime_lq_cfg.get("mode") == "raw_release_name", (
    "Anime LQ Groups must preserve the upstream release-name regex"
)

assert anime_lq_cfg.get("anime_lq") is True, (
    "Anime LQ Groups must be marked anime_lq=true"
)

# ---------------------------------------------------------------------------
# 2. Required Anime hierarchy
# ---------------------------------------------------------------------------

EXPECTED_ANIME_SOURCES = {
    **{
        f"Anime BD T{tier}": tier
        for tier in range(1, 9)
    },
    **{
        f"Anime Web T{tier}": tier
        for tier in range(1, 7)
    },
}

expected_anime_targets = {}

for media, scope in (
    ("Anime Movies", "anime_movie"),
    ("Anime Shows", "anime_show"),
):
    for tier in range(1, 9):
        expected_anime_targets[
            f"{media} BluRay T{tier} Groups"
        ] = {
            "source": f"Anime BD T{tier}",
            "scope": scope,
            "tier": tier,
            "quality": "BluRay",
        }

    for tier in range(1, 7):
        expected_anime_targets[
            f"{media} WEB T{tier} Groups"
        ] = {
            "source": f"Anime Web T{tier}",
            "scope": scope,
            "tier": tier,
            "quality": "WEB",
        }


for name, expected in expected_anime_targets.items():
    assert name in targets, (
        f"Required Anime mapping target missing: {name}"
    )

    cfg = targets[name]

    assert cfg.get("sources") == [expected["source"]], (
        f"{name}: expected source {expected['source']!r}, "
        f"found {cfg.get('sources')!r}"
    )

    assert cfg.get("scope") == expected["scope"], (
        f"{name}: expected scope {expected['scope']!r}, "
        f"found {cfg.get('scope')!r}"
    )

    assert cfg.get("field") == "releaseName", (
        f"{name}: Anime tier must use releaseName, "
        f"found {cfg.get('field')!r}"
    )

    assert cfg.get("anime") is True, (
        f"{name}: Anime target must have anime=true"
    )


actual_anime_targets = {
    name
    for name, cfg in targets.items()
    if cfg.get("anime") is True
}

expected_anime_target_names = set(expected_anime_targets)

missing_anime_targets = sorted(
    expected_anime_target_names - actual_anime_targets
)

unexpected_anime_targets = sorted(
    actual_anime_targets - expected_anime_target_names
)

assert not missing_anime_targets, (
    "Expected Anime target(s) missing:\n  - "
    + "\n  - ".join(missing_anime_targets)
)

assert not unexpected_anime_targets, (
    "Unexpected Anime target(s) found in mapping:\n  - "
    + "\n  - ".join(unexpected_anime_targets)
)


# ---------------------------------------------------------------------------
# 3. Anime Movie/Show parity
#
# Both scopes intentionally map to the same Vidhin Anime source for a given
# quality/tier. Their resolved token sets therefore must remain identical.
# ---------------------------------------------------------------------------

for quality, max_tier in (
    ("BluRay", 8),
    ("WEB", 6),
):
    for tier in range(1, max_tier + 1):
        movie_name = (
            f"Anime Movies {quality} T{tier} Groups"
        )
        show_name = (
            f"Anime Shows {quality} T{tier} Groups"
        )

        movie_tokens = {
            token.casefold()
            for token in defines[movie_name].get("tokens", [])
        }

        show_tokens = {
            token.casefold()
            for token in defines[show_name].get("tokens", [])
        }

        assert movie_tokens == show_tokens, (
            f"Anime Movie/Show token mismatch for "
            f"{quality} T{tier}:\n"
            f"  Movies only: "
            f"{sorted(movie_tokens - show_tokens)}\n"
            f"  Shows only: "
            f"{sorted(show_tokens - movie_tokens)}"
        )


# ---------------------------------------------------------------------------
# 4. Cross-tier collisions
#
# A release-group token may legitimately exist in:
#
#   Movies <-> Shows
#   WEB    <-> BluRay
#
# but it must not occur in two tiers of the SAME Anime hierarchy.
#
# Since Movies and Shows intentionally mirror the same Vidhin source, we can
# validate the canonical source hierarchy once for WEB and once for BluRay.
# ---------------------------------------------------------------------------

def validate_no_cross_tier_collisions(quality, max_tier):
    seen = {}

    for tier in range(1, max_tier + 1):
        target = (
            f"Anime Movies {quality} T{tier} Groups"
        )

        tokens = defines[target].get("tokens")

        assert isinstance(tokens, list), (
            f"{target}: baseline tokens must be a list"
        )

        assert tokens, (
            f"{target}: resolved token list is empty"
        )

        local_seen = set()

        for token in tokens:
            assert isinstance(token, str) and token, (
                f"{target}: invalid token {token!r}"
            )

            key = token.casefold()

            assert key not in local_seen, (
                f"{target}: duplicate token {token!r} "
                "inside the same tier"
            )

            local_seen.add(key)

            if key in seen:
                previous_tier, previous_token = seen[key]

                raise AssertionError(
                    f"Anime {quality} cross-tier collision: "
                    f"{token!r} appears in T{previous_tier} "
                    f"({previous_token!r}) and T{tier}"
                )

            seen[key] = (tier, token)


validate_no_cross_tier_collisions("WEB", 6)
validate_no_cross_tier_collisions("BluRay", 8)


# ---------------------------------------------------------------------------
# 5. Source structure recorded in the accepted baseline
#
# Vidhin may have multiple regex records with the same source name
# (for example a case-sensitive supplemental Anime rule). That is valid.
# What matters here is that every expected Anime source exists in the
# committed baseline and resolves into the expected target.
# ---------------------------------------------------------------------------

baseline_sources = defaultdict(list)

for target_name, entry in defines.items():
    for record in entry.get("records", []):
        source = record.get("source")

        if source:
            baseline_sources[source].append(
                (target_name, record)
            )


for source in EXPECTED_ANIME_SOURCES:
    assert source in baseline_sources, (
        f"Expected Anime source missing from committed baseline: {source}"
    )

assert "Anime LQ Groups" in baseline_sources, (
    "Expected Anime LQ Groups source missing from committed baseline"
)

# ---------------------------------------------------------------------------
# 6. Known Anime regressions
# ---------------------------------------------------------------------------

def tokens_for(name):
    return {
        token.casefold(): token
        for token in defines[name].get("tokens", [])
    }


web_t1 = tokens_for("Anime Movies WEB T1 Groups")
web_t2 = tokens_for("Anime Movies WEB T2 Groups")

assert "vodes" in web_t1, (
    "Regression: Vodes missing from Anime WEB T1"
)

assert "vodes" not in web_t2, (
    "Regression: Vodes leaked into Anime WEB T2"
)

for name in expected_anime_target_names:
    token_keys = tokens_for(name)

    assert "not-vodes" not in token_keys, (
        f"Regression: Not-Vodes leaked into {name}"
    )


bluray_t4 = tokens_for(
    "Anime Movies BluRay T4 Groups"
)

bluray_t5 = tokens_for(
    "Anime Movies BluRay T5 Groups"
)

assert "lazyremux" in bluray_t4, (
    "Regression: LazyRemux missing from Anime BluRay T4"
)

assert "lazyremux" not in bluray_t5, (
    "Regression: LazyRemux leaked into Anime BluRay T5"
)

assert "ultraremux" in bluray_t5, (
    "Regression: UltraRemux missing from Anime BluRay T5"
)

assert "ultraremux" not in bluray_t4, (
    "Regression: UltraRemux leaked into Anime BluRay T4"
)


# ---------------------------------------------------------------------------
# 7. Mapping source-name sanity
#
# This catches accidental additions such as Anime Web T7 / Anime BD T9 to
# the mapping. Detection of NEW tiers appearing only in live Vidhin upstream
# belongs in the sync workflow, not deterministic PR CI.
# ---------------------------------------------------------------------------

anime_source_re = re.compile(
    r"^Anime (?P<quality>BD|Web) T(?P<tier>\d+)$"
)

mapped_anime_sources = set()

for cfg in targets.values():
    if cfg.get("anime") is not True:
        continue

    for source in cfg.get("sources", []):
        mapped_anime_sources.add(source)


unexpected_mapped_sources = sorted(
    mapped_anime_sources - set(EXPECTED_ANIME_SOURCES)
)

missing_mapped_sources = sorted(
    set(EXPECTED_ANIME_SOURCES) - mapped_anime_sources
)

assert not missing_mapped_sources, (
    "Expected Anime upstream source(s) not mapped:\n  - "
    + "\n  - ".join(missing_mapped_sources)
)

assert not unexpected_mapped_sources, (
    "Unexpected Anime upstream source(s) mapped:\n  - "
    + "\n  - ".join(unexpected_mapped_sources)
)


print(
    "Tier-integrity validation passed: "
    "Anime WEB T1-T6 and BluRay T1-T8 are complete, "
    "Movie/Show mappings are aligned, and no cross-tier "
    "Anime release-group collisions were found."
)