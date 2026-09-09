# 🧛 DraCuLa's StreamNZB Template
DraCuLa's custom filtering, scoring and formatter template for [StreamNZB](https://github.com/Gaisberg/streamnzb).

**Current version: V5.2**

V5.2 builds on the generated multi-profile architecture introduced in V5.0 with stronger scoring integrity, adaptive low-score filtering, Vidhin-backed Obfuscated release handling, StreamNZB 5.18.0 / Jhin 0.6.2 compatibility, and improved formatter language/subtitle presentation. The existing `profile.txt` remains the Samsung QN90A-oriented variant, while `profile-neutral.txt` provides a hardware-neutral alternative without the Samsung-specific Dolby Vision compatibility rule. Both profiles share the same Core policy, including bounded high-impact audio normalization, universal video-codec normalization, presentation classifications, Define Library, and formatter architecture.

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
- Hardware-neutral dynamic-range / bit-depth and high-impact audio normalization with Samsung-specific Dolby Vision compatibility handling
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

`profile-neutral.txt` keeps the shared DraCuLa filtering and scoring policy but removes the single Samsung/device-specific playback rule: rejection of Dolby Vision releases without an HDR fallback. Atmos, Dolby Digital Plus, TrueHD, and DTS Lossless normalization are now shared Core policy and therefore apply identically in both profiles.

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

This artifact currently contains **146** rules and remains generated from the canonical ordered rule registry.

### Hardware-Neutral Profile

For setups that should not inherit the Samsung-specific playback compensation policy:

