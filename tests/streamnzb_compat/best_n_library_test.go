package streamnzb_compat

import (
	"fmt"
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

// bestNLibraryProfile compiles the exact production "Best 3 per R/Q",
// "Best 1 Library per R/Q" and "Best 1 Season Pack per R/Q" rules (as
// published, decoded from profile.txt) plus any additionally named
// production rules, against the real Define library. Production-regression
// fidelity: drift between what is tested here and what ships fails CI, per
// CLAUDE.md's two-layer validation philosophy.
func bestNLibraryProfile(t *testing.T, extraRuleNames ...string) *ranking.Profile {
	t.Helper()

	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	names := append([]string{
		"Best 3 per R/Q",
		"Best 1 Library per R/Q",
		"Best 1 Season Pack per R/Q",
	}, extraRuleNames...)

	selected := make([]config.RuleConfig, 0, len(names))
	for _, name := range names {
		selected = append(selected, findProductionRule(t, productionRules, name))
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Best-N Library reservation production regression",
			Preset: "4k",
			Rules:  selected,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production Best-N/Library rule subset: %v", err)
	}
	return profile
}

// bnTitle pairs a release title with an explicit Library flag, rather than
// inferring Library status from the title text (which carries no such
// signal in real production -- Library comes from `release.Release.IsLibrary`
// only, set here directly).
type bnTitle struct {
	title   string
	library bool
}

func lib(title string) bnTitle    { return bnTitle{title: title, library: true} }
func notLib(title string) bnTitle { return bnTitle{title: title, library: false} }

func bnCandidates(ts []bnTitle) []triage.Candidate {
	out := make([]triage.Candidate, 0, len(ts))
	for _, bt := range ts {
		out = append(out, triage.Candidate{
			Release: &release.Release{Title: bt.title, IsLibrary: bt.library},
		})
	}
	return out
}

type bnOutcome struct {
	kept     map[string]bool
	rejected map[string][]string
}

func bnApply(t *testing.T, profile *ranking.Profile, req ranking.Request, ts ...bnTitle) bnOutcome {
	t.Helper()
	kept, rejected := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})
	out := bnOutcome{kept: map[string]bool{}, rejected: map[string][]string{}}
	for _, r := range kept {
		out.kept[r.Candidate.Release.Title] = true
	}
	for _, r := range rejected {
		out.rejected[r.Candidate.Release.Title] = r.Torrent.Rejections
	}
	return out
}

func bnRejectedBy(o bnOutcome, title, ruleName string) bool {
	for _, reason := range o.rejected[title] {
		if strings.Contains(reason, ruleName) {
			return true
		}
	}
	return false
}

// strongMovie/libraryMovie build 2160p WEB-DL movie titles. strongMovie uses
// a real Movies WEB T1 tier group ("ABBIE") to earn the production +500
// "Movies WEB T1" native-policy score, so it genuinely and realistically
// outranks an untiered candidate -- no synthetic score rules.
func strongMovie(tag string) string {
	return fmt.Sprintf("Example.Movie.2020.2160p.WEB-DL.%s-ABBIE", tag)
}
func libraryMovie(tag string) string {
	return fmt.Sprintf("Example.Movie.2020.2160p.WEB-DL.%s-LIBGRP", tag)
}

// 1 + 3: canonical eviction case fixed, and proof the true bucket maximum
// is 4 (3 ordinary + 1 Library), with Library occupying its own dedicated
// slot rather than one of the ordinary Best-3 slots.
func TestLibraryReservation_CanonicalEvictionFixedAndMaxFour(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	libTitle := libraryMovie("A")
	ordinary := []string{strongMovie("B"), strongMovie("C"), strongMovie("D"), strongMovie("E")}

	ts := []bnTitle{lib(libTitle)}
	for _, title := range ordinary {
		ts = append(ts, notLib(title))
	}

	o := bnApply(t, profile, req, ts...)

	if !o.kept[libTitle] {
		t.Fatalf("Library candidate must survive: rejected=%v", o.rejected[libTitle])
	}

	keptOrdinary := 0
	rejectedOrdinary := 0
	for _, title := range ordinary {
		if o.kept[title] {
			keptOrdinary++
		} else if bnRejectedBy(o, title, "Best 3 per R/Q") {
			rejectedOrdinary++
		}
	}

	if keptOrdinary != 3 {
		t.Errorf("expected exactly 3 ordinary candidates kept (Library must not consume an ordinary slot), got %d", keptOrdinary)
	}
	if rejectedOrdinary != 1 {
		t.Errorf("expected exactly 1 ordinary candidate rejected by the Best-3 cap, got %d", rejectedOrdinary)
	}

	total := keptOrdinary
	if o.kept[libTitle] {
		total++
	}
	if total != 4 {
		t.Errorf("expected a total bucket maximum of 4 (3 ordinary + 1 Library), got %d", total)
	}
}

// 2: with no Library candidate present, ordinary behavior is unchanged --
// still exactly Best 3.
func TestLibraryReservation_NoLibraryCandidate_UnchangedBest3(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	titles := []string{strongMovie("A"), strongMovie("B"), strongMovie("C"), strongMovie("D")}
	var ts []bnTitle
	for _, title := range titles {
		ts = append(ts, notLib(title))
	}
	o := bnApply(t, profile, req, ts...)

	kept := 0
	rejected := 0
	for _, title := range titles {
		if o.kept[title] {
			kept++
		} else if bnRejectedBy(o, title, "Best 3 per R/Q") {
			rejected++
		}
	}
	if kept != 3 {
		t.Errorf("expected exactly 3 kept with no Library candidate present, got %d", kept)
	}
	if rejected != 1 {
		t.Errorf("expected exactly 1 rejected by the ordinary cap, got %d", rejected)
	}
}

