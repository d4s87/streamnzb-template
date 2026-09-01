# 🧛 DraCuLa's StreamNZB Template
DraCuLa's custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V5.0**

V5.0 introduces generated profile variants built from one canonical ordered rule registry. The existing `profile.txt` preserves the Samsung QN90A-oriented behavior, while `profile-neutral.txt` provides a hardware-neutral alternative without the five Samsung/device-specific playback compensation rules. Both profiles share the same Core policy, presentation classifications, Define Library, and formatter architecture.

The profile is designed around:
- SeaDex Best / Alternative prioritization
- Movie and Show release-group tiers
- Full Anime WEB T1–T6 and BluRay T1–T8 release-group tiers
- Vidhin-backed Anime LQ filtering with SeaDex Best / Alternative exemptions
- Anime WEB Streaming Service preference scoring using the TRaSH-recommended source hierarchy, with Network-first formatter fallback
- Anime 10-bit / Hi10P detection with formatter labeling
- Anime Uncensored preference with formatter labeling
- Vidhin-backed Movie and Show Bad Dual release-group penalties
- Availability-aware Vidhin-backed Anime Dubs Only preference with a non-stacking `+10` Anime Dual/Multi Audio preference
- Smart 4K Anime and BluRay filtering
- Suspicious 4K upscale detection
- Adaptive low-quality filtering
- Adaptive 1080p Remux preference over SDR 4K WEB-DL
- Corrected-release REPACK / PROPER tie-breaking preference
- Retag soft penalty for recognized redistribution markers
- Anime revision preference for explicit `v0`–`v4` release markers
- Tier-safe NZB availability tie-breaking and native StreamNZB library prioritization
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

After the Define Library is installed, choose the profile that matches your setup.

For the existing **Samsung QN90A-oriented** behavior, import:

[https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)

For the **hardware-neutral** variant, import:

[https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt)

Import the selected URL as a **linked profile** so future updates can also be reviewed and applied with **Refresh**.

Existing users already linked to `profile.txt` can continue refreshing it normally. That URL remains the Samsung behavior-preserving profile and does not silently switch to the neutral policy.

### 3. Import the Formatter

Finally, import the formatter using:

https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt

The formatter can also remain linked for future updates.

### 4. Choose the Appropriate Hardware Policy

`profile.txt` preserves the existing **Samsung QN90A without an AVR or soundbar** behavior.

`profile-neutral.txt` keeps the shared DraCuLa filtering and scoring policy but removes the five Samsung/device-specific playback compensation rules for Dolby Vision, Atmos, TrueHD and DTS Lossless handling.

The neutral profile does **not** remove general format classification or hardware-independent filtering. In particular, **Reject 3D is a Core rule and is present in both profiles**.

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
> The shared Define Library must be imported before either profile. See [Quick Start](#quick-start) for the correct installation order.

The repository publishes two generated StreamNZB profile variants from one canonical ordered rule registry.

### Samsung QN90A Profile

The existing `profile.txt` URL remains the behavior-preserving Samsung QN90A-oriented profile:

**[profile.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile.txt)**

For the recommended linked import:

**[Raw Samsung profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile.txt)**

This artifact retains all **105** existing rules and is generated byte-for-byte identically to the pre-V5 architecture.

### Hardware-Neutral Profile

For setups that should not inherit the Samsung-specific playback compensation policy:

**[profile-neutral.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile-neutral.txt)**

For the recommended linked import:

**[Raw neutral profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt)**

The neutral artifact contains **100** rules. It is the Samsung profile minus exactly these five device-specific rules:

- `DV without HDR fallback`
- `Neutralize Dolby Vision`
- `Reduce Atmos`
- `Reduce TrueHD bonus`
- `Reduce DTS Lossless bonus`

All 100 shared rules are identical and retain the same relative order in both variants. `Reject 3D` is part of the hardware-neutral Core policy and therefore remains present in both profiles.

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

Existing `profile.txt` users do not need to migrate if they want to keep the current Samsung-oriented behavior. To switch to the neutral policy, import `profile-neutral.txt` as a **new linked profile** and switch the relevant StreamNZB profile assignments to it.

Both profile variants require the shared Define Library described below. Import the library before using either profile.

## Define Library

V5.0 uses one shared StreamNZB Define Library for both profile variants and their Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 53 Define rules covering Movie, Show and Anime release-group classifications, including Movie, Show and Anime LQ classifications, separate Movie/Show Bad Dual classifications, and the Anime Dubs Only release-name classification used by the profile.

Anime classifications follow the full Vidhin hierarchy:
- Anime WEB: T1–T6
- Anime BluRay: T1–T8

Separate Anime Movie and Anime Show Defines are provided for every tier.

Anime LQ matching uses Vidhin's upstream release-name regex directly rather than converting it into release-group tokens, preserving the upstream matching semantics.

Bad Dual classifications are synchronized separately from Vidhin's Radarr and Sonarr Bad Dual group definitions. Their upstream group regexes are preserved directly rather than flattened into literal release-group tokens, including regex-specific matching semantics.

The Define Library performs classification only. Matching non-Anime Movie and Show releases receive a `-10,000` profile penalty. Anime Shows are deliberately excluded from the Show Bad Dual penalty; Anime Movies are outside the Movie-scoped Bad Dual classification.

The definitions are synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex) through GitHub Actions. Upstream changes are reviewed through a pull request before becoming part of the library.

