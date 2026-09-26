package streamnzb_compat

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

// TestAdjacentTierCeilingMatrix is the authoritative real-engine regression
// for release-group tier authority. Earlier ceiling regressions
// (TestAnimeTierEffectiveCeilings, TestMovieEditionPreferenceCeilings)
// check a tier gap against a hardcoded "maximum ordinary stack" constant.
// That constant has to be kept in sync by hand every time a new bonus
// becomes reachable by a given content kind, and nothing fails if someone
// forgets: high-impact audio normalization was added as a globally-scoped
// rule (no scope restriction) without ever being added to those constants
// or to the decorated test titles that exercise them, so both tests kept
// passing while real tier inversions became reachable in production.
//
// This test does not use a hardcoded maximum. For every production tier
// family, it decorates the lowest realistic combination of *every*
// currently-reachable ordinary positive bonus onto a lower tier and
// compares the engine's own score directly against a clean candidate one
// tier higher. The engine's output is the contract: if a future change
// (new bonus, widened scope, new codec normalization, etc.) makes a
// decorated lower tier outscore a clean higher tier, this test fails
// immediately regardless of whether anyone remembered to update a
// constant.
func TestAdjacentTierCeilingMatrix(t *testing.T) {
	// Compile the exact published payload (preset, scoring map and rules):
	// a Rules-only profile silently falls back to the preset's native size
	// weight, so any sized candidate here would not be scored as shipped.
	payload := loadProfilePayload(t, "../../profile.txt", "production")
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:    "Adjacent tier ceiling matrix",
			Preset:  payload.Preset,
			Scoring: payload.Scoring,
			Rules:   payload.Rules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)

	fullAvail := triage.AvailState{
		Status:       triage.AvailAvailable,
		OnMyBackbone: true,
		CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
	}

	score := func(kind, title string, avail *triage.AvailState) int {
		t.Helper()

		candidate := triage.Candidate{
			Release: &release.Release{Title: title},
		}
		if avail != nil {
			candidate.Verdict.Avail = *avail
		}

		request := ranking.Request{
			Kind:  kind,
			Title: "Example",
		}

		if kind == ranking.KindSeries || kind == ranking.KindAnimeShow {
			request.Season = 1
			request.Episode = 1
		}

		if kind == ranking.KindAnimeShow || kind == ranking.KindAnimeMovie {
			request.IsAnime = true
		}

		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{candidate},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 {
			t.Fatalf("unexpectedly rejected %q: %+v", title, rejected)
		}
		if len(kept) != 1 {
			t.Fatalf("kept %d releases for %q; want 1", len(kept), title)
		}

		return kept[0].Torrent.Rank
	}

	tok := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}

	// lossyAudioVariant is one entry of a family's realisticLossyAudioVariants
	// list -- an ordered slice rather than a map so iteration order (and
	// therefore t.Logf/t.Errorf ordering) is deterministic across runs.
	type lossyAudioVariant struct {
		label       string
		decorations []string
	}

	// A family describes one production tier ladder and everything needed
	// to build a realistic "fully decorated" title for its lowest tier
	// and a "clean" title for any tier.
	type family struct {
		label        string
		kind         string
		definePrefix string // e.g. "Movies Remux", "Anime Shows BluRay"
		tierCount    int
		// sourceTokens are the release-name tokens establishing the
		// quality/source traits this family's tier rule requires
		// (resolution, remux/bluray/webdl trait, etc.), independent of
		// tier group, codec and decorations. Codec is deliberately not
		// included here (see ordinaryCodec/codecUpgrade below).
		sourceTokens func(isSeries bool) []string
		// ordinaryCodec is the codec the family's clean higher tier is
		// built with — StreamNZB's native rank for it must no longer
		// matter now that codec is neutralized, but it stays realistic
		// (HEVC for families that conventionally ship it, AVC otherwise).
		ordinaryCodec string
		// codecUpgrade is the "better" codec (by StreamNZB's own native
		// rank) applied only to the fully-decorated lower tier. This is
		// the exact shape of the codec-scoring tier-authority regression:
		// StreamNZB ranks AVC +300 and HEVC/AV1 +700 regardless of
		// content kind, so an AVC-baseline family's lower tier is
		// decorated with AV1 (a genuine, real +400 native swing before
		// neutralization) and a HEVC-baseline family's lower tier gets
		// AV1 too, proving HEVC and AV1 stay equalized.
		codecUpgrade string
		// decorations are the extra tokens applied only to the "fully
		// decorated" lower-tier candidate: every ordinary positive bonus
		// actually reachable by this content kind/source combination.
		decorations []string
		// editionDecoration, when set, adds a per-tier-pair check for this
		// family using decorations plus one of the canonical Jhin Edition
		// values that the Edition Preference Layer audit proved was
		// previously uncompensated (a silent native +100 with no DraCuLa
		// offset). "Theatrical" is used universally: its Jhin pattern
		// (`\bTheatrical\b`) has no fragile boundary requirements, unlike
		// Diamond Edition's `\b\.Diamond\.\b`, which consumes both
		// flanking dots and can fuse an adjacent token depending on
		// placement - Theatrical is a clean, representative stand-in for
		// the whole previously-broken class (Anniversary, Ultimate,
		// Collectors, Theatrical, Uncut, Diamond, Remastered all behave
		// identically post-fix: fully neutralized to zero contribution).
		editionDecoration string
		// hdr10PlusDecorations, when set, adds a second per-tier-pair
		// check for this family using decorations plus an HDR10+ marker.
		// HDR10+ is not mutually exclusive with high-impact lossless
		// audio on physical media (a real UHD BluRay/Remux can be
		// mastered in HDR10+ and carry a TrueHD Atmos track at the same
		// time), so this covers a realistic combined interaction that
		// the base decorations list above does not exercise. Left nil
		// for families where the combination is not realistic (WEB
		// encodes rarely if ever advertise both) to keep this coverage
		// about reachable interactions rather than synthetic
		// combinatorics.
		hdr10PlusDecorations []string
		// realisticLossyAudioVariants, when set, adds a further per-variant
		// check reusing the same HDR10+ combo above but with the
		// TrueHD+Atmos audio slot replaced by a native/uncompensated lossy
		// codec (AAC/DTS Lossy tier-authority audit, 2026-09-15 -- the
		// exact realistic combo that reproduced a ~+22/+23 adjacent-tier
		// inversion before "Neutralize AAC"/"Neutralize DTS Lossy" went
		// universal). An ordered slice (not a map) so iteration -- and
		// therefore t.Logf/t.Errorf order -- stays deterministic across
		// runs. Deliberately populated only for families/codecs where the
		// combination is actually authored in the wild: plain DTS Lossy on
		// a physical-media Remux (many catalog UHD Blu-ray discs carry
		// only a lossy DTS core, no lossless option) and AAC or DTS Lossy
		// on a UHD BluRay re-encode (x265 groups routinely re-encode/keep
		// lossy audio while preserving HDR10+ metadata). AAC is
		// deliberately NOT added for Remux (the Blu-ray spec does not
		// author AAC tracks) -- see README/CHANGELOG for the realism
		// rationale; do not add it here without concrete evidence.
		realisticLossyAudioVariants []lossyAudioVariant
	}

	seriesTitle := func(source []string, group string, extras []string) string {
		parts := append([]string{"Example.Show", "S01E01"}, source...)
		parts = append(parts, extras...)
		return joinCeilingParts(parts) + "-" + group
	}

	movieTitle := func(source []string, group string, extras []string) string {
		parts := append([]string{"Example.Movie", "2026"}, source...)
		parts = append(parts, extras...)
		return joinCeilingParts(parts) + "-" + group
	}

	animeShowTitle := func(source []string, group string, extras []string) string {
		parts := append([]string{"Example.Anime", "S01E01"}, source...)
		parts = append(parts, extras...)
		return joinCeilingParts(parts) + "-" + group
	}

	animeMovieTitle := func(source []string, group string, extras []string) string {
		parts := append([]string{"Example.Anime.Movie", "2025"}, source...)
		parts = append(parts, extras...)
		return joinCeilingParts(parts) + "-" + group
	}

	families := []struct {
		family
		build func(source []string, group string, extras []string) string
	}{
		{
			family: family{
				label:             "Movie Remux",
				editionDecoration: "Theatrical",
				kind:              ranking.KindMovie,
				definePrefix:      "Movies Remux",
				tierCount:         3,
				sourceTokens: func(bool) []string {
					return []string{"2160p", "UHD", "BluRay", "REMUX"}
				},
				ordinaryCodec: "HEVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
				hdr10PlusDecorations: []string{
					"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio",
					"REPACK3", "TrueHD", "Atmos", "7.1",
				},
				realisticLossyAudioVariants: []lossyAudioVariant{
					{"DTS Lossy", []string{"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "DTS", "5.1"}},
				},
			},
			build: movieTitle,
		},
		{
			family: family{
				label:             "Movie UHD BluRay",
				editionDecoration: "Theatrical",
				kind:              ranking.KindMovie,
				definePrefix:      "Movies UHD BluRay",
				tierCount:         3,
				sourceTokens: func(bool) []string {
					return []string{"2160p", "UHD", "BluRay"}
				},
				ordinaryCodec: "HEVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
				hdr10PlusDecorations: []string{
					"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio",
					"REPACK3", "TrueHD", "Atmos", "7.1",
				},
				// UHD BluRay is a re-encode (not a Remux), so a smaller
				// x265 release realistically re-encodes/keeps lossy audio
				// (AAC or a retained lossy DTS core) while preserving the
				// disc's HDR10+ metadata -- unlike Remux, AAC is realistic
				// here too.
				realisticLossyAudioVariants: []lossyAudioVariant{
					{"AAC", []string{"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "AAC5.1"}},
					{"DTS Lossy", []string{"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "DTS", "5.1"}},
				},
			},
			build: movieTitle,
		},
		{
			family: family{
				label:             "Movie HD BluRay",
				editionDecoration: "Theatrical",
				kind:              ranking.KindMovie,
				definePrefix:      "Movies HD BluRay",
				tierCount:         3,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
				hdr10PlusDecorations: []string{
					"HDR10Plus", "Open.Matte", "Extended.Edition", "Dual.Audio",
					"REPACK3", "TrueHD", "Atmos", "7.1",
				},
			},
			build: movieTitle,
		},
		{
			family: family{
				label:             "Movie WEB",
				editionDecoration: "Theatrical",
				kind:              ranking.KindMovie,
				definePrefix:      "Movies WEB",
				tierCount:         3,
				sourceTokens: func(bool) []string {
					return []string{"2160p", "WEB-DL"}
				},
				ordinaryCodec: "HEVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3",
					"DDP5.1", "Atmos",
				},
			},
			build: movieTitle,
		},
		{
			family: family{
				label:             "Show Remux",
				editionDecoration: "Theatrical",
				kind:              ranking.KindSeries,
				definePrefix:      "Shows Remux",
				tierCount:         2,
				sourceTokens: func(bool) []string {
					return []string{"2160p", "UHD", "BluRay", "REMUX"}
				},
				ordinaryCodec: "HEVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"S01.COMPLETE", "Dual.Audio", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: seriesTitle,
		},
		{
			family: family{
				label:             "Show BluRay",
				editionDecoration: "Theatrical",
				kind:              ranking.KindSeries,
				definePrefix:      "Shows BluRay",
				tierCount:         2,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"S01.COMPLETE", "Dual.Audio", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: seriesTitle,
		},
		{
			family: family{
				label:             "Show WEB",
				editionDecoration: "Theatrical",
				kind:              ranking.KindSeries,
				definePrefix:      "Shows WEB",
				tierCount:         3,
				sourceTokens: func(bool) []string {
					return []string{"2160p", "WEB-DL"}
				},
				ordinaryCodec: "HEVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"S01.COMPLETE", "Dual.Audio", "REPACK3",
					"DDP5.1", "Atmos",
				},
			},
			build: seriesTitle,
		},
		{
			family: family{
				label:             "Anime Show BluRay",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeShow,
				definePrefix:      "Anime Shows BluRay",
				tierCount:         8,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: animeShowTitle,
		},
		{
			// Remuxes reach the Anime BluRay tiers through the generated
			// Define's upstream source gate (anime_bd sync mode); native
			// remux +1500 stacks with the tier here, so this family is
			// checked on its own rather than assumed from the encode one.
			family: family{
				label:             "Anime Show BluRay REMUX",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeShow,
				definePrefix:      "Anime Shows BluRay",
				tierCount:         8,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay", "REMUX"}
				},
				ordinaryCodec: "AVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: animeShowTitle,
		},
		{
			family: family{
				label:             "Anime Show WEB",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeShow,
				definePrefix:      "Anime Shows WEB",
				tierCount:         6,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "WEB-DL"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"CR", "S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4",
					"REPACK3", "DDP5.1", "Atmos",
				},
			},
			build: animeShowTitle,
		},
		{
			family: family{
				label:             "Anime Movie BluRay",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeMovie,
				definePrefix:      "Anime Movies BluRay",
				tierCount:         8,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Dual", "Audio", "Uncensored", "v4", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: animeMovieTitle,
		},
		{
			family: family{
				label:             "Anime Movie BluRay REMUX",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeMovie,
				definePrefix:      "Anime Movies BluRay",
				tierCount:         8,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "BluRay", "REMUX"}
				},
				ordinaryCodec: "AVC",
				codecUpgrade:  "AV1",
				decorations: []string{
					"Dual", "Audio", "Uncensored", "v4", "REPACK3",
					"TrueHD", "Atmos", "7.1",
				},
			},
			build: animeMovieTitle,
		},
		{
			family: family{
				label:             "Anime Movie WEB",
				editionDecoration: "Theatrical",
				kind:              ranking.KindAnimeMovie,
				definePrefix:      "Anime Movies WEB",
				tierCount:         6,
				sourceTokens: func(bool) []string {
					return []string{"1080p", "WEB-DL"}
				},
				ordinaryCodec: "x264",
				codecUpgrade:  "AV1",
				decorations: []string{
					"CR", "Dual", "Audio", "Uncensored", "v4", "REPACK3",
					"DDP5.1", "Atmos",
				},
			},
			build: animeMovieTitle,
		},
	}

	for _, f := range families {
		f := f

		t.Run(f.label, func(t *testing.T) {
			for tier := 2; tier <= f.tierCount; tier++ {
				lowerTier := tier
				higherTier := tier - 1

				lowerGroup := tok(fmt.Sprintf(
					"%s T%d Groups", f.definePrefix, lowerTier,
				))
				higherGroup := tok(fmt.Sprintf(
					"%s T%d Groups", f.definePrefix, higherTier,
				))

				source := f.sourceTokens(true)

				lowerSource := append(append([]string{}, source...), f.codecUpgrade)
				higherSource := append(append([]string{}, source...), f.ordinaryCodec)

				lowerFullTitle := f.build(lowerSource, lowerGroup, f.decorations)
				higherCleanTitle := f.build(higherSource, higherGroup, nil)

				lowerFull := score(f.kind, lowerFullTitle, &fullAvail)
				higherClean := score(f.kind, higherCleanTitle, nil)

				if lowerFull >= higherClean {
					t.Errorf(
						"%s: T%d fully decorated (%d) does not stay below "+
							"clean T%d (%d); margin=%+d\n  decorated: %s\n  clean:     %s",
						f.label,
						lowerTier,
						lowerFull,
						higherTier,
						higherClean,
						lowerFull-higherClean,
						lowerFullTitle,
						higherCleanTitle,
					)
				}

				// HDR10+ is not mutually exclusive with high-impact
				// lossless audio on physical media, so a release can
				// realistically be decorated with both at once. This is
				// a distinct combination from the base decorations
				// above (which omit HDR10+) and was never covered by
				// any prior ceiling regression. Deliberately no
				// hardcoded expected margin: the point is that any
				// future change consuming the remaining headroom fails
				// this assertion automatically, not that today's exact
				// margin is pinned as a constant to keep in sync.
				if f.hdr10PlusDecorations != nil {
					hdr10Source := append(append([]string{}, source...), f.ordinaryCodec)

					decoratedTitle := f.build(
						hdr10Source, lowerGroup, f.hdr10PlusDecorations,
					)
					cleanTitle := f.build(hdr10Source, higherGroup, nil)

					decoratedLower := score(f.kind, decoratedTitle, &fullAvail)
					cleanHigher := score(f.kind, cleanTitle, nil)

					t.Logf(
						"%s T%d HDR10+/lossless-audio combo: decorated=%d "+
							"clean-T%d=%d margin=%+d",
						f.label,
						lowerTier,
						decoratedLower,
						higherTier,
						cleanHigher,
						decoratedLower-cleanHigher,
					)

					if decoratedLower >= cleanHigher {
						t.Errorf(
							"%s: T%d HDR10+ + lossless audio combo (%d) does "+
								"not stay below clean T%d (%d)\n"+
								"  decorated: %s\n  clean:     %s",
							f.label,
							lowerTier,
							decoratedLower,
							higherTier,
							cleanHigher,
							decoratedTitle,
							cleanTitle,
						)
					}

					// AAC/DTS-Lossy tier-authority audit (2026-09-15): the
					// exact realistic HDR10+ + native-lossy-audio combo
					// that reproduced a ~+22/+23 adjacent-tier inversion
					// against this same cleanHigher before "Neutralize
					// AAC"/"Neutralize DTS Lossy" went universal. Only
					// exercised where the codec/family combination is
					// actually authored in the wild (see
					// realisticLossyAudioVariants doc comment) -- this is
					// not a synthetic algebraic-maximum case.
					for _, lv := range f.realisticLossyAudioVariants {
						variant, lossyDecorations := lv.label, lv.decorations
						lossyTitle := f.build(
							hdr10Source, lowerGroup, lossyDecorations,
						)
						lossyLower := score(f.kind, lossyTitle, &fullAvail)

						t.Logf(
							"%s T%d HDR10+/%s combo: decorated=%d "+
								"clean-T%d=%d margin=%+d",
							f.label,
							lowerTier,
							variant,
							lossyLower,
							higherTier,
							cleanHigher,
							lossyLower-cleanHigher,
						)

						if lossyLower >= cleanHigher {
							t.Errorf(
								"%s: T%d HDR10+ + %s combo (%d) does not "+
									"stay below clean T%d (%d)\n"+
									"  decorated: %s\n  clean:     %s",
								f.label,
								lowerTier,
								variant,
								lossyLower,
								higherTier,
								cleanHigher,
								lossyTitle,
								cleanTitle,
							)
						}
					}
				}

				// Edition-neutrality regression: the Edition Preference
				// Layer audit proved that 6 of Jhin's 10 canonical Edition
				// values were silently uncompensated (a raw native +100
				// with no offsetting DraCuLa rule), and specifically
				// measured a real production inversion for Movie physical
				// media (-42 margin, a fully decorated lower tier
				// outscoring a clean immediately-higher tier). The
				// universal "Neutralize Edition" rule fixes this; this
				// check directly proves it holds for every family, not
				// just Movie, by adding a previously-uncompensated
				// canonical Edition token to the fully decorated lower
				// tier. No hardcoded margin — strict inequality only.
				if f.editionDecoration != "" {
					editionDecorations := append(
						append([]string{}, f.decorations...),
						f.editionDecoration,
					)

					decoratedTitle := f.build(
						lowerSource, lowerGroup, editionDecorations,
					)
					cleanTitle := higherCleanTitle

					decoratedLower := score(f.kind, decoratedTitle, &fullAvail)
					cleanHigher := higherClean

					t.Logf(
						"%s T%d + %s (previously uncompensated edition): "+
							"decorated=%d clean-T%d=%d margin=%+d",
						f.label,
						lowerTier,
						f.editionDecoration,
						decoratedLower,
						higherTier,
						cleanHigher,
						decoratedLower-cleanHigher,
					)

					if decoratedLower >= cleanHigher {
						t.Errorf(
							"%s: T%d fully decorated + %s (%d) does not stay "+
								"below clean T%d (%d); margin=%+d\n"+
								"  decorated: %s\n  clean:     %s",
							f.label,
							lowerTier,
							f.editionDecoration,
							decoratedLower,
							higherTier,
							cleanHigher,
							decoratedLower-cleanHigher,
							decoratedTitle,
							cleanTitle,
						)
					}
				}

				// Inverse-risk case: a negative penalty (Literal RETAG Soft
				// Penalty, -1) applied to the HIGHER tier instead of the
				// lower one. A negative score creates the opposite failure
				// mode from every other case in this matrix: instead of a
				// decorated lower tier climbing too high, a penalized higher
				// tier could fall too low. The invariant is strictly
				// "lower tier < RETAG'd higher tier" for every family — no
				// hardcoded margin, since the point is that any future
				// change consuming the remaining headroom fails this
				// assertion automatically.
				retagHigherSource := append(
					append([]string{}, source...), f.ordinaryCodec,
				)
				retagHigherTitle := f.build(
					retagHigherSource, higherGroup, []string{"RETAG"},
				)
				retagHigher := score(f.kind, retagHigherTitle, nil)

				t.Logf(
					"%s T%d fully decorated (%d) vs RETAG'd clean T%d (%d): "+
						"margin=%+d",
					f.label,
					lowerTier,
					lowerFull,
					higherTier,
					retagHigher,
					retagHigher-lowerFull,
				)

				if lowerFull >= retagHigher {
					t.Errorf(
						"%s: T%d fully decorated (%d) does not stay below "+
							"RETAG'd T%d (%d); margin=%+d\n"+
							"  decorated: %s\n  RETAG'd higher: %s",
						f.label,
						lowerTier,
						lowerFull,
						higherTier,
						retagHigher,
						retagHigher-lowerFull,
						lowerFullTitle,
						retagHigherTitle,
					)
				}

				if f.hdr10PlusDecorations != nil {
					hdr10Source := append(
						append([]string{}, source...), f.ordinaryCodec,
					)
					comboTitle := f.build(
						hdr10Source, lowerGroup, f.hdr10PlusDecorations,
					)
					comboLower := score(f.kind, comboTitle, &fullAvail)

					t.Logf(
						"%s T%d HDR10+/lossless-audio combo (%d) vs RETAG'd "+
							"clean T%d (%d): margin=%+d",
						f.label,
						lowerTier,
						comboLower,
						higherTier,
						retagHigher,
						retagHigher-comboLower,
					)

					if comboLower >= retagHigher {
						t.Errorf(
							"%s: T%d HDR10+ + lossless audio combo (%d) does "+
								"not stay below RETAG'd T%d (%d); margin=%+d\n"+
								"  decorated: %s\n  RETAG'd higher: %s",
							f.label,
							lowerTier,
							comboLower,
							higherTier,
							retagHigher,
							retagHigher-comboLower,
							comboTitle,
							retagHigherTitle,
						)
					}

					// Same AAC/DTS-Lossy realistic-combo evidence as above,
					// checked against the RETAG'd higher tier too.
					for _, lv := range f.realisticLossyAudioVariants {
						variant, lossyDecorations := lv.label, lv.decorations
						lossyTitle := f.build(
							hdr10Source, lowerGroup, lossyDecorations,
						)
						lossyLower := score(f.kind, lossyTitle, &fullAvail)

						t.Logf(
							"%s T%d HDR10+/%s combo (%d) vs RETAG'd clean "+
								"T%d (%d): margin=%+d",
							f.label,
							lowerTier,
							variant,
							lossyLower,
							higherTier,
							retagHigher,
							retagHigher-lossyLower,
						)

						if lossyLower >= retagHigher {
							t.Errorf(
								"%s: T%d HDR10+ + %s combo (%d) does not "+
									"stay below RETAG'd T%d (%d); margin=%+d\n"+
									"  decorated: %s\n  RETAG'd higher: %s",
								f.label,
								lowerTier,
								variant,
								lossyLower,
								higherTier,
								retagHigher,
								retagHigher-lossyLower,
								lossyTitle,
								retagHigherTitle,
							)
						}
					}
				}
			}
		})
	}

	// Dedicated case: AAC and DTS Lossy are now universally neutralized to
	// 0 for every content kind ("Neutralize AAC"/"Neutralize DTS Lossy",
	// AAC/DTS-Lossy tier-authority audit, 2026-09-15 -- they are no longer
	// "untouched native" for Movies/Shows; only Dolby Digital remains a
	// dedicated universal-no-residual case documented separately, see
	// TestDolbyDigitalOrderingRegression). This checks the tightest real
	// Anime boundary (WEB T5->T6, exactly the 80-point minimum) with each
	// codec alone and nothing else decorated, confirming Anime's own
	// effective-zero behavior is unchanged by the move from an Anime-only
	// rule to a universal one.
	for _, tc := range []struct {
		label string
		extra string
	}{
		{"AAC", "AAC2.0"},
		{"DTS Lossy", "DTS.5.1"},
	} {
		tc := tc

		t.Run(fmt.Sprintf("Anime Show WEB %s-only (universal neutralizer)", tc.label), func(t *testing.T) {
			t5 := tok("Anime Shows WEB T5 Groups")
			t6 := tok("Anime Shows WEB T6 Groups")

			source := []string{"1080p", "WEB-DL", "x264"}

			t6Decorated := animeShowTitle(source, t6, []string{tc.extra})
			t5Clean := animeShowTitle(source, t5, nil)

			t6Score := score(ranking.KindAnimeShow, t6Decorated, nil)
			t5Score := score(ranking.KindAnimeShow, t5Clean, nil)

			if t6Score >= t5Score {
				t.Errorf(
					"Anime Show WEB T6 with %s only (%d) does not stay "+
						"below clean T5 (%d); margin=%+d\n"+
						"  T6+%s: %s\n  T5 clean: %s",
					tc.label, t6Score, t5Score, t6Score-t5Score,
					tc.label, t6Decorated, t5Clean,
				)
			}
		})
	}
}

func loadCeilingDefines(t *testing.T) map[string][]string {
	t.Helper()

	data, err := os.ReadFile("../../generated/vidhin-defines.json")
	if err != nil {
		t.Fatalf("read generated Vidhin baseline: %v", err)
	}

	var generated struct {
		Defines map[string]struct {
			Tokens []string `json:"tokens"`
		} `json:"defines"`
	}
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("decode generated Vidhin baseline: %v", err)
	}

	out := make(map[string][]string, len(generated.Defines))
	for k, v := range generated.Defines {
		out[k] = v.Tokens
	}
	return out
}

func joinCeilingParts(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "." + p
	}
	return out
}
