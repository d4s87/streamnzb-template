package streamnzb_compat

// Permanent real-engine regression for "Adaptive low-quality filtering" and
// its Anime sibling "Anime Adaptive Low-Quality Filtering": reject rules for
// HDRip/DVDRip/HDTV-sourced releases, adaptive on result-set density
// (rejects only when more than six recognized-resolution, recognized-source
// alternatives exist). See the adaptive/protective rule interaction audit
// (2026-09-09) for provenance: the rule originally shipped as one
// isAnime-unscoped rule with no SeaDex Best/Alternative exemption and no
// fixture/production regression coverage, so a SeaDex-recommended Anime
// release with an HDTV source could be silently hard-rejected in a dense
// pool despite SeaDex's documented "dominant protected recommendation"
// status.
//
// The fix is two separate rules, not one rule with an inline isAnime
// exemption. StreamNZB/Jhin's rule engine skips a rule outright whenever it
// references any field in an unanswered "tier" (see
// pkg/search/rules/rules.go's tier registry and the "needs a SeaDex lookup"
// skip reason) -- this is a static per-rule check on the compiled
// expression, not a runtime short-circuit, so wrapping the seadex read in
// `isAnime and (...)` does NOT stop the rule from requiring SeaDex data for
// every candidate, Anime or not. Verified directly: with that inline
// wrapping, a Movie HDTV release in a dense pool (nil Seadex, which is what
// every real Movie/Series request actually carries -- SeaDex is only ever
// resolved for Kitsu-addressed requests, see
// pkg/server/stremio/seadex.go:seadexContext) stopped being rejected
// entirely, silently disabling the rule's real-world job. Splitting into two
// independently named rules keeps "Adaptive low-quality filtering"
// completely free of any seadex.* reference (so Movie/Series behavior is
// provably unchanged) and confines the new fail-open-on-missing-SeaDex
// exposure to the new Anime-only rule, where it is correct and intended
// (mirrors "Unknown resolution" and "Anime LQ Penalty").

import (
	"fmt"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
)

const (
	adaptiveLowQualityFilteringRuleName      = "Adaptive low-quality filtering"
	animeAdaptiveLowQualityFilteringRuleName = "Anime Adaptive Low-Quality Filtering"
)

// TestAdaptiveLowQualityFilteringRuleContract locks in both rules' shapes so
// a future edit cannot silently reintroduce a seadex.* reference on the
// Movie/Series rule, or narrow/widen the Anime rule's SeaDex exemption.
func TestAdaptiveLowQualityFilteringRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)

	t.Run("Movie/Series rule carries no SeaDex reference", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, adaptiveLowQualityFilteringRuleName)

		if cfg.EffectiveAction() != config.RuleActionReject {
			t.Fatalf(
				"production rule %q action = %q, want reject",
				adaptiveLowQualityFilteringRuleName,
				cfg.EffectiveAction(),
			)
		}

		const expectedWhen = `not isAnime
and not library
and (
    "hdrip" in traits
    or "dvdrip" in traits
    or "hdtv" in traits
)
and count(
    (
        resolution == "2160p"
        or resolution == "1440p"
        or resolution == "1080p"
        or resolution == "720p"
    )
    and (
        "remux" in traits
        or "bluray" in traits
        or "webdl" in traits
    )
) > 6`

		if cfg.When != expectedWhen {
			t.Fatalf(
				"%s predicate mismatch:\ngot:\n%s\n\nwant:\n%s",
				adaptiveLowQualityFilteringRuleName,
				cfg.When,
				expectedWhen,
			)
		}
	})

	t.Run("Anime rule carries the SeaDex exemption", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, animeAdaptiveLowQualityFilteringRuleName)

		if cfg.EffectiveAction() != config.RuleActionReject {
			t.Fatalf(
				"production rule %q action = %q, want reject",
				animeAdaptiveLowQualityFilteringRuleName,
				cfg.EffectiveAction(),
			)
		}

		const expectedWhen = `isAnime
and not library
and not (seadex.best or seadex.alternative)
and (
    "hdrip" in traits
    or "dvdrip" in traits
    or "hdtv" in traits
)
and count(
    (
        resolution == "2160p"
        or resolution == "1440p"
        or resolution == "1080p"
        or resolution == "720p"
    )
    and (
        "remux" in traits
        or "bluray" in traits
        or "webdl" in traits
    )
) > 6`

		if cfg.When != expectedWhen {
			t.Fatalf(
				"%s predicate mismatch:\ngot:\n%s\n\nwant:\n%s",
				animeAdaptiveLowQualityFilteringRuleName,
				cfg.When,
				expectedWhen,
			)
		}
	})
}

