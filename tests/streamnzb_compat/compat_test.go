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
	expectedProfileSchema = 2
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
	Name             string                           `json:"name"`
	Preset           string                           `json:"preset"`
	StreamNZBProfile int                              `json:"streamnzb_profile"`
	Scoring          map[string]*config.ScoringConfig `json:"scoring"`
	Rules            []config.RuleConfig              `json:"rules"`
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

func loadProfilePayload(
	t *testing.T,
	path string,
	label string,
) profilePayload {
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

	return profile
}

func loadProfileRules(
	t *testing.T,
	path string,
	label string,
) []config.RuleConfig {
	t.Helper()

	return loadProfilePayload(t, path, label).Rules
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

	if len(neutralRules) != 144 {
		t.Fatalf(
			"neutral profile contains %d rules; want 144",
			len(neutralRules),
		)
	}

	deviceRules := map[string]bool{
		"DV without HDR fallback": false,
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

func TestSAOSeasonOneExclusionCompatibility(t *testing.T) {
	const ruleName = "Reject SAO II from SAO Season 1"

	productionRules := loadProductionRules(t)
	cfg := findProductionRule(t, productionRules, ruleName)

	if cfg.EffectiveAction() != config.RuleActionReject {
		t.Fatalf(
			"production rule %q action = %q, want reject",
			ruleName,
			cfg.EffectiveAction(),
		)
	}

	neutralRules := loadNeutralRules(t)
	neutralMatches := 0
	for _, rule := range neutralRules {
		if rule.Name == ruleName {
			neutralMatches++
		}
	}
	if neutralMatches != 1 {
		t.Fatalf(
			"neutral profile contains %d copies of %q; want 1",
			neutralMatches,
			ruleName,
		)
	}

	set, err := rules.Compile([]config.RuleConfig{cfg})
	if err != nil {
		t.Fatalf("compile production rule %q: %v", ruleName, err)
	}

	tests := []struct {
		name         string
		requestTitle string
		season       int
		isAnime      bool
		kind         string
		release      string
		wantReject   bool
	}{
		{
			name:         "reject SAO II reset numbering from base SAO season 1",
			requestTitle: "Sword Art Online",
			season:       1,
			isAnime:      true,
			kind:         "anime_show",
			release:      "Sword.Art.Online.II.S01E01.1080p.WEB-DL-GROUP",
			wantReject:   true,
		},
		{
			name:         "keep genuine base SAO season 1",
			requestTitle: "Sword Art Online",
			season:       1,
			isAnime:      true,
			kind:         "anime_show",
			release:      "Sword.Art.Online.S01E01.1080p.WEB-DL-GROUP",
			wantReject:   false,
		},
		{
			name:         "do not affect SAO II request",
			requestTitle: "Sword Art Online II",
			season:       1,
			isAnime:      true,
			kind:         "anime_show",
			release:      "Sword.Art.Online.II.S01E01.1080p.WEB-DL-GROUP",
			wantReject:   false,
		},
		{
			name:         "do not affect base SAO season 2",
			requestTitle: "Sword Art Online",
			season:       2,
			isAnime:      true,
			kind:         "anime_show",
			release:      "Sword.Art.Online.II.S01E01.1080p.WEB-DL-GROUP",
			wantReject:   false,
		},
		{
			name:         "do not affect non anime request",
			requestTitle: "Sword Art Online",
			season:       1,
			isAnime:      false,
			kind:         "series",
			release:      "Sword.Art.Online.II.S01E01.1080p.WEB-DL-GROUP",
			wantReject:   false,
		},
		{
			name:         "do not catch unrelated II token later in filename",
			requestTitle: "Sword Art Online",
			season:       1,
			isAnime:      true,
			kind:         "anime_show",
			release:      "Sword.Art.Online.S01E01.II.1080p.WEB-DL-GROUP",
			wantReject:   false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			cand := triage.Candidate{
				Release: &release.Release{
					Title: tt.release,
				},
			}

			env := rules.BuildEnv(
				cand,
				jhin.Parse(tt.release),
				rules.Context{
					Kind:    tt.kind,
					IsAnime: tt.isAnime,
					Season:  tt.season,
					Episode: 1,
					Title:   tt.requestTitle,
				},
			)

			out := set.Evaluate(env, tt.kind)
			gotReject := ruleRejected(out, ruleName)

			if gotReject != tt.wantReject {
				t.Fatalf(
					"production SAO exclusion rejected = %v, want %v\n"+
						"request title: %s\n"+
						"season: %d\n"+
						"release: %s\n"+
						"condition: %s\n"+
						"rejections: %+v",
					gotReject,
					tt.wantReject,
					tt.requestTitle,
					tt.season,
					tt.release,
					cfg.When,
					out.Rejections,
				)
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

	for _, schema := range []int{0, 1, 3} {
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

// TestAnimeTierEffectiveCeilings is a narrow arithmetic regression: it
// verifies the intended Anime WEB/BluRay tier-score ladder and a known,
// deliberately partial set of ordinary Anime metadata bonuses (Dual/Multi
// Audio, Uncensored, revision, corrected-release, Complete Season Pack,
// availability, WEB service) against the 80-point minimum adjacent tier
// gap. It intentionally predates Anime audio-codec scoring and does not
// exercise it.
//
// This test does NOT prove that every currently-reachable lower-tier
// combination stays below the next tier up. That guarantee belongs to
// TestAdjacentTierCeilingMatrix (adjacent_tier_ceiling_test.go), which
// decorates each tier family with every reachable ordinary bonus,
// including audio codecs, and compares the engine's own output directly
// instead of a hardcoded stack constant. This exact "hardcoded max stack
// vs. tier gap" pattern is what let the high-impact audio-normalization
// change reach production without anyone re-checking it against Anime's
// much tighter gap (see the Scoring-ceiling audit completed item in the
// project backlog) — kept here only as a secondary sanity check on the
// ladder spacing itself, not as ceiling proof.
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

	// Effective portable Anime Show maxima for the deliberately partial
	// metadata combination this test covers (audio codecs excluded — see
	// the function doc comment above and TestAdjacentTierCeilingMatrix
	// for the combination that includes them):
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
	neutralizeEdition := findProductionRule(
		t,
		productionRules,
		"Neutralize Edition",
	)

	const nativeEditionPoints = 100

	if imax.Points != 700 || imax.Scope != "movie" {
		t.Fatalf(
			"IMAX stored policy drifted: points=%d scope=%q",
			imax.Points,
			imax.Scope,
		)
	}

	if neutralizeEdition.Points != -100 || neutralizeEdition.Scope != "" {
		t.Fatalf(
			"Neutralize Edition policy drifted: points=%d scope=%q",
			neutralizeEdition.Points,
			neutralizeEdition.Scope,
		)
	}

	// IMAX's effective score is now +700, not the pre-fix +800: the
	// universal Neutralize Edition rule cancels Jhin's native +100
	// (IMAX is one of Jhin's 10 canonical Edition values), leaving the
	// stored +700 rule as the entire effective contribution. The Edition
	// Preference Layer audit's mandatory IMAX safety gate directly
	// measured this via the full production pipeline
	// (TestAdjacentTierCeilingMatrix, TestZZZIMAXSafetyGate) and found no
	// tier-authority violation, so +700 is preserved unchanged rather
	// than restored to net +800.
	imaxEffective := imax.Points + nativeEditionPoints + neutralizeEdition.Points
	if imaxEffective != 700 {
		t.Fatalf(
			"IMAX effective policy drifted: "+
				"stored=%d native=%d neutralizer=%d effective=%d",
			imax.Points,
			nativeEditionPoints,
			neutralizeEdition.Points,
			imaxEffective,
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
			wantScore: 200,
		},
		{
			name: "Movie T2 IMAX",
			release: makeRelease(
				"Example.Movie.2026.1080p.WEB-DL.x264",
				movieT2,
				"IMAX",
			),
			kind:      "movie",
			wantScore: 600,
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
			wantScore: 400,
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
			wantScore: 25,
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
			wantScore: -175,
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
			wantScore: 425,
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
			wantScore: -300,
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
			wantScore: -400,
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
			wantScore: -380,
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

// TestLanguageSubtitleParserRegression pins the Jhin v0.6.2 language/subtitle
// surface consumed by the DraCuLa formatter. Languages and subtitle presence
// are intentionally separate: Jhin exports parsed language metadata through
// Languages and only a boolean Subbed flag, not subtitle-language identities.
//
// The compact JA alias case previously recorded an upstream limitation
// tracked in dreulavelle/jhin#39: JA.EN parsed as English only. That was
// fixed in Jhin 0.6.2 (compatibility-audited alongside StreamNZB v5.18.0);
// the case now asserts both languages parse correctly.
func TestLanguageSubtitleParserRegression(t *testing.T) {
	cases := []struct {
		name          string
		release       string
		wantLanguages []string
		wantSubbed    bool
		wantDubbed    bool
		wantHardcoded bool
	}{
		{
			name:          "languages only",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.Japanese.English.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en", "ja"},
		},
		{
			name:          "compact JA alias",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.JA.EN.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en", "ja"},
		},
		{
			name:          "subbed",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.Japanese.English.SUBBED.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en", "ja"},
			wantSubbed:    true,
		},
		{
			name:          "dubbed and subbed",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.Japanese.English.DUBBED.SUBBED.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en", "ja"},
			wantSubbed:    true,
			wantDubbed:    true,
		},
		{
			name:          "hardcoded subtitles",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.Japanese.English.HARDCODED.SUBS.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en", "ja"},
			wantSubbed:    true,
			wantHardcoded: true,
		},
		// Fused language+subtitle tokens (jhin, not PTT): ENGSUB, ESub,
		// VOSTFR, SWESUB, KORSUB, PLSUB, SUBFRENCH. Jhin 0.6.2 added a
		// dedicated compound "subbed" handler for these because no word
		// boundary precedes "sub" in any of them, so the generic subbed
		// handlers never matched and subtitle evidence was previously lost
		// (Languages was already correct on 0.6.1; only Subbed changed).
		// Languages and Subbed are asserted as independent public facts,
		// never as a bound "French subtitles"-style pair.
		{
			name:          "fused ENGSUB",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.ENGSUB.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en"},
			wantSubbed:    true,
		},
		{
			name:          "fused ESub",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.ESub.DDP5.1.H.264-GRP",
			wantLanguages: []string{"en"},
			wantSubbed:    true,
		},
		{
			name:          "fused VOSTFR",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.VOSTFR.DDP5.1.H.264-GRP",
			wantLanguages: []string{"fr"},
			wantSubbed:    true,
		},
		{
			name:          "fused SWESUB",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.SWESUB.DDP5.1.H.264-GRP",
			wantLanguages: []string{"sv"},
			wantSubbed:    true,
		},
		{
			name:          "fused KORSUB",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.KORSUB.DDP5.1.H.264-GRP",
			wantLanguages: []string{"ko"},
			wantSubbed:    true,
		},
		{
			name:          "fused PLSUB",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.PLSUB.DDP5.1.H.264-GRP",
			wantLanguages: []string{"pl"},
			wantSubbed:    true,
		},
		{
			name:          "fused SUBFRENCH",
			release:       "Anime.Show.S01E01.1080p.WEB-DL.SUBFRENCH.DDP5.1.H.264-GRP",
			wantLanguages: []string{"fr"},
			wantSubbed:    true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			got := jhin.Parse(tc.release)

			if !slices.Equal(got.Languages, tc.wantLanguages) {
				t.Errorf(
					"Languages = %v, want %v",
					got.Languages,
					tc.wantLanguages,
				)
			}

			if got.Subbed != tc.wantSubbed {
				t.Errorf(
					"Subbed = %v, want %v",
					got.Subbed,
					tc.wantSubbed,
				)
			}

			if got.Dubbed != tc.wantDubbed {
				t.Errorf(
					"Dubbed = %v, want %v",
					got.Dubbed,
					tc.wantDubbed,
				)
			}

			if got.Hardcoded != tc.wantHardcoded {
				t.Errorf(
					"Hardcoded = %v, want %v",
					got.Hardcoded,
					tc.wantHardcoded,
				)
			}
		})
	}
}

func TestProductionProfileBoundsPresetSizeScoring(t *testing.T) {
	profilePayload := loadProfilePayload(
		t,
		"../../profile.txt",
		"production",
	)

	if profilePayload.Preset != "4k" {
		t.Fatalf(
			"production preset = %q, want 4k",
			profilePayload.Preset,
		)
	}

	expected := map[string]config.ScoringConfig{
		ranking.KindMovie: {
			SizeTargetGB: 20,
			SizeWeight:   500,
		},
		ranking.KindAnimeMovie: {
			SizeTargetGB: 20,
			SizeWeight:   500,
		},
		ranking.KindSeries: {
			SizeTargetGB: 6,
			SizeWeight:   500,
		},
		ranking.KindAnimeShow: {
			SizeTargetGB: 6,
			SizeWeight:   500,
		},
	}

	if len(profilePayload.Scoring) != len(expected) {
		t.Fatalf(
			"production scoring contains %d kinds, want %d: %+v",
			len(profilePayload.Scoring),
			len(expected),
			profilePayload.Scoring,
		)
	}

	for kind, want := range expected {
		got := config.ResolveScoring(
			profilePayload.Scoring,
			kind,
		)

		if got != want {
			t.Fatalf(
				"%s scoring = %+v, want %+v",
				kind,
				got,
				want,
			)
		}
	}

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:    "Production bounded size scoring",
			Preset:  profilePayload.Preset,
			Scoring: profilePayload.Scoring,
			Rules:   profilePayload.Rules,
		},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf(
			"compile production profile: %v",
			err,
		)
	}

	score := func(
		name string,
		title string,
		size int64,
		library bool,
	) int {
		t.Helper()

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{
				Kind:  ranking.KindMovie,
				Title: "Project Hail Mary",
			},
			[]triage.Candidate{{
				Release: &release.Release{
					Title:     title,
					Size:      size,
					IsLibrary: library,
				},
			}},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf(
				"%s: kept=%d rejected=%+v",
				name,
				len(kept),
				rejected,
			)
		}

		return kept[0].Torrent.Rank
	}

	web := score(
		"BYNDR WEB-DL",
		"Project.Hail.Mary.2026.IMAX.2160p.AMZN.WEB-DL.DDP5.1.Atmos.H.265-BYNDR",
		19549082295,
		true,
	)

	remux := score(
		"CiNEPHiLES REMUX",
		"Project.Hail.Mary.2026.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES",
		88029307806,
		false,
	)

	if web != 62439 {
		t.Fatalf(
			"production-equivalent BYNDR score = %d, want 62439",
			web,
		)
	}

	if remux != 62075 {
		t.Fatalf(
			"production-equivalent CiNEPHiLES score = %d, want 62075",
			remux,
		)
	}

	if got := web - remux; got != 364 {
		t.Fatalf(
			"bounded WEB-over-REMUX interaction = %+d, want +364",
			got,
		)
	}
}

func TestMovieAudioNormalizationHierarchy(t *testing.T) {
	data, err := os.ReadFile("../../generated/vidhin-defines.json")
	if err != nil {
		t.Fatalf("read generated Vidhin baseline: %v", err)
	}

	var generated struct {
		Defines map[string]struct {
			Tokens []string `json:"tokens"`
		} `json:"defines"`
	}
	if err := json.Unmarshal(data, &generated); err != nil {
		t.Fatalf("decode generated Vidhin baseline: %v", err)
	}

	defineToken := func(name string) string {
		t.Helper()

		entry, ok := generated.Defines[name]
		if !ok || len(entry.Tokens) == 0 {
			t.Fatalf("missing/empty Define %q", name)
		}

		tokens := append([]string(nil), entry.Tokens...)
		slices.Sort(tokens)
		return tokens[0]
	}

	remuxT1 := defineToken("Movies Remux T1 Groups")
	webT1 := defineToken("Movies WEB T1 Groups")
	blurayT1 := defineToken("Movies UHD BluRay T1 Groups")

	cases := []struct {
		name  string
		title string
	}{
		{
			"REMUX clean",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.REMUX.HEVC-%s",
				remuxT1,
			),
		},
		{
			"REMUX TrueHD Atmos",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.REMUX.HEVC.TrueHD.Atmos.7.1-%s",
				remuxT1,
			),
		},
		{
			"REMUX TrueHD only",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.REMUX.HEVC.TrueHD.7.1-%s",
				remuxT1,
			),
		},
		{
			"REMUX Atmos only",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.REMUX.HEVC.Atmos-%s",
				remuxT1,
			),
		},
		{
			"WEB clean",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.WEB-DL.HEVC-%s",
				webT1,
			),
		},
		{
			"WEB DDPlus",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.WEB-DL.HEVC.DDP5.1-%s",
				webT1,
			),
		},
		{
			"WEB DDPlus Atmos IMAX",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.WEB-DL.HEVC.DDP5.1.Atmos.IMAX-%s",
				webT1,
			),
		},
		{
			"BLURAY clean",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.HEVC-%s",
				blurayT1,
			),
		},
		{
			"BLURAY DTS-HD MA",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.HEVC.DTS-HD.MA.5.1-%s",
				blurayT1,
			),
		},
		{
			"BLURAY DTS-HD MA IMAX",
			fmt.Sprintf(
				"Example.Movie.2026.2160p.UHD.BluRay.HEVC.DTS-HD.MA.5.1.IMAX-%s",
				blurayT1,
			),
		},
	}

	scoreProfile := func(
		name string,
		rules []config.RuleConfig,
	) map[string]int {
		t.Helper()

		profile, err := ranking.Compile(
			config.FilterProfileConfig{
				Name:   name,
				Preset: "4k",
				Rules:  rules,
			},
			loadDefineLibrary(t)...,
		)
		if err != nil {
			t.Fatalf("compile %s profile: %v", name, err)
		}

		scores := make(map[string]int, len(cases))

		for _, tc := range cases {
			kept, rejected := profile.ApplyWithRejected(
				ranking.Request{
					Kind:  ranking.KindMovie,
					Title: "Example Movie",
				},
				[]triage.Candidate{{
					Release: &release.Release{Title: tc.title},
				}},
				jhinrank.RankOptions{},
			)

			if len(rejected) != 0 || len(kept) != 1 {
				t.Fatalf(
					"%s/%s: kept=%d rejected=%+v",
					name,
					tc.name,
					len(kept),
					rejected,
				)
			}

			scores[tc.name] = kept[0].Torrent.Rank
		}

		return scores
	}

	neutral := scoreProfile("Neutral", loadNeutralRules(t))
	samsung := scoreProfile("Samsung", loadProductionRules(t))

	for name, neutralScore := range neutral {
		if samsung[name] != neutralScore {
			t.Fatalf(
				"%s differs between profiles: neutral=%d samsung=%d",
				name,
				neutralScore,
				samsung[name],
			)
		}
	}

	deltas := []struct {
		name string
		with string
		base string
		want int
	}{
		{"TrueHD+Atmos", "REMUX TrueHD Atmos", "REMUX clean", 75},
		{"TrueHD alone", "REMUX TrueHD only", "REMUX clean", 50},
		{"Atmos alone", "REMUX Atmos only", "REMUX clean", 25},
		{"DDPlus", "WEB DDPlus", "WEB clean", 25},
		{"DTS Lossless", "BLURAY DTS-HD MA", "BLURAY clean", 50},
	}

	for _, delta := range deltas {
		if got := neutral[delta.with] - neutral[delta.base]; got != delta.want {
			t.Fatalf(
				"%s effective preference=%d; want %d",
				delta.name,
				got,
				delta.want,
			)
		}
	}

	remux := neutral["REMUX clean"]
	for _, name := range []string{
		"WEB DDPlus Atmos IMAX",
		"BLURAY DTS-HD MA IMAX",
	} {
		if neutral[name] >= remux {
			t.Fatalf(
				"%s score=%d outranked/equaled clean REMUX T1=%d",
				name,
				neutral[name],
				remux,
			)
		}
	}
}

