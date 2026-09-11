# 🧛 DraCuLa's StreamNZB Template

DraCuLa's custom filtering, scoring, Define Library and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V5.2**  
**Compatibility: StreamNZB 5.18.0 / Jhin 0.6.2**

DraCuLa is designed to keep trusted release-group and source quality at the center of StreamNZB ranking while still accounting for availability, Anime-specific preferences, HDR/audio metadata, corrected releases and device compatibility.

The repository publishes two linked profiles:

- `profile.txt` — Samsung QN90A-oriented behavior, including the Dolby Vision compatibility rule.
- `profile-neutral.txt` — the same shared filtering/scoring policy without the Samsung-specific Dolby Vision rejection.

Both profiles use the same shared Define Library and formatter architecture.

## Key features

- SeaDex Best / Alternative prioritization and full Movie, Show and Anime release-group tiers.
- Full Anime WEB T1–T6 and BluRay T1–T8 Vidhin-backed hierarchy.
- Adaptive filtering that keeps weak releases available when search results are sparse.
- Vidhin-backed LQ, Bad Dual, Obfuscated, retag and related release classifications.
- Tier-safe handling of HDR, audio and video-codec metadata so ordinary metadata does not override release-group authority.
- Availability-aware tie-breaking, Library priority and same-release failover.
- Anime-specific service, revision, uncensored, 10-bit and Dual/Multi Audio handling.
- Linked formatter with language/subtitle metadata, corrected-release labels and NZB availability indicators.

For detailed scoring policy and compatibility behavior, see [Scoring Reference](docs/SCORING.md) and [Compatibility and Validation](docs/COMPATIBILITY.md).

## Quick Start

> [!IMPORTANT]
> **Import the Define Library before importing the Profile.**
>
> The profiles reference shared Define rules with `matched()`. Importing a profile first may leave Define references unresolved.

### 1. Import the Define Library

In StreamNZB, import this URL as a **linked Define Library**:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt

Keep it linked so future updates can be applied using StreamNZB's **Refresh** action.

### 2. Import a Profile

Choose the variant that matches your setup.

**Samsung QN90A-oriented profile**

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt

Use this if you want the existing Samsung QN90A behavior. It rejects Dolby Vision releases that do not include an HDR fallback.

**Hardware-neutral profile**

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt

Use this for other TVs or playback chains when you do not want the Samsung-specific Dolby Vision rejection.

Import the selected URL as a **linked profile** so future updates can be reviewed and applied with **Refresh**.

Existing users already linked to `profile.txt` can continue refreshing it normally. That URL remains the Samsung behavior-preserving profile.

### 3. Import the Formatter

Import the normal formatter using:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt

The formatter can also remain linked for future updates.

### 4. Update linked resources

You do **not** need new URLs when a release is published.

Refresh in this order:

1. **Define Library**
2. **Profile**
3. **Formatter** when it changed

StreamNZB shows proposed changes before they are applied.

## Which profile should I use?

### Samsung QN90A

`profile.txt` preserves the behavior originally designed for a Samsung QN90A without an AVR or soundbar.

The Samsung-specific difference is the Dolby Vision compatibility rule: releases with Dolby Vision but no HDR fallback are rejected because the display does not support Dolby Vision.

### Hardware-neutral

`profile-neutral.txt` keeps the same shared DraCuLa filtering and scoring policy but removes that device-specific Dolby Vision rejection.

General format and quality rules remain active. For example, `Reject 3D` is part of the shared Core policy and remains present in both profiles.

If your hardware has different priorities, you can layer local StreamNZB rules on top of either profile without maintaining a fork. See [Personalizing the profile](#personalizing-the-profile).

## Define Library

Both profiles require the shared linked Define Library:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

It provides the release-group classifications used by the profile, including Movie, Show and Anime tiers together with Vidhin-backed classifications such as LQ, Bad Dual, Obfuscated and other supporting groups.

The library is synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex). Upstream semantic changes are proposed through GitHub pull requests before becoming part of the published library.

For implementation details and scoring impact, see [Scoring Reference](docs/SCORING.md).

## Formatter

The normal formatter is published here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**  
**[Raw formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt)**

