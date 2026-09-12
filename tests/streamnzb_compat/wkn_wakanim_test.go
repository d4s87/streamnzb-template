package streamnzb_compat

// Permanent real-engine regression for the "WKN / Wakanim" Anime service
// fallback: a 12th Anime service rule, zero-point, presentation-only,
// mirroring the pre-existing CR/DSNP/NF/AMZN/VRV/FUNi/ABEMA/ADN/B-Global/
// Bilibili/HIDIVE shape exactly. See backlog-roadmap.md's WKN/Wakanim
// refresh audit for the full provenance analysis (upstream Vidhin score is
// 0, no native Jhin v0.6.2 `.Network` value exists for Wakanim, bare "Waka"
// is deliberately excluded despite upstream itself offering it as an
// alternative). No Define is used or added -- see build_profiles.py's
// EXPECTED_PRESENTATION_RULES for the exact set.
//
// Two-layer validation, per project_context.md §2/§5:
//   - TestWKNRuleContract / TestWKNClassification: the isolated-rule layer.
//   - TestWKNScoringInvariance: the exact published rule, run through the
//     full production pipeline, proving zero score/keep/reject impact.

import (
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"
	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

// TestWKNRuleContract proves the exact, minimal shape of the new
// presentation rule against the actual published Samsung profile: zero
// points, score action (no reject/limit).
func TestWKNRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, "WKN")

	if cfg.Points != 0 {
		t.Errorf("WKN points = %d, want 0", cfg.Points)
	}
	if cfg.EffectiveAction() != config.RuleActionScore {
		t.Errorf(
			"WKN action = %q, want score (presentation-only, no reject/limit)",
			cfg.EffectiveAction(),
		)
	}
}

// TestWKNClassification is the production-regression layer: the exact
// published rule (decoded from profile.txt, not a hand-copied fixture) run
// over positive, negative, fused-token, non-WEB, and non-Anime release
// names through the real StreamNZB rule engine.
func TestWKNClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, "WKN")

	cases := []struct {
		name    string
		title   string
		isAnime bool
		want    bool
	}{
		{
			"positive WKN, Anime WEB",
			"Example.Anime.S01E01.1080p.WEB-DL.WKN-SomeGroup",
			true, true,
		},
		{
			"positive Wakanim, Anime WEB",
			"Example.Anime.S01E01.1080p.WEB-DL.Wakanim-SomeGroup",
			true, true,
		},
		{
			"bare Waka must not match (deliberately excluded)",
			"Example.Anime.S01E01.1080p.WEB-DL.Waka-SomeGroup",
			true, false,
		},
		{
			"fused suffix WKNX must not match",
			"Example.Anime.S01E01.1080p.WEB-DL.WKNX-SomeGroup",
			true, false,
		},
		{
			"fused prefix XWKN must not match",
			"Example.Anime.S01E01.1080p.WEB-DL.XWKN-SomeGroup",
			true, false,
		},
		{
			"unrelated Waka-prefixed word must not match",
			"Example.Anime.S01E01.1080p.WEB-DL.Wakandaverse-SomeGroup",
			true, false,
		},
		{
			"WKN present but no qualifying WEB-family trait (BluRay)",
			"Example.Anime.S01E01.1080p.BluRay.WKN-SomeGroup",
			true, false,
		},
		{
			"non-Anime release must not receive WKN classification",
			"Example.Movie.2026.1080p.WEB-DL.WKN-SomeGroup",
			false, false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evaluateIsolatedRule(t, cfg, tc.title, tc.isAnime); got != tc.want {
				t.Errorf("%q (isAnime=%v): WKN matched=%v, want %v", tc.title, tc.isAnime, got, tc.want)
			}
		})
	}
}