// 4: multiple Library candidates remain bounded to the highest-ranked 1.
func TestLibraryReservation_MultipleLibraryCandidates_HighestRankedSurvives(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	// Both Library, but one carries a real production PROPER marker (real
	// "Repack/Proper Preference" scoring, not a synthetic rule) so it
	// genuinely outranks the plain Library candidate.
	strongerLib := "Example.Movie.2020.2160p.WEB-DL.PROPER.A-LIBGRP"
	weakerLib := "Example.Movie.2020.2160p.WEB-DL.B-LIBGRP2"

	o := bnApply(t, profile, req, lib(strongerLib), lib(weakerLib))

	if !o.kept[strongerLib] {
		t.Errorf("expected the higher-ranked Library candidate to survive, rejected=%v", o.rejected[strongerLib])
	}
	if o.kept[weakerLib] {
		t.Error("expected the lower-ranked Library candidate to lose the single Library slot")
	}
	if !bnRejectedBy(o, weakerLib, "Best 1 Library per R/Q") {
		t.Errorf("expected the lower-ranked Library candidate to be rejected specifically by the Library cap, got %v", o.rejected[weakerLib])
	}
}

// 5: a Library candidate ranking far below the ordinary candidates on real
// production scoring still survives -- an intentional reservation policy,
// not quality/rank-gated.
func TestLibraryReservation_WeakLibraryCandidateStillSurvives(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	libTitle := "Example.Movie.2020.2160p.WEB-DL.A-LIBGRP" // untiered, real native score only
	strong := []string{strongMovie("B"), strongMovie("C"), strongMovie("D")}

	ts := []bnTitle{lib(libTitle)}
	for _, title := range strong {
		ts = append(ts, notLib(title))
	}

	o := bnApply(t, profile, req, ts...)

	if !o.kept[libTitle] {
		t.Errorf("expected the Library candidate to survive even though it ranks far below the tier-boosted ordinary candidates, rejected=%v", o.rejected[libTitle])
	}
	for _, title := range strong {
		if !o.kept[title] {
			t.Errorf("expected ordinary candidate %s to survive (only 3 present, all should fit)", title)
		}
	}
}

// 6: Library Series/Anime season packs are not given a separate Library
// reservation and continue through the existing, unmodified Season Pack
// cap -- which stays authoritative on its own ranking, not on Library
// status. Proven two ways: structurally, "Best 1 Library per R/Q" excludes
// season packs entirely (a Library season pack never matches it, so it
// cannot draw on the Library slot); behaviorally, two Library season packs
// still only yield one survivor -- exactly the pre-existing single
// season-pack ceiling, with no additional Library-granted slot.
func TestLibraryReservation_SeasonPackHasNoDedicatedLibraryProtection(t *testing.T) {
	productionRules := loadProductionRules(t)
	library := findProductionRule(t, productionRules, "Best 1 Library per R/Q")
	wantExclusion := `not ((kind == "series" or kind == "anime_show") and seasonPack)`
	if !strings.Contains(library.When, wantExclusion) {
		t.Fatalf("Best 1 Library per R/Q must exclude season packs with the exact clause %q, when=%q", wantExclusion, library.When)
	}

	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 1, Title: "Example Show"}

	firstLibraryPack := "Example.Show.S01.COMPLETE.2160p.WEB-DL.A-LIBGRP"
	secondLibraryPack := "Example.Show.S01.COMPLETE.2160p.WEB-DL.B-LIBGRP2"

	o := bnApply(t, profile, req, lib(firstLibraryPack), lib(secondLibraryPack))

	survivors := 0
	for _, title := range []string{firstLibraryPack, secondLibraryPack} {
		if o.kept[title] {
			survivors++
		}
	}
	if survivors != 1 {
		t.Errorf("expected exactly 1 of 2 competing Library season packs to survive (the existing single season-pack ceiling, no extra Library slot for packs), got %d", survivors)
	}
	if !bnRejectedBy(o, firstLibraryPack, "Best 1 Season Pack per R/Q") && !bnRejectedBy(o, secondLibraryPack, "Best 1 Season Pack per R/Q") {
		t.Error("expected the losing Library season pack to be rejected specifically by the existing, unmodified season-pack cap")
	}
}

// 7: existing SeaDex limit behavior is unaffected by the Library
// reservation -- no extra SeaDex-specific reservation is introduced, and a
// Library+SeaDex-Best candidate coexists cleanly with the separate,
// unmodified global SeaDex cap.
func TestLibraryReservation_SeaDexCapUnaffected(t *testing.T) {
	profile := bestNLibraryProfile(t, "At most 1 SeaDex Best")
	req := ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{Known: true, Best: map[string]bool{
			"libgrp": true, "othergrp": true,
		}},
	}

	libAndSeadexBest := "Example.Anime.S01E01.2160p.WEB-DL.A-LIBGRP"
	otherSeadexBest := "Example.Anime.S01E01.2160p.WEB-DL.B-OTHERGRP"

	o := bnApply(t, profile, req, lib(libAndSeadexBest), notLib(otherSeadexBest))

	if !o.kept[libAndSeadexBest] {
		t.Errorf("expected the Library+SeaDex-Best candidate to survive via its own Library slot, rejected=%v", o.rejected[libAndSeadexBest])
	}
	if o.kept[otherSeadexBest] {
		t.Error("expected the second SeaDex-Best candidate to still be rejected by the existing, unmodified global SeaDex-Best cap")
	}
	if !bnRejectedBy(o, otherSeadexBest, "At most 1 SeaDex Best") {
		t.Errorf("expected rejection specifically by the existing SeaDex cap, got %v", o.rejected[otherSeadexBest])
	}
}

