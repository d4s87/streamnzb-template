package streamnzb_compat

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

const profilePrefix = "SNZBP1:"

type FixtureFile struct {
	Rules []RuleFixture `json:"rules"`
}

type RuleFixture struct {
	Name           string        `json:"name"`
	ProductionRule string        `json:"productionRule,omitempty"`
	When           string        `json:"when"`
	Points         int           `json:"points"`
	Cases          []CaseFixture `json:"cases"`
}

type CaseFixture struct {
	Name     string      `json:"name"`
	Release  string      `json:"release"`
	Kind     string      `json:"kind"`
	Anime    bool        `json:"anime"`
	Expected Expectation `json:"expected"`
}

type Expectation struct {
	TraitsContain []string `json:"traitsContain,omitempty"`
	TraitsExclude []string `json:"traitsExclude,omitempty"`
	BitDepth      *int     `json:"bitDepth,omitempty"`
	Match         bool     `json:"match"`
}

type profilePayload struct {
	Rules []config.RuleConfig `json:"rules"`
}

func loadFixtures(t *testing.T) FixtureFile {
	t.Helper()

	data, err := os.ReadFile("fixtures/rules.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}

	var fixtures FixtureFile
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}

	if len(fixtures.Rules) == 0 {
		t.Fatal("fixture file contains no rules")
	}

	return fixtures
}

func loadProductionRules(t *testing.T) []config.RuleConfig {
	t.Helper()

	data, err := os.ReadFile("../../profile.txt")
	if err != nil {
		t.Fatalf("read production profile: %v", err)
	}

	code := strings.TrimSpace(string(data))
	if !strings.HasPrefix(code, profilePrefix) {
		t.Fatalf("production profile does not start with %q", profilePrefix)
	}

	encoded := strings.TrimPrefix(code, profilePrefix)

	compressed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode production profile Base64URL: %v", err)
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open production profile gzip payload: %v", err)
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read production profile gzip payload: %v", err)
	}

	var profile profilePayload
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatalf("decode production profile JSON: %v", err)
	}

	if len(profile.Rules) == 0 {
		t.Fatal("production profile contains no rules")
	}

	return profile.Rules
}

func findProductionRule(
	t *testing.T,
	productionRules []config.RuleConfig,
	name string,
) config.RuleConfig {
	t.Helper()

	var found []config.RuleConfig

	for _, rule := range productionRules {
		if rule.Name == name {
			found = append(found, rule)
		}
	}

	if len(found) != 1 {
		t.Fatalf(
			"expected exactly one production rule %q, found %d",
			name,
			len(found),
		)
	}

	return found[0]
}

func buildEnv(c CaseFixture) rules.Env {
	cand := triage.Candidate{
		Release: &release.Release{
			Title: c.Release,
		},
	}

	return rules.BuildEnv(
		cand,
		jhin.Parse(c.Release),
		rules.Context{
			Kind:    c.Kind,
			IsAnime: c.Anime,
		},
	)
}

func ruleMatched(out rules.Outcome, name string) bool {
	for _, match := range out.Matched {
		if match.Name == name {
			return true
		}
	}

	return false
}

func runCases(
	t *testing.T,
	rf RuleFixture,
	cfg config.RuleConfig,
) {
	t.Helper()

	set, err := rules.Compile([]config.RuleConfig{cfg})
	if err != nil {
		t.Fatalf(
			"StreamNZB rejected rule %q:\ncondition: %s\nerror: %v",
			cfg.Name,
			cfg.When,
			err,
		)
	}

	for _, cf := range rf.Cases {
		cf := cf

		t.Run(cf.Name, func(t *testing.T) {
			env := buildEnv(cf)
			out := set.Evaluate(env, cf.Kind)

			for _, trait := range cf.Expected.TraitsContain {
				if !slices.Contains(env.Traits, trait) {
					t.Errorf(
						"missing expected trait %q\nrelease: %s\ntraits: %v\nparsed bitDepth: %d",
						trait,
						cf.Release,
						env.Traits,
						env.Parsed.BitDepth,
					)
				}
			}

			for _, trait := range cf.Expected.TraitsExclude {
				if slices.Contains(env.Traits, trait) {
					t.Errorf(
						"unexpected trait %q\nrelease: %s\ntraits: %v\nparsed bitDepth: %d",
						trait,
						cf.Release,
						env.Traits,
						env.Parsed.BitDepth,
					)
				}
			}

			if cf.Expected.BitDepth != nil &&
				env.Parsed.BitDepth != *cf.Expected.BitDepth {
				t.Errorf(
					"parsed bitDepth = %d, want %d\nrelease: %s\ntraits: %v",
					env.Parsed.BitDepth,
					*cf.Expected.BitDepth,
					cf.Release,
					env.Traits,
				)
			}

			gotMatch := ruleMatched(out, cfg.Name)
			if gotMatch != cf.Expected.Match {
				t.Errorf(
					"rule match = %v, want %v\nrelease: %s\nkind: %s\nanime: %v\ncondition: %s\ntraits: %v\nparsed bitDepth: %d\nmatched rules: %+v",
					gotMatch,
					cf.Expected.Match,
					cf.Release,
					cf.Kind,
					cf.Anime,
					cfg.When,
					env.Traits,
					env.Parsed.BitDepth,
					out.Matched,
				)
			}
		})
	}
}

func TestCompatibilityFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	productionRules := loadProductionRules(t)

	for _, rf := range fixtures.Rules {
		rf := rf

		t.Run(rf.Name, func(t *testing.T) {
			// First validate the experimental/reference expression stored
			// with the fixture.
			fixtureRule := config.RuleConfig{
				Name:   rf.Name,
				When:   rf.When,
				Points: rf.Points,
			}

			t.Run("fixture", func(t *testing.T) {
				runCases(t, rf, fixtureRule)
			})

			if rf.ProductionRule == "" {
				return
			}

			// Then run the exact rule shipped inside profile.txt through
			// the same real StreamNZB parser/compiler/evaluator.
			productionRule := findProductionRule(
				t,
				productionRules,
				rf.ProductionRule,
			)

			if productionRule.When != rf.When {
				t.Fatalf(
					"production rule %q drifted from fixture\nfixture:    %s\nproduction: %s",
					rf.ProductionRule,
					rf.When,
					productionRule.When,
				)
			}

			if productionRule.Points != rf.Points {
				t.Fatalf(
					"production rule %q points = %d, fixture expects %d",
					rf.ProductionRule,
					productionRule.Points,
					rf.Points,
				)
			}

			t.Run("production", func(t *testing.T) {
				runCases(t, rf, productionRule)
			})
		})
	}
}
