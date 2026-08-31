# Changelog

## Version History

- **V1** — Initial StreamNZB filtering, scoring and formatter profile.
- **V2** — Major expansion and refinement of the monolithic profile.
- **V3** — Introduced the shared linked Define Library architecture and Vidhin synchronization.
- **V4** — Expanded Anime classification to the full Vidhin WEB T1–T6 and BluRay T1–T8 hierarchy.
- **V4.1** — Added Vidhin-backed Anime LQ filtering and subsequent Anime detection, formatter and compatibility-testing improvements.
- **V4.2** — Added availability-aware HD x265 filtering, adaptive 1080p Remux preference, Anime Uncensored preference, Vidhin-backed Bad Dual penalties, and expanded compatibility validation.
- **V4.3** — Added Anime Streaming Service classification and Network-first formatter fallback, real-engine formatter regression infrastructure, and availability-aware Anime Dubs Only handling.
- **V4.4** — Added ranking and scoring hardening with relative result-score stars, corrected-release and Anime revision preferences, tier-safe audio and availability scoring, Movie edition corrections, same-release failover regression protection, and Intelligent Unknown Resolution fallback.

### Subsequent Improvements

- Added Anime **10-bit** detection using StreamNZB's parsed bit depth.
- Added a `Hi10P` fallback for Anime release names not identified as 10-bit by the parser.
- Added `₁₀ʙɪᴛ` formatter labeling for detected Anime 10-bit releases.
- Added `DUAL` formatter labeling for the existing Dual Audio scoring rule.
- Expanded formatter tier labels to expose Anime WEB T4–T6 and BluRay T4–T8 while preserving the existing non-Anime T1–T3 display behavior.
- Added Vidhin tier-movement reporting to identify release groups moving between upstream tiers.
- Added a StreamNZB compatibility test harness using a pinned real StreamNZB engine.
- Added production-profile regression testing so compatibility fixtures can be validated against the exact rule published in `profile.txt`.

### Testing Approach

Compatibility fixtures are intended for rules where StreamNZB parser or rule-engine behavior requires explicit behavioral validation. Fixtures contain representative positive and negative release names and are evaluated using the real StreamNZB engine.

Published rules can be linked to their fixture through a production-rule reference. CI then verifies that the tested expression and score match the rule embedded in `profile.txt` and executes the same fixture cases against the production rule.

This testing layer complements the existing profile, Define Library, Vidhin synchronization and Anime tier-integrity validation rather than duplicating every profile rule into fixtures.

## Unreleased

Changes in this section are under development and are not part of the latest stable release.

### Added

- Added **Complete Season Pack Preference**: explicitly complete season packs
  for Series and Anime Shows receive a small `+10` ranking preference.
  Ordinary season packs, individual and multi-episode releases, complete
  show packs, Movies, and Anime Movies remain neutral.
- Added pinned real-engine compatibility coverage for the exact published
  Complete Season Pack rule, including Series and Anime Show positives plus
  ordinary packs, single/multi-episode releases, complete show packs, Movies,
  and Anime Movies.
- Added **Movie Edition Preference** for parser-backed Movie editions:
  Director's Cut and Extended Edition share one non-stacking `+25`
  preference using Jhin v0.6's native `edition` metadata.
- Added pinned real-engine coverage for Director's Cut, apostrophe variants,
  Extended Edition, Extended Cut, Movie-only scope, and neutral unsupported
  Final Cut / Criterion Collection / Special Edition forms.
- Added explicit IMAX Enhanced regression coverage. IMAX Enhanced continues
  to receive the existing `+800` IMAX preference and does not receive a
  second stacked edition bonus.

- Added production compatibility validation for Jhin v0.6 rule-name semantics: published profile rule names must be non-empty and unique, and case-only `matched()` / Define-name drift is reported explicitly because `matched()` references are case-sensitive.
- Added permanent real-engine episode-parsing regression coverage for normal multi-episode releases, Anime hybrid season/absolute numbering, absolute episode ranges, dashed Anime season/episode notation, season ranges, and complete season packs against the pinned StreamNZB/Jhin v0.6 engine.

### Changed

- Increased the production profile from **103 to 104 rules** with Complete
  Season Pack Preference. The generated Define Library remains at **53**
  rules because the feature uses native StreamNZB/Jhin facts and introduces
  no new Define dependency.
- Increased the production profile from **104 to 105 rules** with Movie
  Edition Preference. The generated Define Library remains at **53** rules
  because the preference uses native StreamNZB/Jhin `edition` metadata and
  introduces no new Define dependency.
