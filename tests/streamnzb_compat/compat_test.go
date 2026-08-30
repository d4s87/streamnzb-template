package streamnzb_compat

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	jhin "github.com/dreulavelle/jhin/parser"
	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

const (
	profilePrefix         = "SNZBP1:"
	expectedProfileSchema = 1
)

type FixtureFile struct {
	Rules []RuleFixture `json:"rules"`
}

type RuleFixture struct {
	Name           string                 `json:"name"`
	ProductionRule string                 `json:"productionRule,omitempty"`
	Scope          string                 `json:"scope,omitempty"`
	When           string                 `json:"when"`
	Action         string                 `json:"action,omitempty"`
	Points         int                    `json:"points"`
	Cases          []CaseFixture          `json:"cases"`
	AggregateCases []AggregateCaseFixture `json:"aggregateCases,omitempty"`
}

type AggregateCaseFixture struct {
	Name       string        `json:"name"`
	Candidates []CaseFixture `json:"candidates"`
}

type CaseFixture struct {
	Name             string      `json:"name"`
	Release          string      `json:"release"`
	Kind             string      `json:"kind"`
	Anime            bool        `json:"anime"`
	Library          bool        `json:"library,omitempty"`
	IndexerDataKnown bool        `json:"indexerDataKnown,omitempty"`
	Expected         Expectation `json:"expected"`
}

type Expectation struct {
	TraitsContain []string `json:"traitsContain,omitempty"`
	TraitsExclude []string `json:"traitsExclude,omitempty"`
	BitDepth      *int     `json:"bitDepth,omitempty"`
	Match         bool     `json:"match"`
	Rejected      bool     `json:"rejected,omitempty"`
}

