# 🧛 DraCuLa's StreamNZB Template
DraCuLa's custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V4.1**

V4.1 builds on the full Vidhin Anime release-group hierarchy introduced in V4 by adding Vidhin-backed Anime LQ filtering. Matching Anime LQ releases receive a `-10,000` penalty, while SeaDex Best and Alternative recommendations are exempt. Movie and Show release-group classification remains separated from profile scoring through StreamNZB's shared Define Libraries, allowing Vidhin-backed definitions to be maintained independently of the profile.

The profile is designed around:
- SeaDex Best / Alternative prioritization
- Movie and Show release-group tiers
- Full Anime WEB T1–T6 and BluRay T1–T8 release-group tiers
- Vidhin-backed Anime LQ filtering with SeaDex Best / Alternative exemptions
- Crunchyroll and HIDIVE detection for Anime WEB releases
- Anime 10-bit / Hi10P detection with formatter labeling
- Anime Uncensored preference with formatter labeling
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

The latest V4.1 StreamNZB share code is always available here:

**[profile.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile.txt)**

For the recommended linked import, use this URL in StreamNZB:

**[Raw profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)**

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

V4.1 requires the Define Library described below. Import the library before using the profile.

## Define Library

V4.1 uses a shared StreamNZB Define Library for its Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 50 Define rules covering Movie, Show and Anime release-group classifications, including Movie, Show and Anime LQ classifications used by the profile.

Anime classifications follow the full Vidhin hierarchy:
- Anime WEB: T1–T6
- Anime BluRay: T1–T8

Separate Anime Movie and Anime Show Defines are provided for every tier.

Anime LQ matching uses Vidhin's upstream release-name regex directly rather than converting it into release-group tokens, preserving the upstream matching semantics.

The definitions are synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex) through GitHub Actions. Upstream changes are reviewed through a pull request before becoming part of the library.

Matching note: Release-group names are generally matched case-insensitively by the generated StreamNZB Define Library. Upstream case-specific distinctions may be normalized when they do not cause cross-tier ambiguity.

After a library update is published, use **Refresh** in StreamNZB to review and apply the changes.

## Anime Scoring

V4.1 uses the full Vidhin Anime tier hierarchy for both Anime Movies and Anime Shows.

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

Anime releases matching Vidhin's Anime LQ classification receive a `-10,000` penalty. SeaDex Best and Alternative recommendations are exempt from this penalty.

Anime 10-bit releases are detected using StreamNZB's parsed bit depth, with an additional `Hi10P` release-name fallback for common Anime naming conventions. The rule is informational and scores `0` points, so it does not alter release ranking.

The formatter displays matching releases as `₁₀ʙɪᴛ`.

Anime releases explicitly marked as `Uncensored`, `Uncut`, `Unrated`, or with an `AT-X` source variant receive a small `+10` preference. The rule is intentionally Anime-only and acts as a tie-breaking preference rather than replacing the release-group tier hierarchy. Because most Anime BluRay releases are not necessarily labeled as uncensored in their release names, the rule should be interpreted as detecting an explicit uncensored-related marker rather than proving whether every release is censored or uncensored.

The formatter normalizes matching releases to `ᴜɴᴄᴇɴꜱᴏʀᴇᴅ`.

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

## Personalizing the Profile Without Losing Updates

The linked profile can be customized for your own hardware and preferences without maintaining a separate copy of DraCuLa's profile.

The recommended approach is to **add your own scoring rules with unique names** instead of editing existing DraCuLa rules.

For example, if your setup supports Dolby Vision or you want to prioritize high-quality audio, you can add personal rules such as:

- `My DV Bonus`
- `My HQ Audio Bonus`

These local-only rules are preserved when the linked DraCuLa profile is refreshed, allowing your personal scoring preferences to remain layered on top of the upstream profile.

> [!IMPORTANT]
> Avoid modifying an existing DraCuLa rule if you want the change to survive future updates.
>
> StreamNZB merges linked-profile updates by rule name. If a rule has the same name as an upstream DraCuLa rule, the upstream version owns that rule and a future **Refresh** may replace your local edits.
>
> Rules you create yourself with **unique names** remain local and are preserved across profile refreshes.

This makes it possible to keep using the canonical DraCuLa profile while adapting scoring to different TVs, AVRs, soundbars or personal preferences.

For example, a Dolby Vision display can add a local positive DV scoring rule rather than changing DraCuLa's existing Samsung-specific DV handling. Likewise, users who prefer lossless or higher-quality audio can add their own audio bonus rule without maintaining a separate version of the complete profile.

The general rule is:

**Keep DraCuLa rules upstream-managed; add your preferences as uniquely named local rules.**

## Updating

`profile.txt`, `formatter.txt` and `generated/streamnzb-defines.txt` on the `main` branch are the canonical versions.

- `profile.txt` — filtering and scoring policy
- `formatter.txt` — result presentation
- `generated/streamnzb-defines.txt` — Vidhin-backed release-group classifications

If you imported them by URL, use StreamNZB's **Refresh** action to check for updates. StreamNZB will show the proposed changes before anything is applied; updates are never applied automatically.

GitHub's raw-file CDN may take a few minutes to reflect a newly published update.

## Validation

The repository includes automated validation for the profile, Define Library, Vidhin synchronization and Anime tier integrity.

For release-matching logic where StreamNZB parser or rule-engine behavior is important, the repository also includes a compatibility harness that runs test fixtures against a pinned revision of the real StreamNZB engine rather than reimplementing its behavior.

New or changed compatibility-sensitive rules can first be developed as fixtures containing representative positive and negative release names. Once a rule is published, the fixture can reference the production rule by name. The harness then decodes `profile.txt`, locates the exact published rule, verifies that its expression and score have not drifted from the tested fixture, and executes the same cases against the production rule.

This approach provides two layers of behavioral validation:

1. **Fixture validation** — verifies the intended rule against StreamNZB's real parser and rule engine.
2. **Production regression validation** — verifies the exact rule shipped in `profile.txt` against the same cases.

The compatibility harness is intentionally used selectively for rules where parsing, traits, regular expressions or other StreamNZB engine behavior can materially affect matching. It is not intended to duplicate every profile rule into a second configuration file.

Validation runs automatically through GitHub Actions.

## Community

Discussion, setup notes and template updates are available in the [DraCuLa's StreamNZB Template Discord thread](https://discord.com/channels/1470288400157380710/1542856068135125002).

For release-specific changes, always refer to this repository's README and changelog.

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **[TRaSH Guides](https://trash-guides.info/), [Vidhin](https://github.com/Vidhin05/Releases-Regex) and [Tamtaro SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)**, adapted for StreamNZB and Usenet.
