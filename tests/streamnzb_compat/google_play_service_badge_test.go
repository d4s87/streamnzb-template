package streamnzb_compat

// Permanent real-engine regression for the "Google Play" non-Anime
// streaming-service formatter badge (remaining-badges audit, 2026-09-17).
// Kept as its own file, mirroring movies_anywhere_service_badge_test.go's
// and max_service_badge_test.go's precedent, because the collision-safety
// shape genuinely differs from the 20 generic bounded-token badges in
// non_anime_service_badges_test.go, not because the rule is owned or
// scored any differently (it is still zero-point, presentation-owned,
// `not isAnime`, same WEB-traits gate).
//
// Unlike every other service badge, Google Play's own canonical token is
// bare "Play" -- a real English word that can legitimately appear as a
// title's own last word (e.g. the "Child's Play" franchise, "Fair Play").
// A plain bounded-anywhere match (the shape every other simple badge uses)
// would false-positive on real, unrelated titles. Vidhin's own upstream
// classification data avoids exactly this by requiring "Play" to sit
// immediately adjacent to the WEB/WEBDL/WEBRip marker itself -- the same
// adjacency real scene convention already uses to place a service tag
// directly in front of the source-type tag (e.g. "AMZN.WEB-DL",
// "NF.WEBRip"). This file's rule adopts that same adjacency requirement
// instead of the usual bounded-anywhere pattern; it deliberately does not
// use matchesExcept, because this is a narrower *positive* requirement,
// not an exclusion of a covering match.

import (
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const googlePlayRuleName = "Google Play"

func TestGooglePlayRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, googlePlayRuleName)

	if cfg.Points != 0 {
		t.Errorf("rule points = %d, want 0", cfg.Points)
	}
	if cfg.EffectiveAction() != config.RuleActionScore {
		t.Errorf(
			"rule action = %q, want score (presentation-only, no reject/limit)",
			cfg.EffectiveAction(),
		)
	}
}

// TestGooglePlayClassification is the production-regression layer: the
// exact published rule (decoded from profile.txt, not a hand-copied
// fixture) run over the concrete positive/negative/collision cases the
// audit's evidence chain names, through the real StreamNZB rule engine.
func TestGooglePlayClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, googlePlayRuleName)

	tests := []struct {
		name    string
		title   string
		isAnime bool
		want    bool
	}{
		{
			"Play immediately adjacent to WEB-DL matches",
			"Example.Movie.2026.1080p.Play.WEB-DL.DDP5.1.x264-GROUP",
			false,
			true,
		},
		{
			"Play immediately adjacent to WEBRip matches",
			"Example.Movie.2026.1080p.Play.WEBRip.x264-GROUP",
			false,
			true,
		},
		{
			"Play immediately adjacent to fused WEBDL matches",
			"Example_Movie_2026_1080p_Play_WEBDL_x264-GROUP",
			false,
			true,
		},
		{
			"lowercase play immediately adjacent to web-dl matches (case-insensitive)",
			"example.movie.2026.1080p.play.web-dl.ddp5.1.x264-group",
			false,
			true,
		},
		{
			// The mandatory permanent regression: a real, unrelated title
			// ending in the word "Play", with a year/resolution intervening
			// before the WEB-DL tag exactly as real scene naming always
			// places it, must not fire the badge.
			"Childs.Play.<year>...WEB-DL does not match (real title, not adjacent)",
			"Childs.Play.2019.1080p.WEB-DL.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			"another real title ending in Play, not adjacent to WEB, does not match",
			"Fair.Play.2023.1080p.WEB-DL.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			// "Play" fused into a longer word (no boundary before it) must
			// not fire, mirroring every other badge's fused-token guard.
			"Play fused into a longer word does not match",
			"Example.Movie.2026.1080p.WEB-DL.DDP5.1.x264-DisplayGROUP",
			false,
			false,
		},
		{
			"neither Play nor any Google Play signal present does not match",
			"Example.Movie.2026.1080p.AMZN.WEB-DL.DDP5.1-GROUP",
			false,
			false,
		},
		{
			// Naturally covered twice over by this rule's own WEB-adjacency
			// requirement (no literal "web" text is adjacent to "Play" at
			// all in a BluRay release), but kept for consistency with every
			// other badge's non-WEB negative case.
			"does not match a non-WEB release (BluRay)",
			"Example.Movie.2026.1080p.Play.BluRay.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			"does not match when isAnime (not isAnime scope)",
			"Example.Movie.2026.1080p.Play.WEB-DL.DDP5.1.x264-GROUP",
			true,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluateIsolatedRule(t, cfg, tt.title, tt.isAnime); got != tt.want {
				t.Errorf(
					"%q matched = %v, want %v (title %q, isAnime=%v)",
					googlePlayRuleName, got, tt.want, tt.title, tt.isAnime,
				)
			}
		})
	}
}

// TestGooglePlayScoringInvariance proves, through the real full-profile
// pipeline (not just the isolated rule), that a matching Google Play badge
// changes only presentation (MatchedRules / formatter label) and never
// final score, Fetch/keep state, or rejection behavior -- mirroring
// TestMaxScoringInvariance and TestNonAnimeServiceBadgeScoringInvariance
// for the other badges.
func TestGooglePlayScoringInvariance(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Google Play scoring invariance",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	rankOf := func(title string) (rank int, matchedNames []string, kept int, rejected int) {
		t.Helper()

		cand := triage.Candidate{Release: &release.Release{Title: title}}
		request := ranking.Request{Kind: ranking.KindMovie, Title: "Example"}

		keptResults, rejectedResults := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{cand},
			jhinrank.RankOptions{},
		)

		if len(keptResults) != 1 {
			return 0, nil, len(keptResults), len(rejectedResults)
		}

		names := make([]string, 0, len(keptResults[0].Matched))
		for _, m := range keptResults[0].Matched {
			names = append(names, m.Name)
		}

		return keptResults[0].Torrent.Rank, names, len(keptResults), len(rejectedResults)
	}

	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	clean := "Example.Movie.2026.1080p.WEB-DL.DDP5.1.x264-GROUP"
	withGooglePlay := "Example.Movie.2026.1080p.Play.WEB-DL.DDP5.1.x264-GROUP"

	cleanRank, cleanMatched, cleanKept, cleanRejected := rankOf(clean)
	playRank, playMatched, playKept, playRejected := rankOf(withGooglePlay)

	if contains(cleanMatched, googlePlayRuleName) {
		t.Fatal("clean release unexpectedly matched Google Play")
	}
	if !contains(playMatched, googlePlayRuleName) {
		t.Fatal("Google Play release did not match the Google Play rule; nothing to prove invariance over")
	}

	if playRank != cleanRank {
		t.Errorf(
			"final score changed by the Google Play badge: clean=%d play=%d (want equal)",
			cleanRank, playRank,
		)
	}
	if playKept != cleanKept || playKept != 1 {
		t.Errorf(
			"Fetch/keep state changed by the Google Play badge: clean kept=%d play kept=%d (want 1/1)",
			cleanKept, playKept,
		)
	}
	if playRejected != cleanRejected || playRejected != 0 {
		t.Errorf(
			"rejection behavior changed by the Google Play badge: clean rejected=%d play rejected=%d (want 0/0)",
			cleanRejected, playRejected,
		)
	}
}