// TestWKNScoringInvariance proves, through the real full-profile pipeline,
// that a matching WKN/Wakanim release changes only presentation
// (MatchedRules) and never final score, Fetch/keep state, or rejection --
// including when a native Jhin `.Network` value (Crunchyroll) is present
// alongside the WKN text, which must keep scoring independently at zero.
func TestWKNScoringInvariance(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "WKN scoring invariance",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	rankOf := func(title string) (rank int, matchedNames []string, network string, kept int, rejected int) {
		t.Helper()

		parsed := jhin.Parse(title)
		cand := triage.Candidate{Release: &release.Release{Title: title}}
		request := ranking.Request{
			Kind: ranking.KindAnimeShow, Title: "Example Anime",
			Season: 1, Episode: 1, IsAnime: true,
		}

		keptResults, rejectedResults := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{cand},
			jhinrank.RankOptions{},
		)

		if len(keptResults) != 1 {
			return 0, nil, parsed.Network, len(keptResults), len(rejectedResults)
		}

		names := make([]string, 0, len(keptResults[0].Matched))
		for _, m := range keptResults[0].Matched {
			names = append(names, m.Name)
		}

		return keptResults[0].Torrent.Rank, names, parsed.Network, len(keptResults), len(rejectedResults)
	}

	clean := "Example.Anime.S01E01.1080p.WEB-DL.H264-SomeGroup"
	withWKN := "Example.Anime.S01E01.1080p.WEB-DL.WKN-SomeGroup"
	withWakanim := "Example.Anime.S01E01.1080p.WEB-DL.Wakanim-SomeGroup"
	withNetworkAndWKN := "Example.Anime.S01E01.1080p.WEB-DL.Crunchyroll.WKN-SomeGroup"

	cleanRank, cleanMatched, _, cleanKept, cleanRejected := rankOf(clean)

	t.Run("WKN adds zero score", func(t *testing.T) {
		rank, matched, network, kept, rejected := rankOf(withWKN)
		if contains(cleanMatched, "WKN") {
			t.Fatal("clean release unexpectedly matched WKN")
		}
		if !contains(matched, "WKN") {
			t.Fatal("WKN release did not match the WKN rule; nothing to prove invariance over")
		}
		if network != "" {
			t.Errorf("Wakanim/WKN has no native Jhin .Network value; got %q", network)
		}
		if rank != cleanRank {
			t.Errorf("final score changed by WKN: clean=%d wkn=%d (want equal)", cleanRank, rank)
		}
		if kept != cleanKept || kept != 1 {
			t.Errorf("Fetch/keep state changed by WKN: clean kept=%d wkn kept=%d (want 1/1)", cleanKept, kept)
		}
		if rejected != cleanRejected || rejected != 0 {
			t.Errorf("rejection behavior changed by WKN: clean rejected=%d wkn rejected=%d (want 0/0)", cleanRejected, rejected)
		}
	})

	t.Run("Wakanim adds zero score", func(t *testing.T) {
		rank, matched, _, kept, rejected := rankOf(withWakanim)
		if !contains(matched, "WKN") {
			t.Fatal("Wakanim release did not match the WKN rule")
		}
		if rank != cleanRank {
			t.Errorf("final score changed by Wakanim: clean=%d wakanim=%d (want equal)", cleanRank, rank)
		}
		if kept != 1 || rejected != 0 {
			t.Errorf("keep/reject state changed by Wakanim: kept=%d rejected=%d (want 1/0)", kept, rejected)
		}
	})

	t.Run("native Network alongside WKN text: WKN still scores zero, CR scores independently", func(t *testing.T) {
		rank, matched, network, kept, rejected := rankOf(withNetworkAndWKN)
		if network != "Crunchyroll" {
			t.Fatalf("expected parsed Network=Crunchyroll, got %q", network)
		}
		if !contains(matched, "WKN") {
			t.Fatal("expected WKN to still match alongside a native Network value")
		}
		if !contains(matched, "CR") {
			t.Fatal("expected CR to independently match its own native-Network-adjacent title token")
		}
		crRank, _, _, _, _ := rankOf("Example.Anime.S01E01.1080p.WEB-DL.Crunchyroll-SomeGroup")
		if rank != crRank {
			t.Errorf(
				"adding WKN text alongside an existing CR match changed score: cr-only=%d cr+wkn=%d (want equal)",
				crRank, rank,
			)
		}
		if kept != 1 || rejected != 0 {
			t.Errorf("keep/reject state changed: kept=%d rejected=%d (want 1/0)", kept, rejected)
		}
	})
}

// TestWKNDefineLibraryUnaffected asserts this feature added no Define, per
// the approved design (hand-written rule, no sync_vidhin.py involvement).
func TestWKNDefineLibraryUnaffected(t *testing.T) {
	defineLibrary := loadDefineLibrary(t)

	if len(defineLibrary) != 62 {
		t.Fatalf(
			"generated Define Library has %d entries, want 62 -- WKN/Wakanim "+
				"must not add any Define",
			len(defineLibrary),
		)
	}
}
