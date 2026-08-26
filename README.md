# StreamNZB Template

My custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V3**

V3 restructures the release-group logic using StreamNZB's new reusable Define rules. The ranking philosophy remains the same as V2, but the profile is significantly easier to maintain and update.

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

## Formatter

The latest formatter is available here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**

For the recommended linked import, use:

**[Raw formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt)**

The formatter can also remain linked and be manually refreshed when a new version is published.

## Important: Hardware-Specific Rules

This profile is tuned for a **Samsung QN90A without an AVR or soundbar**.

The Samsung QN90A does not support Dolby Vision, so the profile contains custom DV/HDR handling. Audio scoring has also been adjusted for a TV-speaker setup, including reduced weighting for lossless audio and Atmos.

If you use a different TV, Dolby Vision display, AVR, soundbar or other audio setup, review these rules and scores before importing the profile.

## Updating

`profile.txt` and `formatter.txt` on the `main` branch are the canonical versions.

If you imported them by URL, use StreamNZB's **Refresh** action to check for updates. StreamNZB will show the proposed changes before anything is applied; updates are never applied automatically.

GitHub's raw-file CDN may take a few minutes to reflect a newly published update.

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **TRaSH Guides, Vidhin and Tamtaro**, adapted for StreamNZB and Usenet.
