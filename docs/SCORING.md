# DraCuLa Scoring Reference

This document contains the scoring and filtering details that are useful when auditing or modifying DraCuLa's StreamNZB policy. For installation and normal use, start with the repository [README](../README.md).

## Design principle

DraCuLa treats release-group and source authority as the primary quality hierarchy. Metadata such as HDR format, audio codec, video codec, corrected-release markers, availability and editions is either neutralized or bounded so ordinary metadata does not accidentally overturn trusted release-group tiers.

The production profiles are generated from one canonical ordered rule registry. `profile.txt` is the Samsung QN90A-oriented variant; `profile-neutral.txt` shares the same Core policy and removes the Samsung-specific Dolby Vision compatibility rejection.

## Release-group tiers

Movie and Show release groups are tiered, and Anime uses the full Vidhin hierarchy.

Anime WEB tiers for both Anime Movies and Anime Shows:

- T1: +500
- T2: +400
- T3: +300
- T4: +200
- T5: +100
- T6: +20

Anime BluRay tiers:

- T1: +560
- T2: +480
- T3: +400
- T4: +320
- T5: +240
- T6: +160
- T7: +80
- T8: +0

Anime BluRay tiers apply to BluRay encodes and to remuxes. The generated `Anime ... BluRay Tn Groups` Defines keep Vidhin's Anime BD source gate (BluRay/Blu-Ray, BD forms, BDMux, HD-DVD/DVD, NTSC/PAL, xvidvd) with each upstream record's own case sensitivity, and keep the remux-only membership of `PMR` and `NAN0`. A remux or WEB release from a BD-tier group with no such source evidence (for example `WEB-DL.REMUX-ZR` or `1080p-LazyRemux`) gets no Anime BluRay tier.

At equal resolution, REMUX source authority intentionally outranks Anime release-group tier differences. StreamNZB's native remux score (+1500, versus +200 WEB-DL and +100 BluRay encode) is not compensated, so an untiered BluRay remux ranks above a clean BluRay T1 encode or WEB T1, and a BluRay T8 remux ranks above a BluRay T1 encode. Tiers still order remuxes among themselves. This is a REMUX decision, not a general rule that source outranks tiers: BluRay encodes and WEB-DLs differ natively by only 100 points and interleave with the Anime tier ladders. `TestAnimeRemuxSourceAuthority` locks this ordering in.

The smallest adjacent Anime tier gap is 80 points. The repository uses real-engine regression tests rather than a hardcoded maximum-bonus assumption to ensure ordinary metadata cannot invert adjacent tiers.

## Vidhin-backed classifications

The shared Define Library synchronizes release-group classifications from [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex). It includes Movie, Show and Anime tiers together with classifications used for LQ, Bad Dual, Obfuscated, Generated Dynamic HDR, Anime Dubs Only, retag markers and selected audio-tag exclusion groups.

The library performs classification; the profile decides how those classifications affect scoring or filtering.

Important examples:

- Movie and Show LQ classification receives a strong negative score whether it is sourced from a matched release group or from a release-title-derived signal; both classification paths feed the same penalty and the same adaptive low-score filtering policy below, not a separate scoring magnitude or filter.
- Anime LQ receives a strong negative score, with SeaDex Best / Alternative exemptions.
- Movie and Show Bad Dual classifications receive strong penalties.
- Obfuscated classifications receive only a small soft penalty so a release remains usable as a fallback.
- Recognized retag/redistribution markers receive a tiny tie-breaking penalty rather than rejection.
- Generated Dynamic HDR is a Movie-only strong penalty when a matching release group also carries parsed Dolby Vision or HDR10+ metadata.

The Vidhin synchronization workflow fails closed when expected upstream fragments change in ways that require manual review.

## Adaptive filtering

DraCuLa avoids rigid filters when a weak result may still be useful in a sparse search.

### Adaptive low-score filtering

A candidate already classified by the Movie/Show LQ or Bad Dual Defines can be pruned when the result set contains enough substantially better alternatives. The current policy requires at least six other candidates finishing at least 5000 points above the candidate's own final score. Library results are protected.

This is candidate-relative rather than a fixed global score floor.

### Unknown resolution

Unknown-resolution results remain available when the result set is scarce. Weak unknowns are rejected only when there are more than six sufficiently identified alternatives. Library results and recognized trusted-tier releases are protected. Anime has a separate sibling rule so SeaDex-dependent logic stays confined to Anime evaluation.

### Adaptive HD x265

For non-Anime SDR 720p/1080p releases, HEVC/x265 is rejected only when enough suitable same-resolution AVC alternatives exist. The rule protects 2160p, HDR-family formats, Anime, Library results, trusted release groups and HEVC Remuxes. AV1 is unaffected by this filter.

