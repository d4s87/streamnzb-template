package streamnzb_compat

// Permanent real-engine regression for the "Movies Anywhere" non-Anime
// streaming-service formatter badge (StreamNZB v6.2.0 pin-readiness audit,
// classification A). Unlike the 16 generic bounded-token badges in
// non_anime_service_badges_test.go, this rule protects bare "MA" against a
// real, upstream-documented collision -- the "DTS-HD MA" audio-codec token
// -- using the RE2-lookaround-equivalent `matchesExcept` rule-DSL function
// (available since Jhin v0.8.0). Kept as its own file because the
// collision-safety shape genuinely differs from the other 16 badges' plain
// `releaseName matches "..."` contract, not because the rule is owned or
// scored any differently (it is still zero-point, presentation-owned,
// `not isAnime`, same WEB-traits gate).
//
// The upstream matchesExcept implementation itself (dreulavelle/streamnzb
// pkg/search/rules/matchesexcept.go, matchesexcept_test.go) uses this exact
// MA/DTS-HD-MA pair as its own canonical worked example -- this file proves
// DraCuLa's actual published rule against the same case shapes, not just
// the underlying function in isolation.

import (
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const movieAnywhereRuleName = "Movies Anywhere"

func TestMoviesAnywhereRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, movieAnywhereRuleName)

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

// TestMoviesAnywhereClassification is the production-regression layer: the
// exact published rule (decoded from profile.txt, not a hand-copied
// fixture) run over the concrete positive/negative/collision cases the
// audit's evidence chain names, through the real StreamNZB rule engine.
func TestMoviesAnywhereClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, movieAnywhereRuleName)

	tests := []struct {
		name    string
		title   string
		isAnime bool
		want    bool
	}{
		{
			"bare MA service tag alone matches",
			"Example.Movie.2026.1080p.MA.WEB-DL.DDP5.1.x264-GROUP",
			false,
			true,
		},
		{
			"spelled-out Movies Anywhere alias matches",
			"Example.Movie.2026.1080p.Movies.Anywhere.WEB-DL.DDP5.1.x264-GROUP",
			false,
			true,
		},
		{
			"spelled-out Movies-Anywhere (hyphen separator) alias matches",
			"Example.Show.S01E01.1080p.Movies-Anywhere.WEB-DL.DDP5.1.x264-GROUP",
			false,
			true,
		},
		{
			// The exact upstream-audited collision: DTS-HD Master Audio's
			// own "MA" tail must not be read as the streaming-service tag.
			"DTS-HD MA audio-codec token alone does not match",
			"Example.Movie.2026.1080p.AMZN.WEB-DL.DTS-HD.MA.5.1-GROUP",
			false,
			false,
		},
		{
			// The case naive `matches ... and not matches ...` gets wrong:
			// both the service tag and the audio-codec token are present in
			// the same release, and the service tag the rule means is real.
			"MA service tag and DTS-HD MA audio token both present still matches",
			"Example.Movie.2026.1080p.MA.WEB-DL.DTS-HD.MA.5.1-GROUP",
			false,
			true,
		},
		{
			"neither MA nor DTS-HD MA present does not match",
			"Example.Movie.2026.1080p.AMZN.WEB-DL.DDP5.1-GROUP",
			false,
			false,
		},
		{
			// IMAX contains the letters "MA" with no boundary on either
			// side -- the audit's own stated reason this badge is safe.
			"IMAX edition does not collide",
			"Example.Movie.2026.1080p.IMAX.WEB-DL.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			// An ordinary word containing "ma" as a substring, not at a
			// token boundary, must not fire the badge.
			"ordinary word containing \"ma\" (Drama) does not match",
			"Example.Show.Drama.S01E01.1080p.WEB-DL.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			// The MA token fused into a longer release-group tag (no
			// separator on either side) must not fire.
			"MA fused into a longer release-group tag does not match",
			"Example.Movie.2026.1080p.WEB-DL.DDP5.1.x264-XMAGROUPX",
			false,
			false,
		},
		{
			"does not match a non-WEB release (BluRay)",
			"Example.Movie.2026.1080p.MA.BluRay.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			"does not match when isAnime (not isAnime scope)",
			"Example.Movie.2026.1080p.MA.WEB-DL.DDP5.1.x264-GROUP",
			true,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluateIsolatedRule(t, cfg, tt.title, tt.isAnime); got != tt.want {
				t.Errorf(
					"%q matched = %v, want %v (title %q, isAnime=%v)",
					movieAnywhereRuleName, got, tt.want, tt.title, tt.isAnime,
				)
			}
		})
	}
}

// TestMoviesAnywhereScoringInvariance proves, through the real full-profile
// pipeline (not just the isolated rule), that a matching Movies Anywhere
// badge changes only presentation (MatchedRules / formatter label) and
// never final score, Fetch/keep state, or rejection behavior -- mirroring
// TestNonAnimeServiceBadgeScoringInvariance for the other 16 badges.
func TestMoviesAnywhereScoringInvariance(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Movies Anywhere scoring invariance",
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
	withMA := "Example.Movie.2026.1080p.MA.WEB-DL.DDP5.1.x264-GROUP"

	cleanRank, cleanMatched, cleanKept, cleanRejected := rankOf(clean)
	maRank, maMatched, maKept, maRejected := rankOf(withMA)

	if contains(cleanMatched, movieAnywhereRuleName) {
		t.Fatal("clean release unexpectedly matched Movies Anywhere")
	}
	if !contains(maMatched, movieAnywhereRuleName) {
		t.Fatal("MA release did not match the Movies Anywhere rule; nothing to prove invariance over")
	}

	if maRank != cleanRank {
		t.Errorf(
			"final score changed by the Movies Anywhere badge: clean=%d ma=%d (want equal)",
			cleanRank, maRank,
		)
	}
	if maKept != cleanKept || maKept != 1 {
		t.Errorf(
			"Fetch/keep state changed by the Movies Anywhere badge: clean kept=%d ma kept=%d (want 1/1)",
			cleanKept, maKept,
		)
	}
	if maRejected != cleanRejected || maRejected != 0 {
		t.Errorf(
			"rejection behavior changed by the Movies Anywhere badge: clean rejected=%d ma rejected=%d (want 0/0)",
			cleanRejected, maRejected,
		)
	}
}