// 8 + 9: a Library candidate carrying an existing negative classification
// (a real Vidhin-backed LQ group) is never pruned by Adaptive Low-Score
// Filtering, exactly as before the Best-N reservation -- the reservation
// does not bypass, weaken or interact with earlier reject/prune stages,
// and the adaptive-filter exemption is unchanged.
func TestLibraryReservation_NegativeClassificationAndAdaptiveFilterExemptionUnchanged(t *testing.T) {
	// "Movies LQ Penalty" (-10000, real production scoring) is included so
	// the EVO candidate's own finalScore is realistically deeply negative --
	// without it, six untiered alternatives never clear Adaptive Low-Score
	// Filtering's ">= current.finalScore + 5000" gap at all, and the
	// exemption assertion below would pass for the wrong reason (a
	// condition that never fires) rather than the `not library` guard.
	profile := bestNLibraryProfile(t, "Movies LQ Penalty", "Adaptive Low-Score Filtering")
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	// "EVO" is a real Movies LQ Groups member (Vidhin-backed), so it also
	// draws the real -10000 "Movies LQ Penalty". Six untiered alternatives
	// (finalScore 0) then comfortably clear Adaptive Low-Score Filtering's
	// "count(finalScore >= current.finalScore + 5000) >= 6" condition.
	libraryLQ := "Example.Movie.2020.2160p.WEB-DL.X264-EVO"

	ts := []bnTitle{lib(libraryLQ)}
	for i := 0; i < 6; i++ {
		ts = append(ts, notLib(strongMovie(fmt.Sprintf("S%d", i))))
	}

	o := bnApply(t, profile, req, ts...)

	if !o.kept[libraryLQ] {
		t.Errorf("expected the Library LQ-group candidate to remain exempt from Adaptive Low-Score Filtering, rejected=%v", o.rejected[libraryLQ])
	}
	if bnRejectedBy(o, libraryLQ, "Adaptive Low-Score Filtering") {
		t.Error("Library candidate must never be pruned by Adaptive Low-Score Filtering")
	}

	// Control: the identical non-Library candidate, same alternatives, must
	// actually be pruned -- proving the survival above is the `not library`
	// exemption at work, not a condition that simply never fires.
	controlTS := []bnTitle{notLib(libraryLQ)}
	for i := 0; i < 6; i++ {
		controlTS = append(controlTS, notLib(strongMovie(fmt.Sprintf("S%d", i))))
	}
	controlO := bnApply(t, profile, req, controlTS...)

	if controlO.kept[libraryLQ] {
		t.Error("control: expected the non-Library LQ-group candidate to be pruned by Adaptive Low-Score Filtering (if it survives too, the Library assertion above proves nothing)")
	}
	if !bnRejectedBy(controlO, libraryLQ, "Adaptive Low-Score Filtering") {
		t.Errorf("control: expected the non-Library LQ-group candidate to be rejected specifically by Adaptive Low-Score Filtering, got %v", controlO.rejected[libraryLQ])
	}
}

// 10: double-match control -- the ordinary rule textually and behaviorally
// excludes Library candidates, so a Library candidate cannot consume both
// ordinary and Library capacity. Structural (decoded production text) and
// behavioral (real-engine) proof together.
func TestLibraryReservation_DoubleMatchControl(t *testing.T) {
	productionRules := loadProductionRules(t)

	ordinary := findProductionRule(t, productionRules, "Best 3 per R/Q")
	library := findProductionRule(t, productionRules, "Best 1 Library per R/Q")

	if !strings.HasPrefix(ordinary.When, "not library") {
		t.Fatalf("Best 3 per R/Q must exclude Library candidates, when=%q", ordinary.When)
	}
	if strings.HasPrefix(library.When, "not library") {
		t.Fatalf("Best 1 Library per R/Q must match Library candidates, not exclude them, when=%q", library.When)
	}
	if !strings.HasPrefix(library.When, "library") {
		t.Fatalf("Best 1 Library per R/Q must require library, when=%q", library.When)
	}
	if strings.Contains(strings.ToLower(library.When), "seadex") {
		t.Fatalf("Best 1 Library per R/Q must not reference SeaDex, when=%q", library.When)
	}
	if ordinary.GroupBy != library.GroupBy {
		t.Fatalf("Best 3 per R/Q and Best 1 Library per R/Q must share the same resolution+quality grouping")
	}

	// Behavioral proof: a lone Library candidate plus exactly 3 ordinary
	// candidates must retain all 4 -- if Library could also consume an
	// ordinary slot, one of the 3 ordinary candidates would be evicted
	// instead (a bucket of 4 competing for a shared cap of 3 would reject
	// one).
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}
	titles := []string{libraryMovie("A"), strongMovie("B"), strongMovie("C"), strongMovie("D")}
	o := bnApply(t, profile, req, lib(titles[0]), notLib(titles[1]), notLib(titles[2]), notLib(titles[3]))

	for _, title := range titles {
		if !o.kept[title] {
			t.Errorf("expected all 4 candidates (Library does not compete for an ordinary slot) to survive, but %s was rejected: %v", title, o.rejected[title])
		}
	}
}

