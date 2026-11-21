# 🧛 DraCuLa's StreamNZB Template

DraCuLa's custom filtering, scoring, Define Library and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: 6.5.1**  
**Compatibility: StreamNZB 6.5.0 / Jhin 0.8.0**

The compatibility line is the StreamNZB revision tested on `main`. Each release's notes list the baseline that release was validated against; see [Supported baseline](docs/COMPATIBILITY.md#supported-baseline).

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
- Availability-aware tie-breaking, Library priority, a bounded Library Best-N reservation and same-release failover.
- Anime-specific service, revision, uncensored, 10-bit and Dual/Multi Audio handling.
- Non-Anime streaming-service badges on supported WEB releases, preferring Network metadata with collision-safe release-name fallback when unavailable, with no effect on scoring.
- Linked formatter with language/subtitle metadata, corrected-release labels and NZB availability indicators.

For detailed scoring policy and compatibility behavior, see [Scoring Reference](docs/SCORING.md) and [Compatibility and Validation](docs/COMPATIBILITY.md).

## Quick Start

> [!IMPORTANT]
> **DraCuLa requires StreamNZB 6.4.0 or newer.** Upgrade StreamNZB before importing or refreshing the formatter. Older StreamNZB versions are not supported: the formatter still imports, but result names silently fall back to StreamNZB's defaults. See [Compatibility and Validation](docs/COMPATIBILITY.md#supported-baseline).

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

Using StreamNZB's Jellyfin endpoint (Swiftfin, Infuse and similar clients)? See the optional [Jellyfin formatter](#jellyfin-formatter-optional).

### 4. Update linked resources

You do **not** need new URLs when a release is published.

Refresh in this order:

1. **StreamNZB** — upgrade first when a release raises the required version (currently 6.4.0 or newer)
2. **Define Library**
3. **Profile**
4. **Formatter** (and the optional Jellyfin formatter, if you use it) when it changed

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

It displays StreamNZB/Jhin metadata such as language information, subtitle presence, subtitle languages, release status, availability and same-release variants in a compact presentation.

Examples of the language/subtitle line:

- `⛿ EN · JA`
- `⛿ EN · JA · sᴜʙ`
- `⛿ EN · JA · sᴜʙ 🇫🇷 🇩🇪`
- `⛿ sᴜʙ`
- `⛿ sᴜʙ 🇫🇷 🇩🇪`
- `⛿ EN · sᴜʙ 🇫🇷 🇩🇪 🇪🇸 +9`
- `✓ JA · EN`

The stars rate each result against the best one in the list without StreamNZB's built-in resolution bonus, and only filled stars are shown (`★★★` rather than `★★★☆☆`; no stars when the adjusted score is zero or negative), so a strong 1080p release can earn five stars even when a 2160p result ranks above it. Resolution is shown separately, and the numeric score in the description is still the score that sets the order. This requires StreamNZB 6.4.0 or newer.

`✓` means every displayed audio language was measured from the media file's own audio tracks. You can expect it on probed Library results, not on ordinary fresh indexer results. `⛿` means the language information is reported by the indexer or inferred from the release name, or was not fully measured.

The `sᴜʙ` marker is the single subtitle indicator: it now also appears when StreamNZB's merged `FormatContext.Subtitles` contains at least one recognized language, even if the release itself isn't tagged `.Subbed` — known subtitle-language flags are its immediate presentation, not a separate badge. This is a presentation inference from explicit subtitle metadata; StreamNZB's own `.Subbed` field is never mutated or derived from it. The formatter only displays metadata StreamNZB/Jhin exposes; it does not invent subtitle-language identities that are not available in the formatter context, and an unmapped/unknown code is silently skipped rather than shown raw. The formatter shows at most three subtitle-language flags; any additional recognized languages are summarized as `+N` rather than growing the line unbounded.

### Debug formatter

For troubleshooting, an optional verbose formatter is available:

**[formatter-debug.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter-debug.txt)**  
**[Raw debug formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter-debug.txt)**

It exposes detailed parsed metadata, scores, matched rules, availability and other runtime information for results that survive filtering. Rejected results cannot be shown by the formatter because StreamNZB removes them before formatting.

### Jellyfin formatter (optional)

A third, optional formatter is published for people who watch through StreamNZB's Jellyfin endpoint:

**[formatter-jellyfin.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter-jellyfin.txt)** · **[Raw Jellyfin formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter-jellyfin.txt)**

