# DraCuLa Compatibility and Validation

This document contains the technical compatibility and validation details for DraCuLa's StreamNZB Template. For installation and normal use, start with the repository [README](../README.md).

## Supported baseline

Current published compatibility baseline:

- StreamNZB 6.1.0 (commit `2ff93449e59a6597f25fd008e3440920772578c3`)
- Jhin 0.7.1
- StreamNZB profile payload schema v2

StreamNZB v6.0.0 also carries an upstream season-handling fix (`Gaisberg/streamnzb#275`): season is literal end-to-end, so season `0` addresses only the Specials season and no longer doubles as a "no season named" sentinel. Anime absolute-episode/seasonless matching goes through `SeasonlessEpisodeMatchRank`/`TargetMatchRank(Seasonless: true)`; literal Season 0/Specials matching is a separate, still-covered path through `EpisodeMatchRank(0, episode)` — the two are exercised as independent fixtures so they cannot be conflated again.

StreamNZB v6.0.0 also merges indexer-reported language metadata into the rule environment's `languages`/`subtitles` (previously name-parsed only). No current DraCuLa rule consumes either field, so this is a compatibility note, not a behavior change.

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

As of pinned StreamNZB v6.1.0, `FormatContext.Subtitles` is also available: a flat, deduplicated language-code list StreamNZB assembles by merging release-name subtitle parsing, indexer-reported subtitle metadata and probed subtitle tracks. Source attribution does not survive that merge. The production formatter renders `.Subtitles` through StreamNZB's own `flags` template helper, which already dedupes codes and silently skips any code with no unambiguous flag mapping — DraCuLa adds no release-title regexes, no local language normalization, and no alias tables of its own. `.Subbed` and `.Subtitles` are independent signals: `.Subtitles` is rendered on its own condition, never gated on, merged with, or derived from `.Subbed`. The only claim the presentation makes is that subtitle metadata contains these language codes — it does not imply forced/default status, embedded-vs-external, SDH, track count, track identity, source provenance, or a language-to-track binding, and a subtitle code is never treated as an audio-language claim even when it also appears in `.Languages`.

Known upstream limitation: StreamNZB v6.1.0's formatter-facing language normalization (`pttoptions`) currently maps a reported `SLO` tag to `sk` (Slovak) rather than Slovenian `sl`. DraCuLa does not attempt a local workaround, because by the time `.Subtitles` reaches the formatter its source/provenance has already been lost — there is no way to distinguish a mis-normalized `SLO` from a legitimate Slovak `sk` without risking false results.

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

- video-codec neutrality (including VC-1, native to Jhin v0.7.1);
- audio normalization (including DTS:X and DTS-ES, native to Jhin v0.7.1);
- HLG dynamic-range neutrality (native to Jhin v0.7.1);
- canonical edition behavior;
- size scoring;
- adaptive filtering thresholds;
- Define Library synchronization;
- generated profile reproducibility.

`TestPinMoveAdjacentTierMatrix` and `TestPinMoveCombinedAttributeStress` extend the tier-ceiling guarantee to HLG/DTS:X/VC-1/DTS-ES individually and in realistic combination; `TestPinMoveAnimeNeutrality` confirms none of the four introduce a new Anime preference.

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