When a semantic Define Library change is published to `main`, the repository's Discord notification automation can announce the changed classifications and remind linked-library users to use StreamNZB's **Refresh** action. Metadata-only synchronization changes do not generate an update notification.

Matching note: Release-group names are generally matched case-insensitively by the generated StreamNZB Define Library. Upstream case-specific distinctions may be normalized when they do not cause cross-tier ambiguity.

After a library update is published, use **Refresh** in StreamNZB to review and apply the changes.

## Anime Scoring

V5.0 retains the full Vidhin Anime tier hierarchy for both Anime Movies and Anime Shows across both profile variants.

WEB release groups are scored as follows:
- T1: +500
- T2: +400
- T3: +300
- T4: +200
- T5: +100
- T6: +50

BluRay release groups use a wider 70-point tier ladder:
- T1: +500
- T2: +430
- T3: +360
- T4: +290
- T5: +220
- T6: +150
- T7: +80
- T8: +10

The wider BluRay gaps protect the release-group hierarchy from cumulative minor metadata preferences. The current maximum known positive Anime metadata stack is `+41` (`Dual/Multi Audio +10`, `Uncensored +10`, `v4 +4`, `REPACK3 +7`, and Complete Season Pack `+10`), leaving `29` points of headroom below every next-higher clean BluRay tier. Anime WEB scores are intentionally unchanged.

Anime releases matching Vidhin's Anime LQ classification receive a `-10,000` penalty. SeaDex Best and Alternative recommendations are exempt from this penalty.

Anime 10-bit releases are detected using StreamNZB's parsed bit depth, with an additional `Hi10P` release-name fallback for common Anime naming conventions. The rule is informational and scores `0` points, so it does not alter release ranking.

The formatter displays matching releases as `₁₀ʙɪᴛ`.

The profile also detects common **Streaming Services** for Anime WEB releases and applies the small source-preference scale recommended by TRaSH for Anime: Crunchyroll (`CR`) `+6`, Disney+ (`DSNP`) `+5`, Netflix (`NF`) `+4`, Amazon (`AMZN`) `+3`, VRV `+3`, Funimation (`FUNi`) `+2`, ABEMA `+1`, ADN `+1`, while B-Global, Bilibili, and HIDIVE score `0`. These are deliberately small source preferences and remain subordinate to the Anime release-group tier hierarchy.

The formatter remains **Network-first**: when StreamNZB provides `.Network`, that value is displayed as the source. If `.Network` is absent, the formatter falls back to the matched Streaming Service rule. This provides a source label for Anime WEB releases where the service can be identified from the release name without overriding StreamNZB's parsed Network metadata.