It displays StreamNZB/Jhin metadata such as language information, subtitle presence, release status, availability and same-release variants in a compact presentation.

Examples of the language/subtitle line:

- `⛿ EN · JA`
- `⛿ EN · JA · sᴜʙ`
- `⛿ sᴜʙ`

The formatter only displays metadata StreamNZB/Jhin exposes; it does not invent subtitle-language identities that are not available in the formatter context.

### Debug formatter

For troubleshooting, an optional verbose formatter is available:

**[formatter-debug.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter-debug.txt)**  
**[Raw debug formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter-debug.txt)**

It exposes detailed parsed metadata, scores, matched rules, availability and other runtime information for results that survive filtering. Rejected results cannot be shown by the formatter because StreamNZB removes them before formatting.

The formatter presentation is inspired by and adapted from [Tamtaro's SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting) for StreamNZB's data and formatting capabilities.

## How scoring works

DraCuLa uses release-group and source authority as the main quality hierarchy. Other metadata is deliberately bounded so it acts as a preference or tie-breaker rather than silently overpowering trusted tiers.

Examples include:

- small corrected-release preferences for PROPER/REPACK variants;
- bounded availability bonuses;
- adaptive filtering instead of rigid rejection when result sets are sparse;
- neutralization of large native HDR/audio/video-codec ranks where they could overturn tier ordering;
- small Anime-specific preferences that remain subordinate to the Anime tier hierarchy.

The exact points, thresholds and engine-level rationale are documented in [Scoring Reference](docs/SCORING.md).

## Personalizing the profile

Linked profiles can be customized without maintaining a separate copy of DraCuLa's profile.

The recommended approach is to add your own StreamNZB scoring rules with **unique names** instead of editing existing DraCuLa rules.

For example:

- `My DV Bonus`
- `My HQ Audio Bonus`

Local rules with unique names are preserved when the linked profile is refreshed.

> [!IMPORTANT]
> Avoid modifying an existing DraCuLa rule if you want the change to survive future updates.
>
> StreamNZB merges linked-profile updates by rule name. If a local rule has the same name as an upstream DraCuLa rule, a future refresh may replace your edit.

In short: **keep DraCuLa rules upstream-managed and layer your preferences on top with uniquely named local rules.**

## Validation and compatibility

The repository validates both generated profile variants, the Define Library, formatter artifacts, Vidhin synchronization and Anime tier integrity.

Compatibility-sensitive behavior is also tested against a pinned revision of the real StreamNZB engine rather than being reimplemented locally. The current accepted baseline is **StreamNZB 5.18.0 / Jhin 0.6.2**.

A separate scheduled workflow checks the latest upstream StreamNZB release for early warning of compatibility regressions. That check is advisory and does not silently change the supported baseline.

For schema details, real-engine regression coverage, formatter compatibility and CI behavior, see [Compatibility and Validation](docs/COMPATIBILITY.md).

## Updating

The canonical published artifacts on `main` are:

- `generated/streamnzb-defines.txt` — shared Define Library
- `profile.txt` — Samsung QN90A-oriented profile
- `profile-neutral.txt` — hardware-neutral profile
- `formatter.txt` — normal result formatter

If you imported them by URL, use StreamNZB's **Refresh** action to review available changes. Updates are not applied automatically.

GitHub's raw-file CDN may take a few minutes to reflect a newly published update.

## Documentation

- [Scoring Reference](docs/SCORING.md) — detailed scoring, normalization, adaptive filtering and tier-authority policy.
- [Compatibility and Validation](docs/COMPATIBILITY.md) — StreamNZB/Jhin baseline, real-engine tests, schema contract and CI behavior.
- [CHANGELOG.md](CHANGELOG.md) — release-specific changes.

## Community

Discussion, setup notes and template updates are available in the [DraCuLa's StreamNZB Template Discord thread](https://discord.com/channels/1470288400157380710/1542856068135125002).

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including [TRaSH Guides](https://trash-guides.info/), [Vidhin](https://github.com/Vidhin05/Releases-Regex) and [Tamtaro SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting), adapted for StreamNZB and Usenet.