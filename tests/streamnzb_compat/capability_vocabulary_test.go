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

// tierGatedReference is one shipped rule's reference to one tier-gated
// field/function. Tracked per (capability, rule) pair rather than per
// capability alone: a bare capability allowlist would stop reviewing new
// references the moment the *first* rule using a given field was approved,
// silently waving through every later rule that reuses the same
// already-allowlisted field without its own review.
type tierGatedReference struct {
	Capability string
	Rule       string
}

// tierGatedFieldAllowlist is every (field, rule) pair a shipped rule
// currently forms. Checking profile.txt (Samsung) alone is sufficient:
// profile-neutral.txt's core+presentation rule set is a strict subset of it,
// and the one Samsung-only rule ("DV without HDR fallback") references
// neither avail.* nor seadex.*.
var tierGatedFieldAllowlist = []tierGatedReference{
	{Capability: "avail.checkedDaysAgo", Rule: "Recently confirmed"},
	{Capability: "avail.onMyBackbone", Rule: "Alive on our backbone"},
	{Capability: "avail.status", Rule: "Known unavailable"},
	{Capability: "seadex.best", Rule: "Seadex Best"},
	{Capability: "seadex.best", Rule: "At most 1 SeaDex Best"},
	{Capability: "seadex.best", Rule: "Anime Unknown Resolution"},
	{Capability: "seadex.best", Rule: "Anime Adaptive Low-Quality Filtering"},
	{Capability: "seadex.best", Rule: "Anime LQ Penalty"},
	{Capability: "seadex.best", Rule: "Reject bad 4K Anime"},
	{Capability: "seadex.alternative", Rule: "Seadex Alternative"},
	{Capability: "seadex.alternative", Rule: "At most 1 SeaDex Alternative"},
	{Capability: "seadex.alternative", Rule: "Anime Unknown Resolution"},
	{Capability: "seadex.alternative", Rule: "Anime Adaptive Low-Quality Filtering"},
	{Capability: "seadex.alternative", Rule: "Anime LQ Penalty"},
	{Capability: "seadex.alternative", Rule: "Reject bad 4K Anime"},
}

// tierGatedFunctionAllowlist mirrors tierGatedFieldAllowlist for functions.
// Empty today: no function in the pinned vocabulary carries a Tier. Declared
// (rather than omitted) so a future tier-gated function fails this test the
// same way a field does, instead of silently passing.
var tierGatedFunctionAllowlist = []tierGatedReference{}

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

// referencedTierGatedPairs walks every shipped rule and every tier-gated
// capability name in tierByCapability, returning the (capability, rule)
// pairs that actually occur -- the ground truth an allowlist is checked
// against, in both directions.
func referencedTierGatedPairs(t *testing.T, tierByCapability map[string]string) map[tierGatedReference]bool {
	t.Helper()

	productionRules := loadProductionRules(t)
	referenced := make(map[tierGatedReference]bool)
	for _, r := range productionRules {
		for capability := range tierByCapability {
			if wordReferenced(r.When, capability) {
				referenced[tierGatedReference{Capability: capability, Rule: r.Name}] = true
			}
		}
	}
	return referenced
}

// checkTierGatedAllowlist is shared by the field and function tripwires: a
// referenced (capability, rule) pair not in allowlist fails (an unreviewed
// reference), and an allowlist entry no longer referenced, or naming a
// capability the vocabulary no longer reports as tier-gated, also fails (a
// stale entry) -- drift fails in either direction.
func checkTierGatedAllowlist(
	t *testing.T,
	kind string,
	tierByCapability map[string]string,
	allowlist []tierGatedReference,
	sawTraceForDoc string,
) {
	t.Helper()

	referenced := referencedTierGatedPairs(t, tierByCapability)

	allowed := make(map[tierGatedReference]bool, len(allowlist))
	for _, entry := range allowlist {
		allowed[entry] = true
	}

	for ref := range referenced {
		if !allowed[ref] {
			t.Errorf(
				"rule %q references tier-gated %s %q (tier %q), which is not in the allowlist -- "+
					"this %s silently skips its rule whenever the %q tier is absent (%s); review the "+
					"new reference, then add {Capability: %q, Rule: %q} to the allowlist deliberately",
				ref.Rule, kind, ref.Capability, tierByCapability[ref.Capability],
				kind, tierByCapability[ref.Capability], sawTraceForDoc, ref.Capability, ref.Rule,
			)
		}
	}

	for _, entry := range allowlist {
		tier, stillTierGated := tierByCapability[entry.Capability]
		if !stillTierGated {
			t.Errorf(
				"allowlist entry {Capability: %q, Rule: %q} names a %s the pinned vocabulary no "+
					"longer reports as tier-gated, or no longer reports at all -- update or remove the entry",
				entry.Capability, entry.Rule, kind,
			)
			continue
		}
		if !referenced[entry] {
			t.Errorf(
				"allowlist entry {Capability: %q, Rule: %q} (tier %q) no longer holds -- either rule "+
					"%q no longer exists in profile.txt, or it no longer references %q -- remove the stale entry",
				entry.Capability, entry.Rule, tier, entry.Rule, entry.Capability,
			)
		}
	}
}

// TestTierGatedFieldReferencesMatchAllowlist is the primary tripwire: every
// shipped rule that references a tier-gated field must have that exact
// (field, rule) pair in tierGatedFieldAllowlist -- reusing an
// already-allowlisted field from a *different*, unreviewed rule still fails,
// since the pair tracks which rule earned the review, not just which field.
func TestTierGatedFieldReferencesMatchAllowlist(t *testing.T) {
	vocab := rules.Describe()
	tierByField := make(map[string]string)
	for _, f := range vocab.Fields {
		if f.Tier != "" {
			tierByField[f.Name] = f.Tier
		}
	}
	checkTierGatedAllowlist(
		t, "field", tierByField, tierGatedFieldAllowlist,
		"see project_context.md §8's SeaDex trap",
	)
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
	checkTierGatedAllowlist(
		t, "function", tierByFunc, tierGatedFunctionAllowlist,
		"see project_context.md §8's SeaDex trap",
	)
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