// TestAnimeAudioNeutrality proves the Anime side of the scoring-ceiling
// fix directly: every codec DraCuLa touches for audio -- the five
// high-impact/ordering-integrity codecs normalized for everyone
// (TrueHD/DTS Lossless/Atmos/Dolby Digital Plus/Dolby Digital) and the two
// remaining previously-untouched native codecs neutralized for Anime only
// (AAC/DTS Lossy) -- must contribute exactly 0 effective points for an
// Anime release. Anime's 80-point minimum tier gap has no room for any of
// them. Dolby Digital reaches 0 through the universal "Neutralize Dolby
// Digital" rule (see TestDolbyDigitalOrderingRegression), not a dedicated
// Anime-only rule -- "Neutralize Anime Dolby Digital" no longer exists,
// subsumed by the universal fix.
func TestAnimeAudioNeutrality(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Anime audio neutrality",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	group, ok := defines["Anime Shows BluRay T4 Groups"]
	if !ok || len(group) == 0 {
		t.Fatalf("missing/empty Define %q", "Anime Shows BluRay T4 Groups")
	}
	animeGroup := group[0]

	score := func(extra string) int {
		t.Helper()

		title := fmt.Sprintf(
			"Example.Anime.S01E01.1080p.BluRay.x264.%s-%s",
			extra, animeGroup,
		)
		if extra == "" {
			title = fmt.Sprintf(
				"Example.Anime.S01E01.1080p.BluRay.x264-%s",
				animeGroup,
			)
		}

		kept, rejected := profile.ApplyWithRejected(
			ranking.Request{
				Kind:    ranking.KindAnimeShow,
				IsAnime: true,
				Title:   "Example Anime",
				Season:  1,
				Episode: 1,
			},
			[]triage.Candidate{{
				Release: &release.Release{Title: title},
			}},
			jhinrank.RankOptions{},
		)

		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf(
				"%q: kept=%d rejected=%+v", title, len(kept), rejected,
			)
		}

		return kept[0].Torrent.Rank
	}

	clean := score("")

	cases := []struct {
		name  string
		extra string
	}{
		{"TrueHD", "TrueHD"},
		{"DTS Lossless", "DTS-HD.MA.5.1"},
		{"Atmos", "Atmos"},
		{"Dolby Digital Plus", "DDP5.1"},
		{"AAC", "AAC2.0"},
		{"DTS Lossy", "DTS5.1"},
		{"Dolby Digital", "DD5.1"},
		{"TrueHD+Atmos", "TrueHD.Atmos.7.1"},
	}

	for _, tc := range cases {
		if got := score(tc.extra) - clean; got != 0 {
			t.Errorf(
				"Anime %s effective delta=%+d; want 0 (clean=%d)",
				tc.name, got, clean,
			)
		}
	}
}

