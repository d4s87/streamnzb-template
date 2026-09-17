package streamnzb_compat

// Permanent real-engine regression for the "Non-Anime Streaming-Service
// Formatter Badges" feature: 20 zero-point, presentation-only rules that
// give the DraCuLa normal formatter a fallback service label for Movie/
// Series releases from streaming services Jhin's own `.Network` table does
// not recognize. The original 16 came from the initial audit; TVING, Viu,
// iQIYI, and Fandango were added by the remaining-badges audit (2026-09-17)
// once Vidhin's own regex data confirmed a plain, non-lookaround-dependent
// RE2 translation exists for each. Movies Anywhere/Max (matchesExcept) and
// Google Play (WEB-adjacency-gated) have their own dedicated files because
// their collision-safety shapes are genuinely different from this file's
// generic bounded-token contract.
//
// Each rule mirrors the pre-existing Anime CR/DSNP/NF/... shape exactly:
// `not isAnime and (<web traits>) and releaseName matches
// "(?i)(?:^|[. _\[\]-])TOKEN(?:$|[. _\[\]-])"`, owner "presentation",
// points 0. No Define is used or added by this feature -- see
// build_profiles.py's EXPECTED_PRESENTATION_RULES for the exact set.

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

// nonAnimeServiceBadges is the exact 20-service contract this file covers
// (the original 16 plus TVING/Viu/iQIYI/Fandango). Keep in sync with
// build_profiles.py's EXPECTED_PRESENTATION_RULES and the formatter
// source's `$service` fallback chain. iQIYI's own IQIY/IQIYI-alias and
// bare-IQ-exclusion coverage lives in its own dedicated sub-test below,
// alongside the pre-existing Hotstar HTSR/bare-HS pattern.
var nonAnimeServiceBadges = []struct {
	rule  string
	token string
}{
	{"Peacock", "PCOK"},
	{"Paramount+", "PMTP"},
	{"Criterion Channel", "CRiT"},
	{"Roku", "ROKU"},
	{"Syfy", "SYFY"},
	{"DC Universe", "DCU"},
	{"Coupang", "CPNG"},
	{"DMM TV", "DMM-TV"},
	{"FOD", "FOD"},
	{"Hotstar", "HTSR"},
	{"KOCOWA", "KCW"},
	{"U-NEXT", "U-NEXT"},
	{"Viki", "Viki"},
	{"Wavve", "WAVVE"},
	{"WeTV", "WETV"},
	{"Youku", "YOUKU"},
	{"TVING", "TVING"},
	{"Viu", "Viu"},
	{"iQIYI", "IQIYI"},
	{"Fandango", "FAND"},
}

// deferredAmbiguousServiceRuleNames must never appear as a production rule.
//
// "Movies Anywhere"/"MA" and "Max" were removed from this list once the
// StreamNZB v6.2.0 pin-readiness audit's classification-A findings were
// implemented -- see movies_anywhere_service_badge_test.go and
// max_service_badge_test.go for their own dedicated matchesExcept-based
// real-engine regressions, kept separate from this file's generic
// bounded-token contract because their collision-safety shapes are
// genuinely different. "TVING"/"Viu"/"iQIYI"/"Fandango" were removed once
// the remaining-badges audit (2026-09-17) confirmed a plain, non-lookaround
// RE2 translation exists for each -- see nonAnimeServiceBadges above.
// "Google Play" was removed the same day -- see
// google_play_service_badge_test.go for its own dedicated WEB-adjacency-
// gated real-engine regression, kept separate because bare "Play" is a
// real English word (e.g. "Childs.Play...WEB-DL") and needs a narrower
// rule shape than the generic bounded-token contract. "iTunes"/"Showtime"/
// "Stan" were removed the same day too: they are not deferred fallback
// candidates at all -- Jhin's own `.Network` table already recognizes them
// natively, so a DraCuLa fallback badge for them would be redundant, not
// merely unimplemented.
//
// "Comedy Central" is the one remaining entry: its only evidenced
// canonical token is bare "CC" (2 letters, no longer distinctive alias
// exists), which stays too collision-prone to approximate without a real
// release-name corpus proving safety. Do not "fix" this list by adding it
// to nonAnimeServiceBadges -- that would be exactly the un-audited
// shortcut this feature is scoped to avoid.
var deferredAmbiguousServiceRuleNames = []string{
	"Comedy Central",
}

