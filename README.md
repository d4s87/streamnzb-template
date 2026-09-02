# 🧛 DraCuLa's StreamNZB Template
DraCuLa's custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V5.1**

V5.1 builds on the generated multi-profile architecture introduced in V5.0 with stronger scoring integrity, adaptive low-score filtering, Vidhin-backed Obfuscated release handling, StreamNZB 5.16.1 / Jhin 0.6.1 compatibility, and improved formatter language/subtitle presentation. The existing `profile.txt` remains the Samsung QN90A-oriented variant, while `profile-neutral.txt` provides a hardware-neutral alternative without the four Samsung/device-specific playback rules. Both profiles share the same Core policy, presentation classifications, Define Library, and formatter architecture.

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
- Vidhin-backed Obfuscated release soft penalties for Movie and Show families
- Retag soft penalty for recognized redistribution markers
- Anime revision preference for explicit `v0`–`v4` release markers
- Tier-safe NZB availability tie-breaking and native StreamNZB library prioritization
- Same-release failover
- Grouped resolution + quality result limits
- Hardware-neutral dynamic-range / bit-depth scoring with Samsung-specific Dolby Vision compatibility handling and audio preferences
- Formatter display of parsed language metadata and subtitle presence, including clean subtitle-only rendering

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

`profile-neutral.txt` keeps the shared DraCuLa filtering and scoring policy but removes the four Samsung/device-specific playback rules: Dolby Vision without HDR fallback rejection, Atmos compensation, TrueHD compensation, and DTS Lossless compensation.

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

This artifact currently contains **112** rules and remains generated from the canonical ordered rule registry.

### Hardware-Neutral Profile

For setups that should not inherit the Samsung-specific playback compensation policy:

**[profile-neutral.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile-neutral.txt)**

For the recommended linked import:

**[Raw neutral profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt)**

The neutral artifact contains **108** rules. It is the Samsung profile minus exactly these four device-specific rules:

- `DV without HDR fallback`
- `Reduce Atmos`
- `Reduce TrueHD bonus`
- `Reduce DTS Lossless bonus`

`Neutralize Dolby Vision` is now part of the shared Portable Core together with native HDR, HDR10+ and parsed 10-bit compensation. All 108 shared rules are identical and retain the same relative order in both variants. `Reject 3D` is part of the hardware-neutral Core policy and therefore remains present in both profiles.

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

Existing `profile.txt` users do not need to migrate if they want to keep the current Samsung-oriented behavior. To switch to the neutral policy, import `profile-neutral.txt` as a **new linked profile** and switch the relevant StreamNZB profile assignments to it.

Both profile variants require the shared Define Library described below. Import the library before using either profile.

## Define Library

V5.1 continues to use one shared StreamNZB Define Library for both profile variants and their Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 56 published Define rules. Fifty-five are synchronized Vidhin-backed Movie, Show and Anime classifications, including LQ, separate Movie/Show Bad Dual, separate Movie/Show Obfuscated, and Anime Dubs Only classifications. The additional local `Trusted Release Groups` Define is generated from all 47 Movie, Show, Anime Movie, and Anime Show tier Defines and gives profile rules one stable reference to the complete trusted-tier set without duplicating that membership in the profile.

Anime classifications follow the full Vidhin hierarchy:
- Anime WEB: T1–T6
- Anime BluRay: T1–T8

Separate Anime Movie and Anime Show Defines are provided for every tier.

Anime LQ matching uses Vidhin's upstream release-name regex directly rather than converting it into release-group tokens, preserving the upstream matching semantics.

Bad Dual classifications are synchronized separately from Vidhin's Radarr and Sonarr Bad Dual group definitions. Their upstream group regexes are preserved directly rather than flattened into literal release-group tokens, including regex-specific matching semantics.

Obfuscated classifications are synchronized separately from Vidhin's Radarr and Sonarr Obfuscated definitions. Their upstream marker families are preserved, while the two PCRE positive-lookbehind `Scrambled` branches are translated into boolean-equivalent Go-regexp expressions that StreamNZB/Jhin can compile. Synchronization fails closed if those expected upstream fragments change, requiring manual review rather than silently changing classification semantics.

Movies and Anime Movies use the Radarr Obfuscated classification; Series and Anime Shows use the Sonarr classification. Matching releases receive a `-1` soft penalty only. This is a tie-breaker rather than a rejection, so an Obfuscated release remains available when it is otherwise the best or only result.

The Define Library performs classification only. Matching non-Anime Movie and Show releases receive a `-10,000` profile penalty. Anime Shows are deliberately excluded from the Show Bad Dual penalty; Anime Movies are outside the Movie-scoped Bad Dual classification.

