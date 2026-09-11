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

The smallest adjacent Anime tier gap is 80 points. The repository uses real-engine regression tests rather than a hardcoded maximum-bonus assumption to ensure ordinary metadata cannot invert adjacent tiers.

## Vidhin-backed classifications

The shared Define Library synchronizes release-group classifications from [Vidhin05/Releases-Regex](https://github.com/Vidhin05/Releases-Regex). It includes Movie, Show and Anime tiers together with classifications used for LQ, Bad Dual, Obfuscated, Generated Dynamic HDR, Anime Dubs Only, retag markers and selected audio-tag exclusion groups.

The library performs classification; the profile decides how those classifications affect scoring or filtering.

Important examples:

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

The pinned Jhin engine applies native positive ranks to Dolby Vision, HDR10+, HDR and parsed 10-bit metadata. The shared Core compensates those native values so they cannot dominate the release-group hierarchy.

After neutralization, non-Anime HDR10+ receives one small explicit +25 preference. HDR, HDR10, Dolby Vision, parsed 10-bit and Anime HDR10+ remain score-neutral.

The Samsung profile additionally rejects Dolby Vision releases without HDR fallback. The hardware-neutral profile does not perform that device-specific rejection.

## High-impact audio normalization

Jhin also applies comparatively large native scores to audio codecs. DraCuLa neutralizes the high-impact values and restores only small non-Anime residual preferences where tier headroom permits it.

Effective non-Anime preferences are:

- TrueHD: +50
- DTS Lossless: +50
- Atmos: +25
- Dolby Digital Plus: +25

For Anime, those four codecs are score-neutral because the 80-point minimum tier gap is too small to safely absorb the native values. DTS lossy and AAC remain native for Movies/Shows but are neutralized for Anime. Dolby Digital is universally neutralized to avoid it outranking Dolby Digital Plus after compensation.

Selected Vidhin-backed exclusion groups suppress residual Atmos/TrueHD bonuses for release groups known to falsely tag those attributes. The neutralization itself still applies.

## Video codec normalization

StreamNZB's streaming preset applies native ranking to AVC, HEVC and AV1. DraCuLa neutralizes all three video codecs to exactly zero for every content kind so codec choice cannot overturn release-group tiers.

There is no residual HEVC or AV1 preference.

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

## Bounded preset size scoring

The built-in 4K preset can give size a very large ranking contribution. DraCuLa publishes explicit shared size scoring in both profiles with the same upstream target sizes but a maximum contribution of +500:

- Movie / Anime Movie: 20 GB target, max +500
- Series / Anime Show: 6 GB per episode target, max +500

This keeps efficient encodes relevant without allowing file size to dominate the quality hierarchy.

## Anime-specific preferences

Anime WEB streaming-service preferences are deliberately small and subordinate to release-group tiers. The profile also supports presentation and tie-breaking behavior for Anime 10-bit/Hi10P, explicit Uncensored markers, Dual/Multi Audio, revision markers and Complete Season Packs.

The formatter is Network-first: StreamNZB's parsed `.Network` wins when present, with profile presentation rules used as fallback labels where appropriate.

## Editions

DraCuLa neutralizes Jhin's native edition scoring and reintroduces only deliberate profile preferences. IMAX is intentionally a strong Movie-version preference; Director's Cut / Extended Edition and Open Matte use much smaller bounded preferences. Other canonical edition values remain neutral unless explicitly handled by a DraCuLa rule.

IMAX Enhanced behavior is pinned against StreamNZB/Jhin compatibility tests so canonical IMAX Enhanced releases are not incorrectly treated as upscaled.

## Tier-authority validation

`TestAdjacentTierCeilingMatrix` is the main real-engine guard for release-group tier authority. For each production tier family it decorates the lower tier with currently reachable ordinary bonuses and asserts that the real StreamNZB engine still ranks a clean candidate one tier higher above it.

The repository also maintains focused regressions for codec neutrality, edition behavior, formatter output and compatibility-sensitive parser/rule behavior.

For engine/version compatibility details, see [COMPATIBILITY.md](COMPATIBILITY.md).