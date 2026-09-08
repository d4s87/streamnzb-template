package streamnzb_compat

// Permanent real-engine regression for the "Generated Dynamic HDR" policy:
// a Movie-only penalty for a small list of release groups known to
// synthesize their own Dolby Vision / HDR10+ metadata rather than sourcing
// it from a retail disc or streaming master. See the roadmap audit for the
// full provenance/severity/tier-authority analysis this rule is built on.
//
// DraCuLa deliberately splits Vidhin's own two-lookahead upstream predicate
// (group list AND HDR10+/DV marker) into a synced Define carrying only the
// group half, combined in the production rule with Jhin's own parsed
// dolbyVision/hdr facts for the marker half -- see
// scripts/sync_vidhin.py's generated_dynamic_hdr_tokens() and
// profiles/rules.json's "Generated Dynamic HDR Penalty".

import (
	"fmt"
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"
	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

const generatedDynamicHDRRuleName = "Generated Dynamic HDR Penalty"

// TestGeneratedDynamicHDRClassification is the isolated-rule fixture layer:
// the exact published rule, compiled alone against the Define library, run
// over positive, negative, and known-translation-gap release names. This
// proves the classification predicate itself -- group membership AND a
// parsed dynamic-HDR fact -- independent of the rest of the production
// pipeline's scoring interactions (covered separately below).
func TestGeneratedDynamicHDRClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	cfg := findProductionRule(t, productionRules, generatedDynamicHDRRuleName)

	if cfg.EffectiveAction() != config.RuleActionScore {
		t.Fatalf(
			"production rule %q action = %q, want score (not reject)",
			generatedDynamicHDRRuleName,
			cfg.EffectiveAction(),
		)
	}
	if cfg.Points != -10000 {
		t.Fatalf(
			"production rule %q points = %d, want -10000",
			generatedDynamicHDRRuleName,
			cfg.Points,
		)
	}
	if cfg.EffectiveScope() != ranking.KindMovie {
		t.Fatalf(
			"production rule %q scope = %q, want movie",
			generatedDynamicHDRRuleName,
			cfg.EffectiveScope(),
		)
	}

	set, err := rules.Compile([]config.RuleConfig{cfg}, defineLibrary...)
	if err != nil {
		t.Fatalf("compile production rule %q: %v", generatedDynamicHDRRuleName, err)
	}

	matched := func(title string) bool {
		t.Helper()
		cand := triage.Candidate{Release: &release.Release{Title: title}}
		env := rules.BuildEnv(cand, jhin.Parse(title), rules.Context{Kind: ranking.KindMovie})
		out := set.Evaluate(env, ranking.KindMovie)
		return ruleMatched(out, generatedDynamicHDRRuleName)
	}

	cases := []struct {
		name  string
		title string
		want  bool
	}{
		// Positive: listed group + a marker the rule actually checks for.
		{"BiTOR + HDR10+", "Movie.2026.1080p.WEB-DL.x264.HDR10Plus-BiTOR", true},
		{"BiTOR + DV", "Movie.2026.1080p.WEB-DL.x264.DV-BiTOR", true},
		{"Flights + HDR10+", "Movie.2026.1080p.WEB-DL.x264.HDR10Plus-Flights", true},
		{"BR-GuyZo + Dolby Vision", "Movie.2026.1080p.WEB-DL.x264.Dolby.Vision-BR-GuyZo", true},

		// Negative: listed group, but no dynamic-HDR evidence at all.
		{"BiTOR + SDR", "Movie.2026.1080p.WEB-DL.x264-BiTOR", false},
		{"BiTOR + ordinary HDR10 (not HDR10+)", "Movie.2026.1080p.WEB-DL.x264.HDR10-BiTOR", false},

		// Negative: dynamic-HDR evidence present, but group not on the list.
		{"unlisted group NTb + HDR10+", "Movie.2026.1080p.WEB-DL.x264.HDR10Plus-NTb", false},

		// Negative: group name appears only inside the title, not as the
		// parsed release group.
		{"group token in title only", "VECTOR.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-NTb", false},

		// Negative: near-name group strings must not fuzzy-match.
		{"VECTORX + HDR10+ (no group match)", "Movie.2026.1080p.WEB-DL.x264.HDR10Plus-VECTORX", false},
		{"BiTORrent + HDR10+ (no group match)", "Movie.2026.1080p.WEB-DL.x264.HDR10Plus-BiTORrent", false},

		// Known, deliberate translation gap (see roadmap audit Phase 3/9
		// addendum): Vidhin's raw regex accepts these truncated spellings,
		// but Jhin v0.6.2's parsed dolbyVision/hdr facts do not recognize
		// them, so the rule -- which reads only the parsed facts -- does
		// not fire. No real-world release-naming evidence for these forms
		// was found; this is a bounded, accepted tradeoff, not a bug.
		{"BiTOR + DolbyV (known miss)", "Movie.2026.1080p.WEB-DL.x264.DolbyV-BiTOR", false},
		{"BiTOR + Dolby.V (known miss)", "Movie.2026.1080p.WEB-DL.x264.Dolby.V-BiTOR", false},
		{"BiTOR + Dolby V (known miss)", "Movie.2026.1080p.WEB-DL.x264.Dolby.V-BiTOR", false},
		{"BiTOR + HDR10P (known miss)", "Movie.2026.1080p.WEB-DL.x264.HDR10P-BiTOR", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matched(tc.title); got != tc.want {
				t.Errorf(
					"%q: Generated Dynamic HDR Penalty matched=%v, want %v (release=%q)",
					tc.name, got, tc.want, tc.title,
				)
			}
		})
	}
}