Anime releases explicitly marked as `Uncensored`, `Uncut`, `Unrated`, or with an `AT-X` source variant receive a small `+10` preference. The rule is intentionally Anime-only and acts as a tie-breaking preference rather than replacing the release-group tier hierarchy. Because most Anime BluRay releases are not necessarily labeled as uncensored in their release names, the rule should be interpreted as detecting an explicit uncensored-related marker rather than proving whether every release is censored or uncensored.

The profile also recognizes explicit Anime release revision markers from `v0` through `v4`. These are deliberately small tie-breakers: `v0` receives `-1`, while `v1`, `v2`, `v3`, and `v4` receive `+1`, `+2`, `+3`, and `+4` respectively. Unversioned releases and unsupported `v5+` markers receive no Anime-version score.

Anime revision matching is restricted to Anime Movies and Shows through `isAnime`. Common forms such as `01v2`, `E01v3`, dot-separated markers, brackets, and uppercase `V` are supported without treating embedded strings such as `Notv2` or `v20` as revision markers. If a release contains more than one supported `v0`–`v4` marker, the Anime-version rules intentionally apply no score rather than stacking or guessing which revision is valid.

Anime revision scoring is independent of the global REPACK / PROPER preference. A release may therefore receive both its explicit Anime revision tie-breaker and the normal corrected-release bonus when both forms are legitimately present.


Anime releases classified by the Vidhin-backed **Anime Dubs Only** Define receive a small `-10` preference penalty only when another Anime result exists that is not classified as dub-only. This makes the rule a ranking tie-breaker rather than a filter: when all available releases are dub-only, no penalty is applied. Explicit Dual/Multi Audio forms are excluded from the classification, including protected known dub groups.

Explicit Anime **Dual Audio** and **Multi Audio** releases receive one shared, non-stacking **effective `+10` preference**. StreamNZB's 4K preset applies a native `-1000` dubbed/audio rank to these releases, so the published profile rule uses a raw `+1010` compensation. After the complete ranking pipeline, the net preference is exactly `+10`. This keeps Dual/Multi Audio as a same-tier tie-breaker rather than allowing it to replace the release-group hierarchy. A release containing both `Dual Audio` and `Multi Audio` still receives only one effective `+10`.

Audio-language preferences are deliberately small, shared, and non-stacking. Non-Anime Movies and Shows use the same compensation model through StreamNZB's parsed `dubbed` trait, which covers DUBBED, Dual Audio, and Multi Audio releases: the profile's raw `+1010` rule offsets StreamNZB's native `-1000` rank and leaves an effective `+10`. This replaces the historical `Dubbed bonus` (`+500`) plus independent Dual/Multi (`+200`) rules, which produced reachable `+700` and `+900` stacks capable of overriding Movie/Show release-group tiers. Anime remains isolated on its dedicated shared effective `+10` Dual/Multi Audio preference.

Users who prefer dubbed Anime should leave the upstream DraCuLa rule untouched and add a uniquely named local positive scoring rule instead, so their preference survives linked-profile refreshes.

The formatter normalizes matching releases to `ᴜɴᴄᴇɴꜱᴏʀᴇᴅ`.

`LazyRemux` and `UltraRemux` require a narrow profile-side exception because StreamNZB may interpret `Remux` in their release-group names as the `remux` trait. They remain classified by the Define Library as their corresponding Anime BluRay tiers.

The Smart 4K Anime filter recognizes the complete WEB T1–T6 and BluRay T1–T8 hierarchy. Known Anime release groups can therefore pass the 4K release-group trust check regardless of tier, while unknown 4K Anime groups remain filtered.

## Complete Season Pack Preference

Series and Anime Show releases explicitly parsed by StreamNZB as both a
`seasonPack` and `complete` receive a small `+10` ranking preference.

The rule is deliberately narrow. The following remain neutral:

- ordinary season packs such as `S01`
- individual episodes
- multi-episode releases
- complete show-wide packs
- Movies
- Anime Movies

The preference is a tie-breaker rather than a quality override. For Anime
Shows, adding the Complete Season Pack preference raises the maximum known
effective positive minor-metadata stack from `+31` to `+41`. The 70-point
Anime BluRay tier gaps therefore still leave `29` points of headroom below
the next-higher clean tier.

