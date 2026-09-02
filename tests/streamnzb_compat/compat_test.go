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
	streamparser "streamnzb/pkg/search/parser"
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

func loadProfileRules(
	t *testing.T,
	path string,
	label string,
) []config.RuleConfig {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s profile: %v", label, err)
	}

	code := strings.TrimSpace(string(data))
	if !strings.HasPrefix(code, profilePrefix) {
		t.Fatalf(
			"%s profile does not start with %q",
			label,
			profilePrefix,
		)
	}

	encoded := strings.TrimPrefix(code, profilePrefix)

	compressed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf(
			"decode %s profile Base64URL: %v",
			label,
			err,
		)
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf(
			"open %s profile gzip payload: %v",
			label,
			err,
		)
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf(
			"read %s profile gzip payload: %v",
			label,
			err,
		)
	}

	var profile profilePayload
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatalf(
			"decode %s profile JSON: %v",
			label,
			err,
		)
	}

	if err := validateProfileSchema(profile.StreamNZBProfile); err != nil {
		t.Fatal(err)
	}

	if len(profile.Rules) == 0 {
		t.Fatalf("%s profile contains no rules", label)
	}

	return profile.Rules
}

func loadProductionRules(t *testing.T) []config.RuleConfig {
	t.Helper()

	return loadProfileRules(
		t,
		"../../profile.txt",
		"production",
	)
}

func loadNeutralRules(t *testing.T) []config.RuleConfig {
	t.Helper()

	return loadProfileRules(
		t,
		"../../profile-neutral.txt",
		"neutral",
	)
}