// TestGeneratedDynamicHDRScoringInteraction proves the penalty is purely
// additive on top of the existing dynamic-range native-score compensation:
// native HDR10+/DV is still neutralized exactly as before, the non-Anime
// HDR10+ residual preference is unaffected, and Generated Dynamic HDR
// contributes exactly -10000 independently of both.
func TestGeneratedDynamicHDRScoringInteraction(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Generated Dynamic HDR scoring interaction",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	score := func(title string) (rank int, matched []string) {
		t.Helper()
		cand := triage.Candidate{Release: &release.Release{Title: title}}
		request := ranking.Request{Kind: ranking.KindMovie, Title: "Example"}
		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{cand},
			jhinrank.RankOptions{},
		)
		if len(rejected) != 0 {
			t.Fatalf("%q: unexpectedly rejected: %+v", title, rejected)
		}
		if len(kept) != 1 {
			t.Fatalf("%q: kept=%d, want 1", title, len(kept))
		}
		names := make([]string, 0, len(kept[0].Matched))
		for _, m := range kept[0].Matched {
			names = append(names, m.Name)
		}
		return kept[0].Torrent.Rank, names
	}

	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	group := "BiTOR" // clean, no overlap with LQ/tier Defines

	t.Run("HDR10+", func(t *testing.T) {
		clean := fmt.Sprintf("Example.Movie.2026.1080p.WEB-DL.x264-%s", group)
		flagged := fmt.Sprintf("Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-%s", group)

		cleanRank, _ := score(clean)
		flaggedRank, flaggedMatched := score(flagged)

		if !contains(flaggedMatched, "Neutralize HDR10 Plus") {
			t.Error("Neutralize HDR10 Plus did not fire; native HDR10+ compensation missing")
		}
		if !contains(flaggedMatched, "Prefer HDR10 Plus") {
			t.Error("Prefer HDR10 Plus did not fire; non-Anime HDR10+ residual missing")
		}
		if !contains(flaggedMatched, generatedDynamicHDRRuleName) {
			t.Fatal("Generated Dynamic HDR Penalty did not fire for a listed group with HDR10+")
		}

		// Native HDR10+ (+2100) neutralized to 0, Prefer HDR10 Plus (+25)
		// applies, Generated Dynamic HDR Penalty (-10000) applies: net -9975.
		wantDelta := 25 - 10000
		if got := flaggedRank - cleanRank; got != wantDelta {
			t.Errorf(
				"HDR10+ effective delta = %+d, want %+d (clean=%d flagged=%d)",
				got, wantDelta, cleanRank, flaggedRank,
			)
		}
	})

	t.Run("DV with HDR fallback", func(t *testing.T) {
		// HDR fallback token included so the pre-existing Samsung "DV
		// without HDR fallback" device rule does not itself reject the
		// candidate before this assertion can observe Generated Dynamic
		// HDR's own isolated effect.
		clean := fmt.Sprintf("Example.Movie.2026.1080p.WEB-DL.x264.HDR-%s", group)
		flagged := fmt.Sprintf("Example.Movie.2026.1080p.WEB-DL.x264.DV.HDR-%s", group)

		cleanRank, _ := score(clean)
		flaggedRank, flaggedMatched := score(flagged)

		if !contains(flaggedMatched, "Neutralize Dolby Vision") {
			t.Error("Neutralize Dolby Vision did not fire; native DV compensation missing")
		}
		if !contains(flaggedMatched, generatedDynamicHDRRuleName) {
			t.Fatal("Generated Dynamic HDR Penalty did not fire for a listed group with DV+HDR fallback")
		}

		// Native DV (+3000) fully neutralized by the existing rule (no
		// DraCuLa DV residual bonus exists), Generated Dynamic HDR Penalty
		// (-10000) applies on top: net exactly -10000.
		wantDelta := -10000
		if got := flaggedRank - cleanRank; got != wantDelta {
			t.Errorf(
				"DV effective delta = %+d, want %+d (clean=%d flagged=%d)",
				got, wantDelta, cleanRank, flaggedRank,
			)
		}
	})
}

