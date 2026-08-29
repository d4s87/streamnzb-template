# Changelog

## Version History

- **V1** — Initial StreamNZB filtering, scoring and formatter profile.
- **V2** — Major expansion and refinement of the monolithic profile.
- **V3** — Introduced the shared linked Define Library architecture and Vidhin synchronization.
- **V4** — Expanded Anime classification to the full Vidhin WEB T1–T6 and BluRay T1–T8 hierarchy.
- **V4.1** — Added Vidhin-backed Anime LQ filtering and subsequent Anime detection, formatter and compatibility-testing improvements.

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

- Added an Anime-only **Uncensored** preference rule with a `+10` score.
- Added detection for explicit `Uncensored`, `Uncut`, `Unrated`, and `AT-X` release-name markers.
- Added `ᴜɴᴄᴇɴꜱᴏʀᴇᴅ` formatter labeling for matching Anime releases.
- Added StreamNZB compatibility fixtures covering Uncensored marker variants, boundaries, false positives, Anime scope, and Anime Movies.
- Added production-profile regression coverage for the published Uncensored rule.

### Changed

- Increased the production profile from **86 to 87 rules**.

### Fixed

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