type profilePayload struct {
	StreamNZBProfile int                 `json:"streamnzb_profile"`
	Rules            []config.RuleConfig `json:"rules"`
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

func validateProfileSchema(schema int) error {
	if schema != expectedProfileSchema {
		return fmt.Errorf(
			"unsupported StreamNZB profile schema: got %d, expected %d; "+
				"review share-code compatibility before updating the harness",
			schema,
			expectedProfileSchema,
		)
	}

	return nil
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

	if err := validateProfileSchema(profile.StreamNZBProfile); err != nil {
		t.Fatal(err)
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

func loadDefineLibrary(t *testing.T) []config.RuleConfig {
	t.Helper()

	data, err := os.ReadFile("../../generated/streamnzb-defines.txt")
	if err != nil {
		t.Fatalf("read generated Define Library: %v", err)
	}

	var library []config.RuleConfig

	for lineNumber, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		const separator = ": define if "
		idx := strings.Index(line, separator)

		if idx < 1 {
			t.Fatalf(
				"invalid generated Define syntax on line %d: %q",
				lineNumber+1,
				rawLine,
			)
		}

		label := strings.TrimSpace(line[:idx])
		when := strings.TrimSpace(line[idx+len(separator):])

		if when == "" {
			t.Fatalf(
				"generated Define has empty condition on line %d: %q",
				lineNumber+1,
				rawLine,
			)
		}

		name := label
		scope := ""

		if strings.HasSuffix(label, "]") {
			open := strings.LastIndex(label, " [")

			if open < 1 {
				t.Fatalf(
					"invalid generated Define scope on line %d: %q",
					lineNumber+1,
					rawLine,
				)
			}

			name = strings.TrimSpace(label[:open])
			scope = strings.TrimSpace(
				label[open+2 : len(label)-1],
			)
		}

		if name == "" {
			t.Fatalf(
				"generated Define has empty name on line %d",
				lineNumber+1,
			)
		}

		library = append(
			library,
			config.RuleConfig{
				Name:   name,
				Scope:  scope,
				When:   when,
				Action: config.RuleActionDefine,
			},
		)
	}

	if len(library) == 0 {
		t.Fatal("generated Define Library contains no definitions")
	}

	return library
}

func buildEnv(c CaseFixture) rules.Env {
	cand := triage.Candidate{
		Release: &release.Release{
			Title:     c.Release,
			IsLibrary: c.Library,
		},
	}

	return rules.BuildEnv(
		cand,
		jhin.Parse(c.Release),
		rules.Context{
			Kind:             c.Kind,
			IsAnime:          c.Anime,
			IndexerDataKnown: c.IndexerDataKnown,
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
	library ...config.RuleConfig,
) {
	t.Helper()

	set, err := rules.Compile(
		[]config.RuleConfig{cfg},
		library...,
	)
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

func ruleRejected(out rules.Outcome, name string) bool {
	for _, rejection := range out.Rejections {
		if strings.Contains(rejection, name) {
			return true
		}
	}
	return false
}

func runAggregateCases(
	t *testing.T,
	rf RuleFixture,
	cfg config.RuleConfig,
	library ...config.RuleConfig,
) {
	t.Helper()

	if len(rf.AggregateCases) == 0 {
		return
	}

	set, err := rules.Compile(
		[]config.RuleConfig{cfg},
		library...,
	)
	if err != nil {
		t.Fatalf(
			"StreamNZB rejected aggregate rule %q:\ncondition: %s\nerror: %v",
			cfg.Name,
			cfg.When,
			err,
		)
	}

	for _, ac := range rf.AggregateCases {
		ac := ac

		t.Run(ac.Name, func(t *testing.T) {
			if len(ac.Candidates) == 0 {
				t.Fatal("aggregate fixture contains no candidates")
			}

			envs := make([]rules.Env, len(ac.Candidates))

			for i, cf := range ac.Candidates {
				envs[i] = buildEnv(cf)
			}

			state := set.ComputeAggregates(envs)
			if state == nil {
				t.Fatal("ComputeAggregates returned nil state")
			}

			for i, cf := range ac.Candidates {
				state.Inject(&envs[i])

				out := set.Evaluate(envs[i], cf.Kind)

				var got bool
				var want bool
				var outcome string

				if cfg.EffectiveAction() == config.RuleActionReject {
					got = ruleRejected(out, cfg.Name)
					want = cf.Expected.Rejected
					outcome = "rejected"
				} else {
					got = ruleMatched(out, cfg.Name)
					want = cf.Expected.Match
					outcome = "matched"
				}

				if got != want {
					_, reports := set.ReportAggregates(envs)

					t.Errorf(
						"aggregate rule %s = %v, want %v\n"+
							"case: %s\n"+
							"release: %s\n"+
							"kind: %s\n"+
							"anime: %v\n"+
							"library: %v\n"+
							"condition: %s\n"+
							"resolution: %s\n"+
							"codec: %s\n"+
							"hdr: %v\n"+
							"traits: %v\n"+
							"points: %d\n"+
							"matched rules: %+v\n"+
							"rejections: %+v\n"+
							"skipped: %+v\n"+
							"aggregate reports: %+v",
						outcome,
						got,
						want,
						ac.Name,
						cf.Release,
						cf.Kind,
						cf.Anime,
						envs[i].Library,
						cfg.When,
						envs[i].Resolution,
						envs[i].Parsed.Codec,
						envs[i].HDR,
						envs[i].Traits,
						out.Points,
						out.Matched,
						out.Rejections,
						out.Skipped,
						reports,
					)
				}
			}
		})
	}
}

func TestProfileSchemaCompatibility(t *testing.T) {
	if err := validateProfileSchema(expectedProfileSchema); err != nil {
		t.Fatalf(
			"current profile schema %d was rejected: %v",
			expectedProfileSchema,
			err,
		)
	}

	for _, schema := range []int{0, 2} {
		if err := validateProfileSchema(schema); err == nil {
			t.Fatalf(
				"schema %d unexpectedly passed compatibility guard",
				schema,
			)
		}
	}
}

func TestCompatibilityFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	for _, rf := range fixtures.Rules {
		rf := rf

		t.Run(rf.Name, func(t *testing.T) {
			// First validate the experimental/reference expression stored
			// with the fixture.
			fixtureRule := config.RuleConfig{
				Name:   rf.Name,
				Scope:  rf.Scope,
				When:   rf.When,
				Action: rf.Action,
				Points: rf.Points,
			}

			t.Run("fixture", func(t *testing.T) {
				runCases(
					t,
					rf,
					fixtureRule,
					defineLibrary...,
				)

				runAggregateCases(
					t,
					rf,
					fixtureRule,
					defineLibrary...,
				)
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

			if productionRule.Scope != rf.Scope {
				t.Fatalf(
					"production rule %q scope = %q, fixture expects %q",
					rf.ProductionRule,
					productionRule.Scope,
					rf.Scope,
				)
			}

			t.Run("production", func(t *testing.T) {
				runCases(
					t,
					rf,
					productionRule,
					defineLibrary...,
				)

				runAggregateCases(
					t,
					rf,
					productionRule,
					defineLibrary...,
				)
			})
		})
	}
}

func TestAnimeBluRayTierCeilings(t *testing.T) {
	data, err := os.ReadFile(
		"../../generated/vidhin-defines.json",
	)
	if err != nil {
		t.Fatalf(
			"read generated Define baseline: %v",
			err,
		)
	}

	var baseline struct {
		Defines map[string]struct {
			Tokens []string `json:"tokens"`
		} `json:"defines"`
	}

	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf(
			"decode generated Define baseline: %v",
			err,
		)
	}

	tokenForTier := func(tier int) string {
		t.Helper()

		name := fmt.Sprintf(
			"Anime Shows BluRay T%d Groups",
			tier,
		)

		entry, ok := baseline.Defines[name]
		if !ok {
			t.Fatalf(
				"missing Define baseline entry %q",
				name,
			)
		}

		if len(entry.Tokens) == 0 {
			t.Fatalf(
				"%s contains no release-group tokens",
				name,
			)
		}

		tokens := append(
			[]string(nil),
			entry.Tokens...,
		)

		slices.Sort(tokens)

		return tokens[0]
	}

	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	set, err := rules.Compile(
		productionRules,
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf(
			"compile complete production profile: %v",
			err,
		)
	}

	type auditCase struct {
		name     string
		tier     int
		metadata string
	}

	var cases []auditCase

	for tier := 1; tier <= 8; tier++ {
		cases = append(
			cases,
			auditCase{
				name: fmt.Sprintf(
					"T%d clean",
					tier,
				),
				tier: tier,
			},
		)

		if tier >= 2 {
			cases = append(
				cases,
				auditCase{
					name: fmt.Sprintf(
						"T%d full positive minor stack",
						tier,
					),
					tier:     tier,
					metadata: "Dual.Audio.Uncensored.v4.REPACK3",
				},
			)
		}
	}

	buildRelease := func(
		group string,
		metadata string,
	) string {
		parts := []string{
			"Example.Anime",
			"S01E01",
			"1080p",
			"BluRay",
			"x264",
		}

		if metadata != "" {
			parts = append(
				parts,
				strings.Split(metadata, ".")...,
			)
		}

		return strings.Join(parts, ".") + "-" + group
	}

	envs := make([]rules.Env, len(cases))

	for i, c := range cases {
		envs[i] = buildEnv(
			CaseFixture{
				Release: buildRelease(
					tokenForTier(c.tier),
					c.metadata,
				),
				Kind:  "anime_show",
				Anime: true,
			},
		)
	}

	state := set.ComputeAggregates(envs)
	if state == nil {
		t.Fatal(
			"ComputeAggregates returned nil state",
		)
	}

	scores := make(map[string]int)

	for i, c := range cases {
		state.Inject(&envs[i])

		out := set.Evaluate(
			envs[i],
			"anime_show",
		)

		scores[c.name] = out.Points
	}

	expectedClean := map[int]int{
		1: 500,
		2: 430,
		3: 360,
		4: 290,
		5: 220,
		6: 150,
		7: 80,
		8: 10,
	}

	for tier, expected := range expectedClean {
		name := fmt.Sprintf(
			"T%d clean",
			tier,
		)

		if scores[name] != expected {
			t.Fatalf(
				"%s score=%d want=%d",
				name,
				scores[name],
				expected,
			)
		}
	}

	for lowerTier := 2; lowerTier <= 8; lowerTier++ {
		higherTier := lowerTier - 1

		higherName := fmt.Sprintf(
			"T%d clean",
			higherTier,
		)

		lowerCleanName := fmt.Sprintf(
			"T%d clean",
			lowerTier,
		)

		lowerStackName := fmt.Sprintf(
			"T%d full positive minor stack",
			lowerTier,
		)

		higherClean := scores[higherName]
		lowerClean := scores[lowerCleanName]
		lowerStackRaw := scores[lowerStackName]

		if higherClean-lowerClean != 70 {
			t.Fatalf(
				"T%d -> T%d clean gap=%d want=70",
				higherTier,
				lowerTier,
				higherClean-lowerClean,
			)
		}

		// This test intentionally evaluates only the profile-rule
		// layer. The shared Anime Dual/Multi rule is +1010 because
		// it compensates for StreamNZB/jhin's native -1000 dubbed
		// audio score, leaving an effective +10 preference in the
		// complete ranking pipeline.
		//
		// The raw rules-layer metadata stack is therefore:
		//
		//   Dual/Multi +1010
		//   Uncensored   +10
		//   Anime v4      +4
		//   REPACK3       +7
		//                -----
		//                +1031
		//
		// Normalize the +1000 compensation before checking the
		// effective tier ceiling.
		const audioNativeCompensation = 1000

		if lowerStackRaw-lowerClean != 1031 {
			t.Fatalf(
				"T%d raw positive minor stack adds %d points; want 1031",
				lowerTier,
				lowerStackRaw-lowerClean,
			)
		}

		lowerStackEffective :=
			lowerStackRaw - audioNativeCompensation

		if lowerStackEffective-lowerClean != 31 {
			t.Fatalf(
				"T%d effective positive minor stack adds %d points; want 31",
				lowerTier,
				lowerStackEffective-lowerClean,
			)
		}

		if lowerStackEffective >= higherClean {
			t.Fatalf(
				"Anime BluRay tier ceiling inversion: effective %s=%d >= %s=%d",
				lowerStackName,
				lowerStackEffective,
				higherName,
				higherClean,
			)
		}

		if higherClean-lowerStackEffective != 39 {
			t.Fatalf(
				"T%d -> T%d effective ceiling headroom=%d want=39",
				higherTier,
				lowerTier,
				higherClean-lowerStackEffective,
			)
		}
	}
}

func TestMovieEditionPreferenceCeilings(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	imax := findProductionRule(
		t,
		productionRules,
		"IMAX",
	)
	openMatte := findProductionRule(
		t,
		productionRules,
		"Open matte",
	)

	if imax.Points != 800 || imax.Scope != "movie" {
		t.Fatalf(
			"IMAX policy drifted: points=%d scope=%q",
			imax.Points,
			imax.Scope,
		)
	}

	if openMatte.Points != 25 ||
		openMatte.Scope != "movie" {
		t.Fatalf(
			"Open matte policy drifted: points=%d scope=%q",
			openMatte.Points,
			openMatte.Scope,
		)
	}

	set, err := rules.Compile(
		productionRules,
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf(
			"compile production profile: %v",
			err,
		)
	}

	data, err := os.ReadFile(
		"../../generated/vidhin-defines.json",
	)
	if err != nil {
		t.Fatalf(
			"read generated Vidhin data: %v",
			err,
		)
	}

	var generated struct {
		Defines map[string]struct {
			Tokens []string `json:"tokens"`
		} `json:"defines"`
	}

	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf(
			"decode generated Vidhin data: %v",
			err,
		)
	}

	defineToken := func(name string) string {
		entry, ok := generated.Defines[name]
		if !ok {
			t.Fatalf("missing Define %q", name)
		}

		if len(entry.Tokens) == 0 {
			t.Fatalf(
				"Define %q has no tokens",
				name,
			)
		}

		tokens := append(
			[]string(nil),
			entry.Tokens...,
		)
		slices.Sort(tokens)

		return tokens[0]
	}

	movieT1 := defineToken("Movies WEB T1 Groups")
	movieT2 := defineToken("Movies WEB T2 Groups")
	movieT3 := defineToken("Movies WEB T3 Groups")
	showT3 := defineToken("Shows WEB T3 Groups")
	animeBDT8 := defineToken(
		"Anime Shows BluRay T8 Groups",
	)
	animeWEBT6 := defineToken(
		"Anime Shows WEB T6 Groups",
	)

	type auditCase struct {
		name      string
		release   string
		kind      string
		anime     bool
		wantScore int
		wantRules []string
		noRules   []string
	}

	makeRelease := func(
		prefix string,
		group string,
		extra string,
	) string {
		parts := []string{prefix}

		if extra != "" {
			parts = append(parts, extra)
		}

		return strings.Join(parts, ".") + "-" + group
	}

	cases := []auditCase{
		{
			name: "Movie T1 clean",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT1,
				"",
			),
			kind:      "movie",
			wantScore: 500,
		},
		{
			name: "Movie T2 IMAX",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT2,
				"IMAX",
			),
			kind:      "movie",
			wantScore: 1100,
			wantRules: []string{"IMAX"},
		},
		{
			name: "Movie T3 IMAX",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT3,
				"IMAX",
			),
			kind:      "movie",
			wantScore: 900,
			wantRules: []string{"IMAX"},
		},
		{
			name: "Movie T2 Open Matte",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT2,
				"Open.Matte",
			),
			kind:      "movie",
			wantScore: 325,
			wantRules: []string{
				"Open matte",
			},
		},
		{
			name: "Movie T3 Open Matte",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT3,
				"Open.Matte",
			),
			kind:      "movie",
			wantScore: 125,
			wantRules: []string{
				"Open matte",
			},
		},
		{
			name: "Movie T3 IMAX Open Matte",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT3,
				"IMAX.Open.Matte",
			),
			kind:      "movie",
			wantScore: 925,
			wantRules: []string{
				"IMAX",
				"Open matte",
			},
		},
		{
			name: "Show T3 IMAX Open Matte",
			release: makeRelease(
				"Example.Show.S01E01.1080p.WEB-DL.x264",
				showT3,
				"IMAX.Open.Matte",
			),
			kind:      "series",
			wantScore: 100,
			noRules: []string{
				"IMAX",
				"Open matte",
			},
		},
		{
			name: "Anime BluRay T8 IMAX Open Matte",
			release: makeRelease(
				"Example.Anime.S01E01.1080p.BluRay.x264",
				animeBDT8,
				"IMAX.Open.Matte",
			),
			kind:      "anime_show",
			anime:     true,
			wantScore: 10,
			noRules: []string{
				"IMAX",
				"Open matte",
			},
		},
		{
			name: "Anime WEB T6 IMAX Open Matte",
			release: makeRelease(
				"Example.Anime.S01E01.1080p.WEB-DL.x264",
				animeWEBT6,
				"IMAX.Open.Matte",
			),
			kind:      "anime_show",
			anime:     true,
			wantScore: 50,
			noRules: []string{
				"IMAX",
				"Open matte",
			},
		},
	}

	envs := make([]rules.Env, len(cases))

	for i, c := range cases {
		envs[i] = buildEnv(
			CaseFixture{
				Release: c.release,
				Kind:    c.kind,
				Anime:   c.anime,
			},
		)
	}

	state := set.ComputeAggregates(envs)

	if state == nil {
		t.Fatal("ComputeAggregates returned nil")
	}

	results := map[string]int{}

	for i, c := range cases {
		state.Inject(&envs[i])

		out := set.Evaluate(
			envs[i],
			c.kind,
		)

		names := make(
			[]string,
			0,
			len(out.Matched),
		)

		for _, matched := range out.Matched {
			names = append(
				names,
				matched.Name,
			)
		}

		slices.Sort(names)

		if out.Points != c.wantScore {
			t.Fatalf(
				"%s score=%+d want=%+d matched=%v",
				c.name,
				out.Points,
				c.wantScore,
				names,
			)
		}

		for _, want := range c.wantRules {
			if !slices.Contains(names, want) {
				t.Fatalf(
					"%s missing rule %q; matched=%v",
					c.name,
					want,
					names,
				)
			}
		}

		for _, unwanted := range c.noRules {
			if slices.Contains(names, unwanted) {
				t.Fatalf(
					"%s unexpectedly matched rule %q",
					c.name,
					unwanted,
				)
			}
		}

		results[c.name] = out.Points
	}

	if results["Movie T2 Open Matte"] >=
		results["Movie T1 clean"] {
		t.Fatal(
			"Open Matte must not override clean Movie T1",
		)
	}

	if results["Movie T3 Open Matte"] >= 300 {
		t.Fatal(
			"Open Matte must not move Movie T3 into T2 range",
		)
	}

	if results["Movie T2 IMAX"] <=
		results["Movie T1 clean"] {
		t.Fatal(
			"IMAX must remain a strong Movie-version preference",
		)
	}

	if results["Movie T3 IMAX"] <=
		results["Movie T1 clean"] {
		t.Fatal(
			"IMAX must intentionally override Movie tiers",
		)
	}

	if results["Movie T3 IMAX Open Matte"]-
		results["Movie T3 IMAX"] != 25 {
		t.Fatal(
			"Open Matte contribution beside IMAX must remain +25",
		)
	}
}