// ---------------------------------------------------------------------------
// Best-N per-resolution ceiling audit (2026-09-20, backlog-roadmap.md "Best-N
// per-resolution ceiling audit") -- permanent regression coverage.
//
// The audit found "Best 3 per R/Q"/"Best 1 Library per R/Q"/"Best 1 Season
// Pack per R/Q" all group by `resolution + " " + quality`, and the pinned
// Jhin parser (parser/table.go, v0.8.0) resolves `quality` to one of ~28
// distinct literal strings (BluRay REMUX, BluRay, WEB-DL, WEBRip, BDRip,
// HDTV, ...), each its own independent bucket at a given resolution -- so a
// single resolution's ordinary survivor total multiplies with the number of
// populated quality variants rather than being capped in aggregate. The
// audit concluded this is bounded and ordering-safe (no policy change --
// see backlog-roadmap.md), specifically because ApplyWithRejected
// (pkg/search/ranking/service.go) sorts kept results by final score
// *before* applying caps, so a weak-quality-bucket survivor is always
// ordered below stronger candidates, never displacing them. These tests
// lock that audited-and-accepted behavior in permanently, against the exact
// published rules (via bestNLibraryProfile, same production-decode pattern
// as the rest of this file).

// bnQualityToken pairs a release-name token with the exact `quality` string
// the pinned Jhin parser (parser/table.go) resolves it to.
type bnQualityToken struct {
	token   string
	quality string
}

// bnQualityBuckets are six realistic, mutually distinct 1080p quality
// variants -- the same set the audit's real-engine probe used -- spanning
// the native Jhin quality-attr score range (rank/profile.go, pinned "4k"
// preset): BluRay REMUX=10000, WEB-DL=200, BluRay=100, WEBRip=-1000,
// HDTV=-5000. None of DraCuLa's own rules score on `quality` directly
// (confirmed via the audit's grep of profiles/rules.json); only Jhin's
// native ranker differentiates these, and only within-bucket comparisons
// ever see that differentiation, because each quality string is its own
// group_by bucket.
var bnQualityBuckets = []bnQualityToken{
	{"BluRay.REMUX", "BluRay REMUX"},
	{"BluRay", "BluRay"},
	{"WEB-DL", "WEB-DL"},
	{"WEBRip", "WEBRip"},
	{"BDRip", "BDRip"},
	{"HDTV", "HDTV"},
}

func bnMovieQualityTitle(qualityToken, group string) string {
	return fmt.Sprintf("Example.Movie.2020.1080p.%s.X264-%s", qualityToken, group)
}
func bnSeriesQualityTitle(qualityToken, group string) string {
	return fmt.Sprintf("Example.Show.S02E04.1080p.%s.X264-%s", qualityToken, group)
}
func bnAnimeQualityTitle(qualityToken, group string) string {
	return fmt.Sprintf("Example.Anime.S01E01.1080p.%s.X264-%s", qualityToken, group)
}
func bnSeasonPackQualityTitle(qualityToken, group string) string {
	return fmt.Sprintf("Example.Show.S02.COMPLETE.1080p.%s.X264-%s", qualityToken, group)
}

// bnKeptBucketCounts groups kept results by their real parsed
// "resolution quality" env key (rules.BuildEnv), the exact key
// `group_by: resolution + " " + quality` evaluates against -- independent
// of test bookkeeping, so a bucketing regression in the rule itself would
// still be caught even if the release-name helpers above changed.
func bnKeptBucketCounts(t *testing.T, kept []ranking.Result, ctx rules.Context) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, r := range kept {
		env := rules.BuildEnv(r.Candidate, r.Torrent.Data, ctx)
		counts[env.Resolution+" "+env.Quality]++
	}
	return counts
}

// 11: at one resolution, ordinary survivors multiply with the number of
// populated quality buckets rather than being capped in aggregate --
// exactly 3 per bucket, 18 total across the 6 realistic buckets above, for
// Movie, Series episode and Anime Show alike ("Best 3 per R/Q" carries no
// content-kind condition beyond the season-pack exclusion, so the same
// multiplication applies uniformly).
func TestBestNPerResolutionQuality_MultiplyAcrossQualityBuckets(t *testing.T) {
	profile := bestNLibraryProfile(t)

	cases := []struct {
		name string
		req  ranking.Request
		mk   func(qualityToken, group string) string
	}{
		{
			name: "Movie",
			req:  ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
			mk:   bnMovieQualityTitle,
		},
		{
			name: "SeriesEpisode",
			req:  ranking.Request{Kind: ranking.KindSeries, Season: 2, Episode: 4, Title: "Example Show"},
			mk:   bnSeriesQualityTitle,
		},
		{
			name: "AnimeShow",
			req:  ranking.Request{Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime"},
			mk:   bnAnimeQualityTitle,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ts []bnTitle
			for _, b := range bnQualityBuckets {
				for i := 0; i < 5; i++ {
					ts = append(ts, notLib(tc.mk(b.token, fmt.Sprintf("G%d", i))))
				}
			}

			kept, _ := profile.ApplyWithRejected(tc.req, bnCandidates(ts), jhinrank.RankOptions{})

			if len(kept) != 18 {
				t.Errorf("total 1080p ordinary survivors = %d, want 18 (6 quality buckets x 3, no per-resolution cap)", len(kept))
			}

			ctx := rules.Context{Kind: tc.req.Kind, Season: tc.req.Season, Episode: tc.req.Episode, Title: tc.req.Title}
			counts := bnKeptBucketCounts(t, kept, ctx)
			if len(counts) != len(bnQualityBuckets) {
				t.Fatalf("expected survivors spread across all %d quality buckets, got buckets=%v", len(bnQualityBuckets), counts)
			}
			// Check each fixture's own declared `quality` against the real
			// parsed bucket key directly (not just the aggregate bucket
			// count) -- a remapping to a different but still-distinct
			// quality value would otherwise preserve every count here.
			for _, b := range bnQualityBuckets {
				key := "1080p " + b.quality
				if n := counts[key]; n != 3 {
					t.Errorf("bucket %q survivors = %d, want exactly 3", key, n)
				}
			}
		})
	}
}

