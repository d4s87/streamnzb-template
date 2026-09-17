package streamnzb_compat

// This file is a narrow compatibility-contract tripwire, not a general
// schema-diff framework. `ranking.Compile` already fails loudly everywhere
// else in this suite if a shipped rule names a field/function the pinned
// engine does not understand at all -- that already proves plain existence.
// What compiling does *not* catch is a field/function that exists and
// compiles fine but is confidence-tier-gated: it silently skips the whole
// rule/aggregate whenever a request lacks that tier (see project_context.md
// §8, "SeaDex static tier-dependency", real-engine-proven twice in
// production). rules.Describe() reads the same registries the live pipeline
// compiles against (pkg/search/rules/vocabulary.go), so it is authoritative
// for which fields/functions carry a tier today.
//
// Two things are asserted here:
//  1. every tier-gated field/function a shipped rule references is in an
//     explicit, hand-reviewed allowlist -- a new tier-gated reference (or a
//     stale allowlist entry for one that's gone) fails the build, forcing a
//     deliberate decision instead of an accidental skip-prone rule shipping
//     unreviewed;
//  2. a handful of named capabilities DraCuLa's own rule/formatter design
//     depends on by name (the confidence tiers behind the SeaDex split-rule
//     pattern, matchesExcept, the formatter helpers the published templates
//     call) still exist in the pinned vocabulary.

import (
	"regexp"
	"testing"

	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/server/stremio"
)

// tierGatedFieldAllowlist maps every tier-gated rule field a shipped rule
// references to the rule name(s) that reference it. Checking profile.txt
// (Samsung) alone is sufficient: profile-neutral.txt's core+presentation
// rule set is a strict subset of it, and the one Samsung-only rule ("DV
// without HDR fallback") references neither avail.* nor seadex.*.
var tierGatedFieldAllowlist = map[string]string{
	"avail.checkedDaysAgo": "Recently confirmed",
	"avail.onMyBackbone":   "Alive on our backbone",
	"avail.status":         "Known unavailable",
	"seadex.best": "Seadex Best, At most 1 SeaDex Best, Anime Unknown Resolution, " +
		"Anime Adaptive Low-Quality Filtering, Anime LQ Penalty, Reject bad 4K Anime",
	"seadex.alternative": "Seadex Alternative, At most 1 SeaDex Alternative, Anime Unknown Resolution, " +
		"Anime Adaptive Low-Quality Filtering, Anime LQ Penalty, Reject bad 4K Anime",
}

// tierGatedFunctionAllowlist mirrors tierGatedFieldAllowlist for functions.
// Empty today: no function in the pinned vocabulary carries a Tier. Declared
// (rather than omitted) so a future tier-gated function fails this test the
// same way a field does, instead of silently passing.
var tierGatedFunctionAllowlist = map[string]string{}

// knownTierNames are the confidence tiers DraCuLa's own rule design depends
// on existing by name: the SeaDex split-rule pattern (project_context.md §8)
// only works because a rule referencing seadex.* is skipped, specifically,
// when the "seadex" tier is absent -- a rename would silently invalidate
// that reasoning without touching a single line of profiles/rules.json.
var knownTierNames = []string{"avail", "indexer", "measured", "seadex", "tracks"}

// formatterHelperAllowlist is every result-template helper DraCuLa's
// published formatter/debug-formatter sources call today (union of
// tests/streamnzb_compat/formatter.source.json and formatter-debug.source.json).
var formatterHelperAllowlist = []string{
	"contains", "flags", "join", "length", "replace", "score", "size",
	"smallcaps", "stars", "sub", "title", "translate", "truncate", "upper",
}

// TestTierGatedFieldReferencesMatchAllowlist is the primary tripwire: a
// shipped rule referencing a tier-gated field must have that field listed in
// tierGatedFieldAllowlist, and every allowlist entry must still be
// referenced by a shipped rule -- drift fails in either direction.
func TestTierGatedFieldReferencesMatchAllowlist(t *testing.T) {
	vocab := rules.Describe()

	tierByField := make(map[string]string)
	for _, f := range vocab.Fields {
		if f.Tier != "" {
			tierByField[f.Name] = f.Tier
		}
	}

	productionRules := loadProductionRules(t)
	referenced := make(map[string]bool)
	for _, r := range productionRules {
		for name := range tierByField {
			if wordReferenced(r.When, name) {
				referenced[name] = true
			}
		}
	}

	for name := range referenced {
		if _, ok := tierGatedFieldAllowlist[name]; !ok {
			t.Errorf(
				"a shipped rule references tier-gated field %q (tier %q), which is not in "+
					"tierGatedFieldAllowlist -- this field silently skips its rule whenever the %q "+
					"tier is absent (see project_context.md §8's SeaDex trap); review the new "+
					"reference, then add it to the allowlist deliberately",
				name, tierByField[name], tierByField[name],
			)
		}
	}

	for name, owner := range tierGatedFieldAllowlist {
		tier, stillTierGated := tierByField[name]
		if !stillTierGated {
			t.Errorf(
				"tierGatedFieldAllowlist lists %q (for %s), but the pinned vocabulary no longer "+
					"reports it as tier-gated, or no longer reports it at all -- update or remove the entry",
				name, owner,
			)
			continue
		}
		if !referenced[name] {
			t.Errorf(
				"tierGatedFieldAllowlist lists %q (tier %q, for %s), but no shipped rule references "+
					"it any more -- remove the stale entry",
				name, tier, owner,
			)
		}
	}
}

