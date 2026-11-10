# DraCuLa Compatibility and Validation

This document contains the technical compatibility and validation details for DraCuLa's StreamNZB Template. For installation and normal use, start with the repository [README](../README.md).

## Supported baseline

**Minimum supported StreamNZB: 6.4.0.** Older StreamNZB versions are not
supported by the current DraCuLa formatter.

Current published compatibility baseline:

- StreamNZB 6.4.0 (commit `56fbe4914a08c2fc0960b3fce63c923fc24d8ee7`)
- Jhin 0.8.0
- StreamNZB profile payload schema v2

The move from StreamNZB 6.3.0 to 6.4.0 keeps Jhin at 0.8.0 and changes no
parser trait, rule evaluation, aggregate or native score. In `pkg/search`,
`v6.3.0...v6.4.0` extracts the resolution ladder into
`ranking.ResolutionScore()` without changing its values, and adds air-date
acceptance for daily-show episode requests, which is request validation and
not a rule input. Against the exact tagged v6.4.0 source, all
`tests/streamnzb_compat` subtests pass with output identical to a v6.3.0
control run, apart from parallel subtest ordering. The share-code codecs are
unchanged, and no `go.mod` change is needed.

### StreamNZB 6.4.0 minimum (formatter)

StreamNZB 6.4.0 adds `.ScoreWithoutResolution` and
`.TopScoreWithoutResolution` to the formatter context
(`Gaisberg/streamnzb#373`, requested in `Gaisberg/streamnzb#343`). The
production formatter uses them for its stars (see
[Formatter compatibility](#formatter-compatibility)). This is a deliberate,
coordinated raise of the minimum supported StreamNZB version, not a
best-effort compatibility layer:

- DraCuLa publishes one formatter at the existing `formatter.txt` URL. There
  is no separate formatter for StreamNZB 6.3.0 or older.
- Upgrade StreamNZB to 6.4.0 or newer **before** importing or refreshing the
  formatter. Then refresh the Define Library, Profile and Formatter as the
  release notes direct.
- On StreamNZB older than 6.4.0 the formatter still imports, because
  StreamNZB's import-time template validation does not check field names.
  At render time, however, the new fields do not exist, so every result name
  fails to render and StreamNZB silently falls back to its built-in default
  name. Descriptions still render. This behavior was confirmed against the
  real 6.3.0 engine and is unsupported.
- The profile and Define Library do not use any 6.4.0-only feature, but
  DraCuLa validates and supports them only on the current baseline.

The move from StreamNZB 6.2.0 to 6.3.0 is a **compatibility pin update
only**. Jhin stays at 0.8.0. The `v6.2.0...v6.3.0` range does not touch
`pkg/search` or `pkg/release`, so the parser, rule evaluation, aggregates,
native scoring, Library/SeaDex handling and formatter context are unchanged,
and the `SNZBP1`/`SNZBF1` share-code codecs are unchanged. The full harness
passes against the exact tagged v6.3.0 source with output identical to a
v6.2.0 control run, including the production and debug formatter
regressions. The only harness change is `golang.org/x/text` v0.43.0 in
`tests/streamnzb_compat/go.mod`, which StreamNZB 6.3.0 requires.

StreamNZB 6.3.0 adds a server relay for share-code URLs the browser cannot
fetch (`Gaisberg/streamnzb#369`). The browser still fetches first, and the
relay returns the same text for the browser to decode, so linked import and
refresh behave as before. DraCuLa's published `raw.githubusercontent.com`
links send CORS headers and use the direct browser path. This is a
source-based conclusion: the real `SNZBP1` import/refresh path through
StreamNZB's frontend is not exercised by the Go harness.

The earlier move from StreamNZB 6.1.0/Jhin 0.7.1 to 6.2.0/0.8.0 was also a **compatibility
pin update only**. A full real-engine regression audit (source diff of every
commit in the `v6.1.0...v6.2.0` range plus a control-vs-candidate run of the
entire compatibility harness against the exact tagged v6.2.0 source) found no
production rule, formatter, or scoring behavior change — DraCuLa does not
newly implement, depend on, or require any of the following upstream
additions present in this baseline, they are simply compatible with it:

- Jhin 0.8.0's missing-tier evaluation semantics (a rule referencing an
  absent tier settles wherever the answerable side of `and`/`or` is enough
  by itself, rather than skipping the whole rule unconditionally);
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

The check sets `STREAMNZB_REF` for both the rule tests and the formatter regressions, so both run against the selected upstream release. `STREAMNZB_REF` may be a full commit SHA or a lightweight or annotated tag; the formatter harness peels the fetched ref to its commit and fails unless the checkout matches it exactly.

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
- formatter behavior for compatibility-sensitive metadata;
- resolution-independent stars.

The star rating uses
`{{if gt .ScoreWithoutResolution 0}} {{replace (stars 5 .TopScoreWithoutResolution .ScoreWithoutResolution) "☆" ""}}{{end}}`
(StreamNZB 6.4.0+). Only filled stars are shown: the engine's `stars`
helper rounds to the nearest star, and the empty `☆` positions are removed,
so `★★★☆☆` renders as `★★★`. `.ScoreWithoutResolution` is the final score minus only
StreamNZB's built-in resolution ladder (20,000 points per rung, so 2160p
carries 40,000 more than 1080p). DraCuLa rules that score resolution, such as
the `Movies UHD BluRay` tiers, still count. `.TopScoreWithoutResolution` is
the highest adjusted score among the returned results, which need not be the
top-ranked one. Stars therefore rate a release on everything except its pixel
count, which the formatter already shows separately. Ranking, filtering,
Best-N retention and sort order still use the normal `.Score`, and the
description keeps showing `.Score` so the numeric value always explains the
order. Consequences: a strong 1080p release can show five stars below a
weaker 2160p result; 720p and unparsed resolutions, which pay no ladder, rate
on equal terms; a result whose adjusted score is zero or negative shows no
star block at all; and a positive adjusted score below half a star of the
top renders no stars. A real-engine regression ranks mixed-resolution Anime and
Movie pools through the published profile and Define Library and renders
the star labels through StreamNZB's own playlist aggregation, which computes
both top values itself.