The definitions are synchronized with [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex) through GitHub Actions. Upstream changes are reviewed through a pull request before becoming part of the library.

The repository can also publish Discord notifications for important updates. Semantic Define Library changes published to `main` announce the changed classifications and remind linked-library users to use StreamNZB's **Refresh** action; metadata-only synchronization changes remain silent. Newly published GitHub Releases are also announced with a concise release link and a reminder to refresh the **Define Library first** before reviewing profile or formatter updates.

Matching note: Release-group names are generally matched case-insensitively by the generated StreamNZB Define Library. Upstream case-specific distinctions may be normalized when they do not cause cross-tier ambiguity.

After a library update is published, use **Refresh** in StreamNZB to review and apply the changes.

## Formatter Language and Subtitle Metadata

The DraCuLa formatter displays StreamNZB/Jhin's parsed language metadata on a
dedicated `⛿` line. When Jhin also identifies explicit subtitle metadata,
the formatter appends `sᴜʙ` to that line:

- parsed languages only: `⛿ EN · JA`
- parsed languages with subtitles: `⛿ EN · JA · sᴜʙ`
- subtitles without parsed languages: `⛿ sᴜʙ`

The subtitle-only form is handled explicitly so it does not render a leading
separator such as ` · sᴜʙ`.

This presentation follows the data currently exported by
**StreamNZB 5.16.1 / Jhin 0.6.1**. Jhin exposes parsed `Languages` metadata
separately from the boolean `Subbed` flag. It does **not** currently expose a
separate list of subtitle-language identities to StreamNZB's formatter
context. DraCuLa therefore does not infer or fabricate forms such as
`SUB (EN · DE)` from the release name.

The `Languages` field should be understood as Jhin's parsed language metadata
surface rather than a guaranteed inventory of media-file audio tracks.
Permanent compatibility regressions pin ordinary language parsing, explicit
subtitle markers, combined Dubbed/Subbed metadata, and hardcoded-subtitle
behavior against Jhin 0.6.1. Separate real-StreamNZB formatter regressions pin
all three display forms above for both the canonical formatter source and the
published `formatter.txt` artifact.

## Adaptive Low-Score Filtering

DraCuLa applies a candidate-relative **Adaptive Low-Score Filtering** prune
after normal scoring. It targets releases already classified by the
Vidhin-backed Movie/Show **LQ** or **Bad Dual** Defines, but only when the
result set contains enough substantially better alternatives.

A matching candidate is pruned only when all of the following are true:

- it is not already in the local Library;
- it matches `Movies LQ Groups`, `Movies Bad Dual Groups`,
  `Shows LQ Groups`, or `Shows Bad Dual Groups`; and
- at least **6** other candidates finish at least **5000 points above the
  candidate's own final score**.

The comparison is deliberately relative to each candidate through
`finalScore` and `current.finalScore`. This is not a fixed global score floor:
the same low-quality release remains available when fewer than six
substantially better alternatives exist. Sparse searches therefore preserve a
fallback instead of being emptied by an absolute threshold.

This policy depends on StreamNZB's candidate-relative prune aggregates. The
required result-set behavior is pinned to **StreamNZB 5.16.1**, which includes
the upstream fix for issue `#249`. Permanent real-engine regressions verify
both sides of the threshold: a dense weak Movie LQ tail is pruned, while the
same class of candidate survives when the result set is sparse.

## Dynamic Range and Bit-Depth Scoring

DraCuLa treats display-dependent dynamic-range and bit-depth metadata as classification and compatibility information rather than release-quality authority.

The pinned Jhin v0.6 engine natively adds `+3000` for Dolby Vision, `+2100` for HDR10+, `+2000` for HDR, and `+100` for parsed 10-bit metadata. The shared Portable Core first compensates those native ranks with `-3000`, `-2100`, `-2000`, and `-100` respectively so Jhin's built-in display-format scoring cannot override DraCuLa release-group tier authority.

After that compensation, DraCuLa applies one explicit bounded format preference: non-Anime HDR10+ receives `+25`. HDR, HDR10, Dolby Vision, parsed 10-bit, and Anime HDR10+ remain score-neutral. The `+25` HDR10+ preference is shared by both the Samsung and hardware-neutral profiles because it is a release-format preference rather than a Samsung-specific compatibility rule.

The non-Anime-only scope is intentional. The minimum adjacent Anime release-group gap is `80`, while the largest ordinary lower-tier Anime Show WEB metadata stack proven by the pinned engine is `+77`, leaving only `3` points of guaranteed headroom. A meaningful HDR10+ bonus would therefore erase or invert Anime tier authority. Permanent real-engine regressions keep Anime HDR10+ neutral while verifying that a fully decorated lower Movie WEB tier with HDR10+ `+25` still remains `78` points below the next-higher clean tier.