- Expanded the Movie edition ceiling policy so the low-weight Open Matte
  `+25` and Director's Cut / Extended Edition `+25` preferences may combine
  to at most `+50`, remaining below the `200`-point Movie release-group tier
  gap. IMAX remains the deliberate strong exception at `+800`.
- Updated the Anime BluRay tier-ceiling regression for Anime Shows: the
  maximum known effective positive minor-metadata stack increases from
  `+31` to `+41` when Complete Season Pack Preference applies, while the
  existing 70-point tier gaps still leave `29` points of headroom below the
  next-higher clean tier.

- Updated the pinned StreamNZB compatibility revision from `4c0f7b385e5f7bfb514523b908fa04f153dfbbe2` to `f1d55a294b98f4ae7c685ea17cec230b1d12a2bc`, migrating the compatibility harness and formatter regression suite to StreamNZB's Jhin v0.6 rule engine.
- Adapted aggregate compatibility tests to StreamNZB's request-kind-aware Jhin v0.6 aggregate API and updated Unknown Resolution diagnostic assertions for the new engine reporting without changing production scoring or filtering policy.
- Removed the obsolete direct `expr-lang` compatibility-harness dependency after the StreamNZB rule engine migration; Jhin v0.6 now provides the rule-expression engine used by the pinned runtime.

## V4.4

### Added

- Added relative five-star result ranking using StreamNZB's `.TopScore` and `stars` formatter helper. The highest-scoring result renders as `★★★★★`, while lower-scoring results are scaled against the current result-set winner.
- Added real-engine formatter regression coverage for full, partial, negative, and zero-TopScore star rendering.
- Added global corrected-release preference rules: PROPER / REPACK `+5`, REPACK2 `+6`, and REPACK3 `+7`, with mutually exclusive scoring so numbered repacks do not also receive the base bonus.
- Added pinned real-engine compatibility coverage for base, numbered, separated, lowercase, `REAL.*`, unsupported, and false-positive REPACK / PROPER forms, plus structural validation of the published scoring policy.
- Added Anime revision preference rules for explicit `v0`–`v4` release markers: `v0` `-1`, `v1` `+1`, `v2` `+2`, `v3` `+3`, and `v4` `+4`.
- Added mutually exclusive Anime revision matching so releases containing multiple supported `v0`–`v4` markers receive no version score instead of stacking ambiguous revision bonuses.
- Added pinned real-engine compatibility coverage for Anime Shows and Movies, episode-suffix forms such as `01v2`, case variants, REPACK interaction, unsupported `v5+`, false positives, and multi-version non-stacking behavior.
- Added a global **Retag Soft Penalty** of `-1` for recognized redistribution markers including `.heb`, EZTV variants, RARBG, RARTV, and TGx. The rule is intentionally a metadata tie-breaker and never rejects a result.
- Added pinned real-engine compatibility coverage for Retag matching, including spaced and unspaced forms, EZTV variants, case handling, Anime redistribution, clean releases, legitimate Anime bracket groups, and false-positive boundaries.
- Added a dedicated Anime **Dual/Multi Audio Preference** of `+10`, shared between Dual Audio and Multi Audio so the preference remains non-stacking and subordinate to Anime release-group tiers.
- Added pinned real-engine compatibility coverage for Anime/non-Anime Dubbed, Dual Audio and Multi Audio behavior, including Anime Movies and combined Dual+Multi release names.
- Added structural validation for the split Anime/non-Anime audio policy so Anime cannot silently inherit the legacy high-value audio bonuses again.

### Changed

- Replaced the unconditional Unknown Resolution rejection with adaptive **Intelligent Unknown Resolution** handling. Weak unknown-resolution results are pruned only when more than six alternatives have both known resolution and known quality; scarce fallbacks remain available.

- Moved the `💚 ɴᴢʙ` availability indicator from the result name to the description score line, keeping the compact result name focused on resolution, quality, and relative ranking.
- Updated the pinned StreamNZB compatibility revision to `4c0f7b385e5f7bfb514523b908fa04f153dfbbe2` to validate the `.TopScore` and `stars` formatter API against the real engine.
- Increased the production profile from **101 to 110 rules** across the current Unreleased scoring additions: three global corrected-release preference rules, five Anime revision preference rules, and one global Retag soft-penalty rule. The generated Define Library remains at **53** rules because these features use native StreamNZB parser traits and/or direct release-name matching rather than new Vidhin-backed classifications.
- Increased the production profile from **110 to 111 rules** with the dedicated Anime Dual/Multi Audio preference; the subsequent non-Anime audio ceiling correction consolidated three legacy scoring rules into one shared rule, reducing the current production profile to **109 rules**. The generated Define Library remains at **53** rules.
- Replaced the legacy non-Anime `Dubbed bonus` (`+500`), `Dual audio` (`+200`), and `Multi audio` (`+200`) rules with one shared, non-stacking `+10` `Non-Anime Dubbed/Dual/Multi Audio Preference`. Real-engine validation confirmed that DUBBED, Dual Audio, Multi Audio, and combined Dual+Multi releases now receive exactly one `+10` preference instead of reachable `+700`/`+900` stacks.
- Corrected the Anime Streaming Service documentation to reflect the intentional TRaSH-recommended preference scale (`CR +6`, `DSNP +5`, `NF +4`, `AMZN/VRV +3`, `FUNi +2`, `ABEMA/ADN +1`, B-Global/Bilibili/HIDIVE `0`) rather than describing those rules as score-neutral.