// TestDolbyDigitalOrderingRegression proves the Dolby Digital / Dolby
// Digital Plus ordering-integrity fix. Before this fix, plain Dolby Digital
// was left completely native (+50 for Movies/Shows) while Dolby Digital
// Plus was neutralized and given a smaller deliberate residual (effective
// +25) -- meaning the objectively worse codec silently outranked the
// better one by 25 points, for every non-Anime content kind. The universal
// "Neutralize Dolby Digital" rule (-50, no scope, no residual "Prefer"
// counterpart) closes that inversion without adding any new positive
// score, so it cannot consume any adjacent-tier headroom -- it can only
// ever remove points from the fully-decorated stack the ceiling matrix
// already exercises.
func TestDolbyDigitalOrderingRegression(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Dolby Digital ordering regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	groupFor := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}

	score := func(kind string, isAnime bool, title string) int {
		t.Helper()

		request := ranking.Request{Kind: kind, Title: "Example"}
		if kind == ranking.KindSeries || kind == ranking.KindAnimeShow {
			request.Season, request.Episode = 1, 1
		}
		if isAnime {
			request.IsAnime = true
		}

		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{{Release: &release.Release{Title: title}}},
			jhinrank.RankOptions{},
		)
		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf("%q: kept=%d rejected=%+v", title, len(kept), rejected)
		}
		return kept[0].Torrent.Rank
	}

	type isolatedCase struct {
		label    string
		kind     string
		isAnime  bool
		group    string
		wantDD   int
		wantDDP  int
		nonAnime bool
	}

	cases := []isolatedCase{
		{
			label:    "Movie",
			kind:     ranking.KindMovie,
			group:    groupFor("Movies WEB T1 Groups"),
			wantDD:   0,
			wantDDP:  25,
			nonAnime: true,
		},
		{
			label:    "Series",
			kind:     ranking.KindSeries,
			group:    groupFor("Shows WEB T1 Groups"),
			wantDD:   0,
			wantDDP:  25,
			nonAnime: true,
		},
		{
			label:   "Anime Movie",
			kind:    ranking.KindAnimeMovie,
			isAnime: true,
			group:   groupFor("Anime Movies WEB T1 Groups"),
			wantDD:  0,
			wantDDP: 0,
		},
		{
			label:   "Anime Show",
			kind:    ranking.KindAnimeShow,
			isAnime: true,
			group:   groupFor("Anime Shows WEB T1 Groups"),
			wantDD:  0,
			wantDDP: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			var titlePrefix string
			switch tc.kind {
			case ranking.KindSeries:
				titlePrefix = "Example.Show.S01E01"
			case ranking.KindAnimeShow:
				titlePrefix = "Example.Anime.S01E01"
			case ranking.KindAnimeMovie:
				titlePrefix = "Example.Anime.Movie.2025"
			default:
				titlePrefix = "Example.Movie.2026"
			}

			clean := score(tc.kind, tc.isAnime, fmt.Sprintf(
				"%s.1080p.WEB-DL.x264-%s", titlePrefix, tc.group,
			))
			withDD := score(tc.kind, tc.isAnime, fmt.Sprintf(
				"%s.1080p.WEB-DL.DD5.1.x264-%s", titlePrefix, tc.group,
			))
			withDDP := score(tc.kind, tc.isAnime, fmt.Sprintf(
				"%s.1080p.WEB-DL.DDP5.1.x264-%s", titlePrefix, tc.group,
			))

			if got := withDD - clean; got != tc.wantDD {
				t.Errorf("%s DD effective delta=%+d; want %+d", tc.label, got, tc.wantDD)
			}
			if got := withDDP - clean; got != tc.wantDDP {
				t.Errorf("%s DDP effective delta=%+d; want %+d", tc.label, got, tc.wantDDP)
			}
			if tc.nonAnime && withDDP <= withDD {
				t.Errorf(
					"%s: DDP (%d) does not strictly outrank DD (%d)",
					tc.label, withDDP, withDD,
				)
			}
		})
	}

	// Tier-authority interaction: a lower-tier Movie WEB release decorated
	// with DD instead of the usual DDP+Atmos combo must still stay below a
	// clean immediately-higher tier, proving the neutralizer introduces no
	// unexpected scoring interaction when substituted into the same
	// fully-decorated-lower-tier shape the adjacent-tier ceiling matrix
	// already exercises for DDP.
	t.Run("lower tier decorated with DD instead of DDP", func(t *testing.T) {
		t2 := groupFor("Movies WEB T2 Groups")
		t1 := groupFor("Movies WEB T1 Groups")

		decoratedWithDD := joinCeilingParts([]string{
			"Example.Movie", "2026", "2160p", "WEB-DL", "AV1",
			"Open.Matte", "Extended.Edition", "Dual.Audio", "REPACK3",
			"DD5.1", "Atmos",
		}) + "-" + t2
		cleanHigher := joinCeilingParts([]string{
			"Example.Movie", "2026", "2160p", "WEB-DL", "HEVC",
		}) + "-" + t1

		lower := score(ranking.KindMovie, false, decoratedWithDD)
		higher := score(ranking.KindMovie, false, cleanHigher)

		if lower >= higher {
			t.Errorf(
				"decorated-with-DD lower tier (%d) does not stay below clean higher tier (%d)\n  decorated: %s\n  clean:     %s",
				lower, higher, decoratedWithDD, cleanHigher,
			)
		}
	})
}