Jhin exposes parsed `Languages` separately from the boolean `Subbed` flag. DraCuLa therefore displays the metadata that Jhin actually exports and does not invent subtitle-language identities that are not present in the formatter context.

Since StreamNZB v6.1.0, `FormatContext.Subtitles` is also available: a flat, deduplicated language-code list StreamNZB assembles by merging release-name subtitle parsing, indexer-reported subtitle metadata and probed subtitle tracks. Source attribution does not survive that merge. The production formatter renders mapped `.Subtitles` codes through StreamNZB's own `flags` template helper — which already dedupes codes and silently skips any code with no unambiguous flag mapping — placed immediately adjacent to the existing `sᴜʙ` marker (e.g. `sᴜʙ 🇫🇷 🇩🇪`) rather than as a separate badge; DraCuLa adds no release-title regexes, no local language normalization, and no alias tables of its own. The formatter shows `sᴜʙ` when either `.Subbed` is true or `.Subtitles` yields at least one mapped flag — a presentation inference from explicit subtitle metadata, not a mutation or reinterpretation of StreamNZB's own `.Subbed` field, which is read and displayed exactly as reported. The only claim the presentation makes is that subtitle metadata contains these language codes — it does not imply forced/default status, embedded-vs-external, SDH, track count, track identity, source provenance, or a language-to-track binding, and a subtitle code is never treated as an audio-language claim even when it also appears in `.Languages`.

The language marker is source-aware. StreamNZB's `.Languages` is a merge of probed audio-track languages, the indexer's language tag and release-name parsing, and `.LanguageSource` names only the strongest source that contributed (`measured`, `reported`, `inferred`, or empty). So `measured` can sit next to a language the file never confirmed. `.Verified` alone is not enough either: it is true for any probed Library result, even one whose audio tracks were not read. DraCuLa shows `✓` only when `.LanguageSource` is `measured` and every displayed `.Languages` entry is present in `.Probed.AudioLanguages`; otherwise it keeps `⛿`. The subtitle-only line always uses `⛿`. In the current StreamNZB release only Library results carry probe data, so fresh indexer results never show `✓`.

Known upstream limitation: StreamNZB's formatter-facing language normalization currently maps a reported `SLO` tag to `sk` (Slovak) rather than Slovenian `sl`. Confirmed unchanged at the current 6.4.0 baseline (from source). DraCuLa does not attempt a local workaround, because by the time `.Subtitles` reaches the formatter its source/provenance has already been lost — there is no way to distinguish a mis-normalized `SLO` from a legitimate Slovak `sk` without risking false results.

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
for Library candidates than before. StreamNZB 6.3.0 and 6.4.0 do not change the probe
merge or the bundled ffprobe; this is confirmed from source only, not with a
real Profile 8 file.

## `matchesExcept` and the Movies Anywhere / Max badges

