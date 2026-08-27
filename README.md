# StreamNZB Template

My custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V3**

V3 separates release-group classification from profile scoring using StreamNZB's shared Define Libraries. The ranking philosophy remains the same as V2, while Vidhin-backed release-group definitions are maintained independently and can be updated without modifying the profile.

The profile is designed around:
- SeaDex Best / Alternative prioritization
- Movie, Show and Anime release-group tiers
- Crunchyroll and HIDIVE detection for Anime WEB releases
- Smart 4K Anime and BluRay filtering
- Suspicious 4K upscale detection
- Adaptive low-quality filtering
- NZB availability and library scoring
- Same-release failover
- Grouped resolution + quality result limits
- Hardware-specific HDR and audio preferences

## Profile

The latest V3 StreamNZB share code is always available here:

**[profile.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile.txt)**

For the recommended linked import, use this URL in StreamNZB:

**[Raw profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)**

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

V3 requires the Define Library described below. Import the library before using the profile.

## Define Library

V3 uses a shared StreamNZB Define Library for its Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 33 Define rules covering Movie, Show and Anime release-group classifications, including the LQ classifications used by the profile.

The definitions are synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex) through GitHub Actions. Upstream changes are reviewed through a pull request before becoming part of the library.

Matching note: Release-group names are generally matched case-insensitively by the generated StreamNZB Define Library. Upstream case-specific distinctions may be normalized when they do not cause cross-tier ambiguity.

After a library update is published, use **Refresh** in StreamNZB to review and apply the changes.

## Formatter

The latest formatter is available here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**

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

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **[TRaSH Guides](https://trash-guides.info/), [Vidhin](https://github.com/Vidhin05/Releases-Regex) and [Tamtaro SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)**, adapted for StreamNZB and Usenet.