- Normalized positive availability scoring so it acts only as a tie-breaker: `Alive on our backbone` is now `+20` and `Recently confirmed` is `+10`, for a maximum combined positive availability contribution of `+30`.
- Removed the `Very fresh NZB`, `Recent NZB`, `Popular NZB`, `Very popular NZB`, and `Highly popular NZB` score rules. Freshness and grab-count metadata remain formatter-visible but no longer affect ranking directly.
- Reduced the current production profile from **109 to 103 rules** after removing the redundant Library rule and five freshness/popularity scoring rules. The generated Define Library remains at **53** rules.

### Fixed
- Protected useful incomplete-metadata results from adaptive Unknown Resolution pruning when they have recognized quality, match a trusted Movie/Show/Anime release-group tier, are Library results, or are SeaDex Best/Alternative recommendations. Missing SeaDex lookup data fails open, and known-resolution Unknown Quality results remain unaffected. Pinned real-engine regression coverage verifies the complete production policy.
- Rescaled Anime BluRay release-group tiers to +500/+430/+360/+290/+220/+150/+80/+10 (T1–T8), creating uniform 70-point gaps so the known +31 cumulative positive minor-metadata stack cannot overtake the next-higher clean tier; Anime WEB tier scores remain unchanged.

- Fixed a reachable Anime scoring inversion where StreamNZB's `dubbed` parser trait caused Dual/Multi Audio Anime releases to inherit the generic `+500` Dubbed bonus in addition to the legacy `+200` Dual/Multi score. Anime Dual/Multi Audio now receives one `+10` same-tier preference instead of an effective `+700` bonus.

- Fixed **Movie-version preference scoring** so `IMAX` and `Open matte` are explicitly Movie-only instead of global rules that could override Show and Anime release-group hierarchies.
- Corrected the Movie edition preference scale from `IMAX +1000` / `Open Matte +500` to `IMAX +800` / `Open Matte +25`. IMAX remains an intentional strong Movie preference, while Open Matte now remains below the 200-point Movie release-group tier gap. Real-engine regression coverage verifies Movie-only scope, matching boundaries, `+825` combined stacking, and representative tier interactions.
- Fixed Library scoring double-counting by removing the profile-level `Library hit +500` rule. StreamNZB's `4k` preset already supplies the intended native `+500` Library bonus; the previous combination produced an effective `+1000` preference.
- Fixed Anime and non-Anime Dubbed/Dual/Multi Audio scoring against StreamNZB's native `-1000` dubbed/audio rank. The published rules now use raw `+1010` compensation so the complete ranking pipeline produces the intended effective `+10` non-stacking preference.
- Added full-pipeline regression coverage for native Library scoring, the `+30` positive availability ceiling, and effective Anime/non-Anime audio scoring, and updated the Anime BluRay ceiling regression to distinguish raw audio compensation from its effective `+10` preference.


## V4.3

### Added

- Added Vidhin-backed **Anime Dubs Only** classification with a StreamNZB-safe translation of the upstream release-name expression.
- Added an availability-aware **Anime Dubs Only Penalty** of `-10`: dub-only Anime is demoted only when a non-dub Anime alternative exists, so scarce dub-only results remain available.
- Added explicit Dual/Multi Audio protection so legitimate Dual Audio releases are not classified as dub-only, including known dub groups.
- Added aggregate fixture and production-profile regression coverage for dub-only Anime Shows and Movies, Dual Audio protection, non-Anime exclusion, and scarce-result preservation.

