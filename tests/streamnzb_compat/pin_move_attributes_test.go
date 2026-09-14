package streamnzb_compat

// StreamNZB v6.0.0 / Jhin v0.7.1 pin-move compensation regressions.
//
// Jhin v0.7.1 assigns four new native scores DraCuLa did not previously
// need to compensate: HLG (+1500), DTS:X (+2000), VC-1 (+100), DTS-ES
// (+100). All four are neutralized universally with zero residual (see
// "Neutralize HLG"/"Neutralize DTS X"/"Neutralize VC-1"/"Neutralize
// DTS-ES" in profiles/rules.json and the pin-move audit, 2026-09-14/15).
//
// This file is the permanent real-engine proof that the compensated
// v6.0.0 profile preserves adjacent-tier authority, mirroring the
// authoritative-source-of-truth role TestAdjacentTierCeilingMatrix plays
// for the pre-existing bonus set (see project_context.md §5): every
// family's lowest realistic combination of the new attributes, alone and
// combined, must still leave a clean candidate one tier higher on top.

import (
	"fmt"
	"testing"
	"time"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

// pinMoveFamily describes one production tier ladder for the pin-move
// attribute matrix, mirroring adjacent_tier_ceiling_test.go's family shape.
// Kept separate (not shared) because the two tests decorate independently:
// this one must never perturb the codec-upgrade/decoration combination the
// original ceiling matrix already proves.
type pinMoveFamily struct {
	label         string
	kind          string
	definePrefix  string
	tierCount     int
	sourceTokens  []string
	ordinaryCodec string
	// fullStack is the same realistic maximal decoration set
	// adjacent_tier_ceiling_test.go uses for this family (Dual Audio,
	// REPACK3, TrueHD/Atmos, editions, etc.) — reused read-only here to
	// build a "realistic maximal stack + new attribute" case.
	fullStack []string
	build     func(source []string, group string, extras []string) string
}

func pinMoveFamilies() []pinMoveFamily {
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
	return []pinMoveFamily{
		{"Movie Remux", ranking.KindMovie, "Movies Remux", 3,
			[]string{"2160p", "UHD", "BluRay", "REMUX"}, "HEVC",
			[]string{"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "TrueHD", "Atmos", "7.1"}, movieTitle},
		{"Movie UHD BluRay", ranking.KindMovie, "Movies UHD BluRay", 3,
			[]string{"2160p", "UHD", "BluRay"}, "HEVC",
			[]string{"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "TrueHD", "Atmos", "7.1"}, movieTitle},
		{"Movie HD BluRay", ranking.KindMovie, "Movies HD BluRay", 3,
			[]string{"1080p", "BluRay"}, "x264",
			[]string{"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "TrueHD", "Atmos", "7.1"}, movieTitle},
		{"Movie WEB", ranking.KindMovie, "Movies WEB", 3,
			[]string{"2160p", "WEB-DL"}, "HEVC",
			[]string{"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3", "DDP5.1", "Atmos"}, movieTitle},
		{"Show Remux", ranking.KindSeries, "Shows Remux", 2,
			[]string{"2160p", "UHD", "BluRay", "REMUX"}, "HEVC",
			[]string{"S01.COMPLETE", "Dual.Audio", "REPACK3", "TrueHD", "Atmos", "7.1"}, seriesTitle},
		{"Show BluRay", ranking.KindSeries, "Shows BluRay", 2,
			[]string{"1080p", "BluRay"}, "x264",
			[]string{"S01.COMPLETE", "Dual.Audio", "REPACK3", "TrueHD", "Atmos", "7.1"}, seriesTitle},
		{"Show WEB", ranking.KindSeries, "Shows WEB", 3,
			[]string{"2160p", "WEB-DL"}, "HEVC",
			[]string{"S01.COMPLETE", "Dual.Audio", "REPACK3", "DDP5.1", "Atmos"}, seriesTitle},
		{"Anime Show BluRay", ranking.KindAnimeShow, "Anime Shows BluRay", 8,
			[]string{"1080p", "BluRay"}, "x264",
			[]string{"S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "TrueHD", "Atmos", "7.1"}, animeShowTitle},
		{"Anime Show WEB", ranking.KindAnimeShow, "Anime Shows WEB", 6,
			[]string{"1080p", "WEB-DL"}, "x264",
			[]string{"CR", "S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "DDP5.1", "Atmos"}, animeShowTitle},
		{"Anime Movie BluRay", ranking.KindAnimeMovie, "Anime Movies BluRay", 8,
			[]string{"1080p", "BluRay"}, "x264",
			[]string{"Dual", "Audio", "Uncensored", "v4", "REPACK3", "TrueHD", "Atmos", "7.1"}, animeMovieTitle},
		{"Anime Movie WEB", ranking.KindAnimeMovie, "Anime Movies WEB", 6,
			[]string{"1080p", "WEB-DL"}, "x264",
			[]string{"CR", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "DDP5.1", "Atmos"}, animeMovieTitle},
	}
}

func pinMoveScore(t *testing.T, profile *ranking.Profile, kind, title string, avail *triage.AvailState) (int, bool) {
	t.Helper()
	candidate := triage.Candidate{Release: &release.Release{Title: title}}
	if avail != nil {
		candidate.Verdict.Avail = *avail
	}
	request := ranking.Request{Kind: kind, Title: "Example"}
	if kind == ranking.KindSeries || kind == ranking.KindAnimeShow {
		request.Season = 1
		request.Episode = 1
	}
	if kind == ranking.KindAnimeShow || kind == ranking.KindAnimeMovie {
		request.IsAnime = true
	}
	kept, rejected := profile.ApplyWithRejected(request, []triage.Candidate{candidate}, jhinrank.RankOptions{})
	if len(rejected) != 0 || len(kept) != 1 {
		return 0, false
	}
	return kept[0].Torrent.Rank, true
}

// TestNewNativeAttributeNeutralizerContract is the focused, isolated proof
// that each of the four pin-move neutralizers fully cancels its native
// score to exactly zero net contribution, for both a non-Anime and an
// Anime kind — universally, as their "no content-kind restriction"
// production rule shape requires (see project_context.md §7's
// native-score-compensation principle). It is deliberately independent of
// tier/group/decoration context: adding the attribute token to an
// otherwise-identical baseline title must never change the score.
func TestNewNativeAttributeNeutralizerContract(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "contract", Preset: "4k", Rules: productionRules},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	cases := []struct {
		attrLabel string
		token     string
	}{
		{"HLG", "HLG"},
		{"DTS:X", "DTS-X"},
		{"VC-1", "VC-1"},
		{"DTS-ES", "DTS-ES"},
	}

	kinds := []struct {
		label string
		kind  string
	}{
		{"Movie", ranking.KindMovie},
		{"Anime Movie", ranking.KindAnimeMovie},
	}

	for _, c := range cases {
		c := c
		for _, k := range kinds {
			k := k
			t.Run(fmt.Sprintf("%s/%s", c.attrLabel, k.label), func(t *testing.T) {
				baseline := "Example.Movie.2026.1080p.BluRay.HEVC.AAC2.0-Group1"
				if k.kind == ranking.KindAnimeMovie {
					baseline = "Example.Anime.Movie.2025.1080p.BluRay.HEVC.AAC2.0-Group1"
				}
				decorated := baseline[:len(baseline)-len("-Group1")] + "." + c.token + "-Group1"

				baseScore, ok := pinMoveScore(t, profile, k.kind, baseline, nil)
				if !ok {
					t.Fatalf("baseline rejected/ambiguous: %s", baseline)
				}
				decoratedScore, ok := pinMoveScore(t, profile, k.kind, decorated, nil)
				if !ok {
					t.Fatalf("decorated rejected/ambiguous: %s", decorated)
				}

				if decoratedScore != baseScore {
					t.Errorf(
						"%s (%s) not fully neutralized: baseline=%d decorated=%d delta=%+d\n"+
							"  baseline:  %s\n  decorated: %s",
						c.attrLabel, k.label, baseScore, decoratedScore, decoratedScore-baseScore,
						baseline, decorated,
					)
				}
			})
		}
	}
}

// TestPinMoveAdjacentTierMatrix proves, per new native attribute, that a
// lower-tier candidate decorated with that attribute alone, and decorated
// with the family's full realistic bonus stack plus that attribute, both
// still score below a clean candidate one tier higher — for every
// production tier family and every adjacent tier pair. Does not modify or
// reuse TestAdjacentTierCeilingMatrix's own decoration lists, so it can
// never perturb that test's already-proven codec-upgrade scenario.
func TestPinMoveAdjacentTierMatrix(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "pin-move-matrix", Preset: "4k", Rules: productionRules},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}
	defines := loadCeilingDefines(t)
	tok := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}
	fullAvail := triage.AvailState{Status: triage.AvailAvailable, OnMyBackbone: true, CheckedAt: time.Now().Add(-3 * 24 * time.Hour)}

	attrs := []struct {
		label string
		toks  []string
	}{
		{"HLG", []string{"HLG"}},
		{"DTS:X", []string{"DTS-X"}},
		{"VC-1", []string{"VC-1"}},
		{"DTS-ES", []string{"DTS-ES"}},
	}

	for _, f := range pinMoveFamilies() {
		f := f
		t.Run(f.label, func(t *testing.T) {
			for tier := 2; tier <= f.tierCount; tier++ {
				lowerTier, higherTier := tier, tier-1
				lowerGroup := tok(fmt.Sprintf("%s T%d Groups", f.definePrefix, lowerTier))
				higherGroup := tok(fmt.Sprintf("%s T%d Groups", f.definePrefix, higherTier))

				higherTitle := f.build(append(append([]string{}, f.sourceTokens...), f.ordinaryCodec), higherGroup, nil)
				higherScore, ok := pinMoveScore(t, profile, f.kind, higherTitle, nil)
				if !ok {
					t.Fatalf("clean T%d baseline rejected/ambiguous: %s", higherTier, higherTitle)
				}

				for _, a := range attrs {
					lowerSource := append(append([]string{}, f.sourceTokens...), f.ordinaryCodec)

					for _, mode := range []string{"single-attr", "full-stack"} {
						var extras []string
						if mode == "single-attr" {
							extras = append([]string{}, a.toks...)
						} else {
							extras = append(append([]string{}, a.toks...), f.fullStack...)
						}
						lowerTitle := f.build(lowerSource, lowerGroup, extras)
						lowerScore, ok := pinMoveScore(t, profile, f.kind, lowerTitle, &fullAvail)
						if !ok {
							continue
						}
						if lowerScore >= higherScore {
							t.Errorf(
								"%s T%d [%s/%s] (%d) does not stay below clean T%d (%d); margin=%+d\n"+
									"  decorated: %s\n  clean:     %s",
								f.label, lowerTier, a.label, mode, lowerScore, higherTier, higherScore,
								lowerScore-higherScore, lowerTitle, higherTitle,
							)
						}
					}
				}
			}
		})
	}
}

// TestPinMoveCombinedAttributeStress proves the same tier-ceiling property
// holds for realistic combinations of the new attributes together, since
// the pin move introduces all four at once, never in isolation.
func TestPinMoveCombinedAttributeStress(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "pin-move-combined", Preset: "4k", Rules: productionRules},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}
	defines := loadCeilingDefines(t)
	tok := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}
	fullAvail := triage.AvailState{Status: triage.AvailAvailable, OnMyBackbone: true, CheckedAt: time.Now().Add(-3 * 24 * time.Hour)}

	combos := []struct {
		label string
		toks  []string
	}{
		{"HLG+VC-1", []string{"HLG", "VC-1"}},
		{"HLG+DTS-ES", []string{"HLG", "DTS-ES"}},
		{"DTSX+VC-1", []string{"DTS-X", "VC-1"}},
		{"DTSX+DTS-ES", []string{"DTS-X", "DTS-ES"}},
		{"HLG+DTSX+VC-1", []string{"HLG", "DTS-X", "VC-1"}},
		{"HLG+DTSX+DTS-ES", []string{"HLG", "DTS-X", "DTS-ES"}},
		{"VC-1+DTS-ES", []string{"VC-1", "DTS-ES"}},
		{"ALL4", []string{"HLG", "DTS-X", "VC-1", "DTS-ES"}},
	}

	for _, f := range pinMoveFamilies() {
		f := f
		t.Run(f.label, func(t *testing.T) {
			for tier := 2; tier <= f.tierCount; tier++ {
				lowerTier, higherTier := tier, tier-1
				lowerGroup := tok(fmt.Sprintf("%s T%d Groups", f.definePrefix, lowerTier))
				higherGroup := tok(fmt.Sprintf("%s T%d Groups", f.definePrefix, higherTier))

				higherTitle := f.build(append(append([]string{}, f.sourceTokens...), f.ordinaryCodec), higherGroup, nil)
				higherScore, ok := pinMoveScore(t, profile, f.kind, higherTitle, nil)
				if !ok {
					t.Fatalf("clean T%d baseline rejected/ambiguous: %s", higherTier, higherTitle)
				}

				for _, c := range combos {
					lowerSource := append(append([]string{}, f.sourceTokens...), f.ordinaryCodec)
					extras := append(append([]string{}, c.toks...), f.fullStack...)
					lowerTitle := f.build(lowerSource, lowerGroup, extras)
					lowerScore, ok := pinMoveScore(t, profile, f.kind, lowerTitle, &fullAvail)
					if !ok {
						continue
					}
					if lowerScore >= higherScore {
						t.Errorf(
							"%s T%d [%s] (%d) does not stay below clean T%d (%d); margin=%+d\n"+
								"  decorated: %s\n  clean:     %s",
							f.label, lowerTier, c.label, lowerScore, higherTier, higherScore,
							lowerScore-higherScore, lowerTitle, higherTitle,
						)
					}
				}
			}
		})
	}
}

// TestPinMoveAnimeNeutrality proves the four new neutralizers introduce no
// new Anime codec/audio preference: Anime score-neutral policy (see
// project_context.md §7) requires their net contribution be exactly zero
// for Anime, identical to non-Anime, not merely "not positive."
func TestPinMoveAnimeNeutrality(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "pin-move-anime-neutral", Preset: "4k", Rules: productionRules},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	attrs := []string{"HLG", "DTS-X", "VC-1", "DTS-ES"}

	for _, attr := range attrs {
		attr := attr
		t.Run(attr, func(t *testing.T) {
			baseline := "Example.Anime.Movie.2025.1080p.BluRay.HEVC.AAC2.0-Group1"
			decorated := "Example.Anime.Movie.2025.1080p.BluRay.HEVC.AAC2.0." + attr + "-Group1"

			baseScore, ok := pinMoveScore(t, profile, ranking.KindAnimeMovie, baseline, nil)
			if !ok {
				t.Fatalf("baseline rejected/ambiguous: %s", baseline)
			}
			decoratedScore, ok := pinMoveScore(t, profile, ranking.KindAnimeMovie, decorated, nil)
			if !ok {
				t.Fatalf("decorated rejected/ambiguous: %s", decorated)
			}

			if decoratedScore != baseScore {
				t.Errorf(
					"Anime %s introduces a new preference: baseline=%d decorated=%d delta=%+d",
					attr, baseScore, decoratedScore, decoratedScore-baseScore,
				)
			}
		})
	}
}