### Adaptive low-quality source filtering

HDRip, DVDRip and HDTV-style weak-source results are rejected only when the result set already contains more than six recognized-resolution alternatives from stronger source classes such as Remux, BluRay or WEB-DL. Library results remain protected; Anime uses a separate SeaDex-aware sibling rule.

### Adaptive 1080p Remux preference

A non-Anime, non-Library 1080p Remux can receive a small +50 preference when available 4K alternatives are limited to SDR 2160p WEB-DLs. The preference is suppressed when an HDR or HDR10+ 2160p WEB-DL is available.

## Dynamic range and bit depth

DraCuLa treats dynamic-range and bit-depth metadata primarily as compatibility/classification information rather than quality authority.

The pinned Jhin engine applies native positive ranks to Dolby Vision, HDR10+, HDR, HLG and parsed 10-bit metadata. The shared Core compensates those native values so they cannot dominate the release-group hierarchy.

After neutralization, non-Anime HDR10+ receives one small explicit +25 preference. HDR, HDR10, HLG, Dolby Vision, parsed 10-bit and Anime HDR10+ remain score-neutral. HLG's native score (+1500, introduced by Jhin v0.7.1) is fully cancelled with no residual — a compatibility-preservation fix, not a new preference.

The Samsung profile additionally rejects Dolby Vision releases without HDR fallback. The hardware-neutral profile does not perform that device-specific rejection.

## High-impact audio normalization

Jhin also applies comparatively large native scores to audio codecs. DraCuLa neutralizes the high-impact values and restores only small non-Anime residual preferences where tier headroom permits it.

Effective non-Anime preferences are:

- TrueHD: +50
- DTS Lossless: +50
- Atmos: +25
- Dolby Digital Plus: +25

For Anime, those four codecs are score-neutral because the 80-point minimum tier gap is too small to safely absorb the native values. Dolby Digital is universally neutralized to avoid it outranking Dolby Digital Plus after compensation.

DTS Lossy and AAC are also universally neutralized with no residual, for every content kind including Anime. They were previously neutralized for Anime only, on the premise that the 200-point Movie/Show tier gap safely absorbed their native +100 each — the AAC/DTS-Lossy tier-authority audit found that judgment no longer held: combined with the same realistic HDR10+/lossless-audio physical-media decoration the tier-authority regression already measures at only a few points of margin, the native +100 reproduced a real, realistic adjacent-tier inversion (a Movie Remux or Movie UHD BluRay-encode release one tier lower outscoring a clean release one tier higher by roughly 22–23 points). DraCuLa does not treat either native score as positive ranking policy anywhere; this prevents native codec leakage from consuming trusted-tier headroom.

Selected Vidhin-backed exclusion groups suppress residual Atmos/TrueHD bonuses for release groups known to falsely tag those attributes. The neutralization itself still applies.

DTS:X and DTS-ES are their own distinct Jhin v0.7.1 traits, not aliases of DTS Lossless/DTS Lossy — the existing `dts_lossless`-keyed rules do not match them. Both are universally neutralized with no residual, for every content kind including Anime: DTS:X's native +2000 causes outright tier inversions; DTS-ES's native +100 is small, but a non-Anime residual was real-engine-tested and rejected as fragile (it would consume nearly all of the tightest existing tier gap). This mirrors the Dolby Digital pattern above — a compatibility-preservation fix, not a new preference.

## Video codec normalization

StreamNZB's streaming preset applies native ranking to AVC, HEVC, AV1 and VC-1. DraCuLa neutralizes all four video codecs to exactly zero for every content kind so codec choice cannot overturn release-group tiers.

There is no residual HEVC, AV1 or VC-1 preference. VC-1 (native +100, introduced by Jhin v0.7.1) was real-engine-tested with a non-Anime residual and found unsafe for the same reason as DTS-ES: it would consume nearly all of the tightest existing tier gap (Movie Remux, adjacent tiers).

## Corrected releases

Corrected releases receive small global tie-breakers:

- PROPER / REPACK: effective +5
- REPACK2: effective +6
- REPACK3: effective +7

The rules compensate Jhin's native corrected-release score so the complete pipeline ends at those intended effective values. Numbered repacks do not stack with the base REPACK preference.

## Availability and Library scoring

Availability metadata is intentionally minor:

- Alive on our backbone: +20
- Recently confirmed: +10
- Maximum combined positive availability contribution: +30

Freshness and grab-count metadata can still be displayed by the formatter but are not separately scored.

Library results use StreamNZB's native +500 library bonus. DraCuLa does not add a second profile-level library bonus.

### Best-N per resolution/quality bucket

Ordinary (non-season-pack) results are capped per `resolution + quality` bucket:

