# DraCuLa Compatibility and Validation

This document contains the technical compatibility and validation details for DraCuLa's StreamNZB Template. For installation and normal use, start with the repository [README](../README.md).

## Supported baseline

Current published compatibility baseline:

- StreamNZB 5.18.0
- Jhin 0.6.2
- StreamNZB profile payload schema v2

The repository keeps this baseline pinned in deterministic compatibility tests while also running a scheduled check against the latest upstream StreamNZB release to provide early warning of regressions.

The scheduled upstream check is advisory. It does not silently move the supported baseline or block ordinary pull requests.

## Real-engine compatibility harness

For parser, rule-engine and formatter behavior that cannot be safely inferred from static configuration alone, the repository runs fixtures against a pinned revision of the real StreamNZB engine.

The harness validates both intent and shipped artifacts:

1. Fixture validation checks representative positive and negative cases against StreamNZB's real parser and rule engine.
2. Production regression validation decodes the published profile, locates the production rule by name and runs the same cases against the actual shipped rule.

The neutral profile is also decoded and compiled against the same engine and shared Define Library.

## Define Library integration

The compatibility harness loads the generated Define Library when testing rules that use `matched()`. This allows release-group classification, Define scope, profile policy and final-score behavior to be exercised end to end instead of being tested as disconnected pieces.

## Profile share-code contract

StreamNZB uses `SNZBP1:` as the profile share-code container prefix. The `streamnzb_profile` field versions the profile payload semantics.

For the current StreamNZB contract:

- schema `1` is used for a profile without a scoring map;
- schema `2` is used for a profile with a scoring map.

DraCuLa profiles always carry a scoring map, so published profiles are required to emit `streamnzb_profile == 2`. Missing or unexpected schema versions fail validation so compatibility changes receive explicit review.

## Formatter compatibility

The normal and debug formatters are built from readable JSON sources and published as `SNZBF1:` artifacts. Tests verify that the published formatter artifacts stay semantically synchronized with their readable sources.

Pinned real-StreamNZB formatter regressions cover user-facing behaviors such as:

- parsed language metadata;
- subtitle presence and subtitle-only rendering;
- same-release variant visibility;
- PROPER / REPACK status labels;
- ordinary versus backbone-confirmed NZB availability;
- formatter behavior for compatibility-sensitive metadata.

Jhin exposes parsed `Languages` separately from the boolean `Subbed` flag. DraCuLa therefore displays the metadata that Jhin actually exports and does not invent subtitle-language identities that are not present in the formatter context.

## Parser-sensitive behavior

Several production rules depend on exact StreamNZB/Jhin parser semantics. These are protected with permanent compatibility fixtures rather than reimplemented parsers.

Examples include:

- Anime language and subtitle parsing;
- corrected-release traits;
- canonical IMAX and IMAX Enhanced behavior;
- explicit upscale / AI-enhanced detection;
- bit-depth parsing and Hi10P fallback behavior;
- Vidhin-backed matching where regex semantics matter.

## Tier-authority regressions

`TestAdjacentTierCeilingMatrix` is the main table-driven real-engine regression for release-group hierarchy integrity.

It covers the production tier families for Movies, Shows, Anime Movies and Anime Shows. For each adjacent tier pair, the test decorates the lower-tier release with the ordinary positive metadata it can currently receive and verifies through the real engine that a clean candidate one tier higher still ranks above it.

This deliberately avoids relying on a hardcoded maximum-stack constant. A previous scoring audit showed that static assumptions can go stale when new positive metadata is introduced.

Additional focused regressions cover:

- video-codec neutrality;
- audio normalization;
- canonical edition behavior;
- size scoring;
- adaptive filtering thresholds;
- Define Library synchronization;
- generated profile reproducibility.

## Generated profiles

The Samsung and hardware-neutral profiles are generated from one canonical ordered rule registry.

Validation requires:

- `profile.txt` to remain reproducible from the canonical registry;
- `profile-neutral.txt` to remain the same shared ordered policy minus the Samsung-specific Dolby Vision compatibility rule;
- shared rules to preserve their relative order and semantics across both variants.

## Vidhin synchronization

The generated Define Library is synchronized with Vidhin release-group data through GitHub Actions.

The sync process is designed to fail closed when expected upstream regex structures change in ways that require review. Semantic updates are proposed through a pull request before they become part of the published library.

Metadata-only changes are ignored so they do not create unnecessary synchronization pull requests.

## Scheduled upstream check

A scheduled workflow checks the latest StreamNZB release against the repository's compatibility suite. If it fails, GitHub records the failure and uploads the diagnostic log as an artifact for inspection.

The check is an early-warning mechanism only. The repository's normal CI remains pinned to the accepted baseline until a deliberate compatibility update is reviewed and merged.

## CI and workflow security

The repository's validation workflows include deterministic profile/Define/formatter checks, Vidhin synchronization tests, Anime tier integrity checks and the real StreamNZB compatibility harness.

GitHub Actions workflows are also audited for basic security properties, including explicit permissions, avoidance of unsafe privileged-trigger checkout combinations, and avoidance of floating action refs such as `main` or `latest`.

CodeRabbit is used as an advisory PR reviewer. It complements deterministic validation but does not replace or gate the repository's required correctness checks.

For scoring-policy rationale, see [SCORING.md](SCORING.md).