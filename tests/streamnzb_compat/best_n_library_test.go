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