// evaluateIsolatedRule compiles a single production rule (no Define
// dependency for any of these 20) and reports whether it matched a given
// release title under the given anime scope -- the isolated-rule layer of
// this repo's two-layer validation discipline (see CLAUDE.md).
func evaluateIsolatedRule(t *testing.T, cfg config.RuleConfig, title string, isAnime bool) bool {
	t.Helper()

	set, err := rules.Compile([]config.RuleConfig{cfg})
	if err != nil {
		t.Fatalf("compile production rule %q: %v", cfg.Name, err)
	}

	kind := ranking.KindMovie
	if isAnime {
		kind = ranking.KindAnimeMovie
	}

	cand := triage.Candidate{Release: &release.Release{Title: title}}
	env := rules.BuildEnv(
		cand,
		jhin.Parse(title),
		rules.Context{Kind: kind, IsAnime: isAnime},
	)

	out := set.Evaluate(env, kind)
	return ruleMatched(out, cfg.Name)
}

// TestNonAnimeServiceBadgeRuleContract proves the exact, minimal shape of
// each new presentation rule against the actual published Samsung profile:
// zero points, score action (no reject/limit), and `not isAnime` scope
// enforced through the rule's own condition (these rules carry no separate
// `scope` field -- the isAnime guard lives in `when`, exactly like the
// pre-existing Anime service rules use `isAnime` the same way).
func TestNonAnimeServiceBadgeRuleContract(t *testing.T) {
	productionRules := loadProductionRules(t)

	for _, svc := range nonAnimeServiceBadges {
		t.Run(svc.rule, func(t *testing.T) {
			cfg := findProductionRule(t, productionRules, svc.rule)

			if cfg.Points != 0 {
				t.Errorf("rule %q points = %d, want 0", svc.rule, cfg.Points)
			}
			if cfg.EffectiveAction() != config.RuleActionScore {
				t.Errorf(
					"rule %q action = %q, want score (presentation-only, no reject/limit)",
					svc.rule, cfg.EffectiveAction(),
				)
			}
		})
	}

	for _, name := range deferredAmbiguousServiceRuleNames {
		for _, rule := range productionRules {
			if rule.Name == name {
				t.Errorf(
					"deferred/ambiguous service rule %q must not exist in the "+
						"published profile -- its only evidenced canonical token "+
						"remains too collision-prone to approximate without a "+
						"real release-name corpus proving safety",
					name,
				)
			}
		}
	}
}

