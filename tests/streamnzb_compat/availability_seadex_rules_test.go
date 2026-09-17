package streamnzb_compat

// Production-regression coverage for the 7 shipped rules that were
// previously untested anywhere in this harness (confirmed by grep across
// every *_test.go and fixtures/*.json before writing this file): the 3
// availability rules and the 4 SeaDex Best/Alternative rules. Every other
// rule family in profile.txt has at least one dedicated regression; these
// were the one real gap the StreamNZB API audit found (project_context.md
// §11 / backlog-roadmap.md's capability audit) -- closeable entirely with
// the same in-process ranking.Compile/ApplyWithRejected pattern every other
// file here already uses, no HTTP layer needed:
//   - avail.* reaches the rule engine via triage.Candidate.Verdict.Avail
//     (triage.AvailState), exactly as StreamNZB's own
//     pkg/search/rules/rules_test.go constructs it directly.
//   - seadex.* reaches the rule engine via ranking.Request.Seadex
//     (*rules.SeadexContext), resolved per-candidate by parsed release
//     group -- already proven in best_n_library_test.go's
//     TestLibraryReservation_SeaDexCapUnaffected, reused here for the
//     rules that own that behavior rather than a rule that merely
//     coexists with it.

import (
	"strings"
	"testing"
	"time"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

// availabilitySeadexProfile compiles the exact named production rules (as
// published, decoded from profile.txt) against the real Define library.
// Production-regression fidelity: drift between what is tested here and
// what ships fails CI, per CLAUDE.md's two-layer validation philosophy.
func availabilitySeadexProfile(t *testing.T, ruleNames ...string) *ranking.Profile {
	t.Helper()

	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	selected := make([]config.RuleConfig, 0, len(ruleNames))
	for _, name := range ruleNames {
		selected = append(selected, findProductionRule(t, productionRules, name))
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Availability/SeaDex production regression",
			Preset: "4k",
			Rules:  selected,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production availability/SeaDex rule subset: %v", err)
	}
	return profile
}

// availCandidate builds a candidate carrying the availability record the 3
// avail.* rules read, exactly as StreamNZB's own rules_test.go constructs it
// (triage.Candidate.Verdict.Avail), not through the Explain-only Sample path.
func availCandidate(title string, avail triage.AvailState) triage.Candidate {
	return triage.Candidate{
		Release: &release.Release{Title: title},
		Verdict: triage.Verdict{Avail: avail},
	}
}

func applyAvail(t *testing.T, profile *ranking.Profile, candidates ...triage.Candidate) (kept, rejected []ranking.Result) {
	t.Helper()
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}
	return profile.ApplyWithRejected(req, candidates, jhinrank.RankOptions{})
}

func matchedBy(matched []triage.RuleMatch, name string) bool {
	for _, m := range matched {
		if m.Name == name {
			return true
		}
	}
	return false
}

// matchScore returns the named rule's own point contribution, independent of
// the candidate's native resolution/quality/tier score that Torrent.Rank
// otherwise mixes in -- a real 2160p WEB-DL title always carries a large
// native score of its own, so asserting against the total Rank rather than
// the specific rule's Matched entry would silently pass or fail for the
// wrong reason.
func matchScore(matched []triage.RuleMatch, name string) (int, bool) {
	for _, m := range matched {
		if m.Name == name {
			return m.Score, true
		}
	}
	return 0, false
}

func rejectedBy(rejections []string, name string) bool {
	for _, r := range rejections {
		if strings.Contains(r, name) {
			return true
		}
	}
	return false
}

// --- Alive on our backbone: +20, when avail.onMyBackbone ---

