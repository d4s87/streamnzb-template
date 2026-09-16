# DraCuLa Compatibility and Validation

This document contains the technical compatibility and validation details for DraCuLa's StreamNZB Template. For installation and normal use, start with the repository [README](../README.md).

## Supported baseline

Current published compatibility baseline:

- StreamNZB 6.2.0 (commit `c5aa001b0051c0f866768e30c97292c7d1a6c216`)
- Jhin 0.8.0
- StreamNZB profile payload schema v2

The move from StreamNZB 6.1.0/Jhin 0.7.1 to 6.2.0/0.8.0 is a **compatibility
pin update only**. A full real-engine regression audit (source diff of every
commit in the `v6.1.0...v6.2.0` range plus a control-vs-candidate run of the
entire compatibility harness against the exact tagged v6.2.0 source) found no
production rule, formatter, or scoring behavior change — DraCuLa does not
newly implement, depend on, or require any of the following upstream
additions present in this baseline, they are simply compatible with it:

- Jhin 0.8.0's missing-tier evaluation semantics (a rule referencing an
  absent tier settles wherever the answerable side of `and`/`or` is enough
  by itself, rather than skipping the whole rule unconditionally);
- `matchesExcept`, a lookaround-equivalent rule-DSL function;
- formatter `union`/`without` list helpers;
- complete ISO 639-1 language resolution;
- `GET /api/capabilities` and dropped-release diagnostics;
- ffprobe 6.1 / improved Dolby Vision Profile 8 detection (this one does
  improve DraCuLa's real-world accuracy for already-downloaded Library
  candidates — see the Dolby Vision section below — without requiring any
  rule change);
- corrected newznab password-status ingestion (an indexer-side filter
  DraCuLa's rules do not reference).

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

As of pinned StreamNZB v6.1.0, `FormatContext.Subtitles` is also available: a flat, deduplicated language-code list StreamNZB assembles by merging release-name subtitle parsing, indexer-reported subtitle metadata and probed subtitle tracks. Source attribution does not survive that merge. The production formatter renders mapped `.Subtitles` codes through StreamNZB's own `flags` template helper — which already dedupes codes and silently skips any code with no unambiguous flag mapping — placed immediately adjacent to the existing `sᴜʙ` marker (e.g. `sᴜʙ 🇫🇷 🇩🇪`) rather than as a separate badge; DraCuLa adds no release-title regexes, no local language normalization, and no alias tables of its own. The formatter shows `sᴜʙ` when either `.Subbed` is true or `.Subtitles` yields at least one mapped flag — a presentation inference from explicit subtitle metadata, not a mutation or reinterpretation of StreamNZB's own `.Subbed` field, which is read and displayed exactly as reported. The only claim the presentation makes is that subtitle metadata contains these language codes — it does not imply forced/default status, embedded-vs-external, SDH, track count, track identity, source provenance, or a language-to-track binding, and a subtitle code is never treated as an audio-language claim even when it also appears in `.Languages`.

Known upstream limitation: StreamNZB's formatter-facing language normalization currently maps a reported `SLO` tag to `sk` (Slovak) rather than Slovenian `sl`. Confirmed unchanged at the current 6.2.0 baseline. DraCuLa does not attempt a local workaround, because by the time `.Subtitles` reaches the formatter its source/provenance has already been lost — there is no way to distinguish a mis-normalized `SLO` from a legitimate Slovak `sk` without risking false results.

## Dolby Vision Profile 8

StreamNZB 6.2.0 bundles ffprobe 6.1, which corrects Profile 8 Dolby Vision
detection for already-probed releases (an older ffprobe rejected the option
that exposes the DOVI side-data record Profile 8 relies on, so such a
release could measure as plain HDR10). The rule-engine logic that merges a
probe's Dolby Vision reading into the plain `dolbyVision` field DraCuLa's
rules read (`DV without HDR fallback`, `Neutralize Dolby Vision`,
`Generated Dynamic HDR Penalty`) is unchanged between 6.1.0 and 6.2.0 — it
only activates for already-downloaded Library candidates. No DraCuLa rule
change is required: DraCuLa's existing rules simply see more accurate input
for Library candidates than before.

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