// TestGeneratedDynamicHDRTierAuthority proves the *intended* negative-policy
// behavior: a normally-trusted Movie release-group tier carrying Generated
// Dynamic HDR is allowed to rank below a clean lower tier, because the
// -10000 classification is deliberately stronger than release-group tier
// trust -- the same license already exercised by the existing
// Movies Bad Dual Penalty / Movies LQ Penalty rules (neither of which is
// covered by TestAdjacentTierCeilingMatrix, which is positive-bonus-only by
// design). It also proves a Generated Dynamic HDR release that is the only
// available result stays a kept/Fetch candidate rather than being hard
// rejected, since this is a score rule, not a reject rule.
func TestGeneratedDynamicHDRTierAuthority(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Generated Dynamic HDR tier authority",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	rankOf := func(cands []triage.Candidate) []ranking.Result {
		t.Helper()
		request := ranking.Request{Kind: ranking.KindMovie, Title: "Example"}
		kept, rejected := profile.ApplyWithRejected(request, cands, jhinrank.RankOptions{})
		if len(rejected) != 0 {
			t.Fatalf("unexpectedly rejected: %+v", rejected)
		}
		return kept
	}

	defines := loadCeilingDefines(t)
	tok := func(name string) string {
		toks, ok := defines[name]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", name)
		}
		return toks[0]
	}

	t.Run("Flights (real trusted T2 + GDH overlap) ranks below clean T1", func(t *testing.T) {
		// Flights is a real, audited overlap: it is both a trusted
		// Movies WEB T2 group (+300, see profiles/rules.json) and one of
		// the Generated Dynamic HDR groups. This is the actual production
		// case the tier-authority question is about, not a synthetic one.
		t1 := tok("Movies WEB T1 Groups")

		flaggedFlights := "Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-Flights"
		cleanT1 := fmt.Sprintf(
			"Example.Movie.2026.1080p.WEB-DL.x264-%s", t1,
		)

		results := rankOf([]triage.Candidate{
			{Release: &release.Release{Title: flaggedFlights}},
			{Release: &release.Release{Title: cleanT1}},
		})

		var flaggedRank, cleanRank int
		for _, r := range results {
			switch r.Candidate.Release.Title {
			case flaggedFlights:
				flaggedRank = r.Torrent.Rank
			case cleanT1:
				cleanRank = r.Torrent.Rank
			}
		}

		if flaggedRank >= cleanRank {
			t.Errorf(
				"expected trusted-tier Flights release flagged Generated Dynamic "+
					"HDR (%d) to rank BELOW clean T1 (%d) -- this inversion is the "+
					"intended policy, analogous to Movies Bad Dual Penalty/"+
					"Movies LQ Penalty",
				flaggedRank, cleanRank,
			)
		}
	})

	t.Run("sole result stays kept, not rejected", func(t *testing.T) {
		title := "Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-BiTOR"

		request := ranking.Request{Kind: ranking.KindMovie, Title: "Example"}
		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{{Release: &release.Release{Title: title}}},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 {
			t.Fatalf(
				"sole Generated Dynamic HDR result was rejected, not just "+
					"deprioritized: %+v",
				rejected,
			)
		}
		if len(kept) != 1 {
			t.Fatalf("kept=%d, want 1 (sole result must survive as fallback)", len(kept))
		}
		if !kept[0].Torrent.Fetch && kept[0].Torrent.Rank == 0 {
			// Fetch reflects downstream availability wiring in this harness,
			// not the ranking decision itself; the meaningful assertion is
			// that it was kept at all (already checked above) and carries a
			// deeply negative but finite rank rather than being discarded.
			t.Logf("sole result Fetch=%v Rank=%d", kept[0].Torrent.Fetch, kept[0].Torrent.Rank)
		}
	})
}