func TestAliveOnOurBackbone_ScoresWhenOnBackbone(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Alive on our backbone")

	// Status must be Known (Available or Unavailable) for the "avail" tier
	// to be present at all -- TierPresent("avail") reads e.Avail.Known, not
	// OnMyBackbone directly (pkg/search/rules/env.go). A candidate that
	// vouches for OnMyBackbone without a known Status is indistinguishable
	// from one with no availability record, and the rule fails open (no
	// match) exactly as it would for a real release nobody has checked.
	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	avail := triage.AvailState{Status: triage.AvailAvailable, OnMyBackbone: true}
	kept, rejected := applyAvail(t, profile, availCandidate(title, avail))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d kept, %d rejected", len(kept), len(rejected))
	}
	score, ok := matchScore(kept[0].Matched, "Alive on our backbone")
	if !ok {
		t.Fatalf("expected %q to match, matched=%v", "Alive on our backbone", kept[0].Matched)
	}
	if score != 20 {
		t.Errorf("expected rule score 20, got %d", score)
	}
}

func TestAliveOnOurBackbone_DoesNotScoreWhenNotOnBackbone(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Alive on our backbone")

	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	avail := triage.AvailState{Status: triage.AvailAvailable, OnMyBackbone: false}
	kept, _ := applyAvail(t, profile, availCandidate(title, avail))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	if matchedBy(kept[0].Matched, "Alive on our backbone") {
		t.Error("did not expect Alive on our backbone to match when OnMyBackbone is false")
	}
}

func TestAliveOnOurBackbone_DoesNotScoreWhenAvailUnknown(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Alive on our backbone")

	// A genuinely unchecked release (zero AvailState): the "avail" tier is
	// absent, so the rule must fail open rather than be judged against a
	// zero OnMyBackbone.
	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	kept, _ := applyAvail(t, profile, availCandidate(title, triage.AvailState{}))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	if matchedBy(kept[0].Matched, "Alive on our backbone") {
		t.Error("did not expect Alive on our backbone to match with no availability record at all")
	}
}

// --- Recently confirmed: +10, when 0 <= avail.checkedDaysAgo < 30 ---

func TestRecentlyConfirmed_ScoresWithinThirtyDays(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Recently confirmed")

	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	avail := triage.AvailState{Status: triage.AvailAvailable, CheckedAt: time.Now()}
	kept, _ := applyAvail(t, profile, availCandidate(title, avail))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	score, ok := matchScore(kept[0].Matched, "Recently confirmed")
	if !ok {
		t.Fatalf("expected Recently confirmed to match a just-checked release, matched=%v", kept[0].Matched)
	}
	if score != 10 {
		t.Errorf("expected rule score 10, got %d", score)
	}
}

func TestRecentlyConfirmed_DoesNotScoreBeyondThirtyDays(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Recently confirmed")

	// Status is set (tier present) so this genuinely exercises the rule's
	// own "< 30" boundary, not the separate tier-absent fail-open path.
	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	old := time.Now().AddDate(0, 0, -35)
	avail := triage.AvailState{Status: triage.AvailAvailable, CheckedAt: old}
	kept, _ := applyAvail(t, profile, availCandidate(title, avail))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	if matchedBy(kept[0].Matched, "Recently confirmed") {
		t.Error("did not expect Recently confirmed to match a 35-day-old check")
	}
}

func TestRecentlyConfirmed_DoesNotScoreWhenUnknown(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Recently confirmed")

	// A zero CheckedAt (triage.AvailState{}) means "never checked" --
	// AvailState.CheckedDaysAgo() returns -1, which the rule's own
	// ">= 0" guard must exclude rather than treating -1 as "very recent".
	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	kept, _ := applyAvail(t, profile, availCandidate(title, triage.AvailState{}))

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	if matchedBy(kept[0].Matched, "Recently confirmed") {
		t.Error("did not expect Recently confirmed to match an unknown (never-checked) availability record")
	}
}

// --- Known unavailable: reject, when avail.status == "unavailable" ---

func TestKnownUnavailable_RejectsUnavailable(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Known unavailable")

	title := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	kept, rejected := applyAvail(t, profile, availCandidate(title, triage.AvailState{Status: triage.AvailUnavailable}))

	if len(kept) != 0 || len(rejected) != 1 {
		t.Fatalf("expected the candidate to be rejected, got %d kept, %d rejected", len(kept), len(rejected))
	}
	if !rejectedBy(rejected[0].Torrent.Rejections, "Known unavailable") {
		t.Errorf("expected rejection by Known unavailable, got %v", rejected[0].Torrent.Rejections)
	}
}