// TestVideoCodecNeutrality is the permanent real-engine proof for the
// codec-scoring tier-authority regression: StreamNZB's own streaming preset
// gives AVC/HEVC/AV1 different native ranks (+300/+700/+700) regardless of
// content kind, a +400 swing that overturned 7 of 11 production tier
// families. Neutralize AVC/HEVC/AV1 must bring every recognized codec's
// effective contribution to exactly 0, for every content kind, with no
// Anime/non-Anime split (unlike the audio neutralizers above, this policy
// has no bounded residual: the tightest actual non-Anime margin measured
// anywhere in the system, the HDR10+/lossless-audio physical-media combo,
// is only +3, leaving no safe room for any positive codec preference).
func TestVideoCodecNeutrality(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Video codec neutrality",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	groupFor := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}

	type kindCase struct {
		label       string
		kind        string
		isAnime     bool
		hasEpisode  bool
		titlePrefix string
		group       string
	}

	kinds := []kindCase{
		{
			label:       "movie",
			kind:        ranking.KindMovie,
			titlePrefix: "Example.Movie.2026",
			group:       groupFor("Movies WEB T3 Groups"),
		},
		{
			label:       "series",
			kind:        ranking.KindSeries,
			hasEpisode:  true,
			titlePrefix: "Example.Show.S01E01",
			group:       groupFor("Shows WEB T3 Groups"),
		},
		{
			label:       "anime_movie",
			kind:        ranking.KindAnimeMovie,
			isAnime:     true,
			titlePrefix: "Example.Anime.Movie.2025",
			group:       groupFor("Anime Movies BluRay T4 Groups"),
		},
		{
			label:       "anime_show",
			kind:        ranking.KindAnimeShow,
			isAnime:     true,
			hasEpisode:  true,
			titlePrefix: "Example.Anime.S01E01",
			group:       groupFor("Anime Shows BluRay T4 Groups"),
		},
	}

	codecCases := []struct {
		name    string
		aliases []string
	}{
		{"AVC", []string{"x264", "AVC"}},
		{"HEVC", []string{"x265", "HEVC"}},
		{"AV1", []string{"AV1"}},
	}

	for _, kc := range kinds {
		kc := kc

		t.Run(kc.label, func(t *testing.T) {
			score := func(codec string) int {
				t.Helper()

				var title string
				if codec == "" {
					title = fmt.Sprintf(
						"%s.1080p.WEB-DL-%s", kc.titlePrefix, kc.group,
					)
				} else {
					title = fmt.Sprintf(
						"%s.1080p.WEB-DL.%s-%s",
						kc.titlePrefix, codec, kc.group,
					)
				}

				request := ranking.Request{
					Kind:    kc.kind,
					IsAnime: kc.isAnime,
					Title:   "Example",
				}
				if kc.hasEpisode {
					request.Season = 1
					request.Episode = 1
				}

				kept, rejected := profile.ApplyWithRejected(
					request,
					[]triage.Candidate{{
						Release: &release.Release{Title: title},
					}},
					jhinrank.RankOptions{},
				)

				if len(rejected) != 0 || len(kept) != 1 {
					t.Fatalf(
						"%q: kept=%d rejected=%+v",
						title, len(kept), rejected,
					)
				}

				return kept[0].Torrent.Rank
			}

			clean := score("")

			for _, cc := range codecCases {
				for _, alias := range cc.aliases {
					t.Run(cc.name+"/"+alias, func(t *testing.T) {
						if got := score(alias) - clean; got != 0 {
							t.Errorf(
								"%s codec=%s(%s) effective delta=%+d; "+
									"want 0 (clean=%d)",
								kc.label, cc.name, alias, got, clean,
							)
						}
					})
				}
			}
		})
	}
}