**[profile-neutral.txt](https://github.com/d4s87/streamnzb-template/blob/main/profile-neutral.txt)**

For the recommended linked import:

**[Raw neutral profile](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/profile-neutral.txt)**

The neutral artifact contains **145** rules. It is the Samsung profile minus exactly one device-specific rule:

- `DV without HDR fallback`

`Neutralize Dolby Vision` is part of the shared Portable Core together with native HDR, HDR10+ and parsed 10-bit compensation. The shared Core now also normalizes Jhin's high-impact Atmos, Dolby Digital Plus, TrueHD, and DTS Lossless ranks, and AVC/HEVC/AV1 video codec ranks, so audio and codec metadata remain bounded preferences rather than overriding release-group/source authority. The shared Core also carries two independent retag soft penalties: the original `Retag Soft Penalty` for known redistribution-site markers (`.heb`, EZTV, RARBG, RARTV, TGx), and `Literal RETAG Soft Penalty` for a standalone scene `RETAG` token — a different semantic signal kept as its own rule rather than folded into the first. All **145 shared rules** are identical and retain the same relative order in both variants. `Reject 3D` is part of the hardware-neutral Core policy and therefore remains present in both profiles.

Profiles imported by URL remain linked to this repository. Use **Refresh** in StreamNZB to check for updates. Changes are shown in a diff before being applied, and local-only rules are preserved.

Existing `profile.txt` users do not need to migrate if they want to keep the current Samsung-oriented behavior. To switch to the neutral policy, import `profile-neutral.txt` as a **new linked profile** and switch the relevant StreamNZB profile assignments to it.

Both profile variants require the shared Define Library described below. Import the library before using either profile.

## Define Library

V5.2 continues to use one shared StreamNZB Define Library for both profile variants and their Vidhin-backed release-group classifications.

Import the linked library before using the profile:

**[Raw Define Library](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/generated/streamnzb-defines.txt)**

The library currently provides 57 published Define rules. Fifty-six are synchronized Vidhin-backed Movie, Show and Anime classifications, including LQ, separate Movie/Show Bad Dual, separate Movie/Show Obfuscated, Movie Generated Dynamic HDR, and Anime Dubs Only classifications. The additional local `Trusted Release Groups` Define is generated from all 47 Movie, Show, Anime Movie, and Anime Show tier Defines and gives profile rules one stable reference to the complete trusted-tier set without duplicating that membership in the profile.

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
**StreamNZB 5.18.0 / Jhin 0.6.2**. Jhin exposes parsed `Languages` metadata
separately from the boolean `Subbed` flag. It does **not** currently expose a
separate list of subtitle-language identities to StreamNZB's formatter
context. DraCuLa therefore does not infer or fabricate forms such as
`SUB (EN · DE)` from the release name.

The `Languages` field should be understood as Jhin's parsed language metadata
surface rather than a guaranteed inventory of media-file audio tracks.
Permanent compatibility regressions pin ordinary language parsing, explicit
subtitle markers, combined Dubbed/Subbed metadata, and hardcoded-subtitle
behavior against Jhin 0.6.2, including the compact `JA` alias fix (upstream
issue `dreulavelle/jhin#39`) landed in that release. Separate real-StreamNZB
5.18.0 formatter regressions pin
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
required result-set behavior is pinned to **StreamNZB 5.18.0**, which includes
the upstream fix for issue `#249`. Permanent real-engine regressions verify
both sides of the threshold: a dense weak Movie LQ tail is pruned, while the
same class of candidate survives when the result set is sparse.

## Dynamic Range and Bit-Depth Scoring

DraCuLa treats display-dependent dynamic-range and bit-depth metadata as classification and compatibility information rather than release-quality authority.

The pinned Jhin v0.6 engine natively adds `+3000` for Dolby Vision, `+2100` for HDR10+, `+2000` for HDR, and `+100` for parsed 10-bit metadata. The shared Portable Core first compensates those native ranks with `-3000`, `-2100`, `-2000`, and `-100` respectively so Jhin's built-in display-format scoring cannot override DraCuLa release-group tier authority.

After that compensation, DraCuLa applies one explicit bounded format preference: non-Anime HDR10+ receives `+25`. HDR, HDR10, Dolby Vision, parsed 10-bit, and Anime HDR10+ remain score-neutral. The `+25` HDR10+ preference is shared by both the Samsung and hardware-neutral profiles because it is a release-format preference rather than a Samsung-specific compatibility rule.

The non-Anime-only scope is intentional. Anime's minimum adjacent release-group tier gap is only `80` points, far tighter than the `200`-point Movie/Show gap, leaving very little room for an additional positive dynamic-range bonus once every other ordinary Anime preference is considered. A meaningful HDR10+ bonus for Anime would risk eroding or inverting tier authority, so Anime HDR10+ remains fully score-neutral.

Rather than citing a single hand-picked "maximum stack" figure as proof, the permanent `TestAdjacentTierCeilingMatrix` real-engine regression (see [Validation](#validation)) is the authoritative guard for this contract: for every production tier family it decorates the lowest tier with every currently-reachable ordinary bonus and asserts the engine's own score stays below a clean candidate one tier higher. A prior audit found that relying on a hardcoded maximum instead of the engine's actual output is exactly what let a real tier-authority regression reach production undetected (see [High-Impact Audio Normalization](#high-impact-audio-normalization)).

The Samsung QN90A profile retains one device-specific dynamic-range compatibility rule: Dolby Vision releases without an HDR fallback are rejected. Dolby Vision releases that include HDR/HDR10 fallback remain eligible, and Dolby Vision releases with HDR10+ fallback receive the same non-Anime `+25` HDR10+ preference. The hardware-neutral profile performs no Dolby Vision compatibility rejection; Dolby Vision-only remains eligible and score-neutral.

## Generated Dynamic HDR

A small number of release groups are known to generate their own Dolby Vision or HDR10+ metadata rather than sourcing it from a retail disc or streaming master. This does not mean the underlying video is necessarily defective — it means the dynamic-range metadata itself is unverified and generated rather than authoritative, which is exactly the kind of uncertainty DraCuLa strongly deprioritizes without treating as a hard rejection.

The Vidhin-backed `Movies Generated Dynamic HDR Groups` Define identifies this specific group list. Matching a release from one of these groups **and** a parsed Dolby Vision or HDR10+ marker applies a `-10,000` **Movie-only** penalty. Ordinary releases from one of these groups are not penalized at all — the classification only applies when a dynamic-HDR marker is also present. For example, `Flights` remains a normal `Movies WEB T2` release-group on its ordinary releases; only its Generated Dynamic HDR releases receive the `-10,000` penalty.

This is a `score` penalty, not a `reject`: a matching release remains eligible as a last-resort fallback rather than being discarded outright. The classification deliberately overrides ordinary release-group tier trust — a normally trusted higher-tier release carrying this classification can rank below a clean lower-tier release, the same design already used for the Bad Dual and LQ classifications above.

The existing Dolby Vision/HDR10+ native-score neutralization described above remains completely separate and additive: native dynamic-range ranks are still compensated exactly as before, and the non-Anime HDR10+ `+25` preference still applies independently. Generated Dynamic HDR only adds one further, independent scoring layer on top.

The scope is strictly Movie — Series, Anime Movie, and Anime Show are unaffected. There is currently no [Adaptive Low-Score Filtering](#adaptive-low-score-filtering) integration for this classification.

## High-Impact Audio Normalization

Jhin v0.6 also contributes comparatively large native audio ranks: `+2000` for TrueHD, `+2000` for DTS Lossless, `+1000` for Atmos, `+150` for Dolby Digital Plus, and `+100` / `+100` / `+50` for DTS lossy, AAC, and Dolby Digital respectively. Those values are large enough to threaten release-group tier authority when combined with other ordinary preferences.

A post-release scoring-ceiling audit found this had already happened in production: a real-engine adjacent-tier matrix showed a bottom-tier Anime BluRay release decorated with TrueHD/Atmos and other ordinary bonuses outscoring a clean release one tier higher by over 100 points, a Movie Remux/UHD BluRay/HD BluRay release doing the same to its immediate neighbor, and even a plain, undecorated AAC track alone being enough to cross Anime's minimum tier gap. The high-impact audio policy below is the fix for that regression.

The shared Portable Core compensates each high-impact codec in two layers, mirroring the existing HDR10+ `Neutralize`/`Prefer` split:

- a universal `Neutralize` rule fully compensates the native score to `0` for **every** content kind, including Anime;
- a `Prefer` rule then adds a small, deliberate residual bonus, but only when the request is **not** Anime.

Effective results for Movies and Shows:

- TrueHD: `+2000` native, neutralized to `0`, non-Anime preference restores an effective `+50`
- DTS Lossless: `+2000` native, neutralized to `0`, non-Anime preference restores an effective `+50`
- Atmos: `+1000` native, neutralized to `0`, non-Anime preference restores an effective `+25`
- Dolby Digital Plus: `+150` native, neutralized to `0`, non-Anime preference restores an effective `+25`

**Anime results are always exactly `0`** for all four codecs above — Anime's `80`-point minimum adjacent tier gap has no room to safely absorb a positive audio preference of any size.

DTS lossy and AAC remain fully untouched (native `+100` / `+100`) for Movies and Shows, whose `200`-point tier gaps comfortably absorb them. For Anime, both are separately neutralized to `0` as well, because the audit found that even a single untouched codec at its native value was enough on its own to threaten Anime's tier gap.

Dolby Digital is a third case, corrected by a later ordering-integrity audit: it was originally left untouched alongside DTS lossy/AAC on the same "Movies/Shows tier gap safely absorbs it" reasoning, but that left plain Dolby Digital's native `+50` fully uncompensated while Dolby Digital Plus right above it was compensated down to a smaller effective `+25` — so the objectively worse codec silently outranked the better one by `25` points for every non-Anime release. The fix is a universal `Neutralize Dolby Digital` rule (`-50`, no scope, no residual `Prefer` counterpart) that brings Dolby Digital to effective `0` for every content kind. This is a pure correctness fix, not a new preference layer: it removes points rather than adding any, so it cannot consume adjacent-tier headroom. Dolby Digital Plus's own effective `+25` is unchanged. The previous Anime-only `Neutralize Anime Dolby Digital` rule is now subsumed by the universal one and no longer exists.

This is intentionally selective, codec-by-codec normalization rather than a complete audio hierarchy or a blanket Anime exemption from audio scoring generally — Anime Dual/Multi Audio (a language preference, not a codec preference) is unaffected and keeps its own effective `+10`. The goal is to bound every codec that can materially threaten source/tier ordering while preserving a small, real preference for non-Anime content where the tier gap can safely absorb it.

The normalization is identical in `profile.txt` and `profile-neutral.txt`. The permanent `TestAdjacentTierCeilingMatrix` real-engine regression (see [Validation](#validation)) is the authoritative guard for this contract across every production tier family — Movie Remux/UHD BluRay/HD BluRay/WEB, Show Remux/BluRay/WEB, and Anime Show/Movie BluRay/WEB — rather than a hardcoded per-family maximum. A structural check separately fails closed if a `Neutralize` rule ever gains an Anime-conditional clause, or a `Prefer` rule ever loses its non-Anime scoping.

## Video Codec Normalization

StreamNZB's own streaming preset assigns native ranks to each of Jhin's three recognized video codecs regardless of content kind: `+300` for AVC, `+700` for HEVC, and `+700` for AV1 — a `+400` swing between AVC and either modern codec that has nothing to do with DraCuLa's own release-group/source tier design.

A codec-scoring audit found this had already reached production: a real-engine adjacent-tier matrix showed the `+400` swing alone overturning 7 of the 11 production tier families — every family whose ordinary encode is AVC (Movie HD BluRay, Show BluRay, and all four Anime BluRay/WEB families) — with a bottom-tier AV1-encoded release outscoring not just the adjacent tier but every tier above it in that family. Unlike the audio codecs above, no bounded residual preference is safe here: the tightest actual non-Anime margin measured anywhere in the system (the HDR10+/lossless-audio physical-media combination, see above) is only `+3`, leaving no room for any positive codec preference at all.

The shared Portable Core neutralizes all three recognized codecs to exactly `0`, universally, with no Anime/non-Anime split and no `Prefer` residual:

- AVC: `+300` native, neutralized to `0`
- HEVC: `+700` native, neutralized to `0`
- AV1: `+700` native, neutralized to `0`

This is identical in `profile.txt` and `profile-neutral.txt`. The permanent `TestVideoCodecNeutrality` real-engine regression asserts all three codecs contribute exactly `0` for every content kind (Movie, Show, Anime Movie, Anime Show), and `TestAdjacentTierCeilingMatrix` decorates each family's lower tier with the codec that previously broke it, proving the fix holds against the same tier-authority contract as every other bounded preference. A structural check separately fails closed if any of the three rules gains a content-kind condition, or a matching `Prefer` rule is ever introduced without a deliberate, reviewed change.

## Anime Scoring

V5.2 retains the full Vidhin Anime tier hierarchy for both Anime Movies and Anime Shows across both profile variants.

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

The minimum adjacent Anime release-group tier gap is `80` points. Rather than relying on a single hand-picked "maximum ordinary stack" figure as proof that ordinary Anime metadata (Dual/Multi Audio, corrected-release, Anime revision, Uncensored, Complete Season Pack, availability, WEB service preferences, and — critically — audio codec metadata) can never cross that gap, the permanent `TestAdjacentTierCeilingMatrix` real-engine regression (see [Validation](#validation)) directly verifies every adjacent Anime Movie and Anime Show WEB/BluRay tier: a fully decorated lower tier must score below a clean candidate one tier higher, as measured by the engine itself. A prior hardcoded-maximum approach let a real regression (see [High-Impact Audio Normalization](#high-impact-audio-normalization)) reach production undetected, which is why this contract is now engine-verified rather than documentation-verified.

Anime releases matching Vidhin's Anime LQ classification receive a `-10,000` penalty. SeaDex Best and Alternative recommendations are exempt from this penalty.

Anime 10-bit releases are detected using StreamNZB's parsed bit depth, with an additional `Hi10P` release-name fallback for common Anime naming conventions. The presentation rule itself scores `0` points. DraCuLa's shared Core separately compensates Jhin v0.6's native parsed-10-bit `+100` rank, so parsed 10-bit metadata remains informational and cannot erase or invert adjacent Anime release-group tiers.

The formatter displays matching releases as `₁₀ʙɪᴛ`.

The profile also detects common **Streaming Services** for Anime WEB releases and applies the small source-preference scale recommended by TRaSH for Anime: Crunchyroll (`CR`) `+6`, Disney+ (`DSNP`) `+5`, Netflix (`NF`) `+4`, Amazon (`AMZN`) `+3`, VRV `+3`, Funimation (`FUNi`) `+2`, ABEMA `+1`, ADN `+1`, while B-Global, Bilibili, and HIDIVE score `0`. These are deliberately small source preferences and remain subordinate to the Anime release-group tier hierarchy.

The formatter remains **Network-first**: when StreamNZB provides `.Network`, that value is displayed as the source. If `.Network` is absent, the formatter falls back to the matched Streaming Service rule. This provides a source label for Anime WEB releases where the service can be identified from the release name without overriding StreamNZB's parsed Network metadata.

### Non-Anime streaming-service formatter badges

A dedicated audit ("Non-Anime Streaming-Service Formatter Badges") found that Jhin v0.6.2's own `.Network` table only recognizes a handful of major US networks/services (Apple TV, Amazon, Netflix, Disney, HBO/HBO Max, Hulu, plus a few cable networks) and leaves `.Network` empty for many real Movie/Series WEB releases — with no fallback badge at all, unlike Anime's own Streaming Service rules above. The profile now adds **16 zero-point `presentation`-owned fallback rules** for the specific services the audit proved have no native `.Network` coverage and no PCRE-lookaround-dependent upstream regex: Peacock, Paramount+, Criterion Channel, Roku, Syfy, DC Universe, Coupang, DMM TV, FOD, Hotstar, KOCOWA, U-NEXT, Viki, Wavve, WeTV, and Youku.

These rules are presentation-only, exactly mirroring the existing Anime `B-Global`/`Bilibili`/`HIDIVE` shape: `not isAnime`, the same WEB-traits gate the Anime service rules already use, a separator-safe release-token regex, `0` points, no reject/limit/scope change, and no scoring interaction with anything else in the profile. `.Network` remains authoritative — the formatter only falls back to one of these badges when `.Network` is empty, exactly as it already does for the Anime services.

Eleven related services (bare `Max`, Movies Anywhere, Google Play, iTunes, Showtime, Stan, Fandango, Comedy Central, TVING, Viu, and iQIYI) were deliberately **not** added: their upstream Vidhin regexes rely on PCRE lookahead/lookbehind (e.g. excluding "HBO Max" from a bare `Max` badge, or "DTS-HD MA" from a "Movies Anywhere" badge) that Go's RE2 rule engine cannot express directly, the same class of constraint already documented for the Obfuscated `Scrambled` translation. Approximating them with a bare-token regex would reintroduce exactly the false-positive risk the audit was run to avoid, so this profile does not mirror Vidhin's full streaming-service list — only the 16 services proven safe.

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
`+10` to the effective Anime Show metadata stack and is one of the bonuses
covered by the permanent `TestAdjacentTierCeilingMatrix` real-engine
regression (see [Validation](#validation)), which verifies directly through
the engine that a fully decorated lower Anime tier — including this
preference — never outscores a clean candidate one tier higher on the
synchronized Anime WEB/BluRay ladders.

The final resolution/quality ceiling is also season-pack aware. Ordinary
episode/non-pack releases and episodic season packs no longer compete for the
same three slots:

- up to **3** non-pack releases are retained per `resolution + quality`
- up to **1** Series/Anime Show `seasonPack` is retained independently per
  `resolution + quality`

This preserves one strong season-pack alternative without globally increasing
the normal `Best 3 per R/Q` ceiling. All earlier filtering still applies, so a
pack can still be rejected for language, Dolby Vision fallback, HD x265,
low-score, unknown-resolution, or other production policy before it reaches
the final cap.

Within the one-pack bucket, normal ranking still decides the winner. The
existing `Complete Season Pack Preference` therefore gives an otherwise equal
explicitly complete pack the expected `+10` advantage over an ordinary season
pack; packs do not receive a blanket quality override.

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

Jhin v0.6's `Edition` field is a single scalar string covering 10 canonical
values (`Anniversary Edition`, `Ultimate Edition`, `Directors Cut`,
`Extended Edition`, `Collectors Edition`, `Theatrical`, `Uncut`, `IMAX`,
`Diamond Edition`, `Remastered`), and Jhin contributes a generic native
`+100` rank whenever it recognizes *any* of them — with zero
differentiation between values, and no scope of its own (it applies to
every content kind, not just Movies).

DraCuLa neutralizes that native rank universally, then restores only its
own deliberate, reviewed preferences on top:

- **`Neutralize Edition` (universal Core, `-100`, no scope):** cancels
  Jhin's native `+100` unconditionally for every content kind whenever any
  canonical Edition value is parsed. This is the scoring-integrity
  baseline: without an explicit DraCuLa residual, every canonical Edition
  value is effective `0` everywhere, including Series and Anime.
- **`Movie Edition Preference` (Movie-only, stored `+25`):** matches
  `Directors Cut` or `Extended Edition`. Combined with the universal
  neutralizer, its effective score is `+25` — unchanged from before this
  fix.
- **`IMAX` (Movie-only, stored `+700`):** unchanged. Its effective score is
  now `+700` (previously `+800`, when Jhin's native rank was still
  uncompensated for IMAX). A real-engine safety audit — an isolated
  same-tier measurement and the full `TestAdjacentTierCeilingMatrix`
  ceiling matrix with IMAX reachable in the fully decorated lower tier —
  confirmed `+700` introduces no tier-authority violation, so the stored
  value is preserved rather than raised back to net `+800`.
- **`Open Matte` (Movie-only, `+25`):** unchanged. It is matched on raw
  release-name text, not on the parsed `Edition` field, so it has no native
  rank to compensate for and is completely unaffected by the neutralizer.
- **Anime `Uncensored` (Anime-only, `+10`):** unchanged. It independently
  matches `Uncut`/`Unrated`/`Uncensored`/`AT-X` on raw release-name text.
  A release parsed with Edition `Uncut` nets effective `+10` for Anime:
  native `+100`, `Neutralize Edition -100`, `Uncensored +10`.

Every other canonical value — `Anniversary Edition`, `Ultimate Edition`,
`Collectors Edition`, `Theatrical`, `Diamond Edition`, `Remastered`, and
`Uncut`/`IMAX` outside the rules above — is fully neutralized to effective
`0`. There is no blanket preference for any of them; before this fix, each
carried a silent, uncompensated native `+100` that could invert
release-group tier authority (a real measured `-42` margin was found for
Movie physical media, and a razor-thin `+8` margin for Series).

The permanent `TestEditionNeutralityRegression` real-engine regression
(see [Validation](#validation)) asserts the exact effective delta for all
10 canonical values across all 4 content kinds, plus Unrated remaining
unaffected (a separate boolean, not an Edition value) and an
Extended+IMAX shadowing case proving the scalar `Edition` field's native
rank fires exactly once even when a raw-regex rule (IMAX) independently
matches the same release. `TestAdjacentTierCeilingMatrix` was extended so
every production family — not just Movie — directly measures a
previously-uncompensated canonical Edition token on its fully decorated
lower tier. A structural check separately fails closed if
`Neutralize Edition` or `Movie Edition Preference` drift from their
audited scope/points/condition, or if a new positive residual rule for
any other canonical Edition value is ever introduced without a
deliberate, reviewed change.

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
`+700` IMAX preference as ordinary IMAX releases and remain eligible through
the production profile.

Open Matte and Director's Cut / Extended Edition are deliberately much
smaller. Director's Cut and Extended Edition share one parser-backed rule, so
alternate spellings such as `Directors Cut`, `Director's Cut`,
`Extended Edition`, and `Extended Cut` cannot stack with each other.

The maximum ordinary low-weight Movie edition stack is therefore an effective
`+50` when Open Matte and the shared Director's Cut / Extended Edition
preference both apply.

That `+50` edition stack combines with effective Dual/Multi Audio `+10`,
corrected release up to `+7`, positive availability up to `+30`, and audio
codec metadata (see [High-Impact Audio Normalization](#high-impact-audio-normalization))
inside the `200`-point Movie/Show release-group tier gap. Rather than citing
a single hand-picked combined total as proof the gap always holds, the
permanent `TestAdjacentTierCeilingMatrix` real-engine regression (see
[Validation](#validation)) directly verifies every Movie Remux/UHD
BluRay/HD BluRay/WEB and Show Remux/BluRay/WEB tier family: a fully
decorated lower tier must score below a clean candidate one tier higher, as
measured by the engine itself. This replaced an earlier hardcoded-maximum
approach after it let a real adjacent-tier regression reach production
undetected.

Criterion Collection, Final Cut, and generic Special Edition currently
remain score-neutral. The pinned Jhin v0.6 parser does not classify those
forms as edition metadata. DraCuLa deliberately avoids broad raw release-name
fallbacks that could confuse title text or loose markers with canonical
edition metadata.

Pinned compatibility and real-engine regression coverage protects:

- IMAX matching, including IMAX Enhanced fixture behavior
- StreamNZB 5.16.1 / Jhin 0.6.1 IMAX Enhanced parser behavior, including
  continued detection of genuine AI-enhanced/upscaled releases
- effective IMAX `+700` scoring after universal native-edition neutralization
- Movie-only scope
- Open Matte matching
- effective Director's Cut / Extended Edition `+25` scoring after universal native-edition neutralization
- non-stacking alternate-cut behavior
- universal `Neutralize Edition` scoring for all 10 canonical values across
  Movie/Series/Anime Show/Anime Movie, including Unrated remaining
  unaffected and the Extended+IMAX shadowing interaction
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


## Bounded Preset Size Scoring

StreamNZB's built-in `4k` preset normally gives NZB size up to `+3000` ranking points, targeting approximately 20 GB for Movies/Anime Movies and 6 GB per episode for Series/Anime Shows. That upstream default is intentionally strong enough to make efficient encodes competitive with very large Remux releases during streaming, but the full `+3000` ceiling can overwhelm DraCuLa's quality-first source and release-group hierarchy.

DraCuLa therefore publishes an explicit shared profile-level size-scoring policy in both `profile.txt` and `profile-neutral.txt`. The upstream size targets are preserved, but the maximum contribution is bounded to **`+500`** for every supported content kind:

- Movie / Anime Movie: target **20 GB**, maximum **+500**
- Series / Anime Show: target **6 GB per episode**, maximum **+500**

The size factor still tapers linearly away from the target and reaches zero at twice the target, so efficient releases remain meaningfully preferred when quality is otherwise close. Large Remux releases simply no longer lose up to 3000 points of effective headroom solely because of file size.

A production regression reproduces the live `Project Hail Mary` case that exposed this interaction. With DraCuLa's bounded size policy, the ~19.55 GB BYNDR WEB-DL receives about `+489` from size instead of about `+2932`, while the ~88 GB CiNEPHiLES Remux receives `0`; the remaining ordering difference is then attributable to explicit profile preferences such as IMAX and Library status rather than hidden preset size weight.

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

The normal formatter is published here:

**[formatter.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter.txt)**

For the recommended linked import, use:

**[Raw formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter.txt)**

The formatter can remain linked and be manually refreshed when the DraCuLa
template is updated.

The normal formatter uses the stable user-facing name **DraCuLa**.
Individual formatter artifacts are not independently versioned; they evolve
with the DraCuLa template/repository release instead of carrying a second
version number alongside template versions such as V5.2.

### Reliability and corrected-release metadata

The normal formatter keeps several StreamNZB runtime facts compact and
presentation-only:

- when StreamNZB has more than one interchangeable NZB for the same release,
  the indexer/group line appends `⧉N`, where `N` is the total same-release
  variant count; single-copy releases do not show the indicator;
- corrected releases display one compact status label:
  `ᴘʀᴏᴘᴇʀ`, `ʀᴇᴘᴀᴄᴋ`, `ʀᴇᴘᴀᴄᴋ₂`, or `ʀᴇᴘᴀᴄᴋ₃`;
- ordinary reported NZB availability remains `💚 ɴᴢʙ`;
- when StreamNZB also confirms the release healthy on one of the configured
  provider backbones through `Availability.OnMyBackbone`, the badge becomes
  `💚 ɴᴢʙ+`.

These additions expose facts already available from StreamNZB/Jhin and the
matched production rules. They do not change scoring, filtering,
release-group tiers, Library priority, availability bonuses, or same-release
fallback behavior.

Permanent pinned real-StreamNZB 5.18.0 regressions cover both the readable
formatter source and the published `formatter.txt` artifact for:

- same-release variant visibility and single-variant suppression;
- all four corrected-release status labels;
- ordinary versus backbone-confirmed availability.

### Optional Debug Formatter

For troubleshooting, DraCuLa also provides an optional diagnostic formatter:

**[formatter-debug.txt](https://github.com/d4s87/streamnzb-template/blob/main/formatter-debug.txt)**

For a linked StreamNZB import, use:

**[Raw debug formatter](https://raw.githubusercontent.com/d4s87/streamnzb-template/main/formatter-debug.txt)**

The diagnostic formatter uses the stable user-facing name **DraCuLa Debug**.
Like the normal formatter, it does not carry an independent formatter version.

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

The formatter is my attempt to reproduce and adapt the look and presentation
of **[Tamtaro's SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)**
formatter for AIOStreams within StreamNZB's formatter capabilities. It is not
a direct port and has been adapted to work with StreamNZB's available data
and formatting system.

## Important: Hardware-Specific Rules

This profile is tuned for a **Samsung QN90A without an AVR or soundbar**.

The Samsung QN90A does not support Dolby Vision, so the Samsung profile retains a device-specific Dolby Vision compatibility rule that rejects DV releases without HDR fallback. High-impact audio normalization is shared by both profiles rather than being tied to the Samsung speaker setup.

If you use a different TV, Dolby Vision display, AVR, soundbar or other audio setup, review the compatibility rule and shared scoring policy before importing the profile.

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

The rule deliberately protects useful incomplete results. An unknown-resolution result is retained when it still has a recognized quality, matches a trusted Movie, Show, Anime Movie, or Anime Show release-group tier, or is a **Library** result. Trusted tiers are resolved through the generated `Trusted Release Groups` Define, so changes to synchronized tier membership automatically flow into this protection without maintaining a second hard-coded tier list in the profile.

Anime is handled by a separate sibling rule, **Anime Unknown Resolution**, so its SeaDex dependency never touches Movie/Show evaluation — SeaDex data only exists for Kitsu-addressed Anime requests, so a Movie/Series rule referencing it would never see any data to protect with. A **SeaDex Best / Alternative** Anime recommendation is exempt from the Anime rule's rejection; if the request carries no SeaDex data at all (no lookup ran), the Anime rule fails open rather than rejecting the result.

This policy applies only to **Unknown Resolution**. A result with a known resolution but Unknown Quality is not rejected by this rule. Under the pinned StreamNZB engine, resolution contributes explicit ranking points rather than acting as a separate sort key: each step up the resolution ladder is worth 20,000 points, and unparsed resolution (`ResUnknown`) is deliberately priced at the same rung as 720p rather than at the bottom, while 1080p, 1440p, and 2160p each sit one or more 20,000-point rungs above it. The profile does not add a separate score penalty on top of that because none is needed for tier safety; the adaptive rule's job is narrower than compensating for a bottom-ranked Unknown — it prunes vague, unclassified metadata specifically when the result set already has plenty of better-identified alternatives to prefer instead.

### Adaptive HD x265 filtering

The profile uses an availability-aware **Adaptive HD x265** Reject rule for non-Anime SDR HD releases. Instead of unconditionally penalizing HEVC/x265, a 720p or 1080p HEVC candidate is rejected only when more than six suitable same-resolution AVC alternatives are available.

The rule deliberately keeps HEVC when alternatives are scarce. It also exempts **2160p**, **HDR/HDR10+/Dolby Vision**, **Anime**, **Library results**, **trusted Movie/Show release-group tiers** (resolved through the generated `Trusted Release Groups` Define, mirroring Intelligent Unknown Resolution's own trusted-tier protection above), and **HEVC Remuxes**. AV1 is unaffected.

For the alternative count, 1080p considers same-resolution AVC Remux, BluRay, and WEB-DL releases; 720p considers same-resolution AVC BluRay and WEB-DL releases. This adapts the usual HD x265 quality preference to streaming, where preserving a usable result is more important than applying an unconditional codec penalty.

### Adaptive low-quality filtering

The profile rejects HDRip/DVDRip/HDTV-sourced releases adaptively rather than unconditionally: a weak-source result is rejected only when more than six recognized-resolution, recognized-source (Remux/BluRay/WEB-DL) alternatives are available, and it is kept as a fallback in scarcer result sets. **Library** results are always exempt.

Anime is handled by a separate sibling rule, **Anime Adaptive Low-Quality Filtering**, so its SeaDex dependency never touches Movie/Show evaluation. A **SeaDex Best / Alternative** Anime recommendation is exempt from this penalty; if the request carries no SeaDex data at all (no lookup ran), the Anime rule fails open rather than rejecting the result, the same fail-open behavior Intelligent Unknown Resolution uses.

### Adaptive 1080p Remux preference

The profile includes an availability-aware **1080p Remux Preference** rule for non-Anime Movies and Shows. A non-Library 1080p Remux receives a small `+50` preference when the available 4K alternatives are limited to SDR 2160p WEB-DLs.

The bonus is deliberately conditional rather than making 1080p Remux universally outrank 4K. If any 2160p WEB-DL with **HDR or HDR10+** is available, the preference is suppressed so the higher-resolution HDR option can retain its normal ranking advantage.

A Dolby Vision-only 2160p WEB-DL does not suppress the bonus because the default profile is tuned for a Samsung QN90A, which does not support Dolby Vision. A Dolby Vision release that also exposes an HDR fallback does suppress it.

The rule does not apply to Anime, Library results, 2160p Remuxes, or other content kinds. At `+50`, it acts as a targeted cross-resolution tie-breaker rather than overriding the profile's release-group, SeaDex, availability, or other major quality scoring.

## Validation

The repository includes automated validation for both generated profile variants, the Define Library, Vidhin synchronization and Anime tier integrity. The Samsung artifact must remain byte-for-byte reproducible from the canonical source registry, while the neutral artifact is validated as the same ordered policy minus exactly one Samsung/device-specific rule.

For release-matching logic where StreamNZB parser or rule-engine behavior is important, the repository also includes a compatibility harness that runs test fixtures against a pinned revision of the real StreamNZB engine rather than reimplementing its behavior.

The harness separately validates StreamNZB's share-code compatibility contract. The `SNZBP1:` prefix identifies the profile share-code container format, while the `streamnzb_profile` marker versions the profile payload semantics. Upstream (`Gaisberg/streamnzb#267`, StreamNZB 5.18.0) made `streamnzb_profile` a version range: `1` for a profile without a scoring map, `2` for one that has one. DraCuLa's profiles always carry a scoring map, so the harness requires published profiles to emit `streamnzb_profile == 2` unambiguously; a missing or different schema version fails validation so compatibility can be reviewed before the template accepts it.

New or changed compatibility-sensitive rules can first be developed as fixtures containing representative positive and negative release names. Once a rule is published, the fixture can reference the Samsung production rule by name. The harness decodes `profile.txt`, locates the exact published rule, verifies that its expression and score have not drifted from the tested fixture, and executes the same cases against the production rule. The compatibility harness also decodes and compiles the complete `profile-neutral.txt` artifact against the same pinned StreamNZB engine and shared Define Library.

The harness also loads the generated Define Library when compiling compatibility rules. This allows rules using `matched()` to be tested end-to-end against the same shared Define conditions used by the production profile, including release-group classification, Define scope, profile policy and final score behavior.

This approach provides two layers of behavioral validation:

1. **Fixture validation** — verifies the intended rule against StreamNZB's real parser and rule engine.
2. **Production regression validation** — verifies the exact rule shipped in `profile.txt` against the same cases.

The compatibility harness is intentionally used selectively for rules where parsing, traits, regular expressions or other StreamNZB engine behavior can materially affect matching. It is not intended to duplicate every profile rule into a second configuration file.

### Adjacent-tier ceiling matrix

`TestAdjacentTierCeilingMatrix` is a permanent, table-driven real-engine regression covering release-group tier authority for every production tier family: Movie Remux/UHD BluRay/HD BluRay/WEB, Show Remux/BluRay/WEB, and Anime Show/Movie BluRay/WEB. For every adjacent tier pair, it decorates the lower tier with every currently-reachable ordinary positive bonus (editions, dual/multi audio, corrected-release, revision, uncensored, complete season pack, availability, WEB service preference, and audio codec metadata) and asserts, through the engine's own score, that it stays below a clean candidate one tier higher.

It intentionally does not compare against a hardcoded "maximum stack" constant. An earlier generation of ceiling tests did exactly that, and a real tier-authority regression (introduced when high-impact audio normalization was made a globally-scoped rule) reached production because nobody updated the hardcoded constant to include the new bonus — the tests kept passing against a stale assumption instead of the engine's actual behavior. See [High-Impact Audio Normalization](#high-impact-audio-normalization) for the fix. The matrix also includes a dedicated case proving that AAC — a codec intentionally left untouched for Movies/Shows — is still safe for Anime specifically. (Dolby Digital was originally in this same untouched-for-Movies/Shows group; it was later moved to a universal neutralizer by the Dolby Digital ordering-integrity fix described above, so it no longer needs this Anime-specific safety check.)

Each family's lower-tier candidate is also built with the video codec that previously broke it — AV1 for AVC-baseline families, proving the fix against the exact `+400` native swing that caused the regression; AV1 for HEVC-baseline families too, proving HEVC and AV1 stay equalized at `0`. See [Video Codec Normalization](#video-codec-normalization).

Validation runs automatically through GitHub Actions.

## Community

Discussion, setup notes and template updates are available in the [DraCuLa's StreamNZB Template Discord thread](https://discord.com/channels/1470288400157380710/1542856068135125002).

For release-specific changes, always refer to this repository's README and changelog.

## Credits

The filtering and scoring logic takes inspiration from the wider media automation community, including **[TRaSH Guides](https://trash-guides.info/), [Vidhin](https://github.com/Vidhin05/Releases-Regex) and [Tamtaro SEL Template](https://github.com/Tam-Taro/SEL-Filtering-and-Sorting)**, adapted for StreamNZB and Usenet.