Real-engine validation against the pinned StreamNZB/Jhin v0.6 runtime
confirms the `seasonPack` and `complete` facts, their combined condition,
and the expected parser behavior for ordinary packs, complete packs,
individual episodes, multi-episode releases, and complete show packs.

StreamNZB currently exposes these as release-side facts only. It does not
expose request-side metadata telling the profile whether the requested
season has actually finished airing. DraCuLa therefore does not infer that
an ordinary `S01` pack represents a completed season. Automatically
preferring unmarked packs for seasons known to be complete would require
additional upstream season-completion or episode-count request metadata.

## Movie Edition Preferences

Movie-version preferences are explicitly limited to **Movies** so edition
markers cannot alter Show or Anime ranking.

Current Movie edition scoring:

- **IMAX:** `+800`
- **Open Matte:** `+25`
- **Director's Cut / Extended Edition:** one shared, non-stacking `+25`

IMAX is intentionally a strong Movie-version preference. It may outrank a
higher release-group tier when the user is choosing between otherwise
eligible Movie releases; this is deliberate rather than a tier-ceiling bug.

**IMAX Enhanced does not receive a second bonus.** Pinned Jhin v0.6 parses
`IMAX Enhanced` as edition `IMAX`, and the existing bounded IMAX production
rule already matches the release-name marker. It therefore receives the
existing `+800` IMAX preference rather than stacking to `+1600`.

Open Matte and Director's Cut / Extended Edition are deliberately much
smaller. Director's Cut and Extended Edition use StreamNZB/Jhin's parsed
`edition` metadata and share one `+25` rule, so alternate spellings such as
`Directors Cut`, `Director's Cut`, `Extended Edition`, and `Extended Cut`
cannot stack with each other.

The maximum low-weight Movie edition stack is therefore `+50` when Open
Matte and the shared Director's Cut / Extended Edition preference both
apply. This remains safely below the `200`-point Movie release-group tier
gap.

Criterion Collection, Final Cut, and generic Special Edition currently
remain score-neutral. The pinned Jhin v0.6 parser does not classify those
forms as edition metadata. DraCuLa deliberately avoids broad raw release-name
fallbacks that could confuse title text or loose markers with canonical
edition metadata.

Pinned real-engine regression coverage protects:

- IMAX and IMAX Enhanced behavior
- Movie-only scope
- Open Matte matching
- Director's Cut / Extended Edition parsing and scoring
- non-stacking alternate-cut behavior
- neutral Criterion / Final Cut / Special Edition behavior
- representative Movie release-tier ceiling interactions


## Corrected Release Preference

The profile gives legitimate corrected releases a small global scoring preference:

- PROPER / REPACK: `+5`
- REPACK2: `+6`
- REPACK3: `+7`

These bonuses are deliberately small tie-breakers. They do not replace the profile's source, quality, release-group, SeaDex, or availability priorities.

The rules are non-stacking: REPACK2 and REPACK3 receive only their numbered score rather than also receiving the base PROPER / REPACK bonus.

Base PROPER and REPACK detection uses StreamNZB's native `proper` and `repack` parser traits. Narrow release-name matching distinguishes REPACK2 and REPACK3 because the native `repack` trait intentionally classifies those numbered forms as repacks as well. `REAL.PROPER` and `REAL.REPACK` remain base corrected releases, while `REAL.REPACK2` and `REAL.REPACK3` retain their numbered preference.

The preference applies globally, including Anime, and does not require additional Vidhin-backed Define rules.


## Retag Soft Penalty

The profile applies a tiny global `-1` preference penalty to releases
carrying recognized redistribution / retag markers:

- `.heb`
- `[eztv]` and supported EZTV domain variants
- `[rarbg]`
- `[rartv]`
- `[TGx]`

This is intentionally a metadata tie-breaker rather than a quality
judgment or filter. Retagged releases remain fully usable and are never
rejected by this rule.