// TestNonAnimeServiceBadgeClassification is the production-regression layer:
// the exact published rule (decoded from profile.txt, not a hand-copied
// fixture) run over positive, negative, fused-token, non-WEB, and Anime-scope
// release names through the real StreamNZB rule engine.
func TestNonAnimeServiceBadgeClassification(t *testing.T) {
	productionRules := loadProductionRules(t)

	highRiskFused := map[string]bool{
		"Peacock":           true, // PCOK
		"Paramount+":        true, // PMTP
		"Criterion Channel": true, // CRiT
		"DC Universe":       true, // DCU
		"FOD":               true, // FOD
		"KOCOWA":            true, // KCW
		"TVING":             true,
		"Viu":               true,
		"iQIYI":             true, // IQIYI
		"Fandango":          true, // FAND
	}

	for _, svc := range nonAnimeServiceBadges {
		t.Run(svc.rule, func(t *testing.T) {
			cfg := findProductionRule(t, productionRules, svc.rule)

			positive := fmt.Sprintf(
				"Example.Movie.2026.1080p.%s.WEB-DL.DDP5.1.x264-GROUP",
				svc.token,
			)
			nonWeb := fmt.Sprintf(
				"Example.Movie.2026.1080p.%s.BluRay.DDP5.1.x264-GROUP",
				svc.token,
			)

			t.Run("matches in a non-Anime WEB release", func(t *testing.T) {
				if !evaluateIsolatedRule(t, cfg, positive, false) {
					t.Errorf(
						"%q did not match its own audited token in %q",
						svc.rule, positive,
					)
				}
			})

			t.Run("does not match a non-WEB release", func(t *testing.T) {
				if evaluateIsolatedRule(t, cfg, nonWeb, false) {
					t.Errorf(
						"%q matched a non-WEB release %q; rule must require WEB traits",
						svc.rule, nonWeb,
					)
				}
			})

			t.Run("does not match when isAnime (not isAnime scope)", func(t *testing.T) {
				if evaluateIsolatedRule(t, cfg, positive, true) {
					t.Errorf(
						"%q matched an Anime release %q; rule must be scoped not isAnime",
						svc.rule, positive,
					)
				}
			})

			if highRiskFused[svc.rule] {
				t.Run("does not match the token fused into a longer word", func(t *testing.T) {
					fused := fmt.Sprintf(
						"Example.Movie.2026.1080p.WEB-DL.DDP5.1.x264-X%sX",
						svc.token,
					)
					if evaluateIsolatedRule(t, cfg, fused, false) {
						t.Errorf(
							"%q matched %q; token must not fire when fused into a longer token",
							svc.rule, fused,
						)
					}
				})
			}
		})
	}

	// Hotstar/HS distinction: the rule must match the audited "HTSR" token
	// and must NOT match Vidhin's own ambiguous bare "HS" alias, which this
	// feature deliberately excluded (see the audit).
	t.Run("Hotstar matches HTSR but not bare HS", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "Hotstar")

		htsr := "Example.Movie.2026.1080p.HTSR.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, htsr, false) {
			t.Errorf("Hotstar did not match audited token in %q", htsr)
		}

		bareHS := "Example.Movie.2026.1080p.HS.WEB-DL.DDP5.1.x264-GROUP"
		if evaluateIsolatedRule(t, cfg, bareHS, false) {
			t.Errorf(
				"Hotstar matched bare \"HS\" in %q; the ambiguous bare HS "+
					"alias was deliberately excluded by the audit",
				bareHS,
			)
		}
	})

	// iQIYI matches both its full spelled-out form and Vidhin's own
	// canonical short form, but must never match the bare "IQ" alternative
	// Vidhin's own alternation also lists -- a real, plausible abbreviation
	// (e.g. "intelligence quotient") this feature deliberately excludes,
	// the same real-world-token-collision-risk reasoning already applied
	// to Hotstar's bare "HS" above.
	t.Run("iQIYI matches IQIYI and IQIY but not bare IQ", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "iQIYI")

		full := "Example.Movie.2026.1080p.IQIYI.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, full, false) {
			t.Errorf("iQIYI did not match audited token in %q", full)
		}

		short := "Example.Movie.2026.1080p.IQIY.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, short, false) {
			t.Errorf("iQIYI did not match Vidhin's own short alias in %q", short)
		}

		bareIQ := "Example.Movie.2026.1080p.IQ.WEB-DL.DDP5.1.x264-GROUP"
		if evaluateIsolatedRule(t, cfg, bareIQ, false) {
			t.Errorf(
				"iQIYI matched bare \"IQ\" in %q; the ambiguous bare IQ "+
					"alias was deliberately excluded by the audit",
				bareIQ,
			)
		}
	})

	// Peacock's own spelled-out alias, and DC Universe's/Paramount+'s
	// multi-word alias with a flexible internal separator, exactly mirroring
	// the existing DSNP rule's "Disney[ ._-]?Plus" idiom.
	t.Run("Peacock also matches the spelled-out alias", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "Peacock")
		title := "Example.Movie.2026.1080p.Peacock.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, title, false) {
			t.Errorf("Peacock did not match spelled-out alias in %q", title)
		}
	})

	t.Run("Paramount+ also matches Paramount Plus with a flexible separator", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "Paramount+")
		title := "Example.Movie.2026.1080p.Paramount.Plus.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, title, false) {
			t.Errorf("Paramount+ did not match \"Paramount Plus\" alias in %q", title)
		}
	})

	t.Run("DC Universe also matches DC Universe spelled out", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "DC Universe")
		title := "Example.Show.S01E01.1080p.DC.Universe.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, title, false) {
			t.Errorf("DC Universe did not match spelled-out alias in %q", title)
		}
	})

	t.Run("KOCOWA also matches KOCOWA spelled out", func(t *testing.T) {
		cfg := findProductionRule(t, productionRules, "KOCOWA")
		title := "Example.Show.S01E01.1080p.KOCOWA.WEB-DL.DDP5.1.x264-GROUP"
		if !evaluateIsolatedRule(t, cfg, title, false) {
			t.Errorf("KOCOWA did not match spelled-out alias in %q", title)
		}
	})
}