func TestEffectiveAvailabilityLibraryAndAudioPolicy(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	forbidden := map[string]bool{
		"Library hit":        true,
		"Very fresh NZB":     true,
		"Recent NZB":         true,
		"Popular NZB":        true,
		"Very popular NZB":   true,
		"Highly popular NZB": true,
	}

	for _, rule := range productionRules {
		if forbidden[rule.Name] {
			t.Fatalf(
				"removed score rule returned: %s",
				rule.Name,
			)
		}
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Effective scoring regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf(
			"compile production profile: %v",
			err,
		)
	}

	if profile.LibraryScoreBonus != 500 {
		t.Fatalf(
			"native library bonus = %+d, want +500",
			profile.LibraryScoreBonus,
		)
	}

	score := func(
		name string,
		title string,
		kind string,
		anime bool,
		library bool,
		avail triage.AvailState,
	) int {
		t.Helper()

		candidate := triage.Candidate{
			Release: &release.Release{
				Title:     title,
				IsLibrary: library,
			},
		}

		candidate.Verdict.Avail = avail

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{
				Kind:    kind,
				IsAnime: anime,
				Season:  1,
				Episode: 1,
				Title:   "Example",
			},
			[]triage.Candidate{candidate},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 {
			t.Fatalf(
				"%s unexpectedly rejected: %+v",
				name,
				rejected,
			)
		}

		if len(kept) != 1 {
			t.Fatalf(
				"%s kept %d candidates, want 1",
				name,
				len(kept),
			)
		}

		return kept[0].Torrent.Rank
	}

	movieCleanName :=
		"Example.Movie.2025.1080p.WEB-DL.H264-GRP"

	movieDubbedName :=
		"Example.Movie.2025.1080p.WEB-DL.H264.DUBBED-GRP"

	base := score(
		"Movie clean",
		movieCleanName,
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	library := score(
		"Movie library",
		movieCleanName,
		ranking.KindMovie,
		false,
		true,
		triage.AvailState{},
	)

	if got := library - base; got != 500 {
		t.Fatalf(
			"effective Library delta = %+d, want +500",
			got,
		)
	}

	backbone := score(
		"Movie backbone",
		movieCleanName,
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{
			Status:       triage.AvailAvailable,
			OnMyBackbone: true,
			CheckedAt:    time.Now().Add(-90 * 24 * time.Hour),
		},
	)

	if got := backbone - base; got != 20 {
		t.Fatalf(
			"effective backbone delta = %+d, want +20",
			got,
		)
	}

	recent := score(
		"Movie recently confirmed",
		movieCleanName,
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{
			Status:    triage.AvailAvailable,
			CheckedAt: time.Now().Add(-3 * 24 * time.Hour),
		},
	)

	if got := recent - base; got != 10 {
		t.Fatalf(
			"effective recent-confirmation delta = %+d, want +10",
			got,
		)
	}

	both := score(
		"Movie backbone plus recent",
		movieCleanName,
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{
			Status:       triage.AvailAvailable,
			OnMyBackbone: true,
			CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
		},
	)

	if got := both - base; got != 30 {
		t.Fatalf(
			"effective positive availability ceiling = %+d, want +30",
			got,
		)
	}

	dubbed := score(
		"Movie DUBBED",
		movieDubbedName,
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	if got := dubbed - base; got != 10 {
		t.Fatalf(
			"effective non-Anime DUBBED delta = %+d, want +10",
			got,
		)
	}

	animeCleanName :=
		"Example.Anime.S01E01.1080p.WEB-DL.H264-GRP"

	animeDualName :=
		"Example.Anime.S01E01.1080p.WEB-DL.H264.Dual.Audio-GRP"

	animeMultiName :=
		"Example.Anime.S01E01.1080p.WEB-DL.H264.Multi.Audio-GRP"

	animeBase := score(
		"Anime clean",
		animeCleanName,
		ranking.KindAnimeShow,
		true,
		false,
		triage.AvailState{},
	)

	animeDual := score(
		"Anime Dual Audio",
		animeDualName,
		ranking.KindAnimeShow,
		true,
		false,
		triage.AvailState{},
	)

	if got := animeDual - animeBase; got != 10 {
		t.Fatalf(
			"effective Anime Dual Audio delta = %+d, want +10",
			got,
		)
	}

	animeMulti := score(
		"Anime Multi Audio",
		animeMultiName,
		ranking.KindAnimeShow,
		true,
		false,
		triage.AvailState{},
	)

	if got := animeMulti - animeBase; got != 10 {
		t.Fatalf(
			"effective Anime Multi Audio delta = %+d, want +10",
			got,
		)
	}
}