func TestKnownUnavailable_KeepsAvailableAndUnknown(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Known unavailable")

	available := "Example.Movie.2020.2160p.WEB-DL.A-GROUP"
	unknown := "Example.Movie.2020.2160p.WEB-DL.B-GROUP"
	kept, rejected := applyAvail(
		t, profile,
		availCandidate(available, triage.AvailState{Status: triage.AvailAvailable}),
		availCandidate(unknown, triage.AvailState{}),
	)

	if len(kept) != 2 || len(rejected) != 0 {
		t.Fatalf("expected both candidates kept, got %d kept, %d rejected (%v)", len(kept), len(rejected), rejected)
	}
}

// --- Seadex Best / Seadex Alternative: +150000 / +75000, when seadex.best / seadex.alternative ---

func seadexRequest(best, alt []string) ranking.Request {
	bestMap := make(map[string]bool, len(best))
	for _, g := range best {
		bestMap[g] = true
	}
	altMap := make(map[string]bool, len(alt))
	for _, g := range alt {
		altMap[g] = true
	}
	return ranking.Request{
		Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime",
		Seadex: &rules.SeadexContext{Known: true, Best: bestMap, Alt: altMap},
	}
}

func applySeadex(t *testing.T, profile *ranking.Profile, req ranking.Request, titles ...string) (kept, rejected []ranking.Result) {
	t.Helper()
	candidates := make([]triage.Candidate, 0, len(titles))
	for _, title := range titles {
		candidates = append(candidates, triage.Candidate{Release: &release.Release{Title: title}})
	}
	return profile.ApplyWithRejected(req, candidates, jhinrank.RankOptions{})
}

func TestSeadexBest_ScoresRecommendedGroup(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Seadex Best")
	req := seadexRequest([]string{"bestgrp"}, nil)

	bestTitle := "Example.Anime.S01E01.2160p.WEB-DL.A-BESTGRP"
	otherTitle := "Example.Anime.S01E01.2160p.WEB-DL.B-OTHERGRP"
	kept, _ := applySeadex(t, profile, req, bestTitle, otherTitle)

	byTitle := map[string]ranking.Result{}
	for _, r := range kept {
		byTitle[r.Candidate.Release.Title] = r
	}
	score, ok := matchScore(byTitle[bestTitle].Matched, "Seadex Best")
	if !ok {
		t.Fatalf("expected Seadex Best to match the recommended group, matched=%v", byTitle[bestTitle].Matched)
	}
	if score != 150000 {
		t.Errorf("expected rule score 150000, got %d", score)
	}
	if matchedBy(byTitle[otherTitle].Matched, "Seadex Best") {
		t.Error("did not expect Seadex Best to match a group SeaDex did not recommend")
	}
}

func TestSeadexAlternative_ScoresAlternateGroup(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Seadex Alternative")
	req := seadexRequest(nil, []string{"altgrp"})

	altTitle := "Example.Anime.S01E01.2160p.WEB-DL.A-ALTGRP"
	otherTitle := "Example.Anime.S01E01.2160p.WEB-DL.B-OTHERGRP"
	kept, _ := applySeadex(t, profile, req, altTitle, otherTitle)

	byTitle := map[string]ranking.Result{}
	for _, r := range kept {
		byTitle[r.Candidate.Release.Title] = r
	}
	score, ok := matchScore(byTitle[altTitle].Matched, "Seadex Alternative")
	if !ok {
		t.Fatalf("expected Seadex Alternative to match the alternate group, matched=%v", byTitle[altTitle].Matched)
	}
	if score != 75000 {
		t.Errorf("expected rule score 75000, got %d", score)
	}
	if matchedBy(byTitle[otherTitle].Matched, "Seadex Alternative") {
		t.Error("did not expect Seadex Alternative to match a group SeaDex did not list")
	}
}