// TestNonAnimeServiceBadgeScoringInvariance proves, through the real
// full-profile pipeline (not just the isolated rule), that a matching
// service badge rule changes only presentation (MatchedRules / formatter
// label) and never final score, Fetch/keep state, rejection, or limit
// behavior. One representative service (Peacock) is sufficient: all 20
// share the identical zero-point rule shape.
func TestNonAnimeServiceBadgeScoringInvariance(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Non-Anime service badge scoring invariance",
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
	withPeacock := "Example.Movie.2026.1080p.PCOK.WEB-DL.DDP5.1.x264-GROUP"

	cleanRank, cleanMatched, cleanKept, cleanRejected := rankOf(clean)
	peacockRank, peacockMatched, peacockKept, peacockRejected := rankOf(withPeacock)

	if contains(cleanMatched, "Peacock") {
		t.Fatal("clean release unexpectedly matched Peacock")
	}
	if !contains(peacockMatched, "Peacock") {
		t.Fatal("Peacock release did not match the Peacock rule; nothing to prove invariance over")
	}

	if peacockRank != cleanRank {
		t.Errorf(
			"final score changed by the Peacock badge: clean=%d peacock=%d (want equal)",
			cleanRank, peacockRank,
		)
	}
	if peacockKept != cleanKept || peacockKept != 1 {
		t.Errorf(
			"Fetch/keep state changed by the Peacock badge: clean kept=%d peacock kept=%d (want 1/1)",
			cleanKept, peacockKept,
		)
	}
	if peacockRejected != cleanRejected || peacockRejected != 0 {
		t.Errorf(
			"rejection behavior changed by the Peacock badge: clean rejected=%d peacock rejected=%d (want 0/0)",
			cleanRejected, peacockRejected,
		)
	}
}

// TestNonAnimeServiceBadgeDefineLibraryUnaffected asserts this feature added
// no Define. The Define Library count itself has since grown to 62 (61
// Vidhin-backed + 1 local helper) via later, unrelated syncs (Retag Markers,
// Atmos/TrueHD Exclude Groups, then Movies/Shows LQ Release Title) — this
// test only proves this feature's own zero-Define contribution, not an
// absolute count frozen at this feature's original landing.
func TestNonAnimeServiceBadgeDefineLibraryUnaffected(t *testing.T) {
	defineLibrary := loadDefineLibrary(t)

	if len(defineLibrary) != 62 {
		t.Fatalf(
			"generated Define Library has %d entries, want 62 -- this feature "+
				"must not add any Define",
			len(defineLibrary),
		)
	}
}
