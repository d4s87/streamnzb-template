package streamnzb_compat

// Permanent real-engine regression for DraCuLa's "Movies LQ Release Title" /
// "Shows LQ Release Title" Defines: the exact-safe, uncontested subset of
// Vidhin's "LQ (Release Title) (Radarr)"/"(Sonarr)" classifications, synced
// via scripts/sync_vidhin.py's lq_release_title_terms()/
// render_lq_release_title_condition(). See the roadmap audit for the full
// provenance/translation analysis this coverage is built on -- in
// particular why EVO, PiRaTeS, HHWEB, unkn0wn, and BiTOR+2160p are
// deliberately NOT consumed (an open upstream-policy question, a positive
// WEB T3 trust-tier conflict for HHWEB, or a lookbehind translation that is
// only empirically -- not algebraically -- exact).
//
// Two-layer validation, per project_context.md §2/§5:
//   - TestLQReleaseTitleClassification: the isolated Define fixture layer,
//     probing each new Define directly against the real engine.
//   - TestLQReleaseTitleProductionRegression: the exact production rules
//     (Movies/Shows LQ Penalty, Adaptive Low-Score Filtering) as published,
//     re-tested end to end.

import (
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"
	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

// TestLQReleaseTitleClassification proves each new Define's predicate in
// isolation: every approved token fires in its own content-kind scope, the
// jennaortega translation preserves the audited negative-hyphen semantics,
// every deliberately-excluded upstream branch (EVO, PiRaTeS, HHWEB,
// unkn0wn, BiTOR+2160p, and the plain-group duplicates already covered by
// the parsed `group` field) stays unmatched, and ordinary adjacent/fused
// words do not false-positive.
func TestLQReleaseTitleClassification(t *testing.T) {
	defineLibrary := loadDefineLibrary(t)

	probeMatches := func(t *testing.T, defineName, title string, kind string) bool {
		t.Helper()

		probe := config.RuleConfig{
			Name:   "Probe " + defineName,
			Points: -1,
			When:   `matched("` + defineName + `")`,
		}

		set, err := rules.Compile([]config.RuleConfig{probe}, defineLibrary...)
		if err != nil {
			t.Fatalf("compile probe for %q: %v", defineName, err)
		}

		cand := triage.Candidate{Release: &release.Release{Title: title}}
		env := rules.BuildEnv(cand, jhin.Parse(title), rules.Context{Kind: kind})
		out := set.Evaluate(env, kind)

		return ruleMatched(out, probe.Name)
	}

	t.Run("Movies LQ Release Title", func(t *testing.T) {
		const define = "Movies LQ Release Title"

		cases := []struct {
			name  string
			title string
			want  bool
		}{
			// Approved plain tokens, discriminating group (SomeGroup is
			// not on any group-based LQ list) so a match can only come
			// from this Define, not "Movies LQ Groups".
			{"1XBET", "Example.Movie.2026.1080p.WEB-DL.H264.1XBET-SomeGroup", true},
			{"BEN THE MEN (dot)", "Example.Movie.2026.1080p.WEB-DL.H264.BEN.THE.MEN-SomeGroup", true},
			{"R&H", "Example.Movie.2026.1080p.WEB-DL.H264.R&H-SomeGroup", true},
			{"READ NOTE (dot)", "Example.Movie.2026.1080p.WEB-DL.H264.READ.NOTE-SomeGroup", true},
			{"SWTYBLZ", "Example.Movie.2026.1080p.WEB-DL.H264.SWTYBLZ-SomeGroup", true},
			{"TeeWee", "Example.Movie.2026.1080p.WEB-DL.H264.TeeWee-SomeGroup", true},
			{"Will1869", "Example.Movie.2026.1080p.WEB-DL.H264.Will1869-SomeGroup", true},
			{"D3US as -D3US", "Example.Movie.2026.1080p.WEB-DL.H264-D3US", true},
			{"D3US as D3US-", "Example.Movie.2026.1080p.WEB-DL.H264-D3US-TGX", true},

			// jennaortega: title-embedded (not the terminal group tag)
			// must match -- the audited anti-impersonation signal.
			{"jennaortega leading title", "jennaortega.Example.Movie.2026.1080p.WEB-DL.H264-SomeGroup", true},
			{"jennaortega mid title", "Example.jennaortega.Movie.2026.1080p.WEB-DL.H264-SomeGroup", true},

			// jennaortega negative-hyphen semantics: as the terminal
			// group tag (immediately preceded by "-"), this Define must
			// NOT match -- that occurrence is already covered by the
			// parsed-group "Movies LQ Groups" Define instead.
			{"jennaortega as terminal group (excluded)", "Example.Movie.2026.1080p.WEB-DL.H264-jennaortega", false},
			{"jennaortegaUHD as terminal group (excluded)", "Example.Movie.2026.1080p.WEB-DL.H264-jennaortegaUHD", false},

			// jennaortega fused-suffix false-positive controls: the
			// upstream outer alternation's trailing \b applies to every
			// branch including this one, so a fused suffix must not
			// match either.
			{"jennaortegaX fused (no boundary)", "jennaortegaX.Example.Movie.2026.1080p.WEB-DL.H264-SomeGroup", false},
			{"jennaortegaUHDX fused (no boundary)", "jennaortegaUHDX.Example.Movie.2026.1080p.WEB-DL.H264-SomeGroup", false},

			// Deliberately excluded upstream branches must stay unmatched
			// by this Define, in every gating context upstream describes.
			{"EVO (excluded, WEB-DL)", "Example.Movie.2026.1080p.WEB-DL.H264-EVO", false},
			{"EVO (excluded, BluRay)", "Example.Movie.2026.1080p.BluRay.x264-EVO", false},
			{"PiRaTeS (excluded, WEB-DL)", "Example.Movie.2026.1080p.WEB-DL.H264-PiRaTeS", false},
			{"PiRaTeS (excluded, BluRay)", "Example.Movie.2026.1080p.BluRay.x264-PiRaTeS", false},
			{"HHWEB (excluded, MA WEB-DL)", "Example.Movie.2026.1080p.MA.WEB-DL.H264-HHWEB", false},
			{"HHWEB (excluded, plain WEB-DL)", "Example.Movie.2026.1080p.WEB-DL.H264-HHWEB", false},
			{"unkn0wn (excluded, remux)", "Example.Movie.2026.2160p.UHD.BluRay.REMUX.DV.HDR-unKn0wn", false},
			{"unkn0wn (excluded, WEB-DL)", "Example.Movie.2026.1080p.WEB-DL.H264-unKn0wn", false},

			// Plain group-based duplicates already covered via the parsed
			// `group` field are not re-added here.
			{"GalaxyRG duplicate (not re-added)", "Example.Movie.2026.1080p.WEB-DL.H264-GalaxyRG", false},
			{"Feranki1980 duplicate (not re-added)", "Example.Movie.2026.1080p.WEB-DL.H264-Feranki1980", false},
			{"TEKNO3D duplicate (not re-added)", "Example.Movie.2026.1080p.WEB-DL.H264-TEKNO3D", false},

			// Ordinary adjacent/fused-word false-positive controls.
			{"1XBETTER (fused, no boundary)", "Example.Movie.2026.1080p.WEB-DL.H264.1XBETTER-SomeGroup", false},
			{"BENJAMIN THE MENACE (near-miss)", "Example.Movie.2026.1080p.WEB-DL.H264.BENJAMIN.THE.MENACE-SomeGroup", false},
			{"SWTYBLZZZZ (fused)", "Example.Movie.2026.1080p.WEB-DL.H264.SWTYBLZZZZ-SomeGroup", false},
			{"TeeWeezer (fused)", "Example.Movie.2026.1080p.WEB-DL.H264.TeeWeezer-SomeGroup", false},
			{"ordinary clean release", "Example.Movie.2026.1080p.WEB-DL.H264-NTb", false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := probeMatches(t, define, tc.title, ranking.KindMovie); got != tc.want {
					t.Errorf("%q: %s matched=%v, want %v", tc.title, define, got, tc.want)
				}
			})
		}
	})

	t.Run("Shows LQ Release Title", func(t *testing.T) {
		const define = "Shows LQ Release Title"

		cases := []struct {
			name  string
			title string
			want  bool
		}{
			{"BEN THE MEN (dot)", "Example.Show.S01E01.1080p.WEB-DL.H264.BEN.THE.MEN-SomeGroup", true},
			{"R&H", "Example.Show.S01E01.1080p.WEB-DL.H264.R&H-SomeGroup", true},
			{"TeeWee", "Example.Show.S01E01.1080p.WEB-DL.H264.TeeWee-SomeGroup", true},
			{"CREATiVE24", "Example.Show.S01E01.1080p.WEB-DL.H264-CREATiVE24", true},

			// Feranki1980 is already covered by "Shows LQ Groups" (parsed
			// group field); not re-added to the title-based Define.
			{"Feranki1980 duplicate (not re-added)", "Example.Show.S01E01.1080p.WEB-DL.H264-Feranki1980", false},

			// BiTOR+2160p combo excluded entirely, at every resolution.
			{"BiTOR + 2160p (excluded)", "Example.Show.S01E01.2160p.WEB-DL.H265-BiTOR", false},
			{"BiTOR + 1080p (excluded)", "Example.Show.S01E01.1080p.WEB-DL.H265-BiTOR", false},

			// False-positive controls.
			{"CREATiVE24X (fused)", "Example.Show.S01E01.1080p.WEB-DL.H264-CREATiVE24X", false},
			{"TeeWeezer (fused)", "Example.Show.S01E01.1080p.WEB-DL.H264.TeeWeezer-SomeGroup", false},
			{"ordinary clean release", "Example.Show.S01E01.1080p.WEB-DL.H264-NTb", false},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := probeMatches(t, define, tc.title, ranking.KindSeries); got != tc.want {
					t.Errorf("%q: %s matched=%v, want %v", tc.title, define, got, tc.want)
				}
			})
		}
	})

	// Existing group-based LQ behavior is unaffected: a pre-existing
	// group-based token still fires its own Define, and does not
	// spuriously fire the new title-based Define.
	t.Run("existing group-based LQ unchanged", func(t *testing.T) {
		movieTitle := "Example.Movie.2026.1080p.WEB-DL.H264-PSA"
		if !probeMatches(t, "Movies LQ Groups", movieTitle, ranking.KindMovie) {
			t.Error("Movies LQ Groups regressed: PSA no longer matches")
		}
		if probeMatches(t, "Movies LQ Release Title", movieTitle, ranking.KindMovie) {
			t.Error("Movies LQ Release Title unexpectedly matched a plain group-based PSA release")
		}

		showTitle := "Example.Show.S01E01.1080p.WEB-DL.H264-MeGusta"
		if !probeMatches(t, "Shows LQ Groups", showTitle, ranking.KindSeries) {
			t.Error("Shows LQ Groups regressed: MeGusta no longer matches")
		}
		if probeMatches(t, "Shows LQ Release Title", showTitle, ranking.KindSeries) {
			t.Error("Shows LQ Release Title unexpectedly matched a plain group-based MeGusta release")
		}
	})
}