The Samsung QN90A profile retains one device-specific dynamic-range compatibility rule: Dolby Vision releases without an HDR fallback are rejected. Dolby Vision releases that include HDR/HDR10 fallback remain eligible, and Dolby Vision releases with HDR10+ fallback receive the same non-Anime `+25` HDR10+ preference. The hardware-neutral profile performs no Dolby Vision compatibility rejection; Dolby Vision-only remains eligible and score-neutral.

## Anime Scoring

V5.1 retains the full Vidhin Anime tier hierarchy for both Anime Movies and Anime Shows across both profile variants.

WEB release groups are scored as follows for both Anime Movies and Anime Shows:
- T1: +500
- T2: +400
- T3: +300
- T4: +200
- T5: +100
- T6: +20

BluRay release groups use a synchronized 80-point tier ladder for both Anime Movies and Anime Shows:
- T1: +560
- T2: +480
- T3: +400
- T4: +320
- T5: +240
- T6: +160
- T7: +80
- T8: +0

These ladders are intentionally spaced around the **effective** score seen by the complete StreamNZB/Jhin ranking pipeline rather than only the raw DraCuLa rule values.

The largest ordinary positive Anime stack currently proven by the pinned real engine is `+77` for Anime Show WEB results. That ceiling includes effective Dual/Multi Audio, corrected-release, Anime revision, Uncensored, Complete Season Pack, availability, and WEB service preferences where applicable. Anime Movies have a lower maximum because Complete Season Pack does not apply.

The minimum adjacent Anime release-group gap is therefore `80`, leaving at least `3` points of headroom even for the maximum ordinary lower-tier stack. Permanent real-engine regression coverage verifies every adjacent Anime Movie and Anime Show WEB/BluRay tier so a fully decorated lower tier remains below the next-higher clean tier.

Anime releases matching Vidhin's Anime LQ classification receive a `-10,000` penalty. SeaDex Best and Alternative recommendations are exempt from this penalty.

Anime 10-bit releases are detected using StreamNZB's parsed bit depth, with an additional `Hi10P` release-name fallback for common Anime naming conventions. The presentation rule itself scores `0` points. DraCuLa's shared Core separately compensates Jhin v0.6's native parsed-10-bit `+100` rank, so parsed 10-bit metadata remains informational and cannot erase or invert adjacent Anime release-group tiers.

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

The preference is a tie-breaker rather than a quality override. It contributes
`+10` to the effective Anime Show metadata stack and is included in the
scoring-ceiling audit. With all ordinary positive Anime Show metadata that can
legitimately combine, the pinned real engine reaches a maximum `+77` stack.
The synchronized Anime WEB/BluRay ladders use a minimum adjacent gap of `80`,
so even that maximum lower-tier stack remains below the next-higher clean tier.

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

Current **effective** Movie edition scoring:

- **IMAX:** `+800`
- **Open Matte:** `+25`
- **Director's Cut / Extended Edition:** one shared, non-stacking `+25`

Jhin v0.6 contributes a native `+100` rank when its parser recognizes an
edition. DraCuLa therefore stores compensated rule values for parser-backed
Movie editions so the final effective policy remains unchanged:

- **IMAX:** stored `+700` + native `+100` = effective `+800`
- **Director's Cut / Extended Edition:** stored `-75` + native `+100` = effective `+25`
- **Open Matte:** stored/effective `+25`; it is release-name matched and does not receive the native parsed-edition rank

IMAX is intentionally a strong Movie-version preference. It may outrank a
higher release-group tier when the user is choosing between otherwise
eligible Movie releases; this is deliberate rather than a tier-ceiling bug.

The bounded IMAX rule also matches `IMAX Enhanced` without adding a second
DraCuLa IMAX rule-layer bonus.

The previous upstream **StreamNZB/Jhin limitation** affecting IMAX Enhanced is
resolved in **StreamNZB 5.16.1 / Jhin 0.6.1**. The fix originated from
StreamNZB issue `#251`: Jhin no longer interprets the trailing `Enhanced` in
canonical IMAX Enhanced names as an upscale marker.

Pinned parser and real-engine validation now confirms:

- `IMAX` parses as edition `IMAX` and is not upscaled;
- `IMAX.Enhanced` and `IMAX-Enhanced` parse as edition `IMAX` and are not
  upscaled;
- bare `Enhanced` is not treated as an upscale marker;
- compact non-canonical `IMAXEnhanced` remains non-upscaled but is not parsed
  as an IMAX edition;