// TestAdaptiveLowQualityFilteringProductionPolicy protects the shipped rules
// against the real StreamNZB/Jhin engine: ordinary dense/sparse adaptive
// behavior (unchanged for Movie/Series by the audit fix) plus the new Anime
// SeaDex Best/Alternative protection, exercised through the same
// ranking.Request.Seadex path production actually uses (mirrors
// TestIntelligentUnknownResolutionProductionPolicy's approach).
func TestAdaptiveLowQualityFilteringProductionPolicy(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Adaptive low-quality filtering production regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	knownMovieAlternatives := func(n int) []string {
		out := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, fmt.Sprintf(
				"Example.Movie.%02d.2026.1080p.WEB-DL.x264-ALT%d", i, i,
			))
		}
		return out
	}

	knownAnimeAlternatives := func(n int) []string {
		out := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, fmt.Sprintf(
				"Example.Anime.S01E%02d.1080p.WEB-DL.x264-ALT%d", i+1, i,
			))
		}
		return out
	}

	rejectedBy := func(t *testing.T, explanations []*ranking.Explanation, target, ruleName string) bool {
		t.Helper()
		for _, e := range explanations {
			if e.Title != target {
				continue
			}
			for _, r := range e.Rejections {
				if r == "rule: "+ruleName {
					return true
				}
			}
			return false
		}
		t.Fatalf("target release missing from explanations: %q", target)
		return false
	}

	t.Run("dense Movie HDTV pool is rejected", func(t *testing.T) {
		target := "Example.Movie.2026.HDTV.x264-GRP"
		titles := append([]string{target}, knownMovieAlternatives(7)...)
		req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if !rejectedBy(t, explanations, target, adaptiveLowQualityFilteringRuleName) {
			t.Fatal("dense HDTV Movie was not rejected")
		}
	})

	t.Run("sparse Movie HDTV pool survives as fallback", func(t *testing.T) {
		target := "Example.Movie.2026.HDTV.x264-GRP"
		titles := append([]string{target}, knownMovieAlternatives(3)...)
		req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, adaptiveLowQualityFilteringRuleName) {
			t.Fatal("sparse HDTV Movie was unexpectedly rejected")
		}
	})

	t.Run("Library protects dense Movie HDTV pool", func(t *testing.T) {
		target := "Example.Movie.2026.HDTV.x264-GRP"
		titles := append([]string{target}, knownMovieAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindMovie, Title: "Example Movie",
			Sample: &ranking.Sample{IndexerData: true, Library: true},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, adaptiveLowQualityFilteringRuleName) {
			t.Fatal("Library HDTV release was rejected")
		}
	})

	t.Run("Movie HDTV pool carries no SeaDex dependency (realistic nil-Seadex wiring)", func(t *testing.T) {
		// req.Seadex is nil for every real Movie/Series request. This is the
		// permanent guard against ever re-adding a seadex.* reference to the
		// shared Movie/Series rule -- see the package doc comment above for
		// why that would silently disable it instead of narrowly protecting
		// Anime.
		target := "Example.Movie.2026.HDTV.x264-GRP"
		titles := append([]string{target}, knownMovieAlternatives(7)...)
		req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"} // Seadex left nil
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if !rejectedBy(t, explanations, target, adaptiveLowQualityFilteringRuleName) {
			t.Fatal("dense Movie HDTV pool was not rejected under realistic nil-Seadex wiring")
		}
	})

	t.Run("dense Anime HDTV pool is rejected when SeaDex checked no match", func(t *testing.T) {
		// A resolved-but-empty SeaDex lookup (Kitsu mapping found, no
		// cataloged entry) answers non-nil with Known:false -- see
		// pkg/server/stremio/seadex.go:seadexContext's `if entry == nil {
		// return &rules.SeadexContext{} }` branch. This is the ordinary case
		// for a genuine Anime search with SeaDex integration active.
		target := "Example.Anime.S01E01.HDTV.x264-GRP"
		titles := append([]string{target}, knownAnimeAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title:  "Example Anime",
			Seadex: &rules.SeadexContext{Known: false},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if !rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("dense Anime HDTV release with SeaDex checked/no-match was not rejected")
		}
	})

	t.Run("dense Anime HDTV pool survives when SeaDex lookup never ran (fails open)", func(t *testing.T) {
		// req.Seadex nil means no lookup ran at all -- disabled integration,
		// no Kitsu mapping, or an unreachable SeaDex API -- distinct from a
		// resolved "checked, no match" above. The rule must fail open here,
		// exactly like "Unknown resolution"'s own documented fail-open case.
		target := "Example.Anime.S01E01.HDTV.x264-GRP"
		titles := append([]string{target}, knownAnimeAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title: "Example Anime",
			// Seadex intentionally left nil.
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("dense Anime HDTV release with no SeaDex lookup was rejected instead of failing open")
		}
	})

	t.Run("sparse Anime HDTV pool survives as fallback", func(t *testing.T) {
		// SeaDex checked/no-match, not nil, so this proves the survival is
		// genuinely the sparse-pool density logic and not an incidental
		// SeaDex fail-open.
		target := "Example.Anime.S01E01.HDTV.x264-GRP"
		titles := append([]string{target}, knownAnimeAlternatives(3)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title:  "Example Anime",
			Seadex: &rules.SeadexContext{Known: false},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("sparse Anime HDTV release was unexpectedly rejected")
		}
	})

	t.Run("SeaDex Best protects dense Anime HDTV pool", func(t *testing.T) {
		target := "Example.Anime.S01E01.HDTV.x264-BESTGRP"
		titles := append([]string{target}, knownAnimeAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title: "Example Anime",
			Seadex: &rules.SeadexContext{
				Known: true,
				Best:  map[string]bool{"bestgrp": true},
			},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("SeaDex Best HDTV release was rejected")
		}
		for _, e := range explanations {
			if e.Title == target && !e.Fetch {
				t.Fatalf("SeaDex Best HDTV release was not kept: rejections=%v", e.Rejections)
			}
		}
	})

	t.Run("SeaDex Alternative protects dense Anime HDTV pool", func(t *testing.T) {
		target := "Example.Anime.S01E01.HDTV.x264-ALTGRP"
		titles := append([]string{target}, knownAnimeAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title: "Example Anime",
			Seadex: &rules.SeadexContext{
				Known: true,
				Alt:   map[string]bool{"altgrp": true},
			},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("SeaDex Alternative HDTV release was rejected")
		}
		for _, e := range explanations {
			if e.Title == target && !e.Fetch {
				t.Fatalf("SeaDex Alternative HDTV release was not kept: rejections=%v", e.Rejections)
			}
		}
	})

	t.Run("Library protects dense Anime HDTV pool", func(t *testing.T) {
		// SeaDex checked/no-match, not nil, so this proves the survival is
		// genuinely the Library exemption and not an incidental SeaDex
		// fail-open.
		target := "Example.Anime.S01E01.HDTV.x264-GRP"
		titles := append([]string{target}, knownAnimeAlternatives(7)...)
		req := ranking.Request{
			Kind: ranking.KindAnimeShow, IsAnime: true, Season: 1, Episode: 1,
			Title:  "Example Anime",
			Sample: &ranking.Sample{IndexerData: true, Library: true},
			Seadex: &rules.SeadexContext{Known: false},
		}
		explanations, _ := profile.Explain(titles, req, jhinrank.RankOptions{})
		if rejectedBy(t, explanations, target, animeAdaptiveLowQualityFilteringRuleName) {
			t.Fatal("Library Anime HDTV release was rejected")
		}
	})
}
