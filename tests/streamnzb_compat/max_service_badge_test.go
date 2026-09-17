package streamnzb_compat

// Permanent real-engine regression for the "Max" non-Anime streaming-service
// formatter badge (StreamNZB v6.2.0 pin-readiness audit, classification A).
// Kept as its own file, mirroring movies_anywhere_service_badge_test.go's
// precedent, because the collision-safety shape genuinely differs from the
// 16 generic bounded-token badges in non_anime_service_badges_test.go, not
// because the rule is owned or scored any differently (it is still
// zero-point, presentation-owned, `not isAnime`, same WEB-traits gate).
//
// The upstream matchesExcept implementation itself (dreulavelle/streamnzb
// pkg/search/rules/matchesexcept.go, matchesexcept_test.go's
// TestMatchesExceptHandlesAPrefixCollision) uses this exact MAX/HBO-Max pair
// as its own canonical worked example -- this file proves DraCuLa's actual
// published rule against the same case shapes, not just the underlying
// function in isolation.

import (
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const maxRuleName = "Max"

func TestMaxRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, maxRuleName)

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

// TestMaxClassification is the production-regression layer: the exact
// published rule (decoded from profile.txt, not a hand-copied fixture) run
// over the concrete positive/negative/collision cases the audit's evidence
// chain names, through the real StreamNZB rule engine.
func TestMaxClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, maxRuleName)

	tests := []struct {
		name    string
		title   string
		isAnime bool
		want    bool
	}{
		{
			"bare MAX service tag alone matches",
			"Example.Movie.2026.1080p.MAX.WEB-DL.DDP5.1.x264-GROUP",
			false,
			true,
		},
		{
			"bare MAX matches with an underscore-delimited release name",
			"Example_Movie_2026_1080p_MAX_WEBRip_x265-GROUP",
			false,
			true,
		},
		{
			// The exact upstream-audited collision: the "HBO Max" branding
			// prefix must not be read as the standalone service tag.
			"HBO.Max branding (dot separator) alone does not match",
			"Example.Movie.2026.2160p.HBO.Max.WEB-DL.DDP5.1.Atmos-GROUP",
			false,
			false,
		},
		{
			"HBO-Max branding (hyphen separator) alone does not match",
			"Example.Movie.2026.2160p.HBO-Max.WEB-DL.DDP5.1.Atmos-GROUP",
			false,
			false,
		},
		{
			"HBOMax branding (fused, no separator) alone does not match",
			"Example.Movie.2026.2160p.HBOMax.WEB-DL-GROUP",
			false,
			false,
		},
		{
			// The case naive `matches ... and not matches ...` gets wrong:
			// the standalone service tag and the excluded branding prefix
			// are both present in the same release, and the service tag
			// the rule means is real -- matchesExcept asks where each match
			// landed instead of reading the whole name twice.
			"bare MAX service tag and HBO.Max branding both present still matches",
			"Example.Show.S01E01.1080p.MAX.WEB-DL.HBO.Max.Original-GROUP",
			false,
			true,
		},
		{
			"neither MAX nor HBO Max present does not match",
			"Example.Movie.2026.1080p.AMZN.WEB-DL.DDP5.1-GROUP",
			false,
			false,
		},
		{
			// An ordinary word containing "max" as a substring, not at a
			// token boundary, must not fire the badge.
			"ordinary word containing \"max\" (Climax) does not match",
			"Example.Movie.2026.Climax.1080p.WEB-DL.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			// "Maximum" starts with the literal letters "Max" but has no
			// boundary after them -- must not fire.
			"ordinary word starting with \"Max\" (Maximum) does not match",
			"Example.Show.Maximum.Overdrive.S01E01.1080p.WEB-DL.DDP5.1-GROUP",
			false,
			false,
		},
		{
			// The MAX token fused into a longer release-group tag (no
			// separator on either side) must not fire.
			"MAX fused into a longer release-group tag does not match",
			"Example.Movie.2026.1080p.WEB-DL.DDP5.1.x264-XMAXGROUPX",
			false,
			false,
		},
		{
			"does not match a non-WEB release (BluRay)",
			"Example.Movie.2026.1080p.MAX.BluRay.DDP5.1.x264-GROUP",
			false,
			false,
		},
		{
			"does not match when isAnime (not isAnime scope)",
			"Example.Movie.2026.1080p.MAX.WEB-DL.DDP5.1.x264-GROUP",
			true,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluateIsolatedRule(t, cfg, tt.title, tt.isAnime); got != tt.want {
				t.Errorf(
					"%q matched = %v, want %v (title %q, isAnime=%v)",
					maxRuleName, got, tt.want, tt.title, tt.isAnime,
				)
			}
		})
	}
}

// TestMaxScoringInvariance proves, through the real full-profile pipeline
// (not just the isolated rule), that a matching Max badge changes only
// presentation (MatchedRules / formatter label) and never final score,
// Fetch/keep state, or rejection behavior -- mirroring
// TestMoviesAnywhereScoringInvariance and
// TestNonAnimeServiceBadgeScoringInvariance for the other 17 badges.
func TestMaxScoringInvariance(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Max scoring invariance",
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
	withMax := "Example.Movie.2026.1080p.MAX.WEB-DL.DDP5.1.x264-GROUP"

	cleanRank, cleanMatched, cleanKept, cleanRejected := rankOf(clean)
	maxRank, maxMatched, maxKept, maxRejected := rankOf(withMax)

	if contains(cleanMatched, maxRuleName) {
		t.Fatal("clean release unexpectedly matched Max")
	}
	if !contains(maxMatched, maxRuleName) {
		t.Fatal("MAX release did not match the Max rule; nothing to prove invariance over")
	}

	if maxRank != cleanRank {
		t.Errorf(
			"final score changed by the Max badge: clean=%d max=%d (want equal)",
			cleanRank, maxRank,
		)
	}
	if maxKept != cleanKept || maxKept != 1 {
		t.Errorf(
			"Fetch/keep state changed by the Max badge: clean kept=%d max kept=%d (want 1/1)",
			cleanKept, maxKept,
		)
	}
	if maxRejected != cleanRejected || maxRejected != 0 {
		t.Errorf(
			"rejection behavior changed by the Max badge: clean rejected=%d max rejected=%d (want 0/0)",
			cleanRejected, maxRejected,
		)
	}
}