- Up to 3 non-Library candidates survive (`Best 3 per R/Q`).
- Up to 1 additional Library candidate survives (`Best 1 Library per R/Q`) —
  a bounded reservation, not an unconditional exemption: a Library
  candidate cannot also consume one of the 3 ordinary slots, and multiple
  Library candidates in the same bucket remain capped at the single
  highest-ranked one.
- A bucket's maximum size is therefore ordinarily 3, and up to 4 when a
  qualifying Library candidate is present — an intentional, bounded
  retention-policy expansion, not a side effect.
- Series/Anime season packs are unaffected: they continue through the
  existing, separate `Best 1 Season Pack per R/Q` ceiling. Library does
  **not** receive a dedicated season-pack reservation — a Library season
  pack competes for the single season-pack slot on the same terms as any
  other pack.
- Existing SeaDex caps (`At most 1 SeaDex Best` / `At most 1 SeaDex
  Alternative`) are unchanged and independent of this reservation.

## Bounded preset size scoring

The built-in 4K preset can give size a very large ranking contribution. DraCuLa publishes explicit shared size scoring in both profiles with the same upstream target sizes, bounded well below that native contribution:

- Movie: 20 GB target, max +500
- Series: 6 GB per episode target, max +150
- Anime Show / Anime Movie: no size contribution

This keeps efficient encodes relevant without allowing file size to dominate the quality hierarchy. Series carries a lower maximum than Movie: a 2026-09-22 real-engine audit found the size term's slope around the 6 GB target (previously also +500, an 83.3 points/GB slope) could outweigh deliberate technical preferences like HDR10+ (+25) and Atmos (+25) for size differences as small as a few hundred MB — a real, observed inversion where a smaller SDR episode outranked a larger HDR10+/Atmos episode of the same release. Size is intended to be a secondary streaming-cost preference, not a same-tier technical-quality selector; +150 (25 points/GB) keeps that inversion from happening within roughly 1-2 GB of ordinary episode-to-episode size variance while still rewarding an efficient encode meaningfully near the target.

Anime Show and Anime Movie deliberately have no size scoring. Both kinds are absent from the published scoring map, which StreamNZB resolves to no size contribution at all (it does not fall back to the preset's native weight). Their size term (25 points/GB) let ordinary file-size differences invert the much tighter Anime release-group tiers: with the ordinary bonuses the tier-authority matrix already allows, a lower tier won with roughly 0.5-1.3 GB more size, and a clean Anime Movie tier flipped at about 3.2 GB. For Anime releases below the configured target, the size term rewards larger files and can act as a bitrate proxy rather than a secondary streaming-cost preference. The largest weight that kept every adjacent Anime tier ordered left about one point of headroom, too small to be useful. The REMUX source-authority policy above is unchanged. `TestAnimeSizeTierAuthority` locks this in.

## Anime-specific preferences

Anime WEB streaming-service preferences are deliberately small and subordinate to release-group tiers. The profile also supports presentation and tie-breaking behavior for Anime 10-bit/Hi10P, explicit Uncensored markers, Dual/Multi Audio, revision markers and Complete Season Packs.

The formatter is Network-first: StreamNZB's parsed `.Network` wins when present, with profile presentation rules used as fallback labels where appropriate.

## Editions

DraCuLa neutralizes Jhin's native edition scoring and reintroduces only deliberate profile preferences. IMAX is intentionally a strong Movie-version preference; Director's Cut / Extended Edition and Open Matte use much smaller bounded preferences. Other canonical edition values remain neutral unless explicitly handled by a DraCuLa rule.

IMAX Enhanced behavior is pinned against StreamNZB/Jhin compatibility tests so canonical IMAX Enhanced releases are not incorrectly treated as upscaled.

## Tier-authority validation

`TestAdjacentTierCeilingMatrix` is the main real-engine guard for release-group tier authority. For each production tier family it decorates the lower tier with currently reachable ordinary bonuses and asserts that the real StreamNZB engine still ranks a clean candidate one tier higher above it.

`TestPinMoveAdjacentTierMatrix`, `TestPinMoveCombinedAttributeStress` and `TestPinMoveAnimeNeutrality` extend the same guarantee to the four StreamNZB v6.0.0/Jhin v0.7.1 native attributes above (HLG, DTS:X, VC-1, DTS-ES), individually and in realistic combination, across every production tier family. `TestNewNativeAttributeNeutralizerContract` is the focused, tier-independent proof that each of the four neutralizes to exactly zero net contribution for both Anime and non-Anime.

The repository also maintains focused regressions for codec neutrality, edition behavior, formatter output and compatibility-sensitive parser/rule behavior.

For engine/version compatibility details, see [COMPATIBILITY.md](COMPATIBILITY.md).