func TestSeadexBest_NoScoreWhenNoSeadexContext(t *testing.T) {
	// req.Seadex is nil, matching every real non-anime request and any
	// anime request SeaDex could not answer -- the rule must fail open
	// (never match) rather than error or panic.
	profile := availabilitySeadexProfile(t, "Seadex Best")
	req := ranking.Request{Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime"}

	title := "Example.Anime.S01E01.2160p.WEB-DL.A-BESTGRP"
	kept, _ := applySeadex(t, profile, req, title)

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept result, got %d", len(kept))
	}
	if matchedBy(kept[0].Matched, "Seadex Best") {
		t.Error("did not expect Seadex Best to match with no SeaDex context at all")
	}
}

// --- At most 1 SeaDex Best / At most 1 SeaDex Alternative: limit, count 1 ---

func TestAtMostOneSeadexBest_CapsMultipleRecommendedGroups(t *testing.T) {
	// Seadex Best is included alongside its own cap so the two SeaDex-Best
	// candidates actually carry the astronomical score the cap exists to
	// bound -- the cap rule alone, with no score difference, would not
	// exercise "best by final score" tie-breaking the same way production does.
	profile := availabilitySeadexProfile(t, "Seadex Best", "At most 1 SeaDex Best")
	req := seadexRequest([]string{"firstgrp", "secondgrp"}, nil)

	first := "Example.Anime.S01E01.2160p.WEB-DL.A-FIRSTGRP"
	second := "Example.Anime.S01E01.2160p.WEB-DL.B-SECONDGRP"
	kept, rejected := applySeadex(t, profile, req, first, second)

	if len(kept) != 1 || len(rejected) != 1 {
		t.Fatalf("expected exactly 1 of 2 SeaDex-Best candidates to survive, got %d kept, %d rejected", len(kept), len(rejected))
	}
	if !rejectedBy(rejected[0].Torrent.Rejections, "At most 1 SeaDex Best") {
		t.Errorf("expected the losing candidate rejected specifically by At most 1 SeaDex Best, got %v", rejected[0].Torrent.Rejections)
	}
}

func TestAtMostOneSeadexAlternative_CapsMultipleAlternateGroups(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Seadex Alternative", "At most 1 SeaDex Alternative")
	req := seadexRequest(nil, []string{"firstgrp", "secondgrp"})

	first := "Example.Anime.S01E01.2160p.WEB-DL.A-FIRSTGRP"
	second := "Example.Anime.S01E01.2160p.WEB-DL.B-SECONDGRP"
	kept, rejected := applySeadex(t, profile, req, first, second)

	if len(kept) != 1 || len(rejected) != 1 {
		t.Fatalf("expected exactly 1 of 2 SeaDex-Alternative candidates to survive, got %d kept, %d rejected", len(kept), len(rejected))
	}
	if !rejectedBy(rejected[0].Torrent.Rejections, "At most 1 SeaDex Alternative") {
		t.Errorf("expected the losing candidate rejected specifically by At most 1 SeaDex Alternative, got %v", rejected[0].Torrent.Rejections)
	}
}

func TestAtMostOneSeadexBest_DoesNotCapUnrelatedCandidates(t *testing.T) {
	profile := availabilitySeadexProfile(t, "Seadex Best", "At most 1 SeaDex Best")
	req := seadexRequest([]string{"bestgrp"}, nil)

	best := "Example.Anime.S01E01.2160p.WEB-DL.A-BESTGRP"
	other := "Example.Anime.S01E01.2160p.WEB-DL.B-OTHERGRP"
	kept, rejected := applySeadex(t, profile, req, best, other)

	if len(kept) != 2 || len(rejected) != 0 {
		t.Fatalf("expected both candidates kept (only 1 is SeaDex Best, well under the cap), got %d kept, %d rejected", len(kept), len(rejected))
	}
}