func TestNeutralProfileSchemaCompatibility(t *testing.T) {
	neutralRules := loadNeutralRules(t)

	if len(neutralRules) != 108 {
		t.Fatalf(
			"neutral profile contains %d rules; want 108",
			len(neutralRules),
		)
	}

	deviceRules := map[string]bool{
		"DV without HDR fallback":   false,
		"Reduce Atmos":              false,
		"Reduce TrueHD bonus":       false,
		"Reduce DTS Lossless bonus": false,
	}

	reject3D := false

	for _, rule := range neutralRules {
		if _, ok := deviceRules[rule.Name]; ok {
			deviceRules[rule.Name] = true
		}

		if rule.Name == "Reject 3D" {
			reject3D = true
		}
	}

	for name, present := range deviceRules {
		if present {
			t.Fatalf(
				"neutral profile unexpectedly contains device rule %q",
				name,
			)
		}
	}

	if !reject3D {
		t.Fatal("neutral profile is missing core rule \"Reject 3D\"")
	}

	defineLibrary := loadDefineLibrary(t)

	// rules.Compile performs a static compatibility compile without the
	// score-relative runtime attributes supplied by StreamNZB's ranking
	// pipeline. Adaptive Low-Score Filtering intentionally uses finalScore
	// and is covered separately by the released-engine production-policy
	// regression below.
	staticRules := make(
		[]config.RuleConfig,
		0,
		len(neutralRules),
	)
	scoreAwareRules := 0

	for _, rule := range neutralRules {
		if rule.Name == "Adaptive Low-Score Filtering" {
			scoreAwareRules++
			continue
		}

		staticRules = append(staticRules, rule)
	}

	if scoreAwareRules != 1 {
		t.Fatalf(
			"neutral profile contains %d score-aware Adaptive Low-Score rules; want 1",
			scoreAwareRules,
		)
	}

	set, err := rules.Compile(
		staticRules,
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf(
			"compile complete neutral profile: %v",
			err,
		)
	}

	if set == nil {
		t.Fatal("compiled neutral profile is nil")
	}
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

			kind := ac.Candidates[0].Kind

			for _, cf := range ac.Candidates {
				if cf.Kind != kind {
					t.Fatalf(
						"aggregate fixture mixes request kinds: %q and %q",
						kind,
						cf.Kind,
					)
				}
			}

			state := set.ComputeAggregates(envs, kind)
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
					_, reports := set.ReportAggregates(envs, kind)

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

func TestEpisodeParsingCompatibility(t *testing.T) {
	type rankCheck struct {
		season  int
		episode int
		want    int
	}

	tests := []struct {
		name         string
		release      string
		wantSeasons  []int
		wantEpisodes []int
		wantComplete bool
		ranks        []rankCheck
	}{
		{
			name:         "hybrid Anime season episode plus absolute number",
			release:      "Dr.STONE.2019.S04E16-074.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{4},
			wantEpisodes: []int{16},
			ranks: []rankCheck{
				{season: 4, episode: 16, want: 4},
				{season: 4, episode: 74, want: 0},
			},
		},
		{
			name:         "multi episode with repeated E prefix",
			release:      "Show.S01E01-E02.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{1},
			wantEpisodes: []int{1, 2},
			ranks: []rankCheck{
				{season: 1, episode: 1, want: 3},
				{season: 1, episode: 2, want: 3},
				{season: 1, episode: 3, want: 0},
			},
		},
		{
			name:         "compact multi episode",
			release:      "Show.S01E01E02.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{1},
			wantEpisodes: []int{1, 2},
			ranks: []rankCheck{
				{season: 1, episode: 1, want: 3},
				{season: 1, episode: 2, want: 3},
			},
		},
		{
			name:         "compact episode range",
			release:      "Show.S01E01-02.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{1},
			wantEpisodes: []int{1, 2},
			ranks: []rankCheck{
				{season: 1, episode: 1, want: 3},
				{season: 1, episode: 2, want: 3},
			},
		},
		{
			name:         "expanded episode range",
			release:      "Show.S01E01-E03.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{1},
			wantEpisodes: []int{1, 2, 3},
			ranks: []rankCheck{
				{season: 1, episode: 1, want: 3},
				{season: 1, episode: 3, want: 3},
			},
		},
		{
			name:         "Anime absolute episode range",
			release:      "[Group] Anime Title 001-012 [1080p]",
			wantEpisodes: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			ranks: []rankCheck{
				{season: 0, episode: 1, want: 3},
				{season: 0, episode: 12, want: 3},
			},
		},
		{
			name:         "dashed Anime season episode",
			release:      "[SubsPlease] Anime Title S4 - 03 (1080p)",
			wantSeasons:  []int{4},
			wantEpisodes: []int{3},
			ranks: []rankCheck{
				{season: 4, episode: 3, want: 4},
			},
		},
		{
			name:        "compact season range remains a season pack",
			release:     "Show.S03-08.1080p.WEB-DL-GROUP",
			wantSeasons: []int{3, 4, 5, 6, 7, 8},
			ranks: []rankCheck{
				{season: 4, episode: 3, want: 2},
				{season: 8, episode: 20, want: 2},
			},
		},
		{
			name:         "complete single season remains a season pack",
			release:      "Show.S01.COMPLETE.1080p.WEB-DL-GROUP",
			wantSeasons:  []int{1},
			wantComplete: true,
			ranks: []rankCheck{
				{season: 1, episode: 1, want: 2},
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			raw := jhin.Parse(tc.release)
			if raw == nil {
				t.Fatal("jhin.Parse returned nil")
			}

			parsed := streamparser.ParseReleaseTitle(tc.release)
			if parsed == nil {
				t.Fatal("ParseReleaseTitle returned nil")
			}

			if !slices.Equal(raw.Seasons, tc.wantSeasons) {
				t.Fatalf(
					"Jhin seasons = %v, want %v\nrelease: %s",
					raw.Seasons,
					tc.wantSeasons,
					tc.release,
				)
			}

			if !slices.Equal(raw.Episodes, tc.wantEpisodes) {
				t.Fatalf(
					"Jhin episodes = %v, want %v\nrelease: %s",
					raw.Episodes,
					tc.wantEpisodes,
					tc.release,
				)
			}

			if raw.Complete != tc.wantComplete {
				t.Fatalf(
					"Jhin complete = %v, want %v\nrelease: %s",
					raw.Complete,
					tc.wantComplete,
					tc.release,
				)
			}

			if !slices.Equal(parsed.Seasons, tc.wantSeasons) {
				t.Fatalf(
					"StreamNZB seasons = %v, want %v\nrelease: %s",
					parsed.Seasons,
					tc.wantSeasons,
					tc.release,
				)
			}

			if !slices.Equal(parsed.Episodes, tc.wantEpisodes) {
				t.Fatalf(
					"StreamNZB episodes = %v, want %v\nrelease: %s",
					parsed.Episodes,
					tc.wantEpisodes,
					tc.release,
				)
			}

			if parsed.Complete != tc.wantComplete {
				t.Fatalf(
					"StreamNZB complete = %v, want %v\nrelease: %s",
					parsed.Complete,
					tc.wantComplete,
					tc.release,
				)
			}

			for _, check := range tc.ranks {
				got := parsed.EpisodeMatchRank(
					check.season,
					check.episode,
				)

				if got != check.want {
					t.Errorf(
						"EpisodeMatchRank(%d, %d) = %d, want %d\n"+
							"release: %s\n"+
							"seasons: %v\n"+
							"episodes: %v",
						check.season,
						check.episode,
						got,
						check.want,
						tc.release,
						parsed.Seasons,
						parsed.Episodes,
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

func TestAnimeTierEffectiveCeilings(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Anime effective tier ceiling regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	data, err := os.ReadFile(
		"../../generated/vidhin-defines.json",
	)
	if err != nil {
		t.Fatalf(
			"read generated Vidhin baseline: %v",
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
			"decode generated Vidhin baseline: %v",
			err,
		)
	}

	defineToken := func(name string) string {
		t.Helper()

		entry, ok := generated.Defines[name]
		if !ok {
			t.Fatalf("missing Define %q", name)
		}

		if len(entry.Tokens) == 0 {
			t.Fatalf("Define %q has no tokens", name)
		}

		tokens := append([]string(nil), entry.Tokens...)
		slices.Sort(tokens)

		return tokens[0]
	}

	type scoreInput struct {
		title string
		kind  string
		avail triage.AvailState
	}

	score := func(in scoreInput) int {
		t.Helper()

		candidate := triage.Candidate{
			Release: &release.Release{
				Title: in.title,
			},
		}

		candidate.Verdict.Avail = in.avail

		request := ranking.Request{
			Kind:    in.kind,
			IsAnime: true,
			Title:   "Example Anime",
		}

		if in.kind == ranking.KindAnimeShow {
			request.Season = 1
			request.Episode = 1
		}

		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{candidate},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 {
			t.Fatalf(
				"ceiling candidate unexpectedly rejected: %+v",
				rejected,
			)
		}

		if len(kept) != 1 {
			t.Fatalf(
				"ceiling candidate kept %d releases; want 1",
				len(kept),
			)
		}

		return kept[0].Torrent.Rank
	}

	buildShow := func(
		source string,
		group string,
		episode string,
		extras ...string,
	) string {
		parts := []string{
			"Example.Anime",
			episode,
			"1080p",
			source,
			"x264",
		}

		parts = append(parts, extras...)

		return strings.Join(parts, ".") + "-" + group
	}

	buildMovie := func(
		source string,
		group string,
		extras ...string,
	) string {
		parts := []string{
			"Example.Anime.Movie",
			"2025",
			"1080p",
			source,
			"x264",
		}

		parts = append(parts, extras...)

		return strings.Join(parts, ".") + "-" + group
	}

	fullAvailability := triage.AvailState{
		Status:       triage.AvailAvailable,
		OnMyBackbone: true,
		CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
	}

	// Effective portable Anime Show maxima:
	//
	// BluRay:
	//   Dual/Multi Audio       +10
	//   Uncensored             +10
	//   Anime v4                +4
	//   REPACK3                 +7
	//   Complete Season Pack   +10
	//   Backbone availability  +20
	//   Recent confirmation    +10
	//                          ----
	//                           +71
	//
	// WEB additionally includes strongest current service:
	//   CR                       +6
	//                          ----
	//                           +77
	//
	// Anime Movies do not receive Complete Season Pack, so their
	// corresponding maxima are +61 BluRay and +67 WEB.
	const (
		maxShowBluRayStack  = 71
		maxShowWEBStack     = 77
		maxMovieBluRayStack = 61
		maxMovieWEBStack    = 67
		minAnimeTierGap     = 80
	)

	blurayPoints := []int{
		560,
		480,
		400,
		320,
		240,
		160,
		80,
		0,
	}

	webPoints := []int{
		500,
		400,
		300,
		200,
		100,
		20,
	}

	for _, media := range []string{
		"Anime Movies",
		"Anime Shows",
	} {
		for i, want := range blurayPoints {
			name := fmt.Sprintf(
				"%s BluRay T%d",
				media,
				i+1,
			)

			rule := findProductionRule(
				t,
				productionRules,
				name,
			)

			if rule.Points != want {
				t.Fatalf(
					"%s points=%d, want %d",
					name,
					rule.Points,
					want,
				)
			}
		}

		for i, want := range webPoints {
			name := fmt.Sprintf(
				"%s WEB T%d",
				media,
				i+1,
			)

			rule := findProductionRule(
				t,
				productionRules,
				name,
			)

			if rule.Points != want {
				t.Fatalf(
					"%s points=%d, want %d",
					name,
					rule.Points,
					want,
				)
			}
		}
	}

	for i := 0; i < len(blurayPoints)-1; i++ {
		gap := blurayPoints[i] - blurayPoints[i+1]

		if gap < minAnimeTierGap {
			t.Fatalf(
				"BluRay T%d->T%d gap=%d, want >= %d",
				i+1,
				i+2,
				gap,
				minAnimeTierGap,
			)
		}

		if gap <= maxShowBluRayStack {
			t.Fatalf(
				"BluRay T%d->T%d gap=%d does not dominate "+
					"maximum Anime stack %d",
				i+1,
				i+2,
				gap,
				maxShowBluRayStack,
			)
		}
	}

	for i := 0; i < len(webPoints)-1; i++ {
		gap := webPoints[i] - webPoints[i+1]

		if gap < minAnimeTierGap {
			t.Fatalf(
				"WEB T%d->T%d gap=%d, want >= %d",
				i+1,
				i+2,
				gap,
				minAnimeTierGap,
			)
		}

		if gap <= maxShowWEBStack {
			t.Fatalf(
				"WEB T%d->T%d gap=%d does not dominate "+
					"maximum Anime stack %d",
				i+1,
				i+2,
				gap,
				maxShowWEBStack,
			)
		}
	}

	type familyCase struct {
		label       string
		mediaPrefix string
		kind        string
		build       func(
			source string,
			group string,
			extras ...string,
		) string
		maxBluRay int
		maxWEB    int
	}

	showBuild := func(
		source string,
		group string,
		extras ...string,
	) string {
		return buildShow(
			source,
			group,
			"S01.COMPLETE",
			extras...,
		)
	}

	movieBuild := func(
		source string,
		group string,
		extras ...string,
	) string {
		return buildMovie(
			source,
			group,
			extras...,
		)
	}

	families := []familyCase{
		{
			label:       "Anime Show",
			mediaPrefix: "Anime Shows",
			kind:        ranking.KindAnimeShow,
			build:       showBuild,
			maxBluRay:   maxShowBluRayStack,
			maxWEB:      maxShowWEBStack,
		},
		{
			label:       "Anime Movie",
			mediaPrefix: "Anime Movies",
			kind:        ranking.KindAnimeMovie,
			build:       movieBuild,
			maxBluRay:   maxMovieBluRayStack,
			maxWEB:      maxMovieWEBStack,
		},
	}

	for _, family := range families {
		t.Run(family.label, func(t *testing.T) {
			blurayGroup := defineToken(
				fmt.Sprintf(
					"%s BluRay T2 Groups",
					family.mediaPrefix,
				),
			)

			var blurayBaseTitle string

			if family.kind == ranking.KindAnimeShow {
				blurayBaseTitle = buildShow(
					"BluRay",
					blurayGroup,
					"S01E01",
				)
			} else {
				blurayBaseTitle = buildMovie(
					"BluRay",
					blurayGroup,
				)
			}

			blurayBase := score(scoreInput{
				title: blurayBaseTitle,
				kind:  family.kind,
			})

			blurayFull := score(scoreInput{
				title: family.build(
					"BluRay",
					blurayGroup,
					"Dual",
					"Audio",
					"Uncensored",
					"v4",
					"REPACK3",
				),
				kind:  family.kind,
				avail: fullAvailability,
			})

			if got := blurayFull - blurayBase; got != family.maxBluRay {
				t.Fatalf(
					"%s effective BluRay stack=%+d, want %+d",
					family.label,
					got,
					family.maxBluRay,
				)
			}

			webGroup := defineToken(
				fmt.Sprintf(
					"%s WEB T6 Groups",
					family.mediaPrefix,
				),
			)

			var webBaseTitle string

			if family.kind == ranking.KindAnimeShow {
				webBaseTitle = buildShow(
					"WEB-DL",
					webGroup,
					"S01E01",
				)
			} else {
				webBaseTitle = buildMovie(
					"WEB-DL",
					webGroup,
				)
			}

			webBase := score(scoreInput{
				title: webBaseTitle,
				kind:  family.kind,
			})

			webFull := score(scoreInput{
				title: family.build(
					"WEB-DL",
					webGroup,
					"CR",
					"Dual",
					"Audio",
					"Uncensored",
					"v4",
					"REPACK3",
				),
				kind:  family.kind,
				avail: fullAvailability,
			})

			if got := webFull - webBase; got != family.maxWEB {
				t.Fatalf(
					"%s effective WEB stack=%+d, want %+d",
					family.label,
					got,
					family.maxWEB,
				)
			}

			for lowerTier := 2; lowerTier <= 8; lowerTier++ {
				higherTier := lowerTier - 1

				higherGroup := defineToken(
					fmt.Sprintf(
						"%s BluRay T%d Groups",
						family.mediaPrefix,
						higherTier,
					),
				)

				lowerGroup := defineToken(
					fmt.Sprintf(
						"%s BluRay T%d Groups",
						family.mediaPrefix,
						lowerTier,
					),
				)

				var higherTitle string

				if family.kind == ranking.KindAnimeShow {
					higherTitle = buildShow(
						"BluRay",
						higherGroup,
						"S01E01",
					)
				} else {
					higherTitle = buildMovie(
						"BluRay",
						higherGroup,
					)
				}

				higher := score(scoreInput{
					title: higherTitle,
					kind:  family.kind,
				})

				lower := score(scoreInput{
					title: family.build(
						"BluRay",
						lowerGroup,
						"Dual",
						"Audio",
						"Uncensored",
						"v4",
						"REPACK3",
					),
					kind:  family.kind,
					avail: fullAvailability,
				})

				if lower >= higher {
					t.Fatalf(
						"%s BluRay T%d full rank=%d must remain "+
							"below clean T%d rank=%d",
						family.label,
						lowerTier,
						lower,
						higherTier,
						higher,
					)
				}
			}

			for lowerTier := 2; lowerTier <= 6; lowerTier++ {
				higherTier := lowerTier - 1

				higherGroup := defineToken(
					fmt.Sprintf(
						"%s WEB T%d Groups",
						family.mediaPrefix,
						higherTier,
					),
				)

				lowerGroup := defineToken(
					fmt.Sprintf(
						"%s WEB T%d Groups",
						family.mediaPrefix,
						lowerTier,
					),
				)

				var higherTitle string

				if family.kind == ranking.KindAnimeShow {
					higherTitle = buildShow(
						"WEB-DL",
						higherGroup,
						"S01E01",
					)
				} else {
					higherTitle = buildMovie(
						"WEB-DL",
						higherGroup,
					)
				}

				higher := score(scoreInput{
					title: higherTitle,
					kind:  family.kind,
				})

				lower := score(scoreInput{
					title: family.build(
						"WEB-DL",
						lowerGroup,
						"CR",
						"Dual",
						"Audio",
						"Uncensored",
						"v4",
						"REPACK3",
					),
					kind:  family.kind,
					avail: fullAvailability,
				})

				if lower >= higher {
					t.Fatalf(
						"%s WEB T%d full rank=%d must remain "+
							"below clean T%d rank=%d",
						family.label,
						lowerTier,
						lower,
						higherTier,
						higher,
					)
				}
			}
		})
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

	const nativeEditionPoints = 100

	if imax.Points != 700 || imax.Scope != "movie" {
		t.Fatalf(
			"IMAX stored policy drifted: points=%d scope=%q",
			imax.Points,
			imax.Scope,
		)
	}

	if imax.Points+nativeEditionPoints != 800 {
		t.Fatalf(
			"IMAX effective policy drifted: "+
				"stored=%d native=%d effective=%d",
			imax.Points,
			nativeEditionPoints,
			imax.Points+nativeEditionPoints,
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

	// Candidate-relative prune rules use finalScore/current and are
	// evaluated by StreamNZB's ranking layer after individual release
	// scoring. This ceiling test intentionally exercises the lower-level
	// rules engine for single-release edition score math, so exclude the
	// Adaptive Low-Score prune rule here. Its released-engine behavior is
	// covered separately by TestAdaptiveLowScoreProductionPolicy.
	editionScoringRules := make(
		[]config.RuleConfig,
		0,
		len(productionRules),
	)

	for _, rule := range productionRules {
		if rule.Name == "Adaptive Low-Score Filtering" {
			continue
		}

		editionScoringRules = append(
			editionScoringRules,
			rule,
		)
	}

	set, err := rules.Compile(
		editionScoringRules,
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
			wantScore: 1000,
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
			wantScore: 800,
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
			wantScore: 825,
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
			wantScore: 0,
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
			wantScore: 20,
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

	states := make(map[string]*rules.AggregateState)

	for _, kind := range []string{"movie", "series", "anime_show"} {
		kindEnvs := make([]rules.Env, 0)

		for i, c := range cases {
			if c.kind == kind {
				kindEnvs = append(kindEnvs, envs[i])
			}
		}

		if len(kindEnvs) == 0 {
			continue
		}

		state := set.ComputeAggregates(kindEnvs, kind)
		if state == nil {
			t.Fatalf("ComputeAggregates returned nil for %s", kind)
		}

		states[kind] = state
	}

	results := map[string]int{}

	for i, c := range cases {
		state := states[c.kind]
		if state == nil {
			t.Fatalf("missing aggregate state for %s", c.kind)
		}

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

func TestIMAXEnhancedParserRegression(t *testing.T) {
	cases := []struct {
		name         string
		release      string
		wantEdition  string
		wantUpscaled bool
	}{
		{
			name:    "plain",
			release: "Movie.2026.2160p.WEB-DL.x265-GRP",
		},
		{
			name:        "IMAX",
			release:     "Movie.2026.2160p.IMAX.WEB-DL.x265-GRP",
			wantEdition: "IMAX",
		},
		{
			name:    "bare Enhanced",
			release: "Movie.2026.2160p.Enhanced.WEB-DL.x265-GRP",
		},
		{
			name:        "IMAX dotted Enhanced",
			release:     "Movie.2026.2160p.IMAX.Enhanced.WEB-DL.x265-GRP",
			wantEdition: "IMAX",
		},
		{
			name:        "IMAX hyphen Enhanced",
			release:     "Movie.2026.2160p.IMAX-Enhanced.WEB-DL.x265-GRP",
			wantEdition: "IMAX",
		},
		{
			name:    "compact IMAXEnhanced",
			release: "Movie.2026.2160p.IMAXEnhanced.WEB-DL.x265-GRP",
		},
		{
			name:         "AI Enhanced",
			release:      "Movie.2026.2160p.AI.Enhanced.WEB-DL.x265-GRP",
			wantUpscaled: true,
		},
		{
			name:         "Upscaled",
			release:      "Movie.2026.2160p.Upscaled.WEB-DL.x265-GRP",
			wantUpscaled: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := buildEnv(
				CaseFixture{
					Release: c.release,
					Kind:    "movie",
				},
			)

			if env.Edition != c.wantEdition {
				t.Fatalf(
					"edition=%q want=%q release=%q",
					env.Edition,
					c.wantEdition,
					c.release,
				)
			}

			if env.Upscaled != c.wantUpscaled {
				t.Fatalf(
					"upscaled=%v want=%v release=%q",
					env.Upscaled,
					c.wantUpscaled,
					c.release,
				)
			}
		})
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

	proper := score(
		"Movie PROPER",
		"Example.Movie.2025.1080p.WEB-DL.H264.PROPER-GRP",
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	if got := proper - base; got != 5 {
		t.Fatalf(
			"effective PROPER delta = %+d, want +5",
			got,
		)
	}

	repack := score(
		"Movie REPACK",
		"Example.Movie.2025.1080p.WEB-DL.H264.REPACK-GRP",
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	if got := repack - base; got != 5 {
		t.Fatalf(
			"effective REPACK delta = %+d, want +5",
			got,
		)
	}

	repack2 := score(
		"Movie REPACK2",
		"Example.Movie.2025.1080p.WEB-DL.H264.REPACK2-GRP",
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	if got := repack2 - base; got != 6 {
		t.Fatalf(
			"effective REPACK2 delta = %+d, want +6",
			got,
		)
	}

	repack3 := score(
		"Movie REPACK3",
		"Example.Movie.2025.1080p.WEB-DL.H264.REPACK3-GRP",
		ranking.KindMovie,
		false,
		false,
		triage.AvailState{},
	)

	if got := repack3 - base; got != 7 {
		t.Fatalf(
			"effective REPACK3 delta = %+d, want +7",
			got,
		)
	}
}

// TestProductionRankingPreservesSameReleaseVariants protects the template side
// of StreamNZB same-release failover. StreamNZB performs copy merging and
// primary/fallback selection before profile ranking; once a merged release
// reaches the profile, ranking must preserve its attached playback variants.
//
// MergeSameReleaseVariants and DropCopies are runtime-owned behavior covered by
// the pinned StreamNZB test suite. This regression deliberately avoids
// reimplementing those runtime tests or widening this harness's dependency
// graph merely to import the search package.
func TestProductionRankingPreservesSameReleaseVariants(t *testing.T) {
	const (
		primaryURL  = "https://drunkenslug.example/details/456"
		fallbackURL = "https://nzbgeek.example/details/123"
	)

	merged := &release.Release{
		Title:      "Example.Movie.2025.1080p.WEB-DL.DDP5.1.H.264-GRP",
		DetailsURL: primaryURL,
		Link:       "https://drunkenslug.example/get/456",
		GUID:       "slug-456",
		Indexer:    "DrunkenSlug",
		Grabs:      50,
		Variants: []*release.Release{
			{
				Title:      "Example.Movie.2025.1080p.WEB-DL.DDP5.1.H.264-GRP",
				DetailsURL: fallbackURL,
				Link:       "https://nzbgeek.example/get/123",
				GUID:       "geek-123",
				Indexer:    "NZBGeek",
				Grabs:      100,
			},
		},
	}

	if got := merged.CopyCount(); got != 2 {
		t.Fatalf("test setup has %d copies; want 2", got)
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Same-release failover regression",
			Preset: "4k",
			Rules:  loadProductionRules(t),
		},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	kept, rejected := profile.ApplyWithRejected(
		ranking.Request{
			Kind:  ranking.KindMovie,
			Title: "Example Movie",
		},
		[]triage.Candidate{
			{Release: merged},
		},
		jhinrank.RankOptions{},
	)

	if len(rejected) != 0 {
		t.Fatalf(
			"production profile unexpectedly rejected merged release: %#v",
			rejected,
		)
	}

	if len(kept) != 1 {
		t.Fatalf(
			"production profile kept %d releases; want 1",
			len(kept),
		)
	}

	rel := kept[0].Candidate.Release
	if rel == nil {
		t.Fatal("production profile returned nil release")
	}

	if rel.DetailsURL != primaryURL {
		t.Fatalf(
			"production ranking changed primary copy to %q; want %q",
			rel.DetailsURL,
			primaryURL,
		)
	}

	if got := rel.CopyCount(); got != 2 {
		t.Fatalf(
			"production ranking reduced same-release copies to %d; want 2",
			got,
		)
	}

	fallback := rel.CopyAt(1)
	if fallback == nil {
		t.Fatal("production ranking removed fallback copy")
	}

	if fallback.DetailsURL != fallbackURL {
		t.Fatalf(
			"fallback details URL = %q; want %q",
			fallback.DetailsURL,
			fallbackURL,
		)
	}

	if fallback.Indexer != "NZBGeek" {
		t.Fatalf(
			"fallback indexer = %q; want NZBGeek",
			fallback.Indexer,
		)
	}
}

// TestIntelligentUnknownResolutionProductionPolicy protects the adaptive
// fallback policy for releases whose resolution could not be parsed.
//
// Unknown metadata is not itself evidence that a Usenet result is bad.
// Production may reject a weak unknown-resolution/unknown-quality release only
// when the result set contains more than six well-identified alternatives.
// Library, SeaDex, known-quality and recognized release-group results remain
// protected, and a missing SeaDex lookup fails open.
func TestIntelligentUnknownResolutionProductionPolicy(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Intelligent unknown-resolution regression",
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

	unknownRule := findProductionRule(
		t,
		productionRules,
		"Unknown resolution",
	)

	if unknownRule.EffectiveAction() != config.RuleActionReject {
		t.Fatalf(
			"Unknown resolution action=%q, want reject",
			unknownRule.EffectiveAction(),
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
		t.Helper()

		entry, ok := generated.Defines[name]
		if !ok {
			t.Fatalf(
				"missing generated Define %q",
				name,
			)
		}

		if len(entry.Tokens) == 0 {
			t.Fatalf(
				"generated Define %q has no tokens",
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

	movieTierGroup := defineToken(
		"Movies WEB T1 Groups",
	)

	showTierGroup := defineToken(
		"Shows WEB T1 Groups",
	)

	animeTierGroup := defineToken(
		"Anime Shows WEB T1 Groups",
	)

	animeMovieTierGroup := defineToken(
		"Anime Movies WEB T1 Groups",
	)

	knownAlternatives := func(
		kind string,
		count int,
	) []string {
		t.Helper()

		titles := make(
			[]string,
			0,
			count,
		)

		for i := 0; i < count; i++ {
			switch kind {
			case ranking.KindMovie, ranking.KindAnimeMovie:
				titles = append(
					titles,
					fmt.Sprintf(
						"Example.Movie.%02d.2026.1080p.WEB-DL.x264-GRP",
						i+1,
					),
				)

			case ranking.KindSeries:
				titles = append(
					titles,
					fmt.Sprintf(
						"Example.Show.S01E%02d.1080p.WEB-DL.x264-GRP",
						i+1,
					),
				)

			case ranking.KindAnimeShow:
				titles = append(
					titles,
					fmt.Sprintf(
						"Example.Anime.S01E%02d.1080p.WEB-DL.x264-GRP",
						i+1,
					),
				)

			default:
				t.Fatalf(
					"unsupported regression kind %q",
					kind,
				)
			}
		}

		return titles
	}

	type policyCase struct {
		name               string
		target             string
		kind               string
		anime              bool
		alternatives       int
		library            bool
		seadex             *rules.SeadexContext
		wantRejected       bool
		wantSeaDexFailOpen bool
	}

	seadexCheckedNoMatch := func() *rules.SeadexContext {
		return &rules.SeadexContext{
			Known: false,
		}
	}

	cases := []policyCase{
		{
			name:         "dense weak Movie unknown is rejected",
			target:       "Example.Movie.2026-GRP",
			kind:         ranking.KindMovie,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
			wantRejected: true,
		},
		{
			name:         "six alternatives preserve scarce fallback",
			target:       "Example.Movie.2026-GRP",
			kind:         ranking.KindMovie,
			alternatives: 6,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name:         "known quality protects unknown resolution",
			target:       "Example.Movie.2026.WEB-DL.x264-GRP",
			kind:         ranking.KindMovie,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name: "Movie tier group protects weak metadata",
			target: fmt.Sprintf(
				"Example.Movie.2026-%s",
				movieTierGroup,
			),
			kind:         ranking.KindMovie,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name: "Show tier group protects weak metadata",
			target: fmt.Sprintf(
				"Example.Show.S01E01-%s",
				showTierGroup,
			),
			kind:         ranking.KindSeries,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name: "Anime tier group protects weak metadata",
			target: fmt.Sprintf(
				"Example.Anime.S01E01-%s",
				animeTierGroup,
			),
			kind:         ranking.KindAnimeShow,
			anime:        true,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name: "Anime Movie tier group protects weak metadata",
			target: fmt.Sprintf(
				"Example.Anime.Movie.2026-%s",
				animeMovieTierGroup,
			),
			kind:         ranking.KindAnimeMovie,
			anime:        true,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name:         "Library protects weak unknown",
			target:       "Example.Movie.2026-GRP",
			kind:         ranking.KindMovie,
			alternatives: 7,
			library:      true,
			seadex:       seadexCheckedNoMatch(),
		},
		{
			name:         "SeaDex Best protects weak unknown",
			target:       "Example.Anime.S01E01-BESTGRP",
			kind:         ranking.KindAnimeShow,
			anime:        true,
			alternatives: 7,
			seadex: &rules.SeadexContext{
				Known: true,
				Best: map[string]bool{
					"bestgrp": true,
				},
			},
		},
		{
			name:         "SeaDex Alternative protects weak unknown",
			target:       "Example.Anime.S01E01-ALTGRP",
			kind:         ranking.KindAnimeShow,
			anime:        true,
			alternatives: 7,
			seadex: &rules.SeadexContext{
				Known: true,
				Alt: map[string]bool{
					"altgrp": true,
				},
			},
		},
		{
			name:               "missing SeaDex lookup fails open",
			target:             "Example.Movie.2026-GRP",
			kind:               ranking.KindMovie,
			alternatives:       7,
			seadex:             nil,
			wantSeaDexFailOpen: true,
		},
		{
			name:         "known resolution unknown quality is untouched",
			target:       "Example.Movie.2026.1080p.x264-GRP",
			kind:         ranking.KindMovie,
			alternatives: 7,
			seadex:       seadexCheckedNoMatch(),
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			titles := []string{
				tc.target,
			}

			titles = append(
				titles,
				knownAlternatives(
					tc.kind,
					tc.alternatives,
				)...,
			)

			req := ranking.Request{
				Kind:    tc.kind,
				IsAnime: tc.anime,
				Season:  1,
				Episode: 1,
				Title:   "Example",
				Seadex:  tc.seadex,
				Sample: &ranking.Sample{
					IndexerData: true,
					Library:     tc.library,
				},
			}

			explanations, aggregates := profile.Explain(
				titles,
				req,
				jhinrank.RankOptions{},
			)

			var target *ranking.Explanation

			for _, explanation := range explanations {
				if explanation.Title == tc.target {
					target = explanation
					break
				}
			}

			if target == nil {
				t.Fatalf(
					"target release missing from explanations: %q",
					tc.target,
				)
			}

			hasUnknownRejection := false

			for _, rejection := range target.Rejections {
				if strings.Contains(
					rejection,
					"Unknown resolution",
				) {
					hasUnknownRejection = true
					break
				}
			}

			if hasUnknownRejection != tc.wantRejected {
				t.Fatalf(
					"Unknown resolution rejection=%v, want=%v\n"+
						"target=%q\n"+
						"fetch=%v\n"+
						"rejections=%v\n"+
						"skipped=%v",
					hasUnknownRejection,
					tc.wantRejected,
					tc.target,
					target.Fetch,
					target.Rejections,
					target.SkippedRules,
				)
			}

			if tc.wantRejected && target.Fetch {
				t.Fatalf(
					"target remained fetchable despite Unknown resolution rejection",
				)
			}

			if !tc.wantRejected && !target.Fetch {
				t.Fatalf(
					"protected target was rejected\n"+
						"target=%q\n"+
						"rejections=%v\n"+
						"skipped=%v",
					tc.target,
					target.Rejections,
					target.SkippedRules,
				)
			}

			hasSeaDexSkip := false

			for _, skipped := range target.SkippedRules {
				if !strings.Contains(
					skipped,
					"Unknown resolution",
				) {
					continue
				}

				if strings.Contains(
					skipped,
					"no SeaDex lookup",
				) ||
					strings.Contains(
						skipped,
						"needs a SeaDex lookup",
					) {
					hasSeaDexSkip = true
					break
				}
			}

			if tc.wantSeaDexFailOpen {
				if !hasSeaDexSkip {
					t.Fatalf(
						"missing SeaDex lookup did not expose expected fail-open skip\n"+
							"skipped=%v",
						target.SkippedRules,
					)
				}
			} else if hasSeaDexSkip {
				t.Fatalf(
					"SeaDex-aware case unexpectedly skipped Unknown resolution rule\n"+
						"skipped=%v",
					target.SkippedRules,
				)
			}

			var (
				foundAggregate bool
				aggregateCount int
			)

			for _, report := range aggregates {
				source := strings.NewReplacer(
					" ", "",
					"(", "",
					")", "",
				).Replace(report.Source)

				hasResolutionCheck :=
					strings.Contains(source, `resolution!=""`)

				hasQualityCheck :=
					strings.Contains(source, `quality!=""`)

				if hasResolutionCheck && hasQualityCheck {
					foundAggregate = true
					aggregateCount = report.Count
					break
				}
			}

			if !foundAggregate {
				t.Fatal(
					"production aggregate report for well-identified alternatives is missing",
				)
			}

			if aggregateCount != tc.alternatives {
				t.Fatalf(
					"well-identified aggregate count=%d, want=%d",
					aggregateCount,
					tc.alternatives,
				)
			}
		})
	}
}

func TestCandidateRelativePruneCompatibility(t *testing.T) {
	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Candidate-relative prune compatibility",
			Preset: "4k",
			Rules: []config.RuleConfig{
				{Name: "A", When: `group == "AAA"`, Points: 20000},
				{Name: "B", When: `group == "BBB"`, Points: 15000},
				{Name: "C", When: `group == "CCC"`, Points: 10000},
				{Name: "D", When: `group == "DDD"`, Points: 5000},
				{
					Name:   "Candidate-relative weak tail",
					When:   `count(finalScore >= current.finalScore + 5000) >= 3`,
					Action: config.RuleActionPrune,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("compile candidate-relative prune profile: %v", err)
	}

	titles := []string{
		"Movie.2020.1080p.WEB-DL.H264-AAA",
		"Movie.2020.1080p.WEB-DL.H264-BBB",
		"Movie.2020.1080p.WEB-DL.H264-CCC",
		"Movie.2020.1080p.WEB-DL.H264-DDD",
	}

	candidates := make([]triage.Candidate, len(titles))
	for i, title := range titles {
		candidates[i] = triage.Candidate{
			Release: &release.Release{Title: title},
		}
	}

	kept, rejected := profile.ApplyWithRejected(
		ranking.Request{
			Kind:  ranking.KindMovie,
			Title: "Movie",
		},
		candidates,
		jhinrank.RankOptions{},
	)

	if len(kept) != 3 || len(rejected) != 1 {
		t.Fatalf(
			"dense set: kept=%d rejected=%d, want kept=3 rejected=1",
			len(kept),
			len(rejected),
		)
	}

	if rejected[0].Candidate.Release == nil {
		t.Fatal("rejected candidate has nil release")
	}

	if got := rejected[0].Candidate.Release.Title; got != titles[3] {
		t.Fatalf(
			"rejected release = %q, want %q",
			got,
			titles[3],
		)
	}

	sparse := []triage.Candidate{
		{Release: &release.Release{Title: titles[0]}},
		{Release: &release.Release{Title: titles[3]}},
	}

	kept, rejected = profile.ApplyWithRejected(
		ranking.Request{
			Kind:  ranking.KindMovie,
			Title: "Movie",
		},
		sparse,
		jhinrank.RankOptions{},
	)

	if len(kept) != 2 || len(rejected) != 0 {
		t.Fatalf(
			"sparse fallback: kept=%d rejected=%d, want kept=2 rejected=0",
			len(kept),
			len(rejected),
		)
	}
}

// TestAdaptiveLowScoreProductionPolicy protects DraCuLa's
// candidate-relative low-score filtering against the released
// StreamNZB/Jhin engine.
//
// Known Movie/Show LQ or Bad-Dual releases are pruned only when at
// least six alternatives have final scores >= 5000 points higher.
// Sparse result pools therefore retain weak releases as fallbacks.
func TestAdaptiveLowScoreProductionPolicy(t *testing.T) {
	const ruleName = "Adaptive Low-Score Filtering"

	rules := loadProductionRules(t)

	var adaptive *config.RuleConfig

	for i := range rules {
		if rules[i].Name == ruleName {
			adaptive = &rules[i]
			break
		}
	}

	if adaptive == nil {
		t.Fatalf("production rule %q is missing", ruleName)
	}

	if adaptive.Action != config.RuleActionPrune {
		t.Fatalf(
			"%s action=%q; want %q",
			ruleName,
			adaptive.Action,
			config.RuleActionPrune,
		)
	}

	const expectedWhen = `not library
and (
  matched("Movies LQ Groups")
  or matched("Movies Bad Dual Groups")
  or matched("Shows LQ Groups")
  or matched("Shows Bad Dual Groups")
)
and count(finalScore >= current.finalScore + 5000) >= 6`

	if adaptive.When != expectedWhen {
		t.Fatalf(
			"%s predicate mismatch:\ngot:\n%s\n\nwant:\n%s",
			ruleName,
			adaptive.When,
			expectedWhen,
		)
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Adaptive Low-Score production regression",
			Preset: "4k",
			Rules:  rules,
		},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf(
			"compile production Adaptive Low-Score profile: %v",
			err,
		)
	}

	makeCandidate := func(title string) triage.Candidate {
		return triage.Candidate{
			Release: &release.Release{
				Title: title,
			},
		}
	}

	t.Run("dense Movie LQ tail is pruned", func(t *testing.T) {
		candidates := []triage.Candidate{
			makeCandidate(
				"Example.Movie.2026.2160p.WEB-DL.H265-FLUX",
			),
			makeCandidate(
				"Example.Movie.2026.2160p.WEB-DL.H265-NTb",
			),
			makeCandidate(
				"Example.Movie.2026.1080p.BluRay.REMUX.AVC-HiFi",
			),
			makeCandidate(
				"Example.Movie.2026.1080p.WEB-DL.H264-FLUX",
			),
			makeCandidate(
				"Example.Movie.2026.1080p.WEB-DL.H264-NTb",
			),
			makeCandidate(
				"Example.Movie.2026.720p.WEB-DL.H264-FLUX",
			),
			makeCandidate(
				"Example.Movie.2026.720p.WEB-DL.H264-YIFY",
			),
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{
				Kind:  ranking.KindMovie,
				Title: "Example Movie",
			},
			candidates,
			jhinrank.RankOptions{},
		)

		if len(kept) != 6 || len(rejected) != 1 {
			t.Fatalf(
				"dense Movie LQ: kept=%d rejected=%d; "+
					"want kept=6 rejected=1",
				len(kept),
				len(rejected),
			)
		}

		if rejected[0].Candidate.Release == nil {
			t.Fatal(
				"dense Movie LQ rejected candidate has nil release",
			)
		}

		got := rejected[0].Candidate.Release.Title
		want := candidates[len(candidates)-1].Release.Title

		if got != want {
			t.Fatalf(
				"dense Movie LQ rejected=%q; want %q",
				got,
				want,
			)
		}
	})

	t.Run("sparse Movie LQ fallback survives", func(t *testing.T) {
		candidates := []triage.Candidate{
			makeCandidate(
				"Example.Movie.2026.2160p.WEB-DL.H265-FLUX",
			),
			makeCandidate(
				"Example.Movie.2026.720p.WEB-DL.H264-YIFY",
			),
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{
				Kind:  ranking.KindMovie,
				Title: "Example Movie",
			},
			candidates,
			jhinrank.RankOptions{},
		)

		if len(kept) != 2 || len(rejected) != 0 {
			t.Fatalf(
				"sparse Movie LQ: kept=%d rejected=%d; "+
					"want kept=2 rejected=0",
				len(kept),
				len(rejected),
			)
		}
	})
}

// TestHDR10PlusTierCeilings protects the bounded +25 non-Anime
// HDR10+ preference against DraCuLa's ordinary Movie and Series
// release-group tier authority.
//
// The tested maximum lower-tier stacks are:
//
//	Movie: HDR10+ +25, DUBBED +10, REPACK3 +7,
//	       Open Matte +25, Director's Cut +25,
//	       availability +30 = +122.
//
//	Series: HDR10+ +25, DUBBED +10, REPACK3 +7,
//	        availability +30 = +72.
//
// Against the existing 200-point adjacent tier gaps, these must
// preserve +78 Movie and +128 Series headroom.
func TestHDR10PlusTierCeilings(t *testing.T) {
	data, err := os.ReadFile(
		"../../generated/vidhin-defines.json",
	)
	if err != nil {
		t.Fatalf("read generated Vidhin data: %v", err)
	}

	var generated struct {
		Defines map[string]struct {
			Tokens []string `json:"tokens"`
		} `json:"defines"`
	}

	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("decode generated Vidhin data: %v", err)
	}

	defineToken := func(name string) string {
		t.Helper()

		entry, ok := generated.Defines[name]
		if !ok {
			t.Fatalf("missing generated Define %q", name)
		}

		if len(entry.Tokens) == 0 {
			t.Fatalf("generated Define %q has no tokens", name)
		}

		tokens := append([]string(nil), entry.Tokens...)
		slices.Sort(tokens)

		return tokens[0]
	}

	movieT1 := defineToken("Movies WEB T1 Groups")
	movieT2 := defineToken("Movies WEB T2 Groups")
	showT1 := defineToken("Shows WEB T1 Groups")
	showT2 := defineToken("Shows WEB T2 Groups")

	fullAvailability := triage.AvailState{
		Status:       triage.AvailAvailable,
		OnMyBackbone: true,
		CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
	}

	type profileCase struct {
		name  string
		rules []config.RuleConfig
	}

	profiles := []profileCase{
		{
			name:  "Samsung",
			rules: loadProductionRules(t),
		},
		{
			name:  "Neutral",
			rules: loadNeutralRules(t),
		},
	}

	for _, pc := range profiles {
		pc := pc

		t.Run(pc.name, func(t *testing.T) {
			profile, err := ranking.Compile(
				config.FilterProfileConfig{
					Name:   "HDR10+ tier-ceiling regression",
					Preset: "4k",
					Rules:  pc.rules,
				},
				loadDefineLibrary(t)...,
			)
			if err != nil {
				t.Fatalf(
					"compile %s profile: %v",
					pc.name,
					err,
				)
			}

			score := func(
				name string,
				title string,
				kind string,
				avail triage.AvailState,
			) int {
				t.Helper()

				request := ranking.Request{
					Kind:  kind,
					Title: "Example",
				}

				if kind == ranking.KindSeries {
					request.Season = 1
					request.Episode = 1
				}

				candidate := triage.Candidate{
					Release: &release.Release{
						Title: title,
					},
				}

				candidate.Verdict.Avail = avail

				kept, rejected := profile.ApplyWithRejected(
					request,
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
						"%s kept=%d; want 1",
						name,
						len(kept),
					)
				}

				return kept[0].Torrent.Rank
			}

			movieHigher := score(
				"Movie WEB T1 clean HDR10",
				fmt.Sprintf(
					"Example.Movie.2026.1080p.WEB-DL."+
						"x264.HDR10-%s",
					movieT1,
				),
				ranking.KindMovie,
				triage.AvailState{},
			)

			movieLower := score(
				"Movie WEB T2 fully decorated HDR10+",
				fmt.Sprintf(
					"Example.Movie.2026.1080p.WEB-DL.x264."+
						"HDR10Plus.DUBBED.REPACK3."+
						"Open.Matte.Directors.Cut-%s",
					movieT2,
				),
				ranking.KindMovie,
				fullAvailability,
			)

			if headroom := movieHigher - movieLower; headroom != 78 {
				t.Fatalf(
					"Movie T1/T2 HDR10+ headroom=%d; want 78",
					headroom,
				)
			}

			showHigher := score(
				"Show WEB T1 clean HDR10",
				fmt.Sprintf(
					"Example.Show.S01E01.1080p.WEB-DL."+
						"x264.HDR10-%s",
					showT1,
				),
				ranking.KindSeries,
				triage.AvailState{},
			)

			showLower := score(
				"Show WEB T2 fully decorated HDR10+",
				fmt.Sprintf(
					"Example.Show.S01E01.1080p.WEB-DL.x264."+
						"HDR10Plus.DUBBED.REPACK3-%s",
					showT2,
				),
				ranking.KindSeries,
				fullAvailability,
			)

			if headroom := showHigher - showLower; headroom != 128 {
				t.Fatalf(
					"Show T1/T2 HDR10+ headroom=%d; want 128",
					headroom,
				)
			}
		})
	}
}

func TestDynamicRangeAndBitDepthPolicy(t *testing.T) {
	type profileCase struct {
		name    string
		rules   []config.RuleConfig
		samsung bool
	}

	profiles := []profileCase{
		{
			name:    "Samsung",
			rules:   loadProductionRules(t),
			samsung: true,
		},
		{
			name:  "Neutral",
			rules: loadNeutralRules(t),
		},
	}

	type scoreResult struct {
		rank     int
		rejected bool
	}

	for _, pc := range profiles {
		pc := pc

		t.Run(pc.name, func(t *testing.T) {
			profile, err := ranking.Compile(
				config.FilterProfileConfig{
					Name:   "dynamic-range and bit-depth regression",
					Preset: "4k",
					Rules:  pc.rules,
				},
				loadDefineLibrary(t)...,
			)
			if err != nil {
				t.Fatalf(
					"compile %s profile: %v",
					pc.name,
					err,
				)
			}

			score := func(
				name string,
				title string,
				kind string,
				anime bool,
			) scoreResult {
				t.Helper()

				request := ranking.Request{
					Kind:    kind,
					IsAnime: anime,
					Title:   "Example",
				}

				if kind == ranking.KindAnimeShow {
					request.Season = 1
					request.Episode = 1
				}

				candidate := triage.Candidate{
					Release: &release.Release{
						Title: title,
					},
				}

				kept, rejected := profile.ApplyWithRejected(
					request,
					[]triage.Candidate{candidate},
					jhinrank.RankOptions{},
				)

				if len(rejected) != 0 {
					if len(kept) != 0 {
						t.Fatalf(
							"%s: candidate both kept and rejected",
							name,
						)
					}

					return scoreResult{
						rejected: true,
					}
				}

				if len(kept) != 1 {
					t.Fatalf(
						"%s: kept=%d rejected=%d; want exactly one kept candidate",
						name,
						len(kept),
						len(rejected),
					)
				}

				return scoreResult{
					rank: kept[0].Torrent.Rank,
				}
			}

			assertSameRank := func(
				name string,
				base scoreResult,
				got scoreResult,
			) {
				t.Helper()

				if base.rejected {
					t.Fatalf(
						"%s: baseline unexpectedly rejected",
						name,
					)
				}

				if got.rejected {
					t.Fatalf(
						"%s unexpectedly rejected",
						name,
					)
				}

				if got.rank != base.rank {
					t.Fatalf(
						"%s rank=%d, baseline=%d, delta=%+d; want equal effective rank",
						name,
						got.rank,
						base.rank,
						got.rank-base.rank,
					)
				}
			}

			sdr := score(
				"SDR",
				"Example.Movie.2026.1080p.WEB-DL.x264.SDR-GRP",
				ranking.KindMovie,
				false,
			)

			explicit10 := score(
				"explicit 10bit",
				"Example.Movie.2026.1080p.WEB-DL.x264.10bit-GRP",
				ranking.KindMovie,
				false,
			)

			hi10p := score(
				"Hi10P",
				"Example.Movie.2026.1080p.WEB-DL.Hi10P.x264-GRP",
				ranking.KindMovie,
				false,
			)

			hdr := score(
				"HDR",
				"Example.Movie.2026.1080p.WEB-DL.x264.HDR-GRP",
				ranking.KindMovie,
				false,
			)

			hdr10 := score(
				"HDR10",
				"Example.Movie.2026.1080p.WEB-DL.x264.HDR10-GRP",
				ranking.KindMovie,
				false,
			)

			hdr10Plus := score(
				"HDR10 Plus",
				"Example.Movie.2026.1080p.WEB-DL.x264.HDR10Plus-GRP",
				ranking.KindMovie,
				false,
			)

			dv := score(
				"Dolby Vision only",
				"Example.Movie.2026.1080p.WEB-DL.x264.DV-GRP",
				ranking.KindMovie,
				false,
			)

			dvHDR := score(
				"Dolby Vision + HDR",
				"Example.Movie.2026.1080p.WEB-DL.x264.DV.HDR-GRP",
				ranking.KindMovie,
				false,
			)

			dvHDR10 := score(
				"Dolby Vision + HDR10",
				"Example.Movie.2026.1080p.WEB-DL.x264.DV.HDR10-GRP",
				ranking.KindMovie,
				false,
			)

			dvHDR10Plus := score(
				"Dolby Vision + HDR10 Plus",
				"Example.Movie.2026.1080p.WEB-DL.x264.DV.HDR10Plus-GRP",
				ranking.KindMovie,
				false,
			)

			// Portable Core intentionally compensates Jhin v0.6's
			// native ranking authority for display-dependent dynamic
			// range and parsed 10-bit metadata:
			//
			//   Dolby Vision +3000
			//   HDR10+       +2100
			//   HDR          +2000
			//   parsed 10bit  +100
			//
			// After compensation, non-Anime HDR10+ receives DraCuLa's
			// explicit bounded +25 preference. Anime HDR10+ remains
			// score-neutral so the 80-point Anime tier floor continues
			// to dominate the proven +77 ordinary metadata ceiling.
			assertSameRank("explicit 10bit", sdr, explicit10)
			assertSameRank("Hi10P", sdr, hi10p)
			assertSameRank("HDR", sdr, hdr)
			assertSameRank("HDR10", sdr, hdr10)

			if hdr10Plus.rejected {
				t.Fatal("HDR10 Plus unexpectedly rejected")
			}

			if gap := hdr10Plus.rank - sdr.rank; gap != 25 {
				t.Fatalf(
					"HDR10 Plus delta=%+d; want +25",
					gap,
				)
			}

			assertSameRank("Dolby Vision + HDR", sdr, dvHDR)
			assertSameRank("Dolby Vision + HDR10", sdr, dvHDR10)

			if dvHDR10Plus.rejected {
				t.Fatal("Dolby Vision + HDR10 Plus unexpectedly rejected")
			}

			if gap := dvHDR10Plus.rank - sdr.rank; gap != 25 {
				t.Fatalf(
					"Dolby Vision + HDR10 Plus delta=%+d; want +25",
					gap,
				)
			}

			if pc.samsung {
				if !dv.rejected {
					t.Fatal(
						"Samsung profile kept Dolby Vision without HDR fallback",
					)
				}
			} else {
				assertSameRank(
					"Neutral Dolby Vision only",
					sdr,
					dv,
				)
			}

			// The original scoring-ceiling audit used a minimum Anime
			// adjacent-tier gap of 80. Native Jhin 10-bit ranking used
			// to erase T1>T2 authority and invert T5>T6. The Core
			// compensation must preserve the intended tier ladder.
			animeT1 := score(
				"Anime WEB T1 8-bit",
				"Example.Anime.S01E01.1080p.WEB-DL.x264-Arg0",
				ranking.KindAnimeShow,
				true,
			)

			animeT2Ten := score(
				"Anime WEB T2 10-bit",
				"Example.Anime.S01E01.1080p.WEB-DL.x264.10bit-Asakura",
				ranking.KindAnimeShow,
				true,
			)

			animeT5 := score(
				"Anime WEB T5 8-bit",
				"Example.Anime.S01E01.1080p.WEB-DL.x264-BlueLobster",
				ranking.KindAnimeShow,
				true,
			)

			animeT6Ten := score(
				"Anime WEB T6 10-bit",
				"Example.Anime.S01E01.1080p.WEB-DL.x264.10bit-9volt",
				ranking.KindAnimeShow,
				true,
			)

			for name, result := range map[string]scoreResult{
				"Anime WEB T1": animeT1,
				"Anime WEB T2": animeT2Ten,
				"Anime WEB T5": animeT5,
				"Anime WEB T6": animeT6Ten,
			} {
				if result.rejected {
					t.Fatalf(
						"%s unexpectedly rejected",
						name,
					)
				}
			}

			if gap := animeT1.rank - animeT2Ten.rank; gap != 100 {
				t.Fatalf(
					"Anime WEB T1 8-bit - T2 10-bit gap=%d; want 100",
					gap,
				)
			}

			if gap := animeT5.rank - animeT6Ten.rank; gap != 80 {
				t.Fatalf(
					"Anime WEB T5 8-bit - T6 10-bit gap=%d; want 80",
					gap,
				)
			}

			// Dynamic-range metadata must also remain subordinate to
			// the ordinary Movie WEB release-group ladder.
			movieT1 := score(
				"Movie WEB T1 SDR",
				"Example.Movie.2026.1080p.WEB-DL.x264.SDR-FLUX",
				ranking.KindMovie,
				false,
			)

			movieT3HDR10 := score(
				"Movie WEB T3 HDR10",
				"Example.Movie.2026.1080p.WEB-DL.x264.HDR10-BLOOM",
				ranking.KindMovie,
				false,
			)

			if movieT1.rejected || movieT3HDR10.rejected {
				t.Fatal(
					"Movie WEB tier-authority candidate unexpectedly rejected",
				)
			}

			if gap := movieT1.rank - movieT3HDR10.rank; gap != 400 {
				t.Fatalf(
					"Movie WEB T1 SDR - T3 HDR10 gap=%d; want 400",
					gap,
				)
			}

			animeHDR10 := score(
				"Anime HDR10",
				"Example.Anime.S01E01.1080p.WEB-DL.x264.HDR10-GRP",
				ranking.KindAnimeShow,
				true,
			)

			animeHDR10Plus := score(
				"Anime HDR10 Plus",
				"Example.Anime.S01E01.1080p.WEB-DL.x264.HDR10Plus-GRP",
				ranking.KindAnimeShow,
				true,
			)

			assertSameRank(
				"Anime HDR10 Plus",
				animeHDR10,
				animeHDR10Plus,
			)
		})
	}
}