// TestAnimeVersionPreferenceRegression is the permanent real-engine proof for
// the Anime v0-v4 release-version tie-breaker (profiles/rules.json "Anime
// Version vN Preference" rules, shipped since V4.4). It restores the
// dedicated fixture coverage that CHANGELOG [4.4] originally described —
// episode-suffix forms such as 01v2, case variants, REPACK interaction,
// unsupported v5+, false positives, and multi-version non-stacking — which
// had lapsed to only implicit coverage (v4 baked into
// TestAdjacentTierCeilingMatrix's decorations) after the V5.0 rearchitecture.
//
// The v0-v4 regex intentionally matches a version marker fused directly onto
// an episode number with no separator (a real fansub convention, e.g.
// "01v2"), which is why it also accepts being preceded by a bare digit and
// not only a word boundary. That behavior is deliberate and preserved here,
// not something this test should weaken.
func TestAnimeVersionPreferenceRegression(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Anime version preference regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	groupFor := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}

	type familyCase struct {
		label string
		kind  string
		group string
	}

	families := []familyCase{
		{
			label: "Anime Show",
			kind:  ranking.KindAnimeShow,
			group: groupFor("Anime Shows WEB T6 Groups"),
		},
		{
			label: "Anime Movie",
			kind:  ranking.KindAnimeMovie,
			group: groupFor("Anime Movies WEB T6 Groups"),
		},
	}

	for _, fam := range families {
		fam := fam

		t.Run(fam.label, func(t *testing.T) {
			score := func(title string) int {
				t.Helper()

				request := ranking.Request{
					Kind:    fam.kind,
					IsAnime: true,
					Title:   "Example",
				}
				if fam.kind == ranking.KindAnimeShow {
					request.Season = 1
					request.Episode = 1
				}

				kept, rejected := profile.ApplyWithRejected(
					request,
					[]triage.Candidate{{
						Release: &release.Release{Title: title},
					}},
					jhinrank.RankOptions{},
				)

				if len(rejected) != 0 || len(kept) != 1 {
					t.Fatalf(
						"%q: kept=%d rejected=%+v",
						title, len(kept), rejected,
					)
				}

				return kept[0].Torrent.Rank
			}

			var prefix string
			if fam.kind == ranking.KindAnimeShow {
				prefix = "Example.Anime.S01E01"
			} else {
				prefix = "Example.Anime.Movie.2025"
			}

			build := func(extra string) string {
				if extra == "" {
					return fmt.Sprintf(
						"%s.1080p.WEB-DL.x264-%s", prefix, fam.group,
					)
				}
				return fmt.Sprintf(
					"%s.1080p.WEB-DL.x264.%s-%s", prefix, extra, fam.group,
				)
			}

			clean := score(build(""))

			deltaCases := []struct {
				name  string
				title string
				want  int
			}{
				{"v0 explicit", build("v0"), -1},
				{"v1 ordinary", build("v1"), 1},
				{"v2 ordinary", build("v2"), 2},
				{"v3 ordinary", build("v3"), 3},
				{"v4 ordinary", build("v4"), 4},
				{"v2 uppercase", build("V2"), 2},
				{
					"v5 unsupported",
					build("v5"),
					0,
				},
				{
					"multi-version non-stacking",
					fmt.Sprintf(
						"%s.1080p.WEB-DL.x264.v2.v3-%s", prefix, fam.group,
					),
					0,
				},
				{
					"REPACK interaction with v2",
					fmt.Sprintf(
						"%s.1080p.WEB-DL.x264.v2.REPACK-%s", prefix, fam.group,
					),
					7, // +2 version, +5 effective REPACK
				},
			}

			for _, dc := range deltaCases {
				t.Run(dc.name, func(t *testing.T) {
					if got := score(dc.title) - clean; got != dc.want {
						t.Errorf(
							"delta=%+d, want %+d (clean=%d)\n  title=%s",
							got, dc.want, clean, dc.title,
						)
					}
				})
			}

			// Fused episode+version form (real fansub convention): the
			// version marker directly follows the episode/part number with
			// no separator. Must still resolve to the correct version
			// delta, matching the "01v2" case the regex was written for.
			t.Run("fused episode+version 01v2", func(t *testing.T) {
				fusedClean := score(fmt.Sprintf(
					"Example.Anime.01.1080p.WEB-DL.x264-%s", fam.group,
				))
				fusedVersioned := score(fmt.Sprintf(
					"Example.Anime.01v2.1080p.WEB-DL.x264-%s", fam.group,
				))

				if got := fusedVersioned - fusedClean; got != 2 {
					t.Errorf(
						"fused 01v2 delta=%+d, want +2 (clean=%d)",
						got, fusedClean,
					)
				}
			})

			// False positives: tokens containing a bare "v" + digit that
			// must NOT be mistaken for a version marker. Proven by score
			// delta (this pipeline does not expose matched-rule names),
			// matching the pattern every other test in this file uses.
			t.Run("AV1 codec must not match v1", func(t *testing.T) {
				// AV1 itself is separately neutralized to 0 (see
				// TestVideoCodecNeutrality); this proves the version rule
				// specifically does not also fire on the "AV1" substring.
				if got := score(fmt.Sprintf(
					"%s.1080p.WEB-DL.AV1-%s", prefix, fam.group,
				)) - clean; got != 0 {
					t.Errorf("AV1 delta=%+d, want 0 (clean=%d)", got, clean)
				}
			})

			if fam.kind == ranking.KindAnimeShow {
				t.Run("season number S02 must not match v2", func(t *testing.T) {
					s01 := score(fmt.Sprintf(
						"Example.Anime.S01E01.1080p.WEB-DL.x264-%s",
						fam.group,
					))
					s02 := score(fmt.Sprintf(
						"Example.Anime.S02E01.1080p.WEB-DL.x264-%s",
						fam.group,
					))
					if got := s02 - s01; got != 0 {
						t.Errorf(
							"S02E01 vs S01E01 delta=%+d, want 0", got,
						)
					}
				})
			}

			t.Run("group name fused as suffix must not match", func(t *testing.T) {
				plain := score(fmt.Sprintf(
					"%s.1080p.WEB-DL.x264-FakeGroup", prefix,
				))
				fused := score(fmt.Sprintf(
					"%s.1080p.WEB-DL.x264-FakeGroupv2", prefix,
				))
				if got := fused - plain; got != 0 {
					t.Errorf(
						"FakeGroupv2 vs FakeGroup delta=%+d, want 0", got,
					)
				}
			})
		})
	}
}

