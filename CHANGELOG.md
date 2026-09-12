# Changelog

## [Unreleased](https://github.com/d4s87/streamnzb-template/compare/v5.2...HEAD)

Changes in this section are under development and are not part of the latest stable release.

### Features

- **profile:** add `Literal RETAG Soft Penalty` (`-1`, universal shared Core): a metadata tie-breaker for releases carrying a standalone scene `RETAG` token, distinct from the pre-existing `Retag Soft Penalty` rule. That rule remains completely unchanged and continues to cover known redistribution-site markers (`.heb`, EZTV, RARBG, RARTV, TGx) — a different semantic signal (a site re-hosting another group's release) from a group correcting its own release's metadata/naming with the payload unchanged, which is what the new rule targets. Both happen to use the same `-1` magnitude, but are kept as independent rules deliberately, so each stays independently testable and evolvable. Real-engine audit confirmed Jhin has no native `RETAG` concept at all (no parsed field, no score), and confirmed the audited regex boundary is safe: matches `Group.RETAG.1080p`, `Group RETAG 1080p`, `Group_RETAG_1080p`, `Group-RETAG-1080p`, `[RETAG]`, and case-insensitively; correctly excludes `Pretag`, `Retagged`, `RETAGS`, and `RETAG` fused into a longer word.
- **testing:** add the permanent `TestLiteralRetagRegression` real-engine regression covering all four content kinds (Movie/Series/Anime Movie/Anime Show): isolated delta exactly `-1`, case-insensitive and token-boundary matching, the false-positive controls above, and additive stacking with PROPER/REPACK/REPACK2/REPACK3 (`PROPER +5` → `RETAG+PROPER +4`, `REPACK +5` → `RETAG+REPACK +4`, `REPACK2 +6` → `RETAG+REPACK2 +5`, `REPACK3 +7` → `RETAG+REPACK3 +6`). Extend `TestAdjacentTierCeilingMatrix` with the inverse-risk case a negative penalty creates — a RETAG'd higher tier instead of a decorated lower tier — across every production family including the razor-thin HDR10+/lossless-audio physical-media combination, whose margin narrows from `+3` to `+2` and stays safely positive; the assertion is the strict inequality itself, not a hardcoded margin. Extend the `build_profiles.py` structural guard to fail closed if `Literal RETAG Soft Penalty` drifts from its audited scope/condition, or if the pre-existing `Retag Soft Penalty` rule is ever altered.
- **profile:** increase the Samsung profile from 125 to **126** rules and the hardware-neutral profile from 124 to **125** rules for the new rule. Canonical ownership becomes **121 Core**, 4 presentation, and 1 Samsung device rule.
- **profile:** add `Generated Dynamic HDR Penalty` (`-10000`, Movie-only), a dedicated policy audit's conclusion for a small list of release groups (`BiTOR`, `DepraveD`, `Flights`, `GuyZo`/`BR-GuyZo`, `SasukeducK`, `tarunk9c`, `VD0N`, `VECTOR`, `VisionXpert`) known to synthesize their own Dolby Vision/HDR10+ metadata rather than sourcing it from a retail disc or streaming master. New Vidhin-backed Define `Movies Generated Dynamic HDR Groups` syncs only the release-group half of the classification rather than being hand-maintained; the rule combines it with Jhin's own parsed dynamic-range facts instead of duplicating Vidhin's title-marker regex: `matched("Movies Generated Dynamic HDR Groups") and (dolbyVision or any(hdr, # == "HDR10+"))`. Real-engine confirmed: a listed group's ordinary release is completely unaffected — the predicate is group membership AND a claimed dynamic-HDR marker, never a blanket group ban. This is a `score` rule, not a `reject`, so a flagged release remains eligible as a last-resort fallback (both TRaSH's own Radarr Custom Format and Vidhin's own independent ranked-expression profile corroborate `-10000` and the Movie-only scope from two separate upstream sources). The `-10000` classification is intentionally stronger than ordinary release-group tier authority — real-engine proven via the actual `Flights` overlap (a trusted `Movies WEB T2` group also on this list): a `Flights` release flagged Generated Dynamic HDR ranks below a clean `Movies WEB T1` release, the same license already exercised by `Movies Bad Dual Penalty`/`Movies LQ Penalty`. Existing HDR10+/DV native-score neutralization (`Neutralize HDR10 Plus`, `Neutralize Dolby Vision`, `Prefer HDR10 Plus`) remains completely independent and additive; this rule only adds one more scoring layer on top. A fail-closed Vidhin sync guard fails the build if the upstream classification's predicate shape or HDR10+/DV marker vocabulary ever drifts, so a semantic upstream change cannot silently decouple the synced group list from what the rule checks for. Jhin v0.6.2's parsed dynamic-range facts do not currently recognize a few truncated upstream spellings (`DolbyV`/`HDR10P`); DraCuLa deliberately follows the parsed runtime facts rather than duplicating Vidhin's own title-marker regex to cover them, since no real-world release-naming evidence for those truncated forms was found.
- **profile:** increase the Samsung profile from 127 to **128** rules and the hardware-neutral profile from 126 to **127** rules for the new rule. Canonical ownership becomes **123 Core**, 4 presentation, and 1 Samsung device rule. Generated Define Library increases from 56 to **57** Defines.
- **testing:** add the permanent `tests/streamnzb_compat/generated_dynamic_hdr_test.go` real-engine regression: the classification predicate across positive/negative/known-translation-gap fixtures, additive scoring interaction with existing HDR10+/DV neutralization, the intended `Flights`/tier-authority inversion, a sole flagged result staying kept rather than hard-rejected, and `DepraveD`/`SasukeducK` correctly stacking this penalty with their pre-existing `Movies LQ Penalty`. Add synthetic upstream-drift coverage to `tests/test_vidhin_sync.py` proving the new sync guard actually fires on both a shape change and a marker-vocabulary change.
- **formatter:** add 16 non-Anime streaming-service formatter fallback badges, closing a dedicated audit's finding that Jhin v0.6.2's `.Network` table only recognizes a handful of major US networks/services and leaves `.Network` empty for many real Movie/Series WEB releases, with no fallback at all unlike the existing Anime Streaming Service rules. New zero-point `presentation`-owned rules, one per service — `Peacock`, `Paramount+`, `Criterion Channel`, `Roku`, `Syfy`, `DC Universe`, `Coupang`, `DMM TV`, `FOD`, `Hotstar`, `KOCOWA`, `U-NEXT`, `Viki`, `Wavve`, `WeTV`, `Youku` — each mirroring the pre-existing Anime `B-Global`/`Bilibili`/`HIDIVE` shape exactly: `not isAnime`, the same WEB-traits gate the Anime service rules already use, and a separator-safe `(?i)(?:^|[. _\[\]-])TOKEN(?:$|[. _\[\]-])` release-name regex. No Define is used or added; `.Network` remains authoritative and the formatter only falls back to a badge when `.Network` is empty, exactly as the Anime fallback already does. Eleven related services (bare `Max`, Movies Anywhere/`MA`, Google Play, iTunes, Showtime, Stan, Fandango, Comedy Central, TVING, Viu, iQIYI) are deliberately excluded: their upstream Vidhin regexes depend on PCRE lookahead/lookbehind (e.g. excluding "HBO Max" from a bare `Max` badge, or the `DTS-HD MA` audio codec token from a "Movies Anywhere" badge) that Go's RE2 rule engine cannot express directly — the same class of constraint already documented for the Obfuscated `Scrambled` translation. This profile does not mirror Vidhin's full streaming-service list, only the 16 services the audit proved safe.
- **profile:** increase the Samsung profile from 128 to **144** rules and the hardware-neutral profile from 127 to **143** rules for the 16 new presentation rules. Canonical ownership becomes **123 Core**, **20 presentation**, and 1 Samsung device rule. Generated Define Library is unchanged at **57** Defines — this feature adds no Define.
- **testing:** add the permanent `tests/streamnzb_compat/non_anime_service_badges_test.go` real-engine regression: the exact zero-point/score-action rule contract for all 16 services; per-service positive matches, non-WEB negatives, and Anime-scope negatives against the actual decoded `profile.txt` rule; fused-token false-positive controls for the six highest-risk short tokens (`PCOK`, `PMTP`, `CRiT`, `DCU`, `FOD`, `KCW`); the Hotstar/`HTSR`-vs-bare-`HS` distinction; spelled-out alias coverage for `Peacock`, `Paramount+`, `DC Universe`, and `KOCOWA`; a full-pipeline scoring-invariance proof (representative `Peacock` case) that a matching badge changes only `MatchedRules`/the formatter label and never final score, Fetch/keep state, or rejection; and an explicit assertion that the generated Define Library stays at exactly 57 entries. Extend `tests/streamnzb_compat/fixtures/formatter.json` with all 16 rendered-label mappings plus Network-wins, existing-Anime-fallback-wins, two-new-services-deterministic, non-duplication, and no-source-control cases.
- **profile:** add `WKN` (Wakanim), a 12th Anime streaming-service formatter fallback rule, closing a real Anime service with zero prior coverage found by the Vidhin `LQ (Release Title)` coverage audit. New zero-point, `presentation`-owned rule mirroring the pre-existing `CR`/`DSNP`/`NF`/`AMZN`/`VRV`/`FUNi`/`ABEMA`/`ADN`/`B-Global`/`Bilibili`/`HIDIVE` shape exactly: `isAnime`, the same WEB-traits gate the other Anime service rules already use, and the same separator-safe `(?i)(?:^|[. _\[\]-])TOKEN(?:$|[. _\[\]-])` release-name regex, matching `WKN|Wakanim`. No `.Network`-empty guard is added to the rule itself, matching every pre-existing Anime/non-Anime service rule — that precedence lives entirely in the formatter template, not the scoring rule. Bare `Waka` is deliberately excluded even though upstream Vidhin's own classifier includes it as an alternative (`\b(WKN|Waka(nim)?)\b`): real-engine-confirmed boundary-safe against fused tokens, but excluded on the same real-world-token-collision-risk grounds as the historical audit (a legitimate Anime title/character literally named "Waka" would false-positive as a streaming badge). No Define is used or added. `formatter.source.json`'s `.Network`-empty service-fallback whitelist gains a matching `WKN` branch so the badge can render when `.Network` is empty; `.Network` continues to take precedence exactly as before for every other service.
- **profile:** increase the Samsung profile from 146 to **147** rules and the hardware-neutral profile from 145 to **146** rules for the new rule. Canonical ownership becomes **125 Core**, **21 presentation**, and 1 Samsung device rule. Generated Define Library is unchanged at **62** Defines — this feature adds no Define.
- **testing:** add the permanent `tests/streamnzb_compat/wkn_wakanim_test.go` real-engine regression: the exact zero-point/score-action rule contract; positive `WKN`/`Wakanim` matches; the deliberately-excluded bare `Waka` and fused-token (`WKNX`, `XWKN`) negatives; the WEB-traits-gate and Anime-only-scope negatives against the actual decoded `profile.txt` rule; and a full-pipeline scoring-invariance proof that a matching release changes only `MatchedRules` and never final score, Fetch/keep state, or rejection, including alongside an independently-scoring native `.Network` match (`CR`/Crunchyroll). Extend `tests/streamnzb_compat/fixtures/formatter.json` with the `WKN` rendered-label mapping plus Network-wins and non-duplication cases.

### Bug Fixes

- **scoring:** fix a scoring-integrity regression in native video-codec ranking: StreamNZB's universal streaming preset gives AVC `+300` and HEVC/AV1 `+700` regardless of content kind, a `+400` codec swing that overturned 7 of 11 production tier families (Movie HD BluRay, Show BluRay, and all four Anime BluRay/WEB families), with a bottom-tier AV1-encoded release outscoring not just the adjacent tier but every tier above it in that family. DraCuLa now neutralizes all three recognized codecs (`Neutralize AVC`/`Neutralize HEVC`/`Neutralize AV1`) to exactly `0`, universally, with no Anime/non-Anime split and no residual `Prefer` rule — the tightest actual non-Anime margin measured anywhere in the system (the HDR10+/lossless-audio physical-media combination) is only `+3`, leaving no safe room for any positive codec preference. Release-group/source tier scores and StreamNZB preset scoring are unchanged; only the codec attribute itself is neutralized.
- **profile:** increase the Samsung profile from 122 to **125** rules and the hardware-neutral profile from 121 to **124** rules for the three new universal codec neutralizers. Canonical ownership becomes **120 Core**, 4 presentation, and 1 Samsung device rule.
- **testing:** add the permanent `TestVideoCodecNeutrality` real-engine regression covering all 12 combinations of content kind (Movie/Show/Anime Movie/Anime Show) and codec (AVC/HEVC/AV1), including parser aliases (`x264`/`AVC`, `x265`/`HEVC`), each asserting an effective codec delta of exactly `0`. Extend `TestAdjacentTierCeilingMatrix` so every family's fully decorated lower tier includes the codec upgrade that caused the regression (AV1 for AVC-baseline families, AV1 for HEVC-baseline families to prove HEVC/AV1 stay equalized), restoring every formerly-broken family to its pre-regression margin and confirming the existing HDR10+/lossless-audio physical-media combination's `+3` margin is unaffected. Add a structural guard in `build_profiles.py` that fails closed if any of the three neutralizers gains a content-kind condition, or if a matching `Prefer AVC`/`Prefer HEVC`/`Prefer AV1` residual rule is ever introduced without a deliberate, reviewed change.
- **scoring:** fix a scoring-integrity regression in native Edition ranking: Jhin's scalar `Edition` field grants a generic native `+100` rank for any non-empty parsed value, with zero differentiation between its 10 canonical values (`Anniversary Edition`, `Ultimate Edition`, `Directors Cut`, `Extended Edition`, `Collectors Edition`, `Theatrical`, `Uncut`, `IMAX`, `Diamond Edition`, `Remastered`). Only Movie `Directors Cut`/`Extended Edition` were ever compensated (native `+100`, stored `-75`, effective `+25`); every other value stayed `+100` uncompensated everywhere, including entirely outside Movie scope, producing a real measured production tier inversion (`-42` margin) for Movie physical media and a razor-thin `+8` margin for Series. DraCuLa now adds one universal Core neutralizer, `Neutralize Edition` (`-100`, no scope, `edition != ""`), that cancels that native rank unconditionally for every content kind; only explicit, reviewed DraCuLa residuals now reach a non-zero effective score. `Movie Edition Preference` is refactored from stored `-75` to stored `+25` (same condition/scope), preserving its existing effective `+25` exactly. Movie `IMAX` (stored `+700`, scope movie) is unchanged; its effective score becomes `+700` (down from the previous `+800`, since native no longer double-contributes) — a real-engine safety audit (isolated measurement and the full adjacent-tier ceiling matrix with IMAX reachable in the lower-tier decoration stack) confirmed `+700` introduces no new tier-authority violation, so it is preserved rather than restored to net `+800`. Open Matte (`+25`, no native Edition score of its own) and the Anime `Uncensored` rule (`+10`, matches `Uncut`/`Unrated`/`Uncensored`/`AT-X`) are completely unchanged; Uncut now nets `+10` for Anime (fully neutralized native, plus the pre-existing Uncensored residual) exactly as before. No blanket preference exists for Anniversary/Ultimate/Collectors/Theatrical/Diamond/Remastered — they are fully neutralized to effective `0` everywhere.
- **profile:** increase the Samsung profile from 126 to **127** rules and the hardware-neutral profile from 125 to **126** rules for the new universal neutralizer. Canonical ownership becomes **122 Core**, 4 presentation, and 1 Samsung device rule.
- **testing:** add the permanent `TestEditionNeutralityRegression` real-engine regression covering all 10 canonical Edition values across all 4 content kinds (Movie/Series/Anime Show/Anime Movie), asserting the exact effective delta for each (`0` for the 6 previously-uncompensated values, `+25` for Movie Directors Cut/Extended, `+700` for Movie IMAX, `+10` for Anime Uncut), plus Unrated remaining unaffected (a separate boolean, no Edition value) and an Extended+IMAX shadowing case proving the scalar Edition field's native rank fires exactly once even when a raw-regex rule (IMAX) independently matches the same release. Extend `TestAdjacentTierCeilingMatrix` so every one of the 11 production families exercises a previously-uncompensated canonical Edition token (`Theatrical`) on the fully decorated lower tier, directly measuring — rather than inferring — that Movie, Series, and Anime families all stay strictly below the clean higher tier, closing the exposure the audit could not directly measure for Anime. Add a structural guard in `build_profiles.py` that fails closed if `Neutralize Edition` or `Movie Edition Preference` drift from their audited scope/points/condition, or if a new positive residual rule for any other canonical Edition value is ever introduced without a deliberate, reviewed change.
- **scoring:** fix a scoring-integrity ordering inversion between Dolby Digital and Dolby Digital Plus, found by the Optional Granular Audio Hierarchy audit. Plain Dolby Digital was left fully native (`+50` for Movies/Shows, on the same "the `200`-point tier gap absorbs it" reasoning as DTS lossy/AAC), while Dolby Digital Plus right above it was neutralized and given a smaller deliberate residual (effective `+25`) — so the objectively worse codec silently outranked the better one by `25` points on every non-Anime release. DraCuLa now adds one universal Core rule, `Neutralize Dolby Digital` (`-50`, no scope, `"dolby_digital" in traits`), bringing Dolby Digital to effective `0` for every content kind. This is a pure correctness fix, not a new preference layer: no `Prefer Dolby Digital` residual is added, so the change only ever removes points from a decorated stack and cannot consume adjacent-tier headroom. Dolby Digital Plus's own effective `+25` is unchanged. The prior Anime-only `Neutralize Anime Dolby Digital` rule is subsumed by the new universal rule and no longer exists — this is a replacement, not a net-new rule, so profile rule counts are unchanged (Samsung **127**, neutral **126**, Core **122**, 4 presentation, 1 Samsung device rule).
- **testing:** add the permanent `TestDolbyDigitalOrderingRegression` real-engine regression, isolating Dolby Digital and Dolby Digital Plus effective deltas across Movie/Series/Anime Movie/Anime Show (`DD = 0` and `DDP = +25` for Movie/Series with `DDP` strictly outranking `DD`; both `0` for Anime), plus a lower-tier-decorated-with-DD-instead-of-DDP case proving no unexpected tier-authority interaction. Extend `TestAnimeAudioNeutrality`'s documentation to reflect that Dolby Digital now reaches `0` through the universal neutralizer rather than a dedicated Anime-only rule; the assertion itself required no behavior change. Add `validate_dolby_digital_ordering()` to `build_profiles.py`, checked independently from the existing audio-neutralization scoping guard so minimal synthetic scoping fixtures are unaffected: it fails closed on drifted points/condition/scope for `Neutralize Dolby Digital`, drifted points/scoping for `Prefer Dolby Digital Plus`, a resurrected `Neutralize Anime Dolby Digital`, or an unintended `Prefer Dolby Digital` residual — with matching synthetic drift tests in `tests/test_profile_variants.py` proving each guard actually fires. `TestAdjacentTierCeilingMatrix` re-run in full: the razor-thin Movie physical-media HDR10+/lossless-audio-plus-RETAG margin is unaffected at `+2`, confirming the fix removes points only from a codec that was never part of that combo's decoration.
- **scoring:** fix a protection-bypass gap in `Adaptive HD x265`, found by a dedicated real-engine audit of adaptive/protective rule interactions. Unlike `Unknown resolution`, the rule carried no `Trusted Release Groups` exemption, so a trusted-tier HEVC SDR release could be silently rejected in a dense same-resolution AVC pool. Add `not matched("Trusted Release Groups")`, mirroring `Unknown resolution`'s existing exemption exactly. No other behavior change.
- **scoring:** fix a second protection-bypass gap found by the same audit, in `Adaptive low-quality filtering`: the rule carried no `isAnime` restriction and no SeaDex exemption, so a SeaDex Best/Alternative Anime recommendation with an HDRip/DVDRip/HDTV source could be silently hard-rejected in a dense pool, contradicting the documented "SeaDex Best/Alternative remain dominant protected recommendations" policy. The obvious inline fix — `not (isAnime and (seadex.best or seadex.alternative))` — was tried and real-engine-disproven: StreamNZB/Jhin's fail-open behavior is a static per-rule check on whether the compiled expression reads an unanswered field tier, not a runtime short-circuit, so any `seadex.*` reference makes the whole rule require SeaDex data unconditionally. Since SeaDex is only ever resolved for Kitsu-addressed (Anime) requests, the inline form silently disabled the rule for every real Movie/Series search instead of narrowly protecting Anime — confirmed directly: the dense-pool Movie HDTV case that was correctly rejected before stopped being rejected at all with that clause in place. The fix instead splits the rule in two, mirroring the existing `Movies LQ Penalty`/`Shows LQ Penalty`/`Anime LQ Penalty` pattern: `Adaptive low-quality filtering` (name preserved) is now `not isAnime`-scoped with no SeaDex reference at all, so Movie/Series behavior is unchanged; a new `Anime Adaptive Low-Quality Filtering` rule is `isAnime`-scoped with `not (seadex.best or seadex.alternative)`, correctly failing open when no SeaDex lookup ran at all and correctly rejecting when SeaDex was checked and found no match.
- **profile:** increase the Samsung profile from 144 to **145** rules and the hardware-neutral profile from 143 to **144** rules for the new `Anime Adaptive Low-Quality Filtering` rule. Canonical ownership becomes **124 Core**, 20 presentation, and 1 Samsung device rule. Generated Define Library is unchanged at **57** Defines — this is a pure rule-scoping change, no Define/classification changes.
- **testing:** add the permanent `tests/streamnzb_compat/adaptive_low_quality_filtering_test.go` real-engine regression: exact `when`-text contract for both `Adaptive low-quality filtering` and `Anime Adaptive Low-Quality Filtering`; dense/sparse behavior for both; Library protection for both; SeaDex Best/Alternative protection for the Anime rule; and the nil-vs-checked-no-match SeaDex distinction (no lookup ran fails open; a resolved "checked, no match" answer correctly rejects). Add a new Trusted-Release-Group `aggregateCase` to the existing `Adaptive HD x265` fixture, and new dense/sparse fixture coverage for both `Adaptive low-quality filtering` rules, in `tests/streamnzb_compat/fixtures/rules.json`. The Anime rule deliberately has no `CaseFixture` entry of its own — that harness layer never populates SeaDex context, mirroring the pre-existing `Anime LQ Penalty` precedent — so its SeaDex coverage lives only in the production-regression Go test. Two other findings from the same audit (`Unknown resolution`'s Movie/Series SeaDex-wiring inertness, and `Best 3 per R/Q`/`Best 1 Season Pack per R/Q` having no Library/SeaDex/trusted-tier carve-out) remain open by deliberate decision, tracked in the local roadmap, not implemented here.
- **scoring:** fix the `Unknown resolution` Movie/Series SeaDex-wiring gap left open by the audit above. Real-engine tracing confirmed `Unknown resolution` was structurally dead for Movie/Series: `req.Seadex` is only ever populated for Kitsu-addressed (Anime) requests, so a real Movie/Series request always carries a nil SeaDex context; StreamNZB/Jhin's fail-open mechanism skips a rule *in full* (not just the unanswered sub-expression) whenever it reads any tier the request has no data for, and `Unknown resolution`'s `seadex.best`/`seadex.alternative` reference put the whole rule in that tier. A dense pool of well-identified Movie/Series alternatives therefore never pruned a weak unknown-resolution/unknown-quality result in production, contrary to the rule's documented V4.4 intent — only Anime ever benefited from the adaptive prune. The fix splits the rule by content kind: the existing `Unknown resolution` (name preserved — it is the merge key for linked-profile updates) is now `not isAnime`-scoped with every `seadex.*` reference removed, restoring the dense/sparse/Library/known-quality/Trusted-Release-Groups protections it already had using only signals a real Movie/Series request always carries; a new `Anime Unknown Resolution` rule is `isAnime`-scoped and keeps the original SeaDex Best/Alternative protection and fail-open-when-no-lookup-ran behavior completely unchanged. A real-engine split-hypothesis probe confirmed no new metadata was needed — the rule's own existing clauses were sufficient once decoupled from the unconditional SeaDex dependency.
- **profile:** increase the Samsung profile from 145 to **146** rules and the hardware-neutral profile from 144 to **145** rules for the new `Anime Unknown Resolution` rule. Canonical ownership becomes **125 Core**, 20 presentation, and 1 Samsung device rule. Generated Define Library is unchanged at **57** Defines — this is a pure rule-scoping change, no Define/classification changes.
- **testing:** correct `TestIntelligentUnknownResolutionProductionPolicy`'s Movie/Series/Show cases, which previously injected an unreachable `seadexCheckedNoMatch()` context — a state no live Movie/Series request can ever carry — and so proved behavior the real engine cannot produce in production; they now use `seadex: nil`, matching reality, while the Anime cases (checked-no-match, SeaDex Best, SeaDex Alternative) remain unchanged since they already reflected real Anime traffic. Add explicit `when`-text contract assertions for both split rules (non-Anime carries no `seadex` reference and is `not isAnime`-scoped; Anime preserves `seadex.best`/`seadex.alternative` and is `isAnime`-scoped), and new dense-pool rejection cases proving Movie, Series, and Anime each independently reject a weak unknown-resolution/unknown-quality result once their own rule's real production conditions are met.
- **scoring:** fix a Vidhin coverage gap in `Retag Soft Penalty`, found by a dedicated audit of Vidhin classification coverage. The rule's redistribution-marker condition was hand-maintained rather than Vidhin-synced, and had silently fallen behind upstream's own `Retags (Radarr)` classification, which gained `.VAV` and `ORARBG` markers the hand-written condition never picked up — confirmed directly: a release tagged with either marker scored zero retag penalty. `Retag Soft Penalty`'s condition is now sourced from a new Vidhin-synced Define, `Retag Markers`, which unions both `Retags (Radarr)` and `Retags (Sonarr)` (`sources: ["Retags (Radarr)", "Retags (Sonarr)"]`), so the rule always reflects at least the broader of the two upstream lists and any future marker addition on either side is picked up automatically at the next sync instead of drifting again. The rule's own semantics are otherwise completely unchanged: universal (no scope), `-1`, a score rule (never rejects). A new fail-closed guard, `validate_retag_source()`, checks both upstream patterns still contain every currently-known marker (including `.VAV`/`ORARBG`) before every sync, and fails closed with a manual-review error if either source disappears or a marker is silently dropped — the same discipline already used for the Generated Dynamic HDR and Obfuscated `Scrambled` classifications. `Literal RETAG Soft Penalty` (a distinct signal — a standalone scene `RETAG` token, not a redistribution-site marker) is completely unaffected.
- **profile:** Generated Define Library increases from 57 to **58** Defines (**57 Vidhin-backed** + 1 local helper, up from 56 Vidhin-backed) for the new `Retag Markers` Define. Canonical profile rule counts are unchanged — Samsung **146**, neutral **145**, Core **125** + 20 presentation + 1 Samsung device rule — since `Retag Soft Penalty` already existed and no rule was added or removed, only its condition and its backing Define changed.
- **testing:** add the permanent Vidhin-sync-level regression in `tests/test_vidhin_sync.py` proving `Retag Markers`' Radarr/Sonarr union (both source patterns present as independent `releaseName matches` alternatives, so the result is a strict superset of either alone) and `validate_retag_source()`'s fail-closed behavior on a missing source and on `.VAV`/`ORARBG` marker drift specifically. Extend the `Retag Soft Penalty` fixture in `tests/streamnzb_compat/fixtures/rules.json` (exercised at both the isolated-fixture and real-published-profile layers by `TestCompatibilityFixtures`) with new positive cases for `.VAV` and `ORARBG` (including an Anime-scoped `ORARBG` case) and matching token-boundary false-positive controls (`.VAVX`, `ORARBGX`), alongside the pre-existing `.heb`/EZTV/RARBG/RARTV/TGx coverage. Bump the hardcoded Define-count literal in `TestNonAnimeServiceBadgeDefineLibraryUnaffected` (57 → 58) and in `tests/test_profile_defines.py`, and extend that file's `validate_retag_soft_penalty()` to assert the rule's condition is exactly `matched("Retag Markers")` and that the published Define itself still carries every required marker, rather than asserting the old hand-written regex text directly.
- **scoring:** fix a correctness bug in `Prefer Atmos`/`Prefer TrueHD`, found by a dedicated Atmos/TrueHD Exclude Groups audit. Vidhin identifies release groups known to falsely tag Atmos and/or TrueHD in their release names (`W4NK3R`, `HQMUX`: both; `3L`, `CtrlHD`, `DON`: TrueHD only) — Jhin's trait detection is a pure filename-regex match with no way to verify the claim, so before this fix a listed group's false claim earned the identical `+25`/`+50` residual preference as a genuine claim from a trusted, non-excluded group; real-engine measurement confirmed the two were indistinguishable. `Prefer Atmos` gains `and not matched("Atmos Exclude Groups")`; `Prefer TrueHD` gains `and not matched("TrueHD Exclude Groups")`. Both new Vidhin-backed Defines are synced as ordinary standard-mode group Defines (`field: "group"`, no custom sync mode). `Neutralize Atmos`/`Neutralize TrueHD` are completely unchanged — Jhin's native score is still cancelled for every group, claimed or not — and no Anime policy, rejection logic, or the unrelated `Generated Dynamic HDR Penalty` is touched. This is a scoring gate, not a group-wide penalty, mirroring Vidhin's own ranked-expression profile (which withholds the Atmos/TrueHD bonus for these exact groups rather than applying a negative score): an excluded group's ordinary release, and its claim of an attribute it is *not* listed for (e.g. `3L` claiming Atmos, which `3L` is not excluded from), are both completely unaffected. Real-engine confirmed no tier-authority impact — the withheld `+25`/`+50` is trivial against tier gaps and was never large enough to threaten adjacent-tier ordering on its own.
- **sync:** extend `semantic_tokens()` in `scripts/sync_vidhin.py` to fall back to treating the whole pattern as its own classifier when an upstream pattern carries no lookahead assertion at all (a bare group alternation, e.g. `\b(W4NK3R|HQMUX)\b`), rather than silently returning no tokens. Every previously-mapped standard-mode source was verified byte-for-byte unaffected (a full before/after token-extraction diff across every currently-mapped standard target found zero mismatches) — the fallback only activates for a pattern shape no existing mapping used.
- **profile:** Generated Define Library increases from 58 to **60** Defines (**59 Vidhin-backed** + 1 local helper, up from 57 Vidhin-backed) for the two new `Atmos Exclude Groups`/`TrueHD Exclude Groups` Defines. Canonical profile rule counts are unchanged — Samsung **146**, neutral **145**, Core **125** + 20 presentation + 1 Samsung device rule — since no rule was added or removed, only two existing rules' conditions changed.
- **testing:** add the permanent `TestAtmosTrueHDExcludeGroupsRegression` real-engine regression: `HQMUX` (on both exclude lists) loses both residuals individually and combined; `3L` (TrueHD-excluded only) loses only its TrueHD residual while its Atmos claim — a claim it is not listed for — still scores the full `+25`, proving the gate is attribute-specific rather than a blanket group suppression; a non-excluded control at the same tier keeps the full `+25`/`+50`/`+75`; an excluded group's own unclaimed release scores identically to a non-excluded peer at the same tier; `CtrlHD` proves the fix applies identically to Movie and Series scope; and the same excluded-group literal token under an Anime request stays completely unaffected, since `not isAnime` already excludes Anime from both preference rules before the new `matched()` clause is ever reached. Fix a latent token-selection hazard the new Defines exposed in the pre-existing `TestMovieAudioNormalizationHierarchy`: its generic "pick the alphabetically-first tier token" helper could silently select an excluded group (`3L` sorts first in `Movies Remux T1 Groups`), which would have made that test's TrueHD+Atmos assertion measure the *excluded* fix behavior instead of the generic one it intends to test; the helper now explicitly skips all five excluded groups. Bump the hardcoded Define-count literal in `TestNonAnimeServiceBadgeDefineLibraryUnaffected` (58 → 60) and in `tests/test_profile_defines.py`.
- **scoring:** add DraCuLa-side title-derived LQ coverage for the exact-safe, uncontested subset of Vidhin's `LQ (Release Title) (Radarr)`/`(Sonarr)` classifications, found by the Vidhin LQ (Release Title) coverage audit. New Vidhin-synced Defines `Movies LQ Release Title`/`Shows LQ Release Title` extend `Movies LQ Penalty`/`Shows LQ Penalty` and `Adaptive Low-Score Filtering`'s existing `-10000`/candidate-relative-prune predicates (`matched("Movies LQ Groups") or matched("Movies LQ Release Title")`, and the Show equivalent) rather than adding parallel rules, so the pre-existing `-10000` semantics, Library protection, adaptive threshold/count behavior, and same-release fallback all apply unchanged. Approved tokens: Movie `1XBET`, `BEN THE MEN`, `R&H`, `READ NOTE`, `SWTYBLZ`, `TeeWee`, `Will1869`, `D3US`, and an exact RE2-safe translation of `jennaortega`'s fixed one-character negative lookbehind, including the upstream outer alternation's trailing word boundary so a fused suffix (`jennaortegaX`, `jennaortegaUHDX`) cannot match (an anti-impersonation signal, distinct from the pre-existing group-based `jennaortega` entry); Show `BEN THE MEN`, `R&H`, `TeeWee`, `CREATiVE24`. Deliberately **not** consumed: `EVO`/`PiRaTeS` (would narrow an existing unconditional penalty — an open policy question, not a coverage gap), `HHWEB` (conflicts with the existing positive `Movies/Shows WEB T3 Groups` trust tier), `unkn0wn` and the Sonarr `BiTOR`+`2160p` combo (each rests on a variable-length lookbehind/lookahead with no provably-exact RE2 equivalent, only an empirically-safe one), and every plain-group token already covered via the parsed `group` field (`GalaxyRG`, `Feranki1980`, `TEKNO3D`, bare `EVO`, Movie-scope `CREATiVE24`). A new `lq_release_title` sync mode pins every upstream branch — including the excluded ones — byte-for-byte against the audited shape, failing the sync closed on any upstream change instead of silently altering coverage.
- **profile:** Generated Define Library increases from 60 to **62** Defines (**61 Vidhin-backed** + 1 local helper). Canonical profile rule counts are unchanged — Samsung **146**, neutral **145**, Core **125** + 20 presentation + 1 Samsung device rule — since no rule was added or removed, only three existing rules' conditions changed.
- **testing:** add the permanent `tests/streamnzb_compat/lq_release_title_test.go` real-engine regression: isolated Define-level classification for every approved token in its content-kind scope, the jennaortega negative-hyphen semantics, every excluded branch (EVO/PiRaTeS/HHWEB/unkn0wn/BiTOR+2160p) staying unmatched, adjacent/fused-word false-positive controls, and unchanged pre-existing group-based LQ behavior; plus a production-regression layer proving `Movies LQ Penalty`/`Shows LQ Penalty` fire for a title-derived-only match and `Adaptive Low-Score Filtering`'s dense/sparse/Library-protection behavior is identical to the pre-existing group-based case. Bump the hardcoded Define-count literal in `TestNonAnimeServiceBadgeDefineLibraryUnaffected` (60 → 62) and in `tests/test_profile_defines.py`.

### Maintenance

- **anime version hardening:** restore dedicated real-engine regression coverage for the existing Anime `v0`–`v4` preference policy, including fused `01v2` fansub notation, mutual-exclusion behavior, unsupported versions, and false-positive controls; add a fail-closed Vidhin/Define sync guard rejecting any resolved Define token exactly equal to `v0`–`v4`, preventing future release-group names from colliding with version markers without changing production matching semantics.

## [5.2](https://github.com/d4s87/streamnzb-template/compare/v5.1...v5.2) (2026-09-07)

V5.2 advances DraCuLa's ranking and formatter behavior around the released StreamNZB 5.18.0 / Jhin 0.6.2 baseline, adopting `SNZBP1` scoring-map portability (`streamnzb_profile` schema v2). It closes a real scoring-integrity regression in high-impact audio normalization that let low-tier Anime, Movie, and Show releases outscore an adjacent higher tier, extends Vidhin's tier-collision guard from Anime-only to every non-Anime Movie/Show family, and consolidates Discord notification tooling behind one shared, tested payload/webhook layer.

### Features

- **compatibility:** advance the pinned StreamNZB runtime from released **5.17.0** to released **5.18.0** commit `575bde653d3e2b45accb9d3bec86e82f376df652`, and the compatibility harness's Jhin dependency from **0.6.1** to **0.6.2**. The full DraCuLa profile, formatter, tier-integrity, and StreamNZB/Jhin compatibility suites pass against the new pinned runtime, run before any schema change. Jhin 0.6.2 also fixes the compact `JA` alias language-parsing limitation tracked in `dreulavelle/jhin#39`; the permanent `TestLanguageSubtitleParserRegression` compact-JA case now asserts `[en, ja]` instead of the prior English-only workaround.
- **compatibility:** adopt `streamnzb_profile` schema **v2** for both generated profiles, following StreamNZB's `Gaisberg/streamnzb#267` fix (closed by commit `1da3a85572a75bbcdda81fb9f9b48de8c859f6da`, shipped in v5.18.0) that lets `SNZBP1` share codes carry the profile-level `scoring` map. DraCuLa's profiles always carry a scoring map, so both `profile.txt` and `profile-neutral.txt` now emit `streamnzb_profile: 2` unambiguously instead of the prior `1`; the `SNZBP1:` share-code prefix itself is unchanged. This is a compatibility/schema change only — canonical rule count, scoring targets, and published profile behavior are unaffected.
- **testing:** add a permanent share-code round-trip regression proving DraCuLa's exact four-entry scoring map (`movie`/`anime_movie` 20 GB·500, `series`/`anime_show` 6 GB·500) survives `encode_payload`/`decode_share_code` unmodified under schema v2, including an explicit check that no stray content kind (e.g. a `default` entry) or stray per-kind field is introduced.
- **compatibility:** advance the pinned StreamNZB runtime from released **5.16.1** to released **5.17.0** commit `018807c6f8417797ab633427d2de72e68aca3c12`. The full DraCuLa profile and formatter compatibility suites pass unchanged against the new stable runtime. StreamNZB 5.17.0 still embeds Jhin 0.6.1; the post-release upstream Jhin 0.6.2 dependency update is audited separately.
- **profile:** bound StreamNZB's inherited preset size scoring to a shared **+500 maximum** while preserving the existing 4K preset targets (20 GB for Movies/Anime Movies, 6 GB for Series/Anime Shows), preventing efficient WEB encodes from receiving an implicit +3000 advantage over large Remux releases while retaining size as a secondary streaming-cost preference.
- **testing:** add permanent production-profile regression coverage for the bounded size-scoring contract, including exact shared Samsung/Neutral scoring metadata and a real Project Hail Mary WEB-DL-versus-Remux interaction reproduced through the pinned StreamNZB/Jhin ranking pipeline.
- **profile:** normalize Jhin v0.6's high-impact native audio ranks in shared Core policy: TrueHD and DTS Lossless are bounded to effective `+100`, Atmos to an additional `+50`, and Dolby Digital Plus to `+25`, preventing audio metadata from overturning intended Movie source/release-group authority.
- **testing:** add permanent pinned real-StreamNZB/Jhin regression coverage for shared Neutral/Samsung audio normalization, including effective audio deltas and clean T1 Remux headroom over decorated T1 WEB-DL and UHD BluRay results.
- **profile:** reject `Sword Art Online II` releases that reset their own `S01` numbering when the resolved request is specifically the base `Sword Art Online` Season 1 Anime Show, closing a real pinned-StreamNZB short-suffix title-matching collision without weakening normal title matching.
- **testing:** add permanent pinned real-StreamNZB/Jhin regression coverage for the SAO Season 1 / `SAO II` collision, including genuine base-SAO results, explicit SAO II requests, other seasons, non-Anime requests, and unrelated later `II` tokens as negative controls.
- **profile:** make the final resolution/quality ceiling season-pack aware: ordinary episode/non-pack results retain the existing best 3 per R/Q, while Series and Anime Show season packs receive an independent best 1 per R/Q slot, preserving one strong pack alternative without globally widening result sets.
- **testing:** add permanent pinned real-StreamNZB regression coverage for the season-pack-aware 3+1 ceiling, proving three episode releases and one pack survive independently and that the existing `Complete Season Pack Preference +10` makes an otherwise equal COMPLETE pack win the single pack slot.
- **ci:** notify Discord after successful `main` validation when the published `formatter.txt` and/or `formatter-debug.txt` artifacts change, including the triggering commit subject so linked formatter users can see what changed before refreshing without waiting for a template release.
- **ci:** centralize formatter, release, and Vidhin Discord webhook delivery through one tested sender while preserving the existing shared thread, missing-secret skip behavior, and non-fatal workflow delivery semantics.
- **formatter:** expose same-release failover compactly with `⧉N` only when multiple interchangeable NZBs exist for the same release; single-copy results remain uncluttered.
- **formatter:** expose corrected-release status as `ᴘʀᴏᴘᴇʀ`, `ʀᴇᴘᴀᴄᴋ`, `ʀᴇᴘᴀᴄᴋ₂`, or `ʀᴇᴘᴀᴄᴋ₃` using existing parsed facts and exact matched production-rule identities.
- **formatter:** distinguish ordinary reported NZB availability (`💚 ɴᴢʙ`) from releases also confirmed healthy on a configured provider backbone (`💚 ɴᴢʙ+`) through StreamNZB's native `Availability.OnMyBackbone` field.
- **testing:** add permanent pinned real-StreamNZB 5.16.1 candidate and published-formatter regressions for same-release variant visibility/suppression, all four corrected-release labels, and ordinary versus backbone-confirmed availability.

### Changes

- **scoring:** make DraCuLa's NZB size influence explicit instead of inheriting StreamNZB's `4k` preset `+3000` size weight. Both generated profiles now publish the same profile-level `scoring` map with `size_weight: 500`; `SNZBP1` and `streamnzb_profile == 1` remain unchanged, while the internal canonical rules-source schema advances from 1 to 2.
- **profile:** increase the Samsung profile from 114 to **115** rules and the hardware-neutral profile from 110 to **114** rules as high-impact audio normalization moves into shared Core policy and Dolby Digital Plus normalization is added. Canonical ownership becomes **110 Core**, 4 presentation, and **1 Samsung device rule**.
- **architecture:** move Atmos, TrueHD, and DTS Lossless compensation out of the Samsung device layer and into shared Portable Core, add shared Dolby Digital Plus normalization, and leave `DV without HDR fallback` as the only Samsung-specific device rule.
- **profile:** increase the Samsung profile from 113 to **114** rules and the hardware-neutral profile from 109 to **110** rules with the shared SAO Season 1 title-collision safeguard. Canonical ownership becomes **106 Core**, 4 presentation, and 4 Samsung device rules.
- **profile:** increase the Samsung profile from 112 to **113** rules and the hardware-neutral profile from 108 to **109** rules with the shared season-pack R/Q limiter. Canonical ownership becomes **105 Core**, 4 presentation, and 4 Samsung device rules.
- **formatter:** stop independently versioning formatter display names. The normal and diagnostic artifacts now use the stable names **DraCuLa** and **DraCuLa Debug** and inherit the DraCuLa template/repository release lifecycle.
- **formatter:** keep the new reliability and corrected-release metadata presentation-only; no scoring, filtering, tier, Library, availability-bonus, or same-release fallback policy changes are introduced.

### Bug Fixes

- **sync:** extend the fail-closed Vidhin tier-collision/empty-tier guard from Anime-only to the 7 non-Anime Movie/Show tier families (`Movies UHD BluRay`, `Movies HD BluRay`, `Movies Remux`, `Movies WEB`, `Shows BluRay`, `Shows Remux`, `Shows WEB`), closing a gap where a duplicate release-group token across tiers, or a tier resolving to zero tokens, could previously reach production unreviewed.
- **defines:** the new guard surfaced a live Vidhin upstream inconsistency: release group `TOMMY` is listed in both `Movies WEB T1` and `T2` upstream. Existing tier precedence already resolved this in DraCuLa's favor of T1 at render time, so the published `generated/streamnzb-defines.txt` is unchanged byte-for-byte; the mapping now encodes that resolution explicitly with `remove_tokens` instead of leaving it as an unreviewed side effect of precedence.
- **testing:** add synthetic regression coverage proving the new Movie/Show tier-collision/empty-tier check fires on same-family cross-tier token collisions, duplicate tokens within one tier, and an emptied tier, while confirming legitimate cross-family overlap (e.g. the same group in `Movies WEB T1` and `Movies Remux T1`) is not flagged.
- **tooling:** `build_formatter.py` now rejects a `formatter.txt`/`formatter-debug.txt` artifact containing embedded newlines or an empty payload before attempting to decode it, matching the structural guard `build_profiles.py` already applies to `profile.txt`/`profile-neutral.txt`.
- **scoring:** fix a scoring-integrity regression introduced by the shared high-impact audio normalization change: because `Normalize TrueHD`/`DTS Lossless`/`Atmos`/`Dolby Digital Plus` carried no scope restriction, their effective residual bonus reached Anime as well as Movies/Shows. Combined with other ordinary bonuses, this let a real-engine adjacent-tier check show a bottom-tier Anime BluRay release outscoring a clean release one tier higher by over 100 points, a Movie Remux/UHD BluRay/HD BluRay release doing the same to its immediate neighbor, and a plain untouched-native AAC track alone crossing Anime's minimum tier gap. Each of the four codecs is now split into a universal `Neutralize X` rule (compensates the native score to `0` for every content kind, including Anime) and a `Prefer X` rule (the deliberate residual bonus, scoped `not isAnime`), mirroring the existing `Neutralize`/`Prefer HDR10 Plus` pattern. Non-Anime residuals are also trimmed (TrueHD/DTS Lossless `+100` → `+50`, Atmos `+50` → `+25`, Dolby Digital Plus unchanged at `+25`) to close the Movie/Show adjacent-tier overflow. Three new Anime-only rules (`Neutralize Anime AAC`, `Neutralize Anime DTS Lossy`, `Neutralize Anime Dolby Digital`) close the previously-untouched-codec gap for Anime specifically; Movies/Shows keep those three fully native since their 200-point tier gap safely absorbs them.
- **profile:** increase the Samsung profile from 115 to **122** rules and the hardware-neutral profile from 114 to **121** rules for the audio neutralization split above (net +7: four existing rules become eight, plus three new Anime-only neutralizers). Canonical ownership becomes **117 Core**, 4 presentation, and 1 Samsung device rule.
- **testing:** add the permanent, table-driven `TestAdjacentTierCeilingMatrix` real-engine regression covering every production tier family (Movie Remux/UHD BluRay/HD BluRay/WEB, Show Remux/BluRay/WEB, Anime Show/Movie BluRay/WEB) plus a dedicated Anime AAC-only case. Unlike the prior ceiling tests, it asserts directly against the engine's own score for a fully decorated lower tier versus a clean higher tier rather than a hardcoded "maximum stack" constant — the exact assumption that let this regression reach production undetected. Extend the existing audio-normalization regression with isolated TrueHD-alone/Atmos-alone effective-delta checks and add `TestAnimeAudioNeutrality`, asserting all seven audio codecs DraCuLa touches contribute exactly `0` for Anime. Add a structural guard in `build_profiles.py` that fails closed if a universal `Neutralize` rule ever gains an Anime-conditional clause, or a `Prefer`/Anime-only rule ever loses its scoping.

### Maintenance

- **ci:** extract the Discord payload envelope (`content` + `allowed_mentions`), the 2000-character message limit, and the ellipsis-truncation helper used by commit-subject/release-name trimming into a shared `scripts/discord_common.py`, used by `formatter_discord.py`, `release_discord.py`, and `vidhin_discord.py`. Each script's own message-building logic is unchanged; only the previously triplicated envelope/limit/truncation code is unified. Add `tests/test_discord_common.py` and wire it into CI.

## [5.1](https://github.com/d4s87/streamnzb-template/compare/v5.0...v5.1) (2026-09-02)

V5.1 hardens DraCuLa's ranking and formatter behavior around the released StreamNZB 5.16.1 / Jhin 0.6.1 baseline. It adds candidate-relative Adaptive Low-Score Filtering, restores release-group tier authority over native display/edition/corrected-release scoring, adds bounded HDR10+ preference and Vidhin-backed Obfuscated penalties, and improves formatter language/subtitle presentation with permanent real-engine regression coverage.

### Features

- **formatter:** display parsed language metadata together with Jhin's subtitle-presence flag, while rendering subtitle-only results cleanly as `⛿ sᴜʙ` instead of a leading ` · sᴜʙ` separator.
- **testing:** add permanent Jhin 0.6.1 language/subtitle parser regressions plus pinned real-StreamNZB formatter coverage for languages without subtitles, languages with subtitles, and subtitle-only results.
- **profile:** add Adaptive Low-Score Filtering as a candidate-relative post-scoring prune for Movie/Show LQ and Bad Dual releases. A non-Library candidate is removed only when at least six alternatives finish at least 5000 points above that candidate's own final score, preserving the same class of release as a fallback in sparse result sets.
- **testing:** add permanent pinned real-StreamNZB 5.16.1 regression coverage for Adaptive Low-Score Filtering, including dense-tail pruning and sparse-result fallback behavior through `finalScore` / `current.finalScore`.
- **testing:** add permanent StreamNZB 5.16.1 / Jhin 0.6.1 parser regression coverage for issue #251: canonical `IMAX.Enhanced` and `IMAX-Enhanced` remain parsed as IMAX editions without being classified as upscaled, bare `Enhanced` remains neutral, and genuine `AI.Enhanced` / `Upscaled` markers remain classified as upscaled.
- **testing:** extend the permanent pinned real-StreamNZB/Jhin dynamic-range regression for the bounded non-Anime HDR10+ preference, Dolby Vision HDR10+ fallback behavior, Anime HDR10+ neutrality, and release-group ceiling preservation.
- **testing:** add permanent pinned real-StreamNZB/Jhin coverage for Portable Core dynamic-range and bit-depth compensation, Samsung Dolby Vision fallback behavior, neutral-profile Dolby Vision eligibility, Anime WEB tier authority against native 10-bit scoring, and Movie WEB tier authority against HDR10 scoring.
- **testing:** add permanent pinned real-StreamNZB/Jhin scoring-ceiling coverage for Anime Movie/Show tier authority, effective corrected-release scores, effective Movie edition scores, and native-score compensation behavior.
- **profile:** add Vidhin-backed Obfuscated release classification for Movies/Anime Movies and Series/Anime Shows with a `-1` soft penalty, keeping matching releases eligible as tie-breaking fallbacks rather than rejecting them.
- **testing:** add permanent pinned real-StreamNZB coverage for Obfuscated marker matching, Movie/Show classifier separation, Anime Movie/Show behavior, ordinary-release negatives, and the distinct Radarr/Sonarr `Scrambled` boundary semantics.
- **testing:** extend the real-StreamNZB Intelligent Unknown Resolution regression to Anime Movies, completing explicit trusted-tier protection coverage for Movies, Series, Anime Shows, and Anime Movies.
- **automation:** notify Discord when a GitHub Release is published. The workflow triggers only on `release.published`, posts a concise release link and linked-install refresh guidance to the existing DraCuLa Discord thread, disables mentions, and keeps webhook delivery non-fatal.
- **testing:** add regression coverage for stable releases, prereleases, release-name fallback/bounding, required metadata, draft rejection, and non-published release-event rejection.


### Changes

- **formatter:** keep subtitle-language identity deliberately unresolved because StreamNZB 5.16.1 / Jhin 0.6.1 exposes parsed `Languages` plus `Subbed`, but no separate subtitle-language list. DraCuLa therefore reports subtitle presence without guessing forms such as `SUB (EN · DE)`.
- **compatibility:** advance the pinned StreamNZB runtime from 5.16.0 to released **5.16.1** commit `0429d50b347000325fa40ecf8aeb670335d23260` and align the compatibility harness with **Jhin 0.6.1**. The released baseline passes the full DraCuLa profile, formatter, ranking, tier, availability, dynamic-range, and candidate-relative prune regressions.
- **compatibility:** mark the prior IMAX Enhanced parser limitation as resolved upstream by StreamNZB issue `#251` / commit `1c1db3ff81f897ab7adb8664f42c8b524d272a80`. Canonical IMAX Enhanced releases are no longer misclassified as upscaled, while genuine AI-enhanced/upscaled releases remain rejected by the existing production policy; no DraCuLa scoring workaround is required.
- **profile:** increase the Samsung profile from 111 to **112** rules and the hardware-neutral profile from 107 to **108** rules with the shared Adaptive Low-Score Filtering rule. Canonical ownership becomes **104 Core**, 4 presentation, and 4 Samsung device rules.
- **scoring:** add a bounded Portable Core `+25` preference for non-Anime HDR10+ releases after compensating Jhin v0.6's native `+2100` HDR10+ rank. HDR/HDR10 remain neutral, while the smaller bonus acts as a format tie-breaker without overriding Movie/Series release-group tier authority.
- **anime:** keep HDR10+ score-neutral for Anime. The proven maximum ordinary Anime Show WEB metadata stack remains `+77` against the minimum adjacent tier gap of `80`, so adding a meaningful HDR10+ bonus would consume the remaining `3` points of guaranteed tier headroom.
- **profile:** increase the Samsung profile from 110 to 111 rules and the hardware-neutral profile from 106 to 107 rules. Canonical ownership becomes 103 Core, 4 presentation, and 4 Samsung device rules.
- **scoring:** neutralize Jhin v0.6's native display-dependent ranks in the shared Portable Core: Dolby Vision `+3000`, HDR10+ `+2100`, HDR `+2000`, and parsed 10-bit `+100` are compensated by `-3000`, `-2100`, `-2000`, and `-100`. Dynamic-range and bit-depth metadata therefore remain classification/compatibility facts instead of overriding DraCuLa release-group tier authority.
- **architecture:** move `Neutralize Dolby Vision` from the Samsung device layer into Portable Core. The Samsung profile now retains four device-only playback rules, including rejection of Dolby Vision without HDR fallback; the neutral profile applies no Dolby Vision compatibility rejection.
- **profile:** increase the Samsung profile from 107 to 110 rules and the hardware-neutral profile from 102 to 106 rules. Canonical ownership becomes 102 Core, 4 presentation, and 4 Samsung device rules.
- **scoring:** compensate Jhin v0.6's native `+20` PROPER/REPACK rank in the stored corrected-release rules, preserving final effective preferences of `+5` for PROPER/REPACK, `+6` for REPACK2, and `+7` for REPACK3 without double-counting native parser rank.
- **scoring:** compensate Jhin v0.6's native `+100` parsed-edition rank for Movie editions: IMAX is stored at `+700` to remain effective `+800`, while the shared Director's Cut / Extended Edition rule is stored at `-75` to remain effective `+25`; Open Matte remains `+25`.
- **scoring:** re-space both Anime Movie and Anime Show release-group ladders around the full effective metadata ceiling. Anime WEB is now `+500/+400/+300/+200/+100/+20`; Anime BluRay is `+560/+480/+400/+320/+240/+160/+80/+0`. The minimum adjacent gap is `80`, exceeding the proven maximum ordinary Anime stack of `+77`.
- **scoring:** verify non-Anime tier authority without re-spacing: Movie WEB/Remux retain `200`-point gaps with a proven maximum ordinary stack of `+97` and `103` points of headroom; Show WEB/Remux retain `153` points of headroom against the proven `+47` ordinary stack.
- **definitions:** expand the published Define Library from 54 to 56 Defines with `Movies Obfuscated` and `Shows Obfuscated`; the synchronized Vidhin-backed semantic baseline grows from 53 to 55 mappings while the local `Trusted Release Groups` helper remains unchanged.
- **compatibility:** translate only Vidhin's two PCRE positive-lookbehind `Scrambled` branches into boolean-equivalent Go-regexp expressions supported by StreamNZB/Jhin, with fail-closed synchronization guards for upstream regex drift.
- **profile:** increase the Samsung profile from 105 to 107 rules and the hardware-neutral profile from 100 to 102 rules with the two shared Obfuscated soft-penalty rules.
- **profile:** simplify Intelligent Unknown Resolution trusted-tier protection by replacing 47 direct `matched()` tier references with one generated `Trusted Release Groups` Define. The helper is derived from the synchronized Movie, Show, Anime Movie, and Anime Show tier Defines, is published only in the StreamNZB Define Library, and does not alter the 53-entry semantic Vidhin baseline.
- **tooling:** remove the one-time V5 migration guard that prevented intentional post-V5 changes from regenerating `profile.txt`. `scripts/build_profiles.py` continues to validate canonical source structure, ownership, variants, and encode/decode round trips before writing generated artifacts.
- **compatibility:** advance the pinned StreamNZB runtime to the released 5.16.0 commit `4c3c29df4eb02e9e001fa841d9431a629858c2d7`, enabling candidate-relative prune aggregates through `current.finalScore` / `current.finalRank` while preserving the existing V5 profile and formatter compatibility contract.
- **testing:** add a permanent real-StreamNZB regression for candidate-relative pruning, proving that only a sufficiently weak tail release is removed when at least three alternatives beat it by 5000 points and that the same release remains available as a fallback when the result set is sparse.

## [5.0](https://github.com/d4s87/streamnzb-template/compare/v4.5...v5.0) (2026-09-01)

V5.0 introduces the generated multi-profile architecture. The existing Samsung QN90A-oriented `profile.txt` remains behavior-compatible, while the new `profile-neutral.txt` provides a hardware-neutral alternative generated from the same canonical rule source.

### Features

- Added the V5.0 generated multi-profile architecture with a canonical ordered rule registry (`profiles/rules.json`), explicit profile variants (`profiles/variants.json`), and deterministic `scripts/build_profiles.py` generation.
- Added `profile-neutral.txt`, a 100-rule hardware-neutral profile generated from the same policy source as `profile.txt`. It excludes exactly five Samsung/device-specific playback compensation rules while retaining the shared Core and presentation policy, including `Reject 3D`.
- Added permanent profile-variant regression coverage for ownership, deterministic generation, exact Samsung reproduction, neutral rule membership/order, shared-rule identity, and real StreamNZB compilation of the complete neutral artifact.
- Added a GitHub Actions Discord notification for published semantic Vidhin Define Library updates. Notifications are sent only after the changed generated library reaches `main`, summarize added, changed, and removed Define names, suppress metadata-only synchronization changes, and remind linked StreamNZB users to refresh the Define Library first.
- Added permanent regression coverage for notification semantics, including metadata-only suppression, added/changed/removed Defines, published-library-only changes, Discord message construction, and disabled mentions. Webhook delivery is intentionally non-fatal.

### Changes

- Preserved `profile.txt` as the existing 105-rule Samsung QN90A-oriented linked profile while moving profile maintenance to generated variants. Its generated output remains byte-for-byte identical to the pre-V5 artifact.
- Defined the V5 migration contract: existing `profile.txt` users continue refreshing normally; users who want the hardware-neutral policy import `profile-neutral.txt` as a new linked profile and switch their StreamNZB assignments. V5.0 does not redesign `Unknown resolution` or change `Adaptive HD x265`.
- Synchronized the generated Vidhin-backed classification data with current
  upstream mappings. `ATELiER` moves from Movie HD BluRay T2 to T1, and
  `OldT` is added to both Movie and Show Bad Dual Groups.

- Completed the post-V4.5 Vidhin synchronization audit. Upstream
  EVO/PiRaTeS LQ release-title corrections, the new FAND/Fandango
  classification, and TrueHD exclusion-group additions such as `3L` and
  `HQMUX` do not change any currently mapped DraCuLa Define and were not
  manually imported. The production profile remains at 105 rules, the
  generated Define Library remains at 53 rules, and the pinned StreamNZB
  compatibility revision is unchanged.

## [4.5](https://github.com/d4s87/streamnzb-template/compare/v4.4...v4.5) (2026-08-31)

### Features

- Added **Complete Season Pack Preference**: explicitly complete season packs
  for Series and Anime Shows receive a small `+10` ranking preference.
  Ordinary season packs, individual and multi-episode releases, complete
  show packs, Movies, and Anime Movies remain neutral.
- Added pinned real-engine compatibility coverage for the exact published
  Complete Season Pack rule, including Series and Anime Show positives plus
  ordinary packs, single/multi-episode releases, complete show packs, Movies,
  and Anime Movies.
- Added **Movie Edition Preference** for parser-backed Movie editions:
  Director's Cut and Extended Edition share one non-stacking `+25`
  preference using Jhin v0.6's native `edition` metadata.
- Added pinned real-engine coverage for Director's Cut, apostrophe variants,
  Extended Edition, Extended Cut, Movie-only scope, and neutral unsupported
  Final Cut / Criterion Collection / Special Edition forms.
- Added explicit IMAX Enhanced regression coverage. IMAX Enhanced continues
  to receive the existing `+800` IMAX preference and does not receive a
  second stacked edition bonus.
- Added optional **DraCuLa Debug V1** formatter artifact for live
  troubleshooting. It exposes request context, final/top scores, raw and
  parsed release metadata, same-release variants, availability, SeaDex,
  Library/ffprobe state, and every matched profile rule with its individual
  score contribution.
- Added pinned real-StreamNZB regression fixtures for both candidate and
  published debug formatter rendering, including rich diagnostic and
  no-matched-rule cases.

- Added production compatibility validation for Jhin v0.6 rule-name semantics: published profile rule names must be non-empty and unique, and case-only `matched()` / Define-name drift is reported explicitly because `matched()` references are case-sensitive.
- Added permanent real-engine episode-parsing regression coverage for normal multi-episode releases, Anime hybrid season/absolute numbering, absolute episode ranges, dashed Anime season/episode notation, season ranges, and complete season packs against the pinned StreamNZB/Jhin v0.6 engine.

### Changes

- Increased the production profile from **103 to 104 rules** with Complete
  Season Pack Preference. The generated Define Library remains at **53**
  rules because the feature uses native StreamNZB/Jhin facts and introduces
  no new Define dependency.
- Increased the production profile from **104 to 105 rules** with Movie
  Edition Preference. The generated Define Library remains at **53** rules
  because the preference uses native StreamNZB/Jhin `edition` metadata and
  introduces no new Define dependency.
- Expanded the Movie edition ceiling policy so the low-weight Open Matte
  `+25` and Director's Cut / Extended Edition `+25` preferences may combine
  to at most `+50`, remaining below the `200`-point Movie release-group tier
  gap. IMAX remains the deliberate strong exception at `+800`.
- Generalized the formatter build/check tooling so multiple `SNZBF1:`
  artifacts can share the same deterministic source-to-published workflow.
  The compatibility runner now checks and renders both the normal and debug
  published formatters against the pinned StreamNZB runtime.
- Updated the Anime BluRay tier-ceiling regression for Anime Shows: the
  maximum known effective positive minor-metadata stack increases from
  `+31` to `+41` when Complete Season Pack Preference applies, while the
  existing 70-point tier gaps still leave `29` points of headroom below the
  next-higher clean tier.

- Updated the pinned StreamNZB compatibility revision from `4c0f7b385e5f7bfb514523b908fa04f153dfbbe2` to `f1d55a294b98f4ae7c685ea17cec230b1d12a2bc`, migrating the compatibility harness and formatter regression suite to StreamNZB's Jhin v0.6 rule engine.
- Adapted aggregate compatibility tests to StreamNZB's request-kind-aware Jhin v0.6 aggregate API and updated Unknown Resolution diagnostic assertions for the new engine reporting without changing production scoring or filtering policy.
- Removed the obsolete direct `expr-lang` compatibility-harness dependency after the StreamNZB rule engine migration; Jhin v0.6 now provides the rule-expression engine used by the pinned runtime.

## [4.4](https://github.com/d4s87/streamnzb-template/compare/v4.3...v4.4) (2026-08-30)

### Features

- Added relative five-star result ranking using StreamNZB's `.TopScore` and `stars` formatter helper. The highest-scoring result renders as `★★★★★`, while lower-scoring results are scaled against the current result-set winner.
- Added real-engine formatter regression coverage for full, partial, negative, and zero-TopScore star rendering.
- Added global corrected-release preference rules: PROPER / REPACK `+5`, REPACK2 `+6`, and REPACK3 `+7`, with mutually exclusive scoring so numbered repacks do not also receive the base bonus.
- Added pinned real-engine compatibility coverage for base, numbered, separated, lowercase, `REAL.*`, unsupported, and false-positive REPACK / PROPER forms, plus structural validation of the published scoring policy.
- Added Anime revision preference rules for explicit `v0`–`v4` release markers: `v0` `-1`, `v1` `+1`, `v2` `+2`, `v3` `+3`, and `v4` `+4`.
- Added mutually exclusive Anime revision matching so releases containing multiple supported `v0`–`v4` markers receive no version score instead of stacking ambiguous revision bonuses.
- Added pinned real-engine compatibility coverage for Anime Shows and Movies, episode-suffix forms such as `01v2`, case variants, REPACK interaction, unsupported `v5+`, false positives, and multi-version non-stacking behavior.
- Added a global **Retag Soft Penalty** of `-1` for recognized redistribution markers including `.heb`, EZTV variants, RARBG, RARTV, and TGx. The rule is intentionally a metadata tie-breaker and never rejects a result.
- Added pinned real-engine compatibility coverage for Retag matching, including spaced and unspaced forms, EZTV variants, case handling, Anime redistribution, clean releases, legitimate Anime bracket groups, and false-positive boundaries.
- Added a dedicated Anime **Dual/Multi Audio Preference** of `+10`, shared between Dual Audio and Multi Audio so the preference remains non-stacking and subordinate to Anime release-group tiers.
- Added pinned real-engine compatibility coverage for Anime/non-Anime Dubbed, Dual Audio and Multi Audio behavior, including Anime Movies and combined Dual+Multi release names.
- Added structural validation for the split Anime/non-Anime audio policy so Anime cannot silently inherit the legacy high-value audio bonuses again.

### Changes

- Replaced the unconditional Unknown Resolution rejection with adaptive **Intelligent Unknown Resolution** handling. Weak unknown-resolution results are pruned only when more than six alternatives have both known resolution and known quality; scarce fallbacks remain available.

- Moved the `💚 ɴᴢʙ` availability indicator from the result name to the description score line, keeping the compact result name focused on resolution, quality, and relative ranking.
- Updated the pinned StreamNZB compatibility revision to `4c0f7b385e5f7bfb514523b908fa04f153dfbbe2` to validate the `.TopScore` and `stars` formatter API against the real engine.
- Increased the production profile from **101 to 110 rules** across the current Unreleased scoring additions: three global corrected-release preference rules, five Anime revision preference rules, and one global Retag soft-penalty rule. The generated Define Library remains at **53** rules because these features use native StreamNZB parser traits and/or direct release-name matching rather than new Vidhin-backed classifications.
- Increased the production profile from **110 to 111 rules** with the dedicated Anime Dual/Multi Audio preference; the subsequent non-Anime audio ceiling correction consolidated three legacy scoring rules into one shared rule, reducing the current production profile to **109 rules**. The generated Define Library remains at **53** rules.
- Replaced the legacy non-Anime `Dubbed bonus` (`+500`), `Dual audio` (`+200`), and `Multi audio` (`+200`) rules with one shared, non-stacking `+10` `Non-Anime Dubbed/Dual/Multi Audio Preference`. Real-engine validation confirmed that DUBBED, Dual Audio, Multi Audio, and combined Dual+Multi releases now receive exactly one `+10` preference instead of reachable `+700`/`+900` stacks.
- Corrected the Anime Streaming Service documentation to reflect the intentional TRaSH-recommended preference scale (`CR +6`, `DSNP +5`, `NF +4`, `AMZN/VRV +3`, `FUNi +2`, `ABEMA/ADN +1`, B-Global/Bilibili/HIDIVE `0`) rather than describing those rules as score-neutral.

- Normalized positive availability scoring so it acts only as a tie-breaker: `Alive on our backbone` is now `+20` and `Recently confirmed` is `+10`, for a maximum combined positive availability contribution of `+30`.
- Removed the `Very fresh NZB`, `Recent NZB`, `Popular NZB`, `Very popular NZB`, and `Highly popular NZB` score rules. Freshness and grab-count metadata remain formatter-visible but no longer affect ranking directly.
- Reduced the current production profile from **109 to 103 rules** after removing the redundant Library rule and five freshness/popularity scoring rules. The generated Define Library remains at **53** rules.

### Bug Fixes
- Protected useful incomplete-metadata results from adaptive Unknown Resolution pruning when they have recognized quality, match a trusted Movie/Show/Anime release-group tier, are Library results, or are SeaDex Best/Alternative recommendations. Missing SeaDex lookup data fails open, and known-resolution Unknown Quality results remain unaffected. Pinned real-engine regression coverage verifies the complete production policy.
- Rescaled Anime BluRay release-group tiers to +500/+430/+360/+290/+220/+150/+80/+10 (T1–T8), creating uniform 70-point gaps so the known +31 cumulative positive minor-metadata stack cannot overtake the next-higher clean tier; Anime WEB tier scores remain unchanged.

- Fixed a reachable Anime scoring inversion where StreamNZB's `dubbed` parser trait caused Dual/Multi Audio Anime releases to inherit the generic `+500` Dubbed bonus in addition to the legacy `+200` Dual/Multi score. Anime Dual/Multi Audio now receives one `+10` same-tier preference instead of an effective `+700` bonus.

- Fixed **Movie-version preference scoring** so `IMAX` and `Open matte` are explicitly Movie-only instead of global rules that could override Show and Anime release-group hierarchies.
- Corrected the Movie edition preference scale from `IMAX +1000` / `Open Matte +500` to `IMAX +800` / `Open Matte +25`. IMAX remains an intentional strong Movie preference, while Open Matte now remains below the 200-point Movie release-group tier gap. Real-engine regression coverage verifies Movie-only scope, matching boundaries, `+825` combined stacking, and representative tier interactions.
- Fixed Library scoring double-counting by removing the profile-level `Library hit +500` rule. StreamNZB's `4k` preset already supplies the intended native `+500` Library bonus; the previous combination produced an effective `+1000` preference.
- Fixed Anime and non-Anime Dubbed/Dual/Multi Audio scoring against StreamNZB's native `-1000` dubbed/audio rank. The published rules now use raw `+1010` compensation so the complete ranking pipeline produces the intended effective `+10` non-stacking preference.
- Added full-pipeline regression coverage for native Library scoring, the `+30` positive availability ceiling, and effective Anime/non-Anime audio scoring, and updated the Anime BluRay ceiling regression to distinguish raw audio compensation from its effective `+10` preference.


## [4.3](https://github.com/d4s87/streamnzb-template/compare/v4.2...v4.3) (2026-08-30)

### Features

- Added Vidhin-backed **Anime Dubs Only** classification with a StreamNZB-safe translation of the upstream release-name expression.
- Added an availability-aware **Anime Dubs Only Penalty** of `-10`: dub-only Anime is demoted only when a non-dub Anime alternative exists, so scarce dub-only results remain available.
- Added explicit Dual/Multi Audio protection so legitimate Dual Audio releases are not classified as dub-only, including known dub groups.
- Added aggregate fixture and production-profile regression coverage for dub-only Anime Shows and Movies, Dual Audio protection, non-Anime exclusion, and scarce-result preservation.

- Added Anime WEB **Streaming Service** detection for Crunchyroll (`CR`), Disney+ (`DSNP`), Netflix (`NF`), Amazon (`AMZN`), VRV, Funimation (`FUNi`), ABEMA, ADN, B-Global, Bilibili, and HIDIVE.
- Added StreamNZB compatibility fixtures covering positive service markers, boundaries and false positives, Anime scope, BluRay exclusion, and Anime Movie WEB matching.
- Added production-profile regression coverage for all Streaming Service detection rules.
- Added a human-readable formatter development source at `tests/streamnzb_compat/formatter.source.json`.
- Added `scripts/build_formatter.py` to build `formatter.txt` from the human-readable source and verify source/published semantic synchronization.
- Added a formatter render regression harness using StreamNZB's pinned real formatter engine for candidate and production simulation.
- Added formatter fixtures covering Network-first display, Streaming Service fallback priority, duplicate prevention, source/Edition presentation, and representative full Anime WEB and Movie Remux renders.
- Added fresh-render safeguards by disabling the Go test cache and logging SHA-256 fingerprints of the formatter and fixture inputs.

### Changes

- Expanded the generated Define Library from **52 to 53** reusable classifications with `Anime Dubs Only`.

- Increased the production profile from **91 to 101 rules**.
- Updated the formatter to use matched Streaming Service rules as a fallback when `.Network` is unavailable, while preserving `.Network` as the preferred source label.
- Removed the separate Crunchyroll and HIDIVE formatter badges now that those services participate in the unified Network-first source display.
- Extended the StreamNZB compatibility suite to require semantic synchronization between the formatter source and `formatter.txt` and to run the published formatter regression automatically.

### Bug Fixes

- Fixed formatter source/Edition spacing so source plus Edition renders as `♛ source · edition »`, source-only renders as `♛ source »`, and Edition-only no longer receives an orphan leading separator.

## [4.2](https://github.com/d4s87/streamnzb-template/compare/v4.1...v4.2) (2026-08-29)

### Features

- Added an availability-aware **1080p Remux Preference** rule: non-Anime, non-Library 1080p Remuxes receive a `+50` bonus when SDR 2160p WEB-DL alternatives exist but no HDR/HDR10+ 2160p WEB-DL is available.
- Added aggregate compatibility coverage for the 1080p Remux preference across Movies and Shows, including SDR 4K WEB-DL, HDR, HDR10+, Dolby Vision-only, Dolby Vision with HDR fallback, Anime, Library, 2160p Remux, and unsupported content-kind cases.
- Added production-profile regression and structural validation for the published **1080p Remux Preference** rule.
- Added availability-aware **Adaptive HD x265** filtering: non-Anime SDR 720p/1080p HEVC releases are rejected only when more than six suitable same-resolution AVC alternatives exist, while 2160p, HDR/Dolby Vision, Anime, Library, HEVC Remux, and AV1 results remain exempt.
- Added an Anime-only **Uncensored** preference rule with a `+10` score.
- Added detection for explicit `Uncensored`, `Uncut`, `Unrated`, and `AT-X` release-name markers.
- Added `ᴜɴᴄᴇɴꜱᴏʀᴇᴅ` formatter labeling for matching Anime releases.
- Added StreamNZB compatibility fixtures covering Uncensored marker variants, boundaries, false positives, Anime scope, and Anime Movies.
- Added production-profile regression coverage for the published Uncensored rule.
- Added separate Vidhin-backed **Movie Bad Dual Groups** and **Show Bad Dual Groups** classifications.
- Added **Movie Bad Dual Penalty** and **Show Bad Dual Penalty** rules with a `-10,000` score.
- Added raw upstream group-regex synchronization so Bad Dual expressions retain regex-specific semantics instead of being flattened into literal release-group tokens.
- Added compatibility fixtures for Movie/Show Bad Dual matching, Radarr/Sonarr-specific groups, raw-regex behavior, content scope, negative cases, and Anime Show exclusion.
- Added Define-Library-aware compatibility testing so `matched()`-based rules can be exercised against the generated shared Defines and exact production rules.
- Added an explicit StreamNZB profile-schema compatibility guard that pins `streamnzb_profile == 1` and fails when a missing or future schema version requires compatibility review.

### Changes

- Increased the production profile from **86 to 91 rules** across the current Unreleased changes.
- Expanded the generated Define Library from **50 to 52** reusable classifications.
- Extended the StreamNZB compatibility harness to load the generated Define Library when compiling rules that use `matched()`.

### Bug Fixes

- Fixed the Movie and Show Bad Dual penalty rules to use explicit `movie` and `series` scopes instead of appearing as **All Content** in StreamNZB.

## [4.1](https://github.com/d4s87/streamnzb-template/releases/tag/v4.1) (2026-08-29)

### Features

- Added Vidhin-backed **Anime LQ Groups** classification using the upstream release-name regex.
- Added an **Anime LQ Penalty** of `-10,000` for matching Anime releases.
- SeaDex **Best** and **Alternative** recommendations are exempt from the Anime LQ penalty.
- Expanded the shared Define Library from **49 to 50** reusable classifications.
- Added raw `releaseName` regex support to the Vidhin synchronization generator.
- Added validation for Anime LQ mapping, matching behavior, and metadata-only synchronization changes.

### Notes

- Anime LQ matching preserves Vidhin's upstream regex semantics rather than converting the expression into release-group tokens.
- The linked StreamNZB Define Library now contains **50** Define rules.

## 4

### Changes

* Expanded the shared Define Library from 33 to 49 reusable release-group classifications.
* Replaced the compressed Anime T1/T2/T3 model with the full Vidhin Anime tier hierarchy:

  * WEB T1–T6
  * BluRay T1–T8
* Expanded Anime Movie and Show release-group scoring from 12 to 28 rules.
* Updated Anime tier scores to:

  * T1: +500
  * T2: +400
  * T3: +300
  * T4: +200
  * T5: +100
  * T6: +50
  * BluRay T7: +25
  * BluRay T8: +10
* Updated `Reject bad 4K Anime` to recognize the complete Anime WEB and BluRay tier hierarchy.
* Added handling for `LazyRemux` and `UltraRemux`, which StreamNZB may classify with the `remux` trait because of their release-group names while not exposing the expected BluRay trait.
* Preserved the existing non-Anime Movie/Show tier structure and overall V3 ranking and filtering philosophy.

### Validation

* Validated Anime WEB tiers T1–T6 and BluRay tiers T1–T8 against representative release names.
* Verified `LazyRemux` as Anime BluRay T4 and `UltraRemux` as Anime BluRay T5.
* Verified known low-tier 4K Anime groups remain eligible while unknown 4K Anime release groups are rejected.

### Notes

* V4 requires the linked StreamNZB Define Library containing all 49 Define rules.
* Anime release-group classifications remain synchronized with `Vidhin05/Releases-Regex` through the repository's existing GitHub Actions workflow.
* `Define` rules classify releases only; scoring and filtering behaviour remains controlled by the profile.

## 3

### Changes
- Refactored release-group tiers into reusable `Define` rules.
- T1/T2/T3 scoring rules now reference shared group definitions with `matched()`.
- Updated smart 4K filtering rules to reference the same reusable definitions.
- Removed duplicated release-group regex logic from conditional filters.
- Preserved existing V2 ranking/filtering behaviour while improving maintainability.

### Define Library
- Moved Vidhin-backed release-group definitions out of `profile.txt` into a shared StreamNZB Define Library.
- The library is maintained in `generated/streamnzb-defines.txt`.
- Added 33 shared Define rules covering Movie, Show and Anime release-group classifications, including Movie and Show LQ groups.
- Release-group definitions are synchronized with Vidhin05/Releases-Regex through GitHub Actions.
- Upstream changes are reviewed through pull requests before being published to the library.
- `profile.txt` retains the scoring and filtering policy and references library definitions through `matched()`.
- Linked Define Library updates can be reviewed and applied using StreamNZB's **Refresh** action.

### Notes
- `Define` rules do not score, reject, limit, or appear in `.MatchedRules`.
- V3 requires the shared Define Library to be imported before using `profile.txt`.
- This version requires a StreamNZB build with shared Define Library and `matched()` support.

## 2

### Changes
- Expanded and refined the original StreamNZB filtering and scoring profile.
- Improved release-group prioritization and quality-based ranking.
- Added more advanced filtering and scoring logic for Movies, Shows and Anime.
- Added smart filtering for 4K releases and low-quality results.
- Refined Anime handling, including release-group tiers and streaming-source preferences.
- Improved result limiting and fallback behaviour.
- Continued tuning HDR, Dolby Vision and audio preferences for the target hardware setup.

### Notes
- V2 predates the shared StreamNZB Define Library architecture.
- Release-group regexes and classifications were embedded directly in the profile rules.
- At this stage, GitHub was used to distribute the linked `profile.txt` and `formatter.txt`; there was no separately linked or automatically synchronized Define Library.
- V2 formed the behavioral foundation that was later refactored into V3 without intentionally changing its overall ranking and filtering philosophy.

## 1

### Features
- Initial public version of the custom StreamNZB profile.
- Introduced the core filtering and scoring approach for Usenet results.
- Added release-group prioritization for Movies, Shows and Anime.
- Added quality, resolution, HDR/Dolby Vision and audio preferences.
- Added the custom StreamNZB formatter.

### Notes
- V1 used profile-local filtering, scoring and release-group regexes.
- GitHub distribution consisted of the linked `profile.txt` and `formatter.txt`.
- Shared Define Libraries and automated Vidhin synchronization were not yet part of the setup.
