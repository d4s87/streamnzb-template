# 🧛 DraCuLa's StreamNZB Custom Roles & Scoring Template
DraCuLa's custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V4**

V4 expands the shared Define Library and replaces the previous compressed Anime T1/T2/T3 model with the full Vidhin Anime release-group hierarchy. Movie and Show release-group classification remains separated from profile scoring through StreamNZB's shared Define Libraries, allowing Vidhin-backed definitions to be maintained independently of the profile.

The profile is designed around:
- SeaDex Best / Alternative prioritization
- Movie and Show release-group tiers
- Full Anime WEB T1–T6 and BluRay T1–T8 release-group tiers
- Crunchyroll and HIDIVE detection for Anime WEB releases
- Smart 4K Anime and BluRay filtering
- Suspicious 4K upscale detection
- Adaptive low-quality filtering
- NZB availability and library scoring
- Same-release failover
- Grouped resolution + quality result limits
- Hardware-specific HDR and audio preferences

## Quick Start

> [!IMPORTANT]
> **Import the Define Library before importing the Profile.**
>
> The profile references shared `Define` rules with `matched()`. Importing the profile first may result in missing/unresolved Define references.

### 1. Import the Define Library

In StreamNZB, import the following URL as a **linked Define Library**:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt

Keep it linked so future library updates can be applied using StreamNZB's **Refresh** action.

### 2. Import the Profile

After the Define Library is installed, import the profile using:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt

Import it as a **linked profile** so future updates can also be reviewed and applied with **Refresh**.

### 3. Import the Formatter

Finally, import the formatter using:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt

The formatter can also remain linked for future updates.

### 4. Review Hardware-Specific Rules

The default profile is tuned for a **Samsung QN90A without an AVR or soundbar**.

If your display or audio setup differs, review the Dolby Vision, HDR and audio rules/scores after importing.

### 5. Updating

You do **not** need to import new URLs when a new version is released.

For linked resources, use StreamNZB's **Refresh** action to check for updates:

1. Refresh the **Define Library first**.
2. Review and apply its changes.
3. Refresh the **Profile**.
4. Refresh the **Formatter** when it has changed.

The proposed changes can be reviewed before they are applied.

## Profile

> [!IMPORTANT]
> The shared Define Library must be imported before this profile. See [Quick Start](#quick-start) for the correct installation order.

The latest V4 StreamNZB share code is always available here:

**[profile.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile.txt)**

For the recommended linked import, use this URL in StreamNZB:

**[Raw profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)**

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

V4 requires the Define Library described below. Import the library before using the profile.

## Define Library

V4 uses a shared StreamNZB Define Library for its Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 49 Define rules covering Movie, Show and Anime release-group classifications, including the LQ classifications used by the profile.

Anime classifications follow the full Vidhin hierarchy:
- Anime WEB: T1–T6
- Anime BluRay: T1–T8

Separate Anime Movie and Anime Show Defines are provided for every tier.

The definitions are synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex) through GitHub Actions. Upstream changes are reviewed through a pull request before becoming part of the library.

Matching note: Release-group names are generally matched case-insensitively by the generated StreamNZB Define Library. Upstream case-specific distinctions may be normalized when they do not cause cross-tier ambiguity.

After a library update is published, use **Refresh** in StreamNZB to review and apply the changes.

## Anime Scoring

V4 uses the full Vidhin Anime tier hierarchy for both Anime Movies and Anime Shows.

WEB release groups are scored as follows:
- T1: +500
- T2: +400
- T3: +300
- T4: +200
- T5: +100
- T6: +50

BluRay release groups use the same T1–T6 scores, with the additional lower tiers:
- T7: +25
- T8: +10

`LazyRemux` and `UltraRemux` require a narrow profile-side exception because StreamNZB may interpret `Remux` in their release-group names as the `remux` trait. They remain classified by the Define Library as their corresponding Anime BluRay tiers.

The Smart 4K Anime filter recognizes the complete WEB T1–T6 and BluRay T1–T8 hierarchy. Known Anime release groups can therefore pass the 4K release-group trust check regardless of tier, while unknown 4K Anime groups remain filtered.

## Formatter

The latest formatter is available here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**

The formatter is my attempt to reproduce and adapt the look and presentation of **[Tamtaro's SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)** formatter for AIOStreams within StreamNZB's formatter capabilities. It is not a direct port and has been adapted to work with StreamNZB's available data and formatting system.

For the recommended linked import, use:

**[Raw formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt)**

The formatter can also remain linked and be manually refreshed when a new version is published.

## Important: Hardware-Specific Rules

This profile is tuned for a **Samsung QN90A without an AVR or soundbar**.

The Samsung QN90A does not support Dolby Vision, so the profile contains custom DV/HDR handling. Audio scoring has also been adjusted for a TV-speaker setup.

If you use a different TV, Dolby Vision display, AVR, soundbar or other audio setup, review these rules and scores before importing the profile.

## Updating

`profile.txt`, `formatter.txt` and `generated/streamnzb-defines.txt` on the `main` branch are the canonical versions.

- `profile.txt` — filtering and scoring policy
- `formatter.txt` — result presentation
- `generated/streamnzb-defines.txt` — Vidhin-backed release-group classifications

If you imported them by URL, use StreamNZB's **Refresh** action to check for updates. StreamNZB will show the proposed changes before anything is applied; updates are never applied automatically.

GitHub's raw-file CDN may take a few minutes to reflect a newly published update.

## Community

Discussion, setup notes and template updates are available in the [DraCuLa's StreamNZB Template Discord thread](https://discord.com/channels/1470288400157380710/1542856068135125002).

For release-specific changes, always refer to this repository's README and changelog.

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **[TRaSH Guides](https://trash-guides.info/), [Vidhin](https://github.com/Vidhin05/Releases-Regex) and [Tamtaro SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)**, adapted for StreamNZB and Usenet.