// TestLQReleaseTitleProductionRegression proves the exact published rules
// consume the new Defines as intended: Movies/Shows LQ Penalty still score
// -10000 for a release matched only via the title-derived Define, and
// Adaptive Low-Score Filtering's candidate-relative headroom/count
// condition, Library protection, and sparse-pool (same-release) fallback
// all continue to hold identically to the pre-existing group-based case.
func TestLQReleaseTitleProductionRegression(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	moviesPenalty := findProductionRule(t, productionRules, "Movies LQ Penalty")
	const wantMoviesPenaltyWhen = `matched("Movies LQ Groups") or matched("Movies LQ Release Title")`
	if moviesPenalty.When != wantMoviesPenaltyWhen {
		t.Fatalf(
			"Movies LQ Penalty predicate mismatch:\ngot:  %s\nwant: %s",
			moviesPenalty.When,
			wantMoviesPenaltyWhen,
		)
	}
	if moviesPenalty.Points != -10000 || moviesPenalty.EffectiveAction() != config.RuleActionScore {
		t.Fatalf(
			"Movies LQ Penalty points=%d action=%q, want -10000/score",
			moviesPenalty.Points,
			moviesPenalty.EffectiveAction(),
		)
	}

	showsPenalty := findProductionRule(t, productionRules, "Shows LQ Penalty")
	const wantShowsPenaltyWhen = `matched("Shows LQ Groups") or matched("Shows LQ Release Title")`
	if showsPenalty.When != wantShowsPenaltyWhen {
		t.Fatalf(
			"Shows LQ Penalty predicate mismatch:\ngot:  %s\nwant: %s",
			showsPenalty.When,
			wantShowsPenaltyWhen,
		)
	}
	if showsPenalty.Points != -10000 || showsPenalty.EffectiveAction() != config.RuleActionScore {
		t.Fatalf(
			"Shows LQ Penalty points=%d action=%q, want -10000/score",
			showsPenalty.Points,
			showsPenalty.EffectiveAction(),
		)
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "LQ Release Title production regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	makeCandidate := func(title string, library bool) triage.Candidate {
		return triage.Candidate{
			Release: &release.Release{
				Title:     title,
				IsLibrary: library,
			},
		}
	}

	matchedNames := func(kept []ranking.Result) []string {
		if len(kept) == 0 {
			return nil
		}
		names := make([]string, 0, len(kept[0].Matched))
		for _, m := range kept[0].Matched {
			names = append(names, m.Name)
		}
		return names
	}

	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	t.Run("Movies LQ Penalty fires for title-derived-only match", func(t *testing.T) {
		title := "Example.Movie.2026.1080p.WEB-DL.H264.1XBET-SomeGroup"

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
			[]triage.Candidate{makeCandidate(title, false)},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf("kept=%d rejected=%d, want kept=1 rejected=0", len(kept), len(rejected))
		}

		if !contains(matchedNames(kept), "Movies LQ Penalty") {
			t.Fatal("Movies LQ Penalty did not fire for a title-derived-only LQ release")
		}
	})

	t.Run("Shows LQ Penalty fires for title-derived-only match", func(t *testing.T) {
		title := "Example.Show.S01E01.1080p.WEB-DL.H264-CREATiVE24"

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{Kind: ranking.KindSeries, Title: "Example Show", Season: 1, Episode: 1},
			[]triage.Candidate{makeCandidate(title, false)},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf("kept=%d rejected=%d, want kept=1 rejected=0", len(kept), len(rejected))
		}

		if !contains(matchedNames(kept), "Shows LQ Penalty") {
			t.Fatal("Shows LQ Penalty did not fire for a title-derived-only LQ release")
		}
	})

	t.Run("dense Movie tail with a title-derived LQ candidate is pruned", func(t *testing.T) {
		candidates := []triage.Candidate{
			makeCandidate("Example.Movie.2026.2160p.WEB-DL.H265-FLUX", false),
			makeCandidate("Example.Movie.2026.2160p.WEB-DL.H265-NTb", false),
			makeCandidate("Example.Movie.2026.1080p.BluRay.REMUX.AVC-HiFi", false),
			makeCandidate("Example.Movie.2026.1080p.WEB-DL.H264-FLUX", false),
			makeCandidate("Example.Movie.2026.1080p.WEB-DL.H264-NTb", false),
			makeCandidate("Example.Movie.2026.720p.WEB-DL.H264-FLUX", false),
			makeCandidate("Example.Movie.2026.720p.WEB-DL.H264.1XBET-SomeGroup", false),
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
			candidates,
			jhinrank.RankOptions{},
		)

		if len(kept) != 6 || len(rejected) != 1 {
			t.Fatalf(
				"dense pool: kept=%d rejected=%d, want kept=6 rejected=1",
				len(kept),
				len(rejected),
			)
		}

		if rejected[0].Candidate.Release == nil ||
			rejected[0].Candidate.Release.Title != candidates[len(candidates)-1].Release.Title {
			t.Fatalf("unexpected pruned candidate: %+v", rejected[0].Candidate.Release)
		}
	})

	t.Run("sparse pool retains the title-derived LQ candidate", func(t *testing.T) {
		candidates := []triage.Candidate{
			makeCandidate("Example.Movie.2026.2160p.WEB-DL.H265-FLUX", false),
			makeCandidate("Example.Movie.2026.720p.WEB-DL.H264.1XBET-SomeGroup", false),
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
			candidates,
			jhinrank.RankOptions{},
		)

		if len(kept) != 2 || len(rejected) != 0 {
			t.Fatalf(
				"sparse pool: kept=%d rejected=%d, want kept=2 rejected=0",
				len(kept),
				len(rejected),
			)
		}
	})

	t.Run("Library candidate is protected from adaptive pruning", func(t *testing.T) {
		candidates := []triage.Candidate{
			makeCandidate("Example.Movie.2026.2160p.WEB-DL.H265-FLUX", false),
			makeCandidate("Example.Movie.2026.2160p.WEB-DL.H265-NTb", false),
			makeCandidate("Example.Movie.2026.1080p.BluRay.REMUX.AVC-HiFi", false),
			makeCandidate("Example.Movie.2026.1080p.WEB-DL.H264-FLUX", false),
			makeCandidate("Example.Movie.2026.1080p.WEB-DL.H264-NTb", false),
			makeCandidate("Example.Movie.2026.720p.WEB-DL.H264-FLUX", false),
			// Same title-derived LQ signal as the dense-pool case above,
			// but Library-attached: must survive pruning.
			makeCandidate("Example.Movie.2026.720p.WEB-DL.H264.1XBET-SomeGroup", true),
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
			candidates,
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 || len(kept) != 7 {
			t.Fatalf(
				"library-protected pool: kept=%d rejected=%d, want kept=7 rejected=0",
				len(kept),
				len(rejected),
			)
		}
	})
}