The penalty is deliberately much smaller than the profile's major
quality signals, including SeaDex prioritization, release-group tiers,
source and quality scoring. Its purpose is only to prefer
an otherwise equivalent original release over a recognized redistributed
copy.

Matching is narrow rather than treating arbitrary bracketed text as a
retag. This protects legitimate Anime release-group forms such as
`[SubsPlease]`, `[Erai-raws]`, and `[Judas]`.

The rule uses direct release-name matching and does not add a new
Vidhin-backed Define or formatter badge.


## Availability and Library Scoring

Positive NZB availability metadata is intentionally a **small tie-breaker** rather than a second release-quality hierarchy.

The profile now scores only two positive availability signals:

- **Alive on our backbone:** `+20`
- **Recently confirmed:** `+10`

When both apply, the maximum positive availability contribution is therefore `+30`. This remains below the `70`-point minimum gap between adjacent Anime BluRay release-group tiers, so availability alone cannot promote a lower Anime BluRay tier above the next-higher clean tier.

Indexer freshness and grab-count metadata remain visible through the formatter but no longer receive separate profile scoring bonuses. The former `Very fresh NZB`, `Recent NZB`, `Popular NZB`, `Very popular NZB`, and `Highly popular NZB` score rules were removed so informational metadata does not compete with release-group quality.

Known-unavailable handling remains a usability decision rather than a preference: releases known to be unavailable continue to be rejected.

Library results use StreamNZB's native **`+500` library bonus** from the `4k` preset. The former profile-level `Library hit +500` rule was removed because it stacked with that native bonus and produced an unintended effective `+1000`.

The formatter remains independent from this score normalization. Availability, age, grabs, and related NZB metadata can still be displayed even when they do not contribute ranking points.

Pinned full-ranking-pipeline regression coverage verifies the effective `+500` Library bonus, `+20` backbone preference, `+10` recent-confirmation preference, combined `+30` availability ceiling, and effective `+10` Anime/non-Anime audio preferences.

## Formatter

The latest formatter is available here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**


### Optional Debug Formatter

For troubleshooting, DraCuLa also provides an optional diagnostic formatter:

**[formatter-debug.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter-debug.txt)**

For a linked StreamNZB import, use:

**[Raw debug formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter-debug.txt)**

The debug formatter is intentionally verbose and is not intended to replace
the normal formatter for everyday use. For each result that survives
StreamNZB filtering it exposes:

- request content kind and Anime classification
- final result score and current top score
- raw release name and parsed title/media metadata
- release group, edition, network, indexer, size, age, and grabs
- same-release failover variant count and indexers
- availability status, backbone result, age, and compression
- SeaDex lookup/result state
- Library and ffprobe verification state
- measured codec/profile/dynamic-range information when available
- every matched profile rule with that rule's individual score contribution

The formatter also makes an important runtime boundary explicit:
**rejected releases and rejecting rules cannot be displayed**, because
StreamNZB removes rejected candidates before result formatting runs. Use
StreamNZB's own validation/rule diagnostics when troubleshooting releases
that never reach the result list.

Both the normal and debug formatters are built from readable JSON sources
and exercised through the pinned real StreamNZB formatter engine. Their
published `SNZBF1:` artifacts are checked for semantic synchronization in
the compatibility suite.

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

`profile.txt`, `profile-neutral.txt`, `formatter.txt` and `generated/streamnzb-defines.txt` on the `main` branch are the canonical published artifacts.

- `profile.txt` — Samsung QN90A behavior-preserving filtering and scoring policy
- `profile-neutral.txt` — hardware-neutral filtering and scoring policy
- `formatter.txt` — result presentation
- `generated/streamnzb-defines.txt` — Vidhin-backed release-group classifications

If you imported them by URL, use StreamNZB's **Refresh** action to check for updates. StreamNZB will show the proposed changes before anything is applied; updates are never applied automatically.

GitHub's raw-file CDN may take a few minutes to reflect a newly published update.

### Intelligent Unknown Resolution fallback