Jellyfin only receives a formatter's **name**: StreamNZB labels each release in the client's version picker with the rendered name, flattened to one line. The normal formatter keeps its name short (`4K 〈ʀᴇᴍᴜx〉 ★★★`) and puts size, audio, HDR and group in the description, which Jellyfin never receives, so different releases can look identical in a picker. DraCuLa Jellyfin puts the information that tells releases apart into the name:

- `4K REMUX DV+HDR ★★★★ · 68 GB · Atmos TrueHD 7.1 · EN · HEVC 10bit`
- `PACK 4K WEB-DL DV+HDR ★★★★ · 94 GB · DD+ 5.1 · EN · HEVC · NTb`
- `1080p BluRay ★★ · 2.8 GB · FLAC 2.0 · JA/EN DUAL · HEVC · FLE`
- `LIB 1080p WEB-DL ★★★ · 6.2 GB · DD+ 5.1 · EN✓ · AVC · NTb`
- `1080p WEB-DL ★★★★★ · 3.4 GB · DD+ 5.1 · EN · AVC · NTb`

Fields appear in this order: `LIB` (Library result) and `PACK` (season pack or complete series), resolution, source quality, HDR/Dolby Vision tokens, stars, file size, audio format and channels, languages with `DUAL`, video codec with bit depth, and release group. Essential information comes first. The label never exceeds 72 code points (Unicode characters, not display width): the essential fields are length-capped and optional fields are added in that order only while they fit, so a later field can be dropped on a long label. The stars are the same as in the normal formatter.

- HDR tokens are the ones the release name lists (`DV`, `HDR`, `HDR10+`, `HLG`, `SDR`). The label does not claim an HDR fallback or device compatibility.
- Lossless and lossy audio stay distinguishable (`TrueHD`, `DTS-HD MA`, `FLAC` versus `DD+`, `DD`, `DTS`); at most two audio formats and one channel layout are shown.
- Languages are listed as codes (`EN/FR/DE +3`). A trailing `✓` means the release's whole language list, including any codes hidden behind `+N`, was measured from the media file's own audio tracks, as in the normal formatter. No mark means that was not established for the full list: the codes may come from the release name or the indexer, and some may even have been measured. `DUAL` repeats the release name's dual-audio claim and is not a verification.
- Sizes use decimal units in three ranges: below 1 GB StreamNZB's own MB form (`450 MB`), from 1 GB to below 9.95 GB one decimal (`2.8 GB`), and from 9.95 GB whole gigabytes, rounded once to the nearest (9.95 GB reads `10 GB`).
- When nothing is known about a release the label reads `Unknown` rather than falling back to the raw release title.

**Using it**

1. Import it like the normal formatter: **Settings → Formatting**, then paste the code or the raw URL above (a linked import can be refreshed later).
2. Bind it on the **Streams** page by choosing it as the stream's **Format profile**.
3. Formatter selection is **per stream, not per client**. StreamNZB does not pick a formatter because a request came from Jellyfin. If you only use Jellyfin, bind it to your existing stream. If you want the normal formatter in Stremio and this one in Jellyfin, create a separate stream for Jellyfin: a Jellyfin login *is* a stream (the username is the stream name), so a separate stream also has its own Jellyfin login, watch progress and favourites.
4. The formatter has an empty description, because Jellyfin does not use it; StreamNZB falls back to its built-in description wherever one is shown, such as Stremio. It is not intended for a stream you read in Stremio.

**What to expect.** The label is a single line. Some clients wrap or truncate long labels, and others scroll them; the design was tested visually in one mobile Jellyfin client, where one- and two-line labels showed without visible truncation, but other clients and screen sizes were not tested and no universal rendering is claimed. Clients that never show a version picker may receive little or no benefit, and Findroid may be one of them. `PACK` is detected from the season and "complete" metadata StreamNZB exposes, so Anime batch releases whose names carry neither are not marked.

It requires StreamNZB 6.4.0 or newer, like the normal formatter.

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

Compatibility-sensitive behavior is also tested against a pinned revision of the real StreamNZB engine rather than being reimplemented locally. The current accepted baseline is **StreamNZB 6.5.0 / Jhin 0.8.0**.

A separate scheduled workflow checks the latest upstream StreamNZB release for early warning of compatibility regressions. That check is advisory and does not silently change the supported baseline.

For schema details, real-engine regression coverage, formatter compatibility and CI behavior, see [Compatibility and Validation](docs/COMPATIBILITY.md).

## Updating

The canonical published artifacts on `main` are:

- `generated/streamnzb-defines.txt` — shared Define Library
- `profile.txt` — Samsung QN90A-oriented profile
- `profile-neutral.txt` — hardware-neutral profile
- `formatter.txt` — normal result formatter
- `formatter-jellyfin.txt` — optional result formatter for Jellyfin clients

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