// TestLiteralRetagRegression is the permanent real-engine proof for the
// "Literal RETAG Soft Penalty" rule (profiles/rules.json): a standalone
// scene RETAG token scores exactly -1, universally, distinct from the
// pre-existing "Retag Soft Penalty" redistribution-marker rule (.heb, EZTV,
// RARBG, RARTV, TGx), which this test does not touch or exercise.
func TestLiteralRetagRegression(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "literal RETAG regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	groupFor := func(defineName string) string {
		toks, ok := defines[defineName]
		if !ok || len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", defineName)
		}
		return toks[0]
	}

	type kindCase struct {
		label string
		kind  string
		group string
		build func(extra string) string
	}

	kinds := []kindCase{
		{
			label: "movie",
			kind:  ranking.KindMovie,
			group: groupFor("Movies WEB T3 Groups"),
			build: func(extra string) string {
				t := "Example.Movie.2026.1080p.WEB-DL.x264"
				if extra != "" {
					t += "." + extra
				}
				return t
			},
		},
		{
			label: "series",
			kind:  ranking.KindSeries,
			group: groupFor("Shows WEB T3 Groups"),
			build: func(extra string) string {
				t := "Example.Show.S01E01.1080p.WEB-DL.x264"
				if extra != "" {
					t += "." + extra
				}
				return t
			},
		},
		{
			label: "anime_movie",
			kind:  ranking.KindAnimeMovie,
			group: groupFor("Anime Movies WEB T6 Groups"),
			build: func(extra string) string {
				t := "Example.Anime.Movie.2025.1080p.WEB-DL.x264"
				if extra != "" {
					t += "." + extra
				}
				return t
			},
		},
		{
			label: "anime_show",
			kind:  ranking.KindAnimeShow,
			group: groupFor("Anime Shows WEB T6 Groups"),
			build: func(extra string) string {
				t := "Example.Anime.S01E01.1080p.WEB-DL.x264"
				if extra != "" {
					t += "." + extra
				}
				return t
			},
		},
	}

	for _, kc := range kinds {
		kc := kc

		t.Run(kc.label, func(t *testing.T) {
			score := func(title string) int {
				t.Helper()

				request := ranking.Request{
					Kind:    kc.kind,
					IsAnime: kc.kind == ranking.KindAnimeMovie || kc.kind == ranking.KindAnimeShow,
					Title:   "Example",
				}
				if kc.kind == ranking.KindSeries || kc.kind == ranking.KindAnimeShow {
					request.Season = 1
					request.Episode = 1
				}

				kept, rejected := profile.ApplyWithRejected(
					request,
					[]triage.Candidate{{
						Release: &release.Release{
							Title: title + "-" + kc.group,
						},
					}},
					jhinrank.RankOptions{},
				)

				if len(rejected) != 0 || len(kept) != 1 {
					t.Fatalf(
						"%q: kept=%d rejected=%+v",
						title, len(kept), rejected,
					)
				}

				return kept[0].Torrent.Rank
			}

			clean := score(kc.build(""))

			// Isolated delta, case-insensitive, boundary forms.
			isolatedCases := []struct {
				name  string
				extra string
			}{
				{"uppercase dot-separated", "RETAG"},
				{"lowercase", "retag"},
				{"mixed case", "ReTaG"},
			}
			for _, ic := range isolatedCases {
				t.Run(ic.name, func(t *testing.T) {
					if got := score(kc.build(ic.extra)) - clean; got != -1 {
						t.Errorf("delta=%+d, want -1", got)
					}
				})
			}

			// Token-boundary false positives: must NOT match.
			falsePositiveCases := []struct {
				name  string
				extra string
			}{
				{"Pretag prefix", "Pretag"},
				{"Retagged suffix", "Retagged"},
				{"RETAGS plural", "RETAGS"},
			}
			for _, fp := range falsePositiveCases {
				t.Run(fp.name, func(t *testing.T) {
					got := score(kc.build(fp.extra)) - clean
					if got != 0 {
						t.Errorf(
							"%s: delta=%+d, want 0 (no false-positive match)",
							fp.name, got,
						)
					}
				})
			}
			t.Run("fused into prior word", func(t *testing.T) {
				fused := kc.build("") + "GroupRETAG"
				got := score(fused) - clean
				if got != 0 {
					t.Errorf(
						"fused into prior word: delta=%+d, want 0", got,
					)
				}
			})

			// PROPER/REPACK additive interaction.
			properRepackCases := []struct {
				name  string
				extra string
				want  int
			}{
				{"PROPER alone", "PROPER", 5},
				{"RETAG + PROPER", "RETAG.PROPER", 4},
				{"REPACK alone", "REPACK", 5},
				{"RETAG + REPACK", "RETAG.REPACK", 4},
				{"REPACK2 alone", "REPACK2", 6},
				{"RETAG + REPACK2", "RETAG.REPACK2", 5},
				{"REPACK3 alone", "REPACK3", 7},
				{"RETAG + REPACK3", "RETAG.REPACK3", 6},
			}
			for _, pc := range properRepackCases {
				t.Run(pc.name, func(t *testing.T) {
					if got := score(kc.build(pc.extra)) - clean; got != pc.want {
						t.Errorf(
							"delta=%+d, want %+d\n  title=%s",
							got, pc.want, kc.build(pc.extra),
						)
					}
				})
			}
		})
	}
}

