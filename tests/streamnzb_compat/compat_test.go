package streamnzb_compat

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

type FixtureFile struct {
	Rules []RuleFixture `json:"rules"`
}

type RuleFixture struct {
	Name   string        `json:"name"`
	When   string        `json:"when"`
	Points int           `json:"points"`
	Cases  []CaseFixture `json:"cases"`
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

func TestCompatibilityFixtures(t *testing.T) {
	fixtures := loadFixtures(t)

	for _, rf := range fixtures.Rules {
		rf := rf

		t.Run(rf.Name, func(t *testing.T) {
			set, err := rules.Compile([]config.RuleConfig{
				{
					Name:   rf.Name,
					When:   rf.When,
					Points: rf.Points,
				},
			})
			if err != nil {
				t.Fatalf(
					"StreamNZB rejected rule %q:\ncondition: %s\nerror: %v",
					rf.Name,
					rf.When,
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

					gotMatch := ruleMatched(out, rf.Name)
					if gotMatch != cf.Expected.Match {
						t.Errorf(
							"rule match = %v, want %v\nrelease: %s\nkind: %s\nanime: %v\ncondition: %s\ntraits: %v\nparsed bitDepth: %d\nmatched rules: %+v",
							gotMatch,
							cf.Expected.Match,
							cf.Release,
							cf.Kind,
							cf.Anime,
							rf.When,
							env.Traits,
							env.Parsed.BitDepth,
							out.Matched,
						)
					}
				})
			}
		})
	}
}