- Added Anime WEB **Streaming Service** detection for Crunchyroll (`CR`), Disney+ (`DSNP`), Netflix (`NF`), Amazon (`AMZN`), VRV, Funimation (`FUNi`), ABEMA, ADN, B-Global, Bilibili, and HIDIVE.
- Added StreamNZB compatibility fixtures covering positive service markers, boundaries and false positives, Anime scope, BluRay exclusion, and Anime Movie WEB matching.
- Added production-profile regression coverage for all Streaming Service detection rules.
- Added a human-readable formatter development source at `tests/streamnzb_compat/formatter.source.json`.
- Added `scripts/build_formatter.py` to build `formatter.txt` from the human-readable source and verify source/published semantic synchronization.
- Added a formatter render regression harness using StreamNZB's pinned real formatter engine for candidate and production simulation.
- Added formatter fixtures covering Network-first display, Streaming Service fallback priority, duplicate prevention, source/Edition presentation, and representative full Anime WEB and Movie Remux renders.
- Added fresh-render safeguards by disabling the Go test cache and logging SHA-256 fingerprints of the formatter and fixture inputs.

### Changed

- Expanded the generated Define Library from **52 to 53** reusable classifications with `Anime Dubs Only`.

- Increased the production profile from **91 to 101 rules**.
- Updated the formatter to use matched Streaming Service rules as a fallback when `.Network` is unavailable, while preserving `.Network` as the preferred source label.
- Removed the separate Crunchyroll and HIDIVE formatter badges now that those services participate in the unified Network-first source display.
- Extended the StreamNZB compatibility suite to require semantic synchronization between the formatter source and `formatter.txt` and to run the published formatter regression automatically.

### Fixed

- Fixed formatter source/Edition spacing so source plus Edition renders as `♛ source · edition »`, source-only renders as `♛ source »`, and Edition-only no longer receives an orphan leading separator.

## V4.2

### Added

- Added an availability-aware **1080p Remux Preference** rule: non-Anime, non-Library 1080p Remuxes receive a `+50` bonus when SDR 2160p WEB-DL alternatives exist but no HDR/HDR10+ 2160p WEB-DL is available.
- Added aggregate compatibility coverage for the 1080p Remux preference across Movies and Shows, including SDR 4K WEB-DL, HDR, HDR10+, Dolby Vision-only, Dolby Vision with HDR fallback, Anime, Library, 2160p Remux, and unsupported content-kind cases.
- Added production-profile regression and structural validation for the published **1080p Remux Preference** rule.
- Added availability-aware **Adaptive HD x265** filtering: non-Anime SDR 720p/1080p HEVC releases are rejected only when more than six suitable same-resolution AVC alternatives exist, while 2160p, HDR/Dolby Vision, Anime, Library, HEVC Remux, and AV1 results remain exempt.
- Added an Anime-only **Uncensored** preference rule with a `+10` score.
- Added detection for explicit `Uncensored`, `Uncut`, `Unrated`, and `AT-X` release-name markers.
- Added `ᴜɴᴄᴇɴꜱᴏʀᴇᴅ` formatter labeling for matching Anime releases.
- Added StreamNZB compatibility fixtures covering Uncensored marker variants, boundaries, false positives, Anime scope, and Anime Movies.
- Added production-profile regression coverage for the published Uncensored rule.
- Added separate Vidhin-backed **Movie Bad Dual Groups** and **Show Bad Dual Groups** classifications.
- Added **Movie Bad Dual Penalty** and **Show Bad Dual Penalty** rules with a `-10,000` score.
- Added raw upstream group-regex synchronization so Bad Dual expressions retain regex-specific semantics instead of being flattened into literal release-group tokens.
- Added compatibility fixtures for Movie/Show Bad Dual matching, Radarr/Sonarr-specific groups, raw-regex behavior, content scope, negative cases, and Anime Show exclusion.
- Added Define-Library-aware compatibility testing so `matched()`-based rules can be exercised against the generated shared Defines and exact production rules.
- Added an explicit StreamNZB profile-schema compatibility guard that pins `streamnzb_profile == 1` and fails when a missing or future schema version requires compatibility review.

### Changed

- Increased the production profile from **86 to 91 rules** across the current Unreleased changes.
- Expanded the generated Define Library from **50 to 52** reusable classifications.
- Extended the StreamNZB compatibility harness to load the generated Define Library when compiling rules that use `matched()`.

### Fixed

- Fixed the Movie and Show Bad Dual penalty rules to use explicit `movie` and `series` scopes instead of appearing as **All Content** in StreamNZB.

## V4.1

### Added

- Added Vidhin-backed **Anime LQ Groups** classification using the upstream release-name regex.
- Added an **Anime LQ Penalty** of `-10,000` for matching Anime releases.
- SeaDex **Best** and **Alternative** recommendations are exempt from the Anime LQ penalty.
- Expanded the shared Define Library from **49 to 50** reusable classifications.
- Added raw `releaseName` regex support to the Vidhin synchronization generator.
- Added validation for Anime LQ mapping, matching behavior, and metadata-only synchronization changes.