// 12: Library reservation multiplies per bucket exactly like the ordinary
// cap -- each populated quality bucket gets its own independent +1 Library
// slot (4 total per bucket), not a single Library slot shared across the
// whole resolution.
func TestBestNPerResolutionQuality_LibraryReservationMultipliesPerBucket(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	var ts []bnTitle
	for _, b := range bnQualityBuckets {
		for i := 0; i < 5; i++ {
			ts = append(ts, notLib(bnMovieQualityTitle(b.token, fmt.Sprintf("G%d", i))))
		}
		ts = append(ts, lib(bnMovieQualityTitle(b.token, "LIBGRP")))
	}

	kept, _ := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})
	if len(kept) != 24 {
		t.Errorf("total 1080p survivors incl. Library = %d, want 24 (6 buckets x 4)", len(kept))
	}

	counts := bnKeptBucketCounts(t, kept, rules.Context{Kind: req.Kind, Title: req.Title})
	for _, b := range bnQualityBuckets {
		key := "1080p " + b.quality
		if n := counts[key]; n != 4 {
			t.Errorf("bucket %q survivors incl. Library = %d, want exactly 4 (3 ordinary + 1 Library)", key, n)
		}
	}
}

// 13: season-pack capacity is the same per-bucket-not-per-resolution shape
// -- 1 survivor per populated quality bucket, multiplying to 6 total across
// the 6 realistic buckets at one resolution, not a single pack slot shared
// resolution-wide.
func TestBestNPerResolutionQuality_SeasonPackCapacityPerBucket(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindSeries, Season: 2, Episode: 4, Title: "Example Show"}

	var ts []bnTitle
	for _, b := range bnQualityBuckets {
		for i := 0; i < 3; i++ {
			ts = append(ts, notLib(bnSeasonPackQualityTitle(b.token, fmt.Sprintf("G%d", i))))
		}
	}

	kept, _ := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})
	if len(kept) != len(bnQualityBuckets) {
		t.Errorf("total 1080p season-pack survivors = %d, want %d (1 per quality bucket)", len(kept), len(bnQualityBuckets))
	}

	counts := bnKeptBucketCounts(t, kept, rules.Context{Kind: req.Kind, Season: req.Season, Episode: req.Episode, Title: req.Title})
	for _, b := range bnQualityBuckets {
		key := "1080p " + b.quality
		if n := counts[key]; n != 1 {
			t.Errorf("bucket %q season-pack survivors = %d, want exactly 1", key, n)
		}
	}
}

// 14: SeaDex caps remain a genuine global cap with no group_by at all --
// the contrast case proving the multiplication above is specific to the
// group_by'd Best-N rules, not a universal DraCuLa pattern. Across 4
// distinct resolution+quality buckets at the same resolution, still only 1
// SeaDex Best candidate survives.
func TestBestNPerResolutionQuality_SeaDexBestGlobalCapAcrossBuckets(t *testing.T) {
	profile := bestNLibraryProfile(t, "At most 1 SeaDex Best")

	buckets := bnQualityBuckets[:4] // BluRay REMUX / BluRay / WEB-DL / WEBRip
	groups := []string{"SDX0", "SDX1", "SDX2", "SDX3"}
	seadexBest := map[string]bool{}
	for _, g := range groups {
		seadexBest[strings.ToLower(g)] = true
	}

	req := ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{Known: true, Best: seadexBest},
	}

	var ts []bnTitle
	for i, b := range buckets {
		ts = append(ts, notLib(bnAnimeQualityTitle(b.token, groups[i])))
	}

	kept, rejected := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})

	// Verify the 4 offered candidates genuinely parse into 4 distinct
	// resolution+quality buckets before trusting the global-cap collapse
	// below -- otherwise a broken group_by that accidentally collapsed them
	// into fewer buckets could produce the same "1 survivor" result for the
	// wrong reason (an ordinary per-bucket cap, not a real global one).
	ctx := rules.Context{Kind: req.Kind, Season: req.Season, Episode: req.Episode, Title: req.Title}
	offeredBuckets := map[string]int{}
	for _, r := range append(append([]ranking.Result{}, kept...), rejected...) {
		env := rules.BuildEnv(r.Candidate, r.Torrent.Data, ctx)
		offeredBuckets[env.Resolution+" "+env.Quality]++
	}
	for _, b := range buckets {
		key := "1080p " + b.quality
		if offeredBuckets[key] != 1 {
			t.Fatalf("test setup invalid: expected exactly 1 offered candidate in bucket %q, got %d (buckets seen=%v)", key, offeredBuckets[key], offeredBuckets)
		}
	}
	if len(offeredBuckets) != len(buckets) {
		t.Fatalf("test setup invalid: expected exactly %d distinct resolution+quality buckets among the offered candidates, got %v", len(buckets), offeredBuckets)
	}

	if len(kept) != 1 {
		t.Errorf("SeaDex Best survivors across %d distinct resolution/quality buckets = %d, want 1 (unconditional global cap, no group_by)", len(buckets), len(kept))
	}
}

