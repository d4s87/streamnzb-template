# StreamNZB Template

My custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

## Current Version

**V3** 
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

The latest V2 StreamNZB share code is always available here:

**[profile.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile.txt)**

Or access the share code directly:

**[Raw profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)**

The raw URL remains unchanged when the profile is updated, so it can always be used to retrieve the latest version.

## Formatter

The latest formatter is available here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**

Or directly:

**[Raw formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt)**

## Important: Hardware-Specific Rules

This profile is tuned for a **Samsung QN90A without an AVR or soundbar**.

The Samsung QN90A does not support Dolby Vision, so the profile contains custom DV/HDR handling. Audio scoring has also been adjusted for a TV-speaker setup, including reduced weighting for lossless audio and Atmos.

If you use a different TV, Dolby Vision display, AVR, soundbar or other audio setup, review these rules and scores before importing the profile.

## Updating

`profile.txt` and `formatter.txt` on the `main` branch are the canonical versions.

Future changes will update these files rather than create new download links, so existing links will continue pointing to the latest version.

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **TRaSH Guides, Vidhin and Tamtaro**, adapted for StreamNZB and Usenet.