### Notes

- Anime LQ matching preserves Vidhin's upstream regex semantics rather than converting the expression into release-group tokens.
- The linked StreamNZB Define Library now contains **50** Define rules.

## V4

### Changed

* Expanded the shared Define Library from 33 to 49 reusable release-group classifications.
* Replaced the compressed Anime T1/T2/T3 model with the full Vidhin Anime tier hierarchy:

  * WEB T1–T6
  * BluRay T1–T8
* Expanded Anime Movie and Show release-group scoring from 12 to 28 rules.
* Updated Anime tier scores to:

  * T1: +500
  * T2: +400
  * T3: +300
  * T4: +200
  * T5: +100
  * T6: +50
  * BluRay T7: +25
  * BluRay T8: +10
* Updated `Reject bad 4K Anime` to recognize the complete Anime WEB and BluRay tier hierarchy.
* Added handling for `LazyRemux` and `UltraRemux`, which StreamNZB may classify with the `remux` trait because of their release-group names while not exposing the expected BluRay trait.
* Preserved the existing non-Anime Movie/Show tier structure and overall V3 ranking and filtering philosophy.

### Validation

* Validated Anime WEB tiers T1–T6 and BluRay tiers T1–T8 against representative release names.
* Verified `LazyRemux` as Anime BluRay T4 and `UltraRemux` as Anime BluRay T5.
* Verified known low-tier 4K Anime groups remain eligible while unknown 4K Anime release groups are rejected.

### Notes

* V4 requires the linked StreamNZB Define Library containing all 49 Define rules.
* Anime release-group classifications remain synchronized with `Vidhin05/Releases-Regex` through the repository's existing GitHub Actions workflow.
* `Define` rules classify releases only; scoring and filtering behaviour remains controlled by the profile.

## V3

### Changed
- Refactored release-group tiers into reusable `Define` rules.
- T1/T2/T3 scoring rules now reference shared group definitions with `matched()`.
- Updated smart 4K filtering rules to reference the same reusable definitions.
- Removed duplicated release-group regex logic from conditional filters.
- Preserved existing V2 ranking/filtering behaviour while improving maintainability.

### Define Library
- Moved Vidhin-backed release-group definitions out of `profile.txt` into a shared StreamNZB Define Library.
- The library is maintained in `generated/streamnzb-defines.txt`.
- Added 33 shared Define rules covering Movie, Show and Anime release-group classifications, including Movie and Show LQ groups.
- Release-group definitions are synchronized with Vidhin05/Releases-Regex through GitHub Actions.
- Upstream changes are reviewed through pull requests before being published to the library.
- `profile.txt` retains the scoring and filtering policy and references library definitions through `matched()`.
- Linked Define Library updates can be reviewed and applied using StreamNZB's **Refresh** action.

### Notes
- `Define` rules do not score, reject, limit, or appear in `.MatchedRules`.
- V3 requires the shared Define Library to be imported before using `profile.txt`.
- This version requires a StreamNZB build with shared Define Library and `matched()` support.

## V2

### Changed
- Expanded and refined the original StreamNZB filtering and scoring profile.
- Improved release-group prioritization and quality-based ranking.
- Added more advanced filtering and scoring logic for Movies, Shows and Anime.
- Added smart filtering for 4K releases and low-quality results.
- Refined Anime handling, including release-group tiers and streaming-source preferences.
- Improved result limiting and fallback behaviour.
- Continued tuning HDR, Dolby Vision and audio preferences for the target hardware setup.

### Notes
- V2 predates the shared StreamNZB Define Library architecture.
- Release-group regexes and classifications were embedded directly in the profile rules.
- At this stage, GitHub was used to distribute the linked `profile.txt` and `formatter.txt`; there was no separately linked or automatically synchronized Define Library.
- V2 formed the behavioral foundation that was later refactored into V3 without intentionally changing its overall ranking and filtering philosophy.

## V1

### Added
- Initial public version of the custom StreamNZB profile.
- Introduced the core filtering and scoring approach for Usenet results.
- Added release-group prioritization for Movies, Shows and Anime.
- Added quality, resolution, HDR/Dolby Vision and audio preferences.
- Added the custom StreamNZB formatter.

### Notes
- V1 used profile-local filtering, scoring and release-group regexes.
- GitHub distribution consisted of the linked `profile.txt` and `formatter.txt`.
- Shared Define Libraries and automated Vidhin synchronization were not yet part of the setup.