// 15: the final kept-result order is global score order -- ApplyWithRejected
// sorts kept results by final score before caps run (see
// pkg/search/ranking/service.go's own doc comment: "Score is the only
// ordering currency there is"), so a weak-quality-bucket survivor never
// displaces a stronger one, it is simply ordered below it. Native Jhin
// quality-attr scoring (rank/profile.go) gives BluRay REMUX a native score
// of 10000 against HDTV's -5000, so every REMUX survivor must sort strictly
// ahead of the lone HDTV survivor despite both surviving the same
// resolution's Best-3 cap independently.
func TestBestNPerResolutionQuality_GlobalScoreOrderNeverDisplacedByWeakBucket(t *testing.T) {
	profile := bestNLibraryProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	var ts []bnTitle
	for i := 0; i < 5; i++ {
		ts = append(ts, notLib(bnMovieQualityTitle("BluRay.REMUX", fmt.Sprintf("REMUXG%d", i))))
	}
	hdtvTitle := bnMovieQualityTitle("HDTV", "HDTVGRP")
	ts = append(ts, notLib(hdtvTitle))

	kept, _ := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})

	if len(kept) != 4 {
		t.Fatalf("expected 4 total survivors (3 REMUX + 1 HDTV, independent buckets), got %d", len(kept))
	}

	// Monotonic non-increasing final score across the whole returned order
	// -- the general "global order" property, not just this scenario's
	// specific shape.
	for i := 1; i < len(kept); i++ {
		if kept[i].Torrent.Rank > kept[i-1].Torrent.Rank {
			t.Errorf("kept[%d].Torrent.Rank (%d) > kept[%d].Torrent.Rank (%d): kept results must be in non-increasing global score order",
				i, kept[i].Torrent.Rank, i-1, kept[i-1].Torrent.Rank)
		}
	}

	// The lone HDTV survivor must be present (not evicted by REMUX
	// abundance) but must sort strictly behind every REMUX survivor --
	// never displacing, only trailing.
	hdtvIndex := -1
	for i, r := range kept {
		if r.Candidate.Release.Title == hdtvTitle {
			hdtvIndex = i
		}
	}
	if hdtvIndex == -1 {
		t.Fatalf("expected the lone HDTV candidate to survive despite 5 REMUX alternatives at the same resolution (buckets do not compete), but it did not survive")
	}
	if hdtvIndex != len(kept)-1 {
		t.Errorf("expected the HDTV survivor to sort last (weakest global score) in the returned kept order, got index %d of %d", hdtvIndex, len(kept))
	}
	for i := 0; i < hdtvIndex; i++ {
		if !strings.Contains(kept[i].Candidate.Release.Title, "REMUXG") {
			t.Errorf("expected every candidate ahead of the HDTV survivor to be a REMUX candidate, got %q at index %d", kept[i].Candidate.Release.Title, i)
		}
		// Non-increasing order alone permits an equal-rank regression that
		// still happens to keep HDTV last; require the real native score
		// gap (REMUX 10000 vs HDTV -5000) to hold strictly.
		if kept[i].Torrent.Rank <= kept[hdtvIndex].Torrent.Rank {
			t.Errorf("REMUX rank %d must be strictly greater than HDTV rank %d", kept[i].Torrent.Rank, kept[hdtvIndex].Torrent.Rank)
		}
	}
}

// ---------------------------------------------------------------------------
// Library / SeaDex / season-pack retention overlap audit (2026-09-23,
// backlog-roadmap.md "Library / SeaDex / season-pack retention overlap
// audit") -- permanent regression coverage.
//
// The audit found the three per-resolution+quality caps ("Best 3 per R/Q",
// "Best 1 Library per R/Q", "Best 1 Season Pack per R/Q") are mutually
// exclusive by construction (each `when` clause excludes the other two
// classes), but SeaDex's global caps ("At most 1 SeaDex Best"/"Alternative")
// carry no such exclusion at all -- a release can and does belong to one of
// the three per-bucket classes *and* the global SeaDex cap simultaneously,
// by design (applyCaps, pkg/search/ranking/service.go, checks every
// LimitMatch a result carries; surviving requires room in all of them). The
// audit's own real-engine probe (scratch, deleted after) confirmed this
// holds for season packs (not previously exercised) exactly as it already
// did for Library (TestLibraryReservation_SeaDexCapUnaffected, above). These
// tests lock that audited-and-accepted behavior in permanently, against the
// exact published rules (via bestNLibraryProfile, same production-decode
// pattern as the rest of this file).

// 16: a season pack that is also SeaDex Best consumes *both* its own
// season-pack bucket slot and the global SeaDex-Best cap -- proven two ways
// in one fixture: a competing (non-SeaDex) pack in the same bucket loses the
// season-pack cap, and an unrelated ordinary SeaDex-Best candidate in a
// different resolution bucket loses the global cap, even though nothing
// about it competes with the season-pack bucket at all.
func TestSeasonPackSeaDexBest_ConsumesGlobalCapAcrossBuckets(t *testing.T) {
	profile := bestNLibraryProfile(t, "Seadex Best", "At most 1 SeaDex Best")
	req := ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{Known: true, Best: map[string]bool{"packgrp": true, "othergrp": true}},
	}

	packSeadexBest := "Example.Anime.S01.COMPLETE.2160p.WEB-DL.A-PACKGRP"
	competingPack := "Example.Anime.S01.COMPLETE.2160p.WEB-DL.B-COMPETE"
	ordinaryElsewhere := "Example.Anime.S01E01.1080p.WEB-DL.C-OTHERGRP"

	o := bnApply(t, profile, req, notLib(packSeadexBest), notLib(competingPack), notLib(ordinaryElsewhere))

	if !o.kept[packSeadexBest] {
		t.Fatalf("expected the season-pack+SeaDex-Best candidate to survive, rejected=%v", o.rejected[packSeadexBest])
	}
	if o.kept[competingPack] {
		t.Error("expected the competing non-SeaDex pack in the same bucket to lose the season-pack cap")
	}
	if !bnRejectedBy(o, competingPack, "Best 1 Season Pack per R/Q") {
		t.Errorf("expected the competing pack rejected specifically by the season-pack cap, got %v", o.rejected[competingPack])
	}
	if o.kept[ordinaryElsewhere] {
		t.Error("expected the unrelated ordinary SeaDex-Best candidate in a different resolution bucket to lose the global SeaDex cap")
	}
	if !bnRejectedBy(o, ordinaryElsewhere, "At most 1 SeaDex Best") {
		t.Errorf("expected the unrelated candidate rejected specifically by the global SeaDex cap (proving the season pack's SeaDex flag reached it), got %v", o.rejected[ordinaryElsewhere])
	}

	total := 0
	for _, title := range []string{packSeadexBest, competingPack, ordinaryElsewhere} {
		if o.kept[title] {
			total++
		}
	}
	if total != 1 {
		t.Errorf("expected exactly 1 survivor across both caps combined, got %d", total)
	}
}