The profile uses an adaptive **Intelligent Unknown Resolution** rule instead of unconditionally rejecting every result whose resolution could not be parsed. Unknown-resolution results remain available when the result set is scarce, and weak unknowns are rejected only when more than six alternatives have both a known resolution and known quality.

The rule deliberately protects useful incomplete results. An unknown-resolution result is retained when it still has a recognized quality, matches a trusted Movie, Show, or Anime release-group tier, is a **Library** result, or is a **SeaDex Best / Alternative** recommendation. If SeaDex lookup data is unavailable for the request, the rule fails open rather than rejecting the result.

This policy applies only to **Unknown Resolution**. A result with a known resolution but Unknown Quality is not rejected by this rule. StreamNZB already places Unknown Resolution at the bottom of its native resolution ranking, so the profile does not add a separate score penalty; it only prunes weak unknowns when the result set has enough well-identified alternatives.

### Adaptive HD x265 filtering

The profile uses an availability-aware **Adaptive HD x265** Reject rule for non-Anime SDR HD releases. Instead of unconditionally penalizing HEVC/x265, a 720p or 1080p HEVC candidate is rejected only when more than six suitable same-resolution AVC alternatives are available.

The rule deliberately keeps HEVC when alternatives are scarce. It also exempts **2160p**, **HDR/HDR10+/Dolby Vision**, **Anime**, **Library results**, and **HEVC Remuxes**. AV1 is unaffected.

For the alternative count, 1080p considers same-resolution AVC Remux, BluRay, and WEB-DL releases; 720p considers same-resolution AVC BluRay and WEB-DL releases. This adapts the usual HD x265 quality preference to streaming, where preserving a usable result is more important than applying an unconditional codec penalty.

### Adaptive 1080p Remux preference

The profile includes an availability-aware **1080p Remux Preference** rule for non-Anime Movies and Shows. A non-Library 1080p Remux receives a small `+50` preference when the available 4K alternatives are limited to SDR 2160p WEB-DLs.

The bonus is deliberately conditional rather than making 1080p Remux universally outrank 4K. If any 2160p WEB-DL with **HDR or HDR10+** is available, the preference is suppressed so the higher-resolution HDR option can retain its normal ranking advantage.

A Dolby Vision-only 2160p WEB-DL does not suppress the bonus because the default profile is tuned for a Samsung QN90A, which does not support Dolby Vision. A Dolby Vision release that also exposes an HDR fallback does suppress it.

The rule does not apply to Anime, Library results, 2160p Remuxes, or other content kinds. At `+50`, it acts as a targeted cross-resolution tie-breaker rather than overriding the profile's release-group, SeaDex, availability, or other major quality scoring.

## Validation

The repository includes automated validation for both generated profile variants, the Define Library, Vidhin synchronization and Anime tier integrity. The Samsung artifact must remain byte-for-byte reproducible from the canonical source registry, while the neutral artifact is validated as the same ordered policy minus exactly five Samsung/device-specific rules.

For release-matching logic where StreamNZB parser or rule-engine behavior is important, the repository also includes a compatibility harness that runs test fixtures against a pinned revision of the real StreamNZB engine rather than reimplementing its behavior.

The harness separately validates StreamNZB's share-code compatibility contract. The `SNZBP1:` prefix identifies the profile share-code container format, while the `streamnzb_profile` marker versions the profile payload semantics. The harness currently requires `streamnzb_profile == 1`; a missing or different schema version fails validation so compatibility can be reviewed before the template accepts it.

New or changed compatibility-sensitive rules can first be developed as fixtures containing representative positive and negative release names. Once a rule is published, the fixture can reference the Samsung production rule by name. The harness decodes `profile.txt`, locates the exact published rule, verifies that its expression and score have not drifted from the tested fixture, and executes the same cases against the production rule. The compatibility harness also decodes and compiles the complete `profile-neutral.txt` artifact against the same pinned StreamNZB engine and shared Define Library.

The harness also loads the generated Define Library when compiling compatibility rules. This allows rules using `matched()` to be tested end-to-end against the same shared Define conditions used by the production profile, including release-group classification, Define scope, profile policy and final score behavior.

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