`matchesExcept(text, pattern, exceptPattern)` is a lookaround-equivalent
rule-DSL function, available since Jhin 0.8.0: `pattern` counts as a match
only where no match of `exceptPattern` covers the same offset. DraCuLa's
`Movies Anywhere` non-Anime streaming-service formatter badge was the first
production rule to depend on it, resolving the one concrete collision that
previously kept the badge deferred: bare `MA` (the service tag) also reads
as the tail of `DTS-HD MA` (DTS-HD Master Audio), and a release can carry
both. An ordinary `matches ... and not matches ...` expression reads the
whole release name twice and cannot tell the two apart, so it silently
drops the service badge on any release that also happens to use DTS-HD MA
audio. `matchesExcept` asks where each match landed instead, so the badge
still fires when both are present.

The `Max` badge uses the same function for a different collision shape:
bare `MAX` (the service tag) also reads as the tail of the `HBO Max`
branding prefix, and a release can carry both — e.g. a release tagged with
the standalone service and separately mentioning "HBO Max" branding.
`matchesExcept` excludes only the `HBO Max` occurrence, so the standalone
`MAX` tag elsewhere in the same release name still fires the badge.

Both badges remain zero-point, `presentation`-owned, `not isAnime`-scoped,
and gated by the same WEB-family traits as every other non-Anime service
badge — only the collision guard on the bare token is new in each case.

## Google Play's WEB-adjacency-gated fallback

The `Google Play` non-Anime streaming-service formatter badge does **not**
use `matchesExcept` — its collision shape is different in kind, not just in
detail, from Movies Anywhere/Max. Its canonical token is the bare English
word `Play`, which real, unrelated titles legitimately use as their own
last word (the *Child's Play* franchise renders as
`Childs.Play.<year>.1080p.WEB-DL...` in scene convention). A plain
bounded-anywhere match — the shape every other simple badge uses — would
false-positive on titles like that.

The rule instead requires `Play` to sit immediately adjacent to the
`WEB`/`WEBDL`/`WEBRip` marker itself, mirroring the same adjacency real
scene naming already uses to place a service tag directly in front of the
source-type tag (`AMZN.WEB-DL`, `NF.WEBRip`). In `Childs.Play.<year>...
WEB-DL`, the year/resolution intervene between `Play` and `WEB`, so the
rule correctly does not fire. No new DSL feature is required for this —
it is a narrower *positive* pattern, not an exclusion of a covering match.
`.Network` remains authoritative; the badge is a formatter fallback only,
rendered exactly like every other non-Anime service badge when `.Network`
is empty.

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

`scripts/vidhin_mapping.json` holds one local LQ case exception: `case_insensitive_group_overrides: ["GHOSTS"]` on `Shows LQ Groups`. The generator renders it as `group matches "(?i)^(GHOSTS)$"`, a whole parsed-group match that never matches the release name. The rest of the upstream case-sensitive record, `iVy`, is rendered unchanged. The override stays valid only while `GHOSTS`, spelled exactly, appears in exactly one case-sensitive upstream `LQ (Sonarr)` record. If upstream removes it, respells it, duplicates it or makes it case-insensitive again, the sync fails closed and the override must be re-reviewed or removed. `tests/streamnzb_compat/vidhin_ghosts_lq_sync_test.go` locks the resulting behavior.

Each sync PR includes an advisory SAFE / REVIEW_REQUIRED / BLOCKED assessment. It is built from a semantic diff of the generated Defines, their consumers in both published profiles, and old-vs-new real-engine probes. It never merges or approves a PR. The `Validate Vidhin sync PR` check independently re-runs it, with the pinned engine, against the exact head commit of each `chore/vidhin-sync` PR. See [VIDHIN_SYNC_ASSESSMENT.md](VIDHIN_SYNC_ASSESSMENT.md).

## Scheduled upstream check

A scheduled workflow checks the latest StreamNZB release against the repository's compatibility suite. If it fails, GitHub records the failure and uploads the diagnostic log as an artifact for inspection.

The check is an early-warning mechanism only. The repository's normal CI remains pinned to the accepted baseline until a deliberate compatibility update is reviewed and merged.

## CI and workflow security

The repository's validation workflows include deterministic profile/Define/formatter checks, Vidhin synchronization tests, Anime tier integrity checks and the real StreamNZB compatibility harness.

GitHub Actions workflows are also audited for basic security properties, including explicit permissions, avoidance of unsafe privileged-trigger checkout combinations, and avoidance of floating action refs such as `main` or `latest`.

CodeRabbit is used as an advisory PR reviewer. It complements deterministic validation but does not replace or gate the repository's required correctness checks.

For scoring-policy rationale, see [SCORING.md](SCORING.md).