// 17: three-way overlap -- a Library season pack that is also SeaDex Best.
// Proves all four audited properties in one fixture: (a) the native Library
// bonus is added exactly once (isolated as a strict +500 score delta against
// an otherwise-identical non-Library twin carrying the same SeaDex-Best
// flag and season-pack shape); (b) it draws no dedicated Library
// reservation -- its bucket's survivor count stays at 1, not the 4 a
// qualifying "Best 1 Library per R/Q" match would allow, because that rule
// structurally excludes season packs; (c) it still wins its season-pack
// bucket against a genuinely competing pack; (d) it still consumes the
// global SeaDex-Best cap, rejecting an unrelated ordinary SeaDex-Best
// candidate elsewhere -- exactly like the two-way case above, with Library
// layered on top and contributing only its own scalar bonus, nothing more.
func TestThreeWay_LibrarySeasonPackSeaDexBest_DeterministicOverlap(t *testing.T) {
	profile := bestNLibraryProfile(t, "Seadex Best", "At most 1 SeaDex Best")
	req := ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{Known: true, Best: map[string]bool{
			"trigrp": true, "trigrp2": true, "othergrp2": true,
		}},
	}

	threeWay := "Example.Anime.S01.COMPLETE.2160p.WEB-DL.A-TRIGRP"
	// twinNonLibrary is structurally identical (same resolution/quality/
	// completeness, its own SeaDex-Best-flagged group) except it is not a
	// Library result -- isolates the Library bonus as a pure score delta,
	// uncontaminated by any other rule's contribution.
	twinNonLibrary := "Example.Anime.S01.COMPLETE.2160p.WEB-DL.B-TRIGRP2"
	competingPack := "Example.Anime.S01.COMPLETE.2160p.WEB-DL.C-COMPETE"
	ordinaryElsewhere := "Example.Anime.S01E01.1080p.WEB-DL.D-OTHERGRP2"

	ts := []bnTitle{lib(threeWay), notLib(twinNonLibrary), notLib(competingPack), notLib(ordinaryElsewhere)}
	kept, rejected := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})

	byTitle := map[string]ranking.Result{}
	for _, r := range append(append([]ranking.Result{}, kept...), rejected...) {
		byTitle[r.Candidate.Release.Title] = r
	}

	threeWayResult, ok := byTitle[threeWay]
	if !ok {
		t.Fatalf("three-way candidate missing from results entirely")
	}
	twinResult, ok := byTitle[twinNonLibrary]
	if !ok {
		t.Fatalf("non-Library twin candidate missing from results entirely")
	}
	if !threeWayResult.Torrent.Fetch {
		t.Fatalf("expected the three-way candidate to survive, rejected=%v", threeWayResult.Torrent.Rejections)
	}

	// (a) Library bonus applied exactly once: a strict +500 delta against the
	// otherwise-identical non-Library twin, nothing more and nothing less.
	if delta := threeWayResult.Torrent.Rank - twinResult.Torrent.Rank; delta != 500 {
		t.Errorf("expected exactly +500 Library bonus delta over the non-Library twin, got %d (three-way=%d, twin=%d)",
			delta, threeWayResult.Torrent.Rank, twinResult.Torrent.Rank)
	}

	// (b) no dedicated Library reservation for a season pack: its bucket
	// keeps exactly 1 survivor (itself), not 2 -- the twin and the competing
	// pack both lose the single season-pack slot.
	ctx := rules.Context{Kind: req.Kind, IsAnime: req.IsAnime, Season: req.Season, Episode: req.Episode, Title: req.Title}
	counts := bnKeptBucketCounts(t, kept, ctx)
	if n := counts["2160p WEB-DL"]; n != 1 {
		t.Errorf("expected exactly 1 survivor in the season-pack bucket (no extra Library slot), got %d", n)
	}
	if twinResult.Torrent.Fetch {
		t.Error("expected the non-Library twin to lose the single season-pack slot to the higher-scoring three-way candidate")
	}
	if !rejectedBy(twinResult.Torrent.Rejections, "Best 1 Season Pack per R/Q") {
		t.Errorf("expected the twin rejected specifically by the season-pack cap, got %v", twinResult.Torrent.Rejections)
	}

	// (c) the genuinely competing (unflagged) pack also loses the same slot.
	competingResult, ok := byTitle[competingPack]
	if !ok || competingResult.Torrent.Fetch {
		t.Errorf("expected the competing pack to lose the season-pack slot too")
	}
	if ok && !rejectedBy(competingResult.Torrent.Rejections, "Best 1 Season Pack per R/Q") {
		t.Errorf("expected the competing pack rejected specifically by the season-pack cap, got %v", competingResult.Torrent.Rejections)
	}

	// (d) the global SeaDex-Best cap is still consumed by the three-way
	// candidate, rejecting an unrelated ordinary SeaDex-Best candidate in a
	// different resolution bucket that never touches the season-pack bucket.
	elsewhereResult, ok := byTitle[ordinaryElsewhere]
	if !ok || elsewhereResult.Torrent.Fetch {
		t.Errorf("expected the unrelated ordinary SeaDex-Best candidate elsewhere to lose the global SeaDex cap")
	}
	if ok && !rejectedBy(elsewhereResult.Torrent.Rejections, "At most 1 SeaDex Best") {
		t.Errorf("expected rejection specifically by the global SeaDex cap, got %v", elsewhereResult.Torrent.Rejections)
	}

	if len(kept) != 1 || kept[0].Candidate.Release.Title != threeWay {
		t.Errorf("expected the three-way candidate to be the sole overall survivor, got kept=%v", overlapTitles(kept))
	}
}

// overlapTitles renders a result slice's titles for a failure message.
func overlapTitles(results []ranking.Result) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Candidate.Release.Title)
	}
	return out
}

// 18: DEFENSIVE BEHAVIOR LOCK, not a policy endorsement. Proves current,
// deterministic behavior if upstream SeaDex data ever flags the same
// release group both Best and Alternative for the same title simultaneously
// -- structurally possible today because SeadexContext's Best/Alt maps
// (pkg/search/rules/env.go) are independent with no mutual exclusion
// anywhere in this repo or the engine, and "Seadex Best"/"Seadex
// Alternative" are two independent `when: seadex.best`/`seadex.alternative`
// scoring rules with no cross-exclusion in profiles/rules.json. Whether this
// shape is ever actually produced depends entirely on the upstream SeaDex
// data source, outside this repo's scope -- DraCuLa only reads the two
// booleans at face value. This test locks in what happens *if* it occurs:
// both rules fire (score sums, not overrides), and the one candidate
// consumes both global caps, at the expense of two otherwise-independent
// legitimate Best/Alternative candidates elsewhere. If this behavior is
// ever deliberately changed, update this test alongside that change rather
// than treating a failure here as a regression to revert.
func TestSeaDexBestAndAlternative_SameGroupBothFlags_LocksCurrentBehavior(t *testing.T) {
	profile := bestNLibraryProfile(t, "Seadex Best", "Seadex Alternative", "At most 1 SeaDex Best", "At most 1 SeaDex Alternative")
	req := ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{
			Known: true,
			Best:  map[string]bool{"bothgrp": true, "onlybestgrp": true},
			Alt:   map[string]bool{"bothgrp": true, "onlyaltgrp": true},
		},
	}

	both := "Example.Anime.S01E01.2160p.WEB-DL.A-BOTHGRP"
	onlyBest := "Example.Anime.S01E01.1080p.WEB-DL.B-ONLYBESTGRP"
	onlyAlt := "Example.Anime.S01E01.720p.WEB-DL.C-ONLYALTGRP"

	ts := []bnTitle{notLib(both), notLib(onlyBest), notLib(onlyAlt)}
	kept, rejected := profile.ApplyWithRejected(req, bnCandidates(ts), jhinrank.RankOptions{})

	byTitle := map[string]ranking.Result{}
	for _, r := range kept {
		byTitle[r.Candidate.Release.Title] = r
	}
	bothResult, ok := byTitle[both]
	if !ok {
		t.Fatalf("expected the both-flagged candidate to survive, kept=%v", overlapTitles(kept))
	}

	bestScore, bestOK := matchScore(bothResult.Matched, "Seadex Best")
	altScore, altOK := matchScore(bothResult.Matched, "Seadex Alternative")
	if !bestOK || !altOK {
		t.Fatalf("expected both Seadex Best and Seadex Alternative to match the same candidate, matched=%v", bothResult.Matched)
	}
	if bestScore != 150000 {
		t.Errorf("expected Seadex Best's own contribution to remain 150000, got %d", bestScore)
	}
	if altScore != 75000 {
		t.Errorf("expected Seadex Alternative's own contribution to remain 75000, got %d", altScore)
	}
	if sum := bestScore + altScore; sum != 225000 {
		t.Errorf("expected the total SeaDex contribution to be the sum of both current rule values (150000+75000=225000), got %d", sum)
	}

	rejByTitle := map[string][]string{}
	for _, r := range rejected {
		rejByTitle[r.Candidate.Release.Title] = r.Torrent.Rejections
	}
	if _, stillKept := byTitle[onlyBest]; stillKept {
		t.Error("expected the legitimate lone-Best candidate to lose the global SeaDex-Best cap to the both-flagged candidate")
	}
	if !rejectedBy(rejByTitle[onlyBest], "At most 1 SeaDex Best") {
		t.Errorf("expected the lone-Best candidate rejected specifically by the global SeaDex-Best cap, got %v", rejByTitle[onlyBest])
	}
	if _, stillKept := byTitle[onlyAlt]; stillKept {
		t.Error("expected the legitimate lone-Alternative candidate to lose the global SeaDex-Alternative cap to the both-flagged candidate")
	}
	if !rejectedBy(rejByTitle[onlyAlt], "At most 1 SeaDex Alternative") {
		t.Errorf("expected the lone-Alternative candidate rejected specifically by the global SeaDex-Alternative cap, got %v", rejByTitle[onlyAlt])
	}
}