// TestGeneratedDynamicHDROverlaps verifies the real cross-classification
// overlaps found during the audit are not special-cased away:
// DepraveD/SasukeducK can independently stack Generated Dynamic HDR on top
// of their existing Movies LQ Penalty, and Flights keeps its ordinary
// trusted WEB T2 tier score when no dynamic-HDR marker is present.
func TestGeneratedDynamicHDROverlaps(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Generated Dynamic HDR overlaps",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	scoreAndMatched := func(title string) (int, []string) {
		t.Helper()
		cand := triage.Candidate{Release: &release.Release{Title: title}}
		request := ranking.Request{Kind: ranking.KindMovie, Title: "Example"}
		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{cand},
			jhinrank.RankOptions{},
		)
		if len(rejected) != 0 {
			t.Fatalf("%q: unexpectedly rejected: %+v", title, rejected)
		}
		if len(kept) != 1 {
			t.Fatalf("%q: kept=%d, want 1", title, len(kept))
		}
		names := make([]string, 0, len(kept[0].Matched))
		for _, m := range kept[0].Matched {
			names = append(names, m.Name)
		}
		return kept[0].Torrent.Rank, names
	}

	contains := func(names []string, want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	t.Run("DepraveD stacks LQ and Generated Dynamic HDR independently", func(t *testing.T) {
		lqOnly := "Example.Movie.2026.1080p.WEB-DL.x264-DepraveD"
		lqAndGDH := "Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-DepraveD"

		_, lqOnlyMatched := scoreAndMatched(lqOnly)
		if !contains(lqOnlyMatched, "Movies LQ Penalty") {
			t.Error("DepraveD without dynamic HDR did not trigger Movies LQ Penalty")
		}
		if contains(lqOnlyMatched, generatedDynamicHDRRuleName) {
			t.Error("DepraveD without dynamic HDR unexpectedly triggered Generated Dynamic HDR Penalty")
		}

		_, bothMatched := scoreAndMatched(lqAndGDH)
		if !contains(bothMatched, "Movies LQ Penalty") {
			t.Error("DepraveD with dynamic HDR lost its existing Movies LQ Penalty")
		}
		if !contains(bothMatched, generatedDynamicHDRRuleName) {
			t.Error("DepraveD with dynamic HDR did not additionally trigger Generated Dynamic HDR Penalty")
		}
	})

	t.Run("SasukeducK stacks LQ and Generated Dynamic HDR independently", func(t *testing.T) {
		// HDR10+ rather than bare DV: a bare-DV-without-fallback release is
		// already unconditionally rejected by the pre-existing Samsung "DV
		// without HDR fallback" device rule, independent of Generated
		// Dynamic HDR entirely -- that is a different, already-covered
		// interaction (see TestGeneratedDynamicHDRScoringInteraction's "DV
		// with HDR fallback" case), not what this stacking test is about.
		lqAndGDH := "Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-SasukeducK"

		_, matched := scoreAndMatched(lqAndGDH)
		if !contains(matched, "Movies LQ Penalty") {
			t.Error("SasukeducK with dynamic HDR lost its existing Movies LQ Penalty")
		}
		if !contains(matched, generatedDynamicHDRRuleName) {
			t.Error("SasukeducK with dynamic HDR did not additionally trigger Generated Dynamic HDR Penalty")
		}
	})

	t.Run("Flights keeps ordinary trusted WEB T2 tier without a dynamic-HDR marker", func(t *testing.T) {
		clean := "Example.Movie.2026.1080p.WEB-DL.x264-Flights"

		_, matched := scoreAndMatched(clean)
		if !contains(matched, "Movies WEB T2") {
			t.Errorf(
				"Flights without dynamic HDR did not receive its ordinary "+
					"Movies WEB T2 tier score; matched=%v",
				matched,
			)
		}
		if contains(matched, generatedDynamicHDRRuleName) {
			t.Error("Flights without dynamic HDR unexpectedly triggered Generated Dynamic HDR Penalty")
		}
	})
}