- `AI.Enhanced` and explicit `Upscaled` releases are still correctly
  classified as upscaled and rejected by the production `Reject Upscaled`
  policy.

Canonical IMAX Enhanced releases therefore receive the same single effective
`+800` IMAX preference as ordinary IMAX releases and remain eligible through
the production profile.

Open Matte and Director's Cut / Extended Edition are deliberately much
smaller. Director's Cut and Extended Edition share one parser-backed rule, so
alternate spellings such as `Directors Cut`, `Director's Cut`,
`Extended Edition`, and `Extended Cut` cannot stack with each other.

The maximum ordinary low-weight Movie edition stack is therefore an effective
`+50` when Open Matte and the shared Director's Cut / Extended Edition
preference both apply.

The full scoring-ceiling audit also combines that `+50` edition stack with
effective Dual/Multi Audio `+10`, corrected release up to `+7`, and positive
availability up to `+30`. The resulting maximum ordinary Movie stack is
`+97`, leaving `103` points of headroom inside the `200`-point Movie
release-group tier gap. Equivalent Show WEB/Remux tests reach only `+47`,
leaving `153` points of headroom.

Criterion Collection, Final Cut, and generic Special Edition currently
remain score-neutral. The pinned Jhin v0.6 parser does not classify those
forms as edition metadata. DraCuLa deliberately avoids broad raw release-name
fallbacks that could confuse title text or loose markers with canonical
edition metadata.

Pinned compatibility and real-engine regression coverage protects:

- IMAX matching, including IMAX Enhanced fixture behavior
- StreamNZB 5.16.1 / Jhin 0.6.1 IMAX Enhanced parser behavior, including
  continued detection of genuine AI-enhanced/upscaled releases
- effective IMAX `+800` scoring after native-edition compensation
- Movie-only scope
- Open Matte matching
- effective Director's Cut / Extended Edition `+25` scoring after native-edition compensation
- non-stacking alternate-cut behavior
- neutral Criterion / Final Cut / Special Edition behavior
- full non-Anime Movie WEB/Remux ceiling interactions


## Corrected Release Preference

The profile gives legitimate corrected releases a small global scoring preference:

- PROPER / REPACK: `+5`
- REPACK2: `+6`
- REPACK3: `+7`

These bonuses are deliberately small tie-breakers. They do not replace the profile's source, quality, release-group, SeaDex, or availability priorities.

The rules are non-stacking: REPACK2 and REPACK3 receive only their numbered effective score rather than also receiving the base PROPER / REPACK preference.

Jhin v0.6 already contributes a native `+20` rank to parsed PROPER/REPACK releases. DraCuLa compensates that native score in the stored profile rules so the complete ranking pipeline preserves the intended small tie-breakers:

- PROPER / REPACK: stored `-15` + native `+20` = effective `+5`
- REPACK2: stored `-14` + native `+20` = effective `+6`
- REPACK3: stored `-13` + native `+20` = effective `+7`

Base PROPER and REPACK detection uses StreamNZB's native `proper` and `repack` parser traits. Narrow release-name matching distinguishes REPACK2 and REPACK3 because the native `repack` trait intentionally classifies those numbered forms as repacks as well. `REAL.PROPER` and `REAL.REPACK` remain base corrected releases, while `REAL.REPACK2` and `REAL.REPACK3` retain their numbered preference.

Pinned full-ranking regression coverage verifies the final effective `+5/+6/+7` contract rather than only the compensated stored rule values.

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

When both apply, the maximum positive availability contribution is therefore `+30`. The scoring-ceiling audit includes that full availability contribution inside the maximum ordinary metadata stacks: Anime uses a minimum adjacent tier gap of `80`, while non-Anime Movie/Show release-group families retain `200`-point gaps. Availability alone therefore remains far below either hierarchy.

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

The rule deliberately protects useful incomplete results. An unknown-resolution result is retained when it still has a recognized quality, matches a trusted Movie, Show, Anime Movie, or Anime Show release-group tier, is a **Library** result, or is a **SeaDex Best / Alternative** recommendation. Trusted tiers are resolved through the generated `Trusted Release Groups` Define, so changes to synchronized tier membership automatically flow into this protection without maintaining a second hard-coded tier list in the profile. If SeaDex lookup data is unavailable for the request, the rule fails open rather than rejecting the result.

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

The repository includes automated validation for both generated profile variants, the Define Library, Vidhin synchronization and Anime tier integrity. The Samsung artifact must remain byte-for-byte reproducible from the canonical source registry, while the neutral artifact is validated as the same ordered policy minus exactly four Samsung/device-specific rules.

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