// TestTierGatedFunctionReferencesMatchAllowlist is the function-side
// counterpart of TestTierGatedFieldReferencesMatchAllowlist. It passes
// vacuously today (no function is tier-gated at this pin, and the allowlist
// is empty) but fails immediately the day either side changes.
func TestTierGatedFunctionReferencesMatchAllowlist(t *testing.T) {
	vocab := rules.Describe()

	tierByFunc := make(map[string]string)
	for _, fn := range vocab.Functions {
		if fn.Tier != "" {
			tierByFunc[fn.Name] = fn.Tier
		}
	}

	productionRules := loadProductionRules(t)
	referenced := make(map[string]bool)
	for _, r := range productionRules {
		for name := range tierByFunc {
			if wordReferenced(r.When, name) {
				referenced[name] = true
			}
		}
	}

	for name := range referenced {
		if _, ok := tierGatedFunctionAllowlist[name]; !ok {
			t.Errorf(
				"a shipped rule references tier-gated function %q (tier %q), which is not in "+
					"tierGatedFunctionAllowlist -- review the new reference, then add it deliberately",
				name, tierByFunc[name],
			)
		}
	}

	for name, owner := range tierGatedFunctionAllowlist {
		if !referenced[name] {
			t.Errorf(
				"tierGatedFunctionAllowlist lists %q (for %s), but no shipped rule references it "+
					"any more -- remove the stale entry",
				name, owner,
			)
		}
	}
}

// TestRuleVocabularyCarriesKnownCapabilities asserts a handful of named
// capabilities DraCuLa's rule design depends on by name still exist in the
// pinned vocabulary, independent of any specific rule referencing them today.
func TestRuleVocabularyCarriesKnownCapabilities(t *testing.T) {
	vocab := rules.Describe()

	tiers := make(map[string]bool, len(vocab.Tiers))
	for _, tier := range vocab.Tiers {
		tiers[tier.Name] = true
	}
	for _, name := range knownTierNames {
		if !tiers[name] {
			t.Errorf(
				"pinned vocabulary no longer reports tier %q, which DraCuLa's SeaDex split-rule "+
					"pattern (project_context.md §8) depends on by name",
				name,
			)
		}
	}

	hasMatchesExcept := false
	for _, fn := range vocab.Functions {
		if fn.Name == "matchesExcept" {
			hasMatchesExcept = true
			break
		}
	}
	if !hasMatchesExcept {
		t.Error("pinned vocabulary no longer reports matchesExcept, which the Movies Anywhere/Max service badges depend on")
	}
}

// TestFormatterHelpersExistInVocabulary confirms every helper the published
// formatter/debug-formatter sources call is still in stremio.FormatHelpers().
// Field-path existence is deliberately not checked here: Go template
// {{range}}/{{with}} blocks reassign "." to a loop/branch-local value, so a
// blind ".Field" text scan over the template source cannot reliably tell a
// top-level FormatContext field from a per-variant loop-local one without
// actually parsing the template -- exactly the "generic schema-diff
// framework" this file is meant to avoid becoming. A genuinely missing
// formatter field already fails loudly and precisely at
// ./scripts/test_formatter.sh's real-engine render, which is the authoritative
// check for that.
func TestFormatterHelpersExistInVocabulary(t *testing.T) {
	available := make(map[string]bool)
	for _, h := range stremio.FormatHelpers() {
		available[h.Name] = true
	}
	for _, name := range formatterHelperAllowlist {
		if !available[name] {
			t.Errorf(
				"formatterHelperAllowlist lists helper %q, which the pinned vocabulary no longer "+
					"reports -- formatter.source.json/formatter-debug.source.json needs review",
				name,
			)
		}
	}
}

// wordReferenced reports whether name appears in when as a whole token
// (never as part of a longer identifier or inside another token), which is
// enough to distinguish a real field/function reference from an unrelated
// substring for the rule expression language's actual grammar (bare or
// dotted identifiers, no other syntax that could produce a false positive
// here in DraCuLa's own shipped when clauses).
func wordReferenced(when, name string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(when)
}