// TestEditionNeutralityRegression is the permanent real-engine proof for
// the edition-scoring tier-authority regression discovered by the Edition
// Preference Layer audit: Jhin's scalar Edition field grants a generic
// native +100 rank for any non-empty parsed value, with zero
// differentiation between the 10 canonical values (Anniversary Edition,
// Ultimate Edition, Directors Cut, Extended Edition, Collectors Edition,
// Theatrical, Uncut, IMAX, Diamond Edition, Remastered). Only Movie
// Directors Cut/Extended Edition were ever compensated (native +100,
// stored -75, effective +25); every other value stayed +100
// uncompensated everywhere, including outside Movie scope entirely,
// producing a real measured production tier inversion (-42 margin) for
// Movie physical media and an untested exposure for Series/Anime. The
// universal "Neutralize Edition" rule (-100, no scope) now cancels that
// native rank unconditionally; only explicit, reviewed DraCuLa residuals
// (Movie Directors Cut/Extended +25, Movie IMAX +700, Anime Uncensored
// +10) reach a non-zero effective score.
//
// Unlike TestMovieEditionPreferenceCeilings (which uses the narrower
// rules.Compile/Evaluate custom-rule-only layer and must add a hardcoded
// nativeEditionPoints=100 constant by hand), this test uses the full
// production ranking.Compile/ApplyWithRejected pipeline, so native Jhin
// ranking is included automatically and every asserted delta is the true
// effective production score change - no hand-added constant to drift.
func TestEditionNeutralityRegression(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Edition neutrality regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)
	tok := func(name string) string {
		e, ok := defines[name]
		if !ok || len(e) == 0 {
			t.Fatalf("missing/empty Define %q", name)
		}
		return e[0]
	}

	fullAvail := triage.AvailState{
		Status:       triage.AvailAvailable,
		OnMyBackbone: true,
		CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
	}

	score := func(kind, title string) int {
		t.Helper()

		candidate := triage.Candidate{
			Release: &release.Release{Title: title},
		}
		candidate.Verdict.Avail = fullAvail

		request := ranking.Request{Kind: kind, Title: "Example"}
		if kind == ranking.KindSeries || kind == ranking.KindAnimeShow {
			request.Season = 1
			request.Episode = 1
		}
		if kind == ranking.KindAnimeShow || kind == ranking.KindAnimeMovie {
			request.IsAnime = true
		}

		kept, rejected := profile.ApplyWithRejected(
			request,
			[]triage.Candidate{candidate},
			jhinrank.RankOptions{},
		)
		if len(rejected) != 0 {
			t.Fatalf("unexpectedly rejected %q: %+v", title, rejected)
		}
		if len(kept) != 1 {
			t.Fatalf("kept %d releases for %q; want 1", len(kept), title)
		}
		return kept[0].Torrent.Rank
	}

	// Canonical Edition tokens, verified via a direct jhin.Parse probe to
	// parse cleanly into the exact canonical Edition value (edition set,
	// codec intact) at this placement - right after quality/resolution,
	// before audio/codec - for all 4 content kinds. Diamond Edition's
	// `\b\.Diamond\.\b` pattern consumes both flanking dots and can fuse
	// an adjacent token (breaking that token's own boundary-based match)
	// depending on placement; this placement was verified safe.
	editionTokens := map[string]string{
		"Anniversary Edition": "15th.Anniversary.Edition",
		"Ultimate Edition":    "Ultimate.Edition",
		"Directors Cut":       "Directors.Cut",
		"Extended Edition":    "Extended.Edition",
		"Collectors Edition":  "Collectors.Edition",
		"Theatrical":          "Theatrical",
		"Uncut":               "Uncut",
		"IMAX":                "IMAX",
		"Diamond Edition":     "Diamond.Edition",
		"Remastered":          "Remastered",
	}

	type kindSetup struct {
		kind       string
		prefix     string
		suffix     string
		definePref string
	}

	setups := map[string]kindSetup{
		"movie": {
			ranking.KindMovie,
			"Example.Movie.2026.1080p.WEB-DL",
			"DDP5.1.x264",
			"Movies WEB",
		},
		"series": {
			ranking.KindSeries,
			"Example.Show.2026.S01E01.1080p.WEB-DL",
			"DDP5.1.x264",
			"Shows WEB",
		},
		"anime_show": {
			ranking.KindAnimeShow,
			"Example.Anime.2026.S01E01.1080p.WEB-DL",
			"AAC.x264",
			"Anime Shows WEB",
		},
		"anime_movie": {
			ranking.KindAnimeMovie,
			"Example.Anime.Movie.2026.1080p.WEB-DL",
			"AAC.x264",
			"Anime Movies WEB",
		},
	}

	buildTitle := func(s kindSetup, extra string) string {
		group := tok(s.definePref + " T1 Groups")
		if extra == "" {
			return s.prefix + "." + s.suffix + "-" + group
		}
		// "X" buffer tokens on both sides of the inserted extra: Diamond
		// Edition's `\b\.Diamond\.\b` pattern consumes both flanking
		// dots, which - without a buffer - fuses "WEB-DL" with the token
		// on either side (e.g. dropping the "-DL" suffix entirely,
		// corrupting quality parsing, not just edition), verified via a
		// direct jhin.Parse probe. The buffers absorb that fusion
		// harmlessly for every canonical Edition token.
		return s.prefix + ".X." + extra + ".X." + s.suffix + "-" + group
	}

	// Expected effective deltas (full production score, native + custom
	// combined) relative to the same-kind, same-tier clean baseline.
	// "-" entries: the value is not expected to differ from the universal
	// baseline row and is covered by the shared "uncompensated" case.
	uncompensated := []string{
		"Anniversary Edition", "Ultimate Edition", "Collectors Edition",
		"Theatrical", "Diamond Edition", "Remastered",
	}

	for kindName, s := range setups {
		clean := score(s.kind, buildTitle(s, ""))

		for _, name := range uncompensated {
			got := score(s.kind, buildTitle(s, editionTokens[name])) - clean
			if got != 0 {
				t.Errorf(
					"%s edition=%s: effective delta=%+d; want 0 (clean=%d)",
					kindName, name, got, clean,
				)
			}
		}

		// Uncut: no residual outside Anime (Anime's separate "Uncensored"
		// raw-regex rule also matches the literal "Uncut" text and grants
		// +10, on top of the fully neutralized native Edition score).
		uncutWant := 0
		if kindName == "anime_show" || kindName == "anime_movie" {
			uncutWant = 10
		}
		if got := score(s.kind, buildTitle(s, editionTokens["Uncut"])) - clean; got != uncutWant {
			t.Errorf(
				"%s edition=Uncut: effective delta=%+d; want %+d (clean=%d)",
				kindName, got, uncutWant, clean,
			)
		}

		// Directors Cut / Extended Edition: +25 residual, Movie only.
		movieResidualWant := 0
		if kindName == "movie" {
			movieResidualWant = 25
		}
		for _, name := range []string{"Directors Cut", "Extended Edition"} {
			got := score(s.kind, buildTitle(s, editionTokens[name])) - clean
			if got != movieResidualWant {
				t.Errorf(
					"%s edition=%s: effective delta=%+d; want %+d (clean=%d)",
					kindName, name, got, movieResidualWant, clean,
				)
			}
		}

		// IMAX: raw releaseName regex, scope=movie, stored +700. Effective
		// delta is +700 for Movie (native neutralized, custom rule takes
		// over entirely) and 0 for every other kind (scope-excluded, and
		// native contribution is neutralized the same as any other
		// canonical Edition value).
		imaxWant := 0
		if kindName == "movie" {
			imaxWant = 700
		}
		if got := score(s.kind, buildTitle(s, editionTokens["IMAX"])) - clean; got != imaxWant {
			t.Errorf(
				"%s edition=IMAX: effective delta=%+d; want %+d (clean=%d)",
				kindName, got, imaxWant, clean,
			)
		}

		// Unrated: a separate boolean field with no Edition value of its
		// own (edition stays ""), so it must be completely unaffected by
		// Neutralize Edition. Anime's Uncensored rule also matches the
		// literal "Unrated" text independently of the Edition field.
		unratedWant := 0
		if kindName == "anime_show" || kindName == "anime_movie" {
			unratedWant = 10
		}
		if got := score(s.kind, buildTitle(s, "Unrated")) - clean; got != unratedWant {
			t.Errorf(
				"%s Unrated: effective delta=%+d; want %+d (clean=%d)",
				kindName, got, unratedWant, clean,
			)
		}

		// Extended+IMAX shadowing: Edition is scalar, so only one value
		// (whichever the parser table matches first) is stored - proving
		// the native neutralizer fires exactly once regardless of how
		// many edition tokens appear. IMAX's raw releaseName regex is
		// independent of the parsed Edition field, so it still fires on
		// its own even when the parsed Edition is "Extended Edition".
		shadowWant := 0
		if kindName == "movie" {
			// Native (+100, once) - Neutralize Edition (-100) +
			// Movie Edition Preference (+25, parsed Extended Edition) +
			// IMAX (+700, independent raw regex) = +725.
			shadowWant = 725
		}
		shadowExtra := editionTokens["Extended Edition"] + ".IMAX"
		if got := score(s.kind, buildTitle(s, shadowExtra)) - clean; got != shadowWant {
			t.Errorf(
				"%s Extended+IMAX shadow: effective delta=%+d; want %+d (clean=%d)",
				kindName, got, shadowWant, clean,
			)
		}
	}
}
