package stremio

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/auth"
	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/parser"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const draculaFormatterPrefix = "SNZBF1:"

type draculaFormatterPayload struct {
	StreamNZBFormatProfile    int    `json:"streamnzb_format_profile"`
	Name                      string `json:"name"`
	ResultNameTemplate        string `json:"result_name_template"`
	ResultDescriptionTemplate string `json:"result_description_template"`
}

type draculaFormatterFixtureFile struct {
	Cases []draculaFormatterCase `json:"cases"`
}

type draculaFormatterCase struct {
	Name         string          `json:"name"`
	Context      json.RawMessage `json:"context"`
	Contains     []string        `json:"contains,omitempty"`
	Excludes     []string        `json:"excludes,omitempty"`
	Counts       map[string]int  `json:"counts,omitempty"`
	NameContains []string        `json:"name_contains,omitempty"`
	NameExcludes []string        `json:"name_excludes,omitempty"`
	NameCounts   map[string]int  `json:"name_counts,omitempty"`
}

func decodeDraculaShareCode(raw []byte, prefix string) ([]byte, error) {
	code := strings.TrimSpace(string(raw))

	if !strings.HasPrefix(code, prefix) {
		return nil, fmt.Errorf(
			"share code does not start with %q",
			prefix,
		)
	}

	encoded := strings.TrimPrefix(
		code,
		prefix,
	)

	compressed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf(
			"decode formatter Base64URL: %w",
			err,
		)
	}

	reader, err := gzip.NewReader(
		bytes.NewReader(compressed),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"open formatter gzip payload: %w",
			err,
		)
	}
	defer reader.Close()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf(
			"read formatter gzip payload: %w",
			err,
		)
	}

	return decoded, nil
}

func loadDraculaFormatter(
	t *testing.T,
) draculaFormatterPayload {
	t.Helper()

	path := os.Getenv("DRACULA_FORMATTER_PATH")
	if path == "" {
		t.Fatal("DRACULA_FORMATTER_PATH is not set")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read formatter %q: %v",
			path,
			err,
		)
	}

	raw := data

	if strings.HasPrefix(
		strings.TrimSpace(string(data)),
		draculaFormatterPrefix,
	) {
		raw, err = decodeDraculaShareCode(data, draculaFormatterPrefix)
		if err != nil {
			t.Fatal(err)
		}
	}

	var payload draculaFormatterPayload

	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf(
			"decode formatter payload %q: %v",
			path,
			err,
		)
	}

	if payload.StreamNZBFormatProfile != 1 {
		t.Fatalf(
			"unsupported formatter schema: got %d, expected 1",
			payload.StreamNZBFormatProfile,
		)
	}

	if strings.TrimSpace(
		payload.ResultDescriptionTemplate,
	) == "" {
		t.Fatal(
			"result_description_template is empty",
		)
	}

	if err := ValidateResultTemplates(
		payload.ResultNameTemplate,
		payload.ResultDescriptionTemplate,
	); err != nil {
		t.Fatalf(
			"StreamNZB rejected formatter templates: %v",
			err,
		)
	}

	return payload
}

func loadDraculaFormatterFixtures(
	t *testing.T,
) draculaFormatterFixtureFile {
	t.Helper()

	path := os.Getenv(
		"DRACULA_FORMATTER_CASES_PATH",
	)

	if path == "" {
		t.Fatal(
			"DRACULA_FORMATTER_CASES_PATH is not set",
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read formatter fixtures %q: %v",
			path,
			err,
		)
	}

	var fixtures draculaFormatterFixtureFile

	if err := json.Unmarshal(
		data,
		&fixtures,
	); err != nil {
		t.Fatalf(
			"decode formatter fixtures: %v",
			err,
		)
	}

	if len(fixtures.Cases) == 0 {
		t.Fatal(
			"formatter fixture file contains no cases",
		)
	}

	return fixtures
}

func renderDraculaFormatter(
	t *testing.T,
	templateText string,
	ctx FormatContext,
) string {
	t.Helper()

	tpl, err := compileFormatTemplate(
		templateText,
	)

	if err != nil {
		t.Fatalf(
			"compile formatter through StreamNZB: %v",
			err,
		)
	}

	const fallback = "DRACULA_FORMATTER_FALLBACK"

	rendered := renderResultTemplate(
		tpl,
		ctx,
		fallback,
	)

	if rendered == fallback {
		t.Fatal(
			"formatter unexpectedly used StreamNZB fallback rendering",
		)
	}

	return rendered
}

func TestDraculaFormatterFixtures(
	t *testing.T,
) {
	payload := loadDraculaFormatter(t)
	fixtures := loadDraculaFormatterFixtures(t)

	for _, fixture := range fixtures.Cases {
		fixture := fixture

		t.Run(
			fixture.Name,
			func(t *testing.T) {
				var ctx FormatContext

				if len(fixture.Context) > 0 {
					if err := json.Unmarshal(
						fixture.Context,
						&ctx,
					); err != nil {
						t.Fatalf(
							"decode FormatContext: %v",
							err,
						)
					}
				}

				// Most small regression fixtures only need the description.
				// Rich end-to-end fixtures opt into result-name rendering with
				// name_contains/name_excludes/name_counts. This avoids forcing
				// sparse contexts through a name template that legitimately
				// renders empty and therefore triggers StreamNZB's fallback.
				if len(fixture.NameContains) > 0 ||
					len(fixture.NameExcludes) > 0 ||
					len(fixture.NameCounts) > 0 {
					renderedName := renderDraculaFormatter(
						t,
						payload.ResultNameTemplate,
						ctx,
					)

					t.Logf(
						"rendered result name:\n%s",
						renderedName,
					)

					for _, expected := range fixture.NameContains {
						if !strings.Contains(
							renderedName,
							expected,
						) {
							t.Errorf(
								"expected result name to contain %q\nrendered:\n%s",
								expected,
								renderedName,
							)
						}
					}

					for _, unexpected := range fixture.NameExcludes {
						if strings.Contains(
							renderedName,
							unexpected,
						) {
							t.Errorf(
								"expected result name not to contain %q\nrendered:\n%s",
								unexpected,
								renderedName,
							)
						}
					}

					for text, expectedCount := range fixture.NameCounts {
						actual := strings.Count(
							renderedName,
							text,
						)

						if actual != expectedCount {
							t.Errorf(
								"%q rendered %d times in result name, want %d\nrendered:\n%s",
								text,
								actual,
								expectedCount,
								renderedName,
							)
						}
					}
				}

				rendered := renderDraculaFormatter(
					t,
					payload.ResultDescriptionTemplate,
					ctx,
				)

				// Deliberately visible with go test -v.
				// This is the formatter simulation output.
				t.Logf(
					"rendered result description:\n%s",
					rendered,
				)

				for _, expected := range fixture.Contains {
					if !strings.Contains(
						rendered,
						expected,
					) {
						t.Errorf(
							"expected output to contain %q\nrendered:\n%s",
							expected,
							rendered,
						)
					}
				}

				for _, unexpected := range fixture.Excludes {
					if strings.Contains(
						rendered,
						unexpected,
					) {
						t.Errorf(
							"expected output not to contain %q\nrendered:\n%s",
							unexpected,
							rendered,
						)
					}
				}

				for text, expectedCount := range fixture.Counts {
					actual := strings.Count(
						rendered,
						text,
					)

					if actual != expectedCount {
						t.Errorf(
							"%q rendered %d times, want %d\nrendered:\n%s",
							text,
							actual,
							expectedCount,
							rendered,
						)
					}
				}
			},
		)
	}

	// The fixtures above inject FormatContext.Subtitles directly, which
	// proves template rendering but says nothing about StreamNZB's own
	// subtitle-source merge/dedup path. This exercises the real
	// construction path instead: newFormatContext -> releaseSubtitleCodes
	// -> language.MergeLanguageCodes, across all three supported
	// sources -- the release name (parsed via the real jhin-backed
	// parser), the indexer's own Release.Subtitles tag, and a probed
	// file's tagged MediaCaps.SubtitleLanguages -- with a deliberate
	// cross-source normalization/dedup overlap ("FR" from the indexer,
	// "fr" from the probe) rather than a same-source repeat.
	t.Run(
		"real StreamNZB subtitle source merge (newFormatContext)",
		func(t *testing.T) {
			title := "Example.Movie.2026.1080p.WEB-DL.Arabic.Subs-GROUP"

			meta := parser.ParseReleaseTitle(title)
			if len(meta.Subtitles) == 0 {
				t.Fatal(
					"release-name parse produced no subtitle language; " +
						"fixture title no longer exercises the parsed-subtitle source",
				)
			}

			cand := triage.Candidate{
				Release: &release.Release{
					Title:     title,
					Subtitles: []string{"FR"},
				},
				Metadata: meta,
				Verdict: triage.Verdict{
					Probed: &release.MediaCaps{
						TracksProbed:      true,
						SubtitleLanguages: []string{"de", "fr"},
					},
				},
			}

			ctx := newFormatContext(
				cand, 1, 1, 0, 0, "", "", "Example Movie", "", false, 0,
			)

			want := []string{"ar", "fr", "de"}
			if !slices.Equal(ctx.Subtitles, want) {
				t.Fatalf(
					"newFormatContext merged .Subtitles = %v, want %v "+
						"(parsed=%v reported=%v probed=%v)",
					ctx.Subtitles,
					want,
					meta.Subtitles,
					cand.Release.Subtitles,
					cand.Verdict.Probed.SubtitleLanguages,
				)
			}

			if !strings.Contains(
				payload.ResultDescriptionTemplate,
				"SUBS ",
			) {
				// Only the debug formatter (DraCuLa Debug) carries the
				// SUBS diagnostic line; production intentionally does
				// not, so there is nothing further to render here.
				return
			}

			rendered := renderDraculaFormatter(
				t,
				payload.ResultDescriptionTemplate,
				ctx,
			)

			wantLine := "SUBS raw=ar,fr,de | mapped=\U0001F1F8\U0001F1E6,\U0001F1EB\U0001F1F7,\U0001F1E9\U0001F1EA"
			if !strings.Contains(rendered, wantLine) {
				t.Errorf(
					"expected debug output to contain %q\nrendered:\n%s",
					wantLine,
					rendered,
				)
			}
		},
	)

	// Source-aware language marker: the .Languages branch shows ✓ only when
	// .LanguageSource is "measured" AND every displayed language is one of
	// the probed audio track languages. .Languages is a union of measured,
	// reported and inferred codes, so "measured" alone can sit next to a
	// language the file never confirmed. Contexts come from the real
	// newFormatContext path with real probe data, not fixture injection.
	t.Run(
		"source-aware language marker (newFormatContext)",
		func(t *testing.T) {
			const measuredFrag = `{{if and (eq .LanguageSource "measured") (not (without .Probed.AudioLanguages .Languages))}}✓{{else}}⛿{{end}} `
			tpl := payload.ResultDescriptionTemplate
			if !strings.Contains(tpl, "⛿") {
				// The debug formatter has no language marker line.
				return
			}
			if !strings.Contains(tpl, measuredFrag) {
				t.Fatal("production language marker no longer uses the measured-subset condition")
			}
			// The formatter as it was before the marker became source-aware.
			previous := strings.Replace(tpl, measuredFrag, "⛿ ", 1)

			probe := func(audio, subs []string) *release.MediaCaps {
				return &release.MediaCaps{
					TracksProbed:      true,
					AudioStreams:      max(len(audio), 1),
					AudioLanguages:    audio,
					SubtitleLanguages: subs,
					DurationSeconds:   1440,
				}
			}

			cases := []struct {
				name     string
				title    string
				reported []string
				subs     []string
				probed   *release.MediaCaps
				source   string
				line     string // expected language line; "" = none
			}{
				{"fully measured", "Example.Anime.S01E01.1080p.BluRay.x264-GRP", nil, nil,
					probe([]string{"ja", "en"}, nil), "measured", "✓ JA · EN"},
				{"partially measured merged set", "Example.Anime.S01E01.1080p.BluRay.JPN.ENG.x264-GRP", nil, nil,
					probe([]string{"ja"}, nil), "measured", "⛿ JA · EN"},
				{"indexer-reported", "Example.Movie.2026.1080p.WEB-DL.x264-GRP", []string{"English"}, nil,
					nil, "reported", "⛿ EN"},
				{"name-only inferred", "Example.Movie.2026.1080p.WEB-DL.German.DL.x264-GRP", nil, nil,
					nil, "inferred", "⛿ DE"},
				{"untagged probe audio, inferred fallback", "Example.Movie.2026.1080p.BluRay.German.x264-GRP", nil, nil,
					probe(nil, nil), "inferred", "⛿ DE"},
				{"subtitle-only", "Example.Movie.2026.1080p.WEB-DL.x264-GRP", nil, []string{"FR", "DE"},
					nil, "", "⛿ sᴜʙ 🇫🇷 🇩🇪"},
				{"no language metadata", "Example.Movie.2026.1080p.WEB-DL.x264-GRP", nil, nil,
					nil, "", ""},
				{"fully measured with subtitle overflow", "Example.Movie.2026.1080p.BluRay.x264-GRP", nil, nil,
					probe([]string{"en"}, []string{"en", "fr", "de", "es", "it"}), "measured", "✓ EN · sᴜʙ 🇬🇧 🇫🇷 🇩🇪 +2"},
			}

			languageLine := func(rendered string) string {
				for _, l := range strings.Split(rendered, "\n") {
					if strings.HasPrefix(l, "✓ ") || strings.HasPrefix(l, "⛿ ") {
						return l
					}
				}
				return ""
			}

			for _, tc := range cases {
				cand := triage.Candidate{
					Release: &release.Release{
						Title:     tc.title,
						Languages: tc.reported,
						Subtitles: tc.subs,
					},
					Metadata: parser.ParseReleaseTitle(tc.title),
					Verdict:  triage.Verdict{Probed: tc.probed},
				}
				ctx := newFormatContext(cand, 1, 1, 0, 0, "", "", "Example", "", false, 0)
				if ctx.LanguageSource != tc.source {
					t.Errorf("%s: .LanguageSource = %q, want %q", tc.name, ctx.LanguageSource, tc.source)
				}

				rendered := renderDraculaFormatter(t, tpl, ctx)
				if got := languageLine(rendered); got != tc.line {
					t.Errorf("%s: language line %q, want %q\nrendered:\n%s", tc.name, got, tc.line, rendered)
				}

				// Only the marker may differ from the previous formatter,
				// and only where every displayed language was measured.
				before := renderDraculaFormatter(t, previous, ctx)
				want := before
				if strings.HasPrefix(tc.line, "✓ ") {
					want = strings.Replace(before, "⛿ ", "✓ ", 1)
				}
				if rendered != want {
					t.Errorf("%s: output differs from the previous formatter beyond the marker\nbefore:\n%s\nafter:\n%s", tc.name, before, rendered)
				}
			}
		},
	)

	// Resolution-independent stars (StreamNZB 6.4.0, Gaisberg/streamnzb#373).
	// The production name template rates each result with the built-in
	// resolution ladder removed, while ranking and the description's numeric
	// score keep the normal .Score. Candidates come from the published
	// profile.txt and Define Library through the real ranking pipeline and
	// the real newFormatContext, mapped as handlers_playlist.go/facade.go do.
	t.Run(
		"resolution-independent stars (real ranking pipeline)",
		func(t *testing.T) {
			if !strings.Contains(payload.ResultDescriptionTemplate, "⛿") {
				// The debug formatter keeps raw score/top diagnostics.
				return
			}
			const starsFrag = `{{if gt .TopScoreWithoutResolution 0}} {{stars 5 .TopScoreWithoutResolution .ScoreWithoutResolution}}{{end}}`
			if !strings.Contains(payload.ResultNameTemplate, starsFrag) {
				t.Fatal("production name template no longer renders stars from the resolution-independent score fields")
			}
			if strings.Contains(payload.ResultNameTemplate, "stars 5 .TopScore .Score") {
				t.Fatal("production name template still renders stars from .TopScore/.Score")
			}
			if !strings.Contains(payload.ResultDescriptionTemplate, `{{translate "0123456789" "₀₁₂₃₄₅₆₇₈₉" .Score}}`) ||
				strings.Contains(payload.ResultDescriptionTemplate, "ScoreWithoutResolution") {
				t.Fatal("production description must show the normal ranking .Score")
			}

			root := filepath.Join(filepath.Dir(os.Getenv("DRACULA_FORMATTER_CASES_PATH")), "..", "..", "..")
			profile := draculaProductionProfile(t, root)

			// Frieren: a 2160p WEB T4 release, a strong 1080p BluRay T1
			// dual-audio release and a weak untiered 1080p WEB release.
			const (
				uhdWeb  = "Frieren.Beyond.Journeys.End.S01E01.2160p.CR.WEB-DL.AAC2.0.H.264-VARYG"
				bluray  = "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.Dual-Audio.FLAC2.0.x265-FLE"
				weakWeb = "Frieren.Beyond.Journeys.End.S01E01.1080p.WEB.H264-SubsPlease"
			)
			animeCands, anime := draculaRankedContexts(t, profile, ranking.Request{
				Kind: ranking.KindAnimeShow, IsAnime: true,
				Title: "Frieren Beyond Journeys End", Season: 1, Episode: 1,
			}, weakWeb, bluray, uhdWeb)
			if got := draculaContextTitles(anime); !slices.Equal(got, []string{uhdWeb, bluray, weakWeb}) {
				t.Fatalf("ranking order changed: %v", got)
			}
			for i, ctx := range anime {
				if i > 0 && ctx.Score > anime[i-1].Score {
					t.Errorf("order no longer follows .Score at #%d", i+1)
				}
				if ctx.TopScoreWithoutResolution != anime[1].ScoreWithoutResolution {
					t.Errorf("%s: TopScoreWithoutResolution = %d, want the BluRay's %d",
						ctx.ReleaseTitle, ctx.TopScoreWithoutResolution, anime[1].ScoreWithoutResolution)
				}
			}
			ladder := func(ctx FormatContext) int { return ctx.Score - ctx.ScoreWithoutResolution }
			if ladder(anime[0]) != ranking.ResolutionScore(rank.Res2160p) ||
				ladder(anime[1]) != ranking.ResolutionScore(rank.Res1080p) {
				t.Errorf("ScoreWithoutResolution removed more than the built-in ladder: 2160p %d, 1080p %d",
					ladder(anime[0]), ladder(anime[1]))
			}
			if anime[0].ScoreWithoutResolution >= anime[1].ScoreWithoutResolution {
				t.Fatalf("fixture premise lost: BluRay merit %d must exceed 2160p WEB merit %d",
					anime[1].ScoreWithoutResolution, anime[0].ScoreWithoutResolution)
			}

			// Star expectations run on labels from StreamNZB's own playlist
			// aggregation (facade.go), which computes both top values itself.
			names := draculaPlaylistLabels(t, payload.ResultNameTemplate, animeCands, anime)
			for i, ctx := range anime {
				t.Logf("#%d score=%d swr=%d %q", i+1, ctx.Score, ctx.ScoreWithoutResolution, names[i])
			}
			if !strings.Contains(names[1], "★★★★★") {
				t.Errorf("strong 1080p BluRay should rate five stars below a 2160p result:\n%s", names[1])
			}
			if strings.Contains(names[0], "★★★★★") {
				t.Errorf("2160p WEB should not rate five stars from resolution alone:\n%s", names[0])
			}
			if strings.Count(names[0], "★") <= strings.Count(names[2], "★") {
				t.Errorf("weak 1080p WEB should rate below the 2160p WEB:\n%s\n%s", names[0], names[2])
			}
			desc := renderDraculaFormatter(t, payload.ResultDescriptionTemplate, anime[1])
			if sub := draculaSubscript(anime[1].Score); !strings.Contains(desc, sub) {
				t.Errorf("description should show ranking score %s:\n%s", sub, desc)
			}

			// Movies: a resolution-scoped user rule (Movies UHD BluRay T1,
			// resolution == "2160p") stays in the adjusted score; 720p and
			// unparsed resolution pay no ladder, so their scores are equal.
			const (
				webT1   = "Example.Movie.2024.2160p.WEB-DL.DDP5.1.H.265-ABBIE"
				uhdBD   = "Example.Movie.2024.2160p.UHD.BluRay.x265-DON"
				hd720   = "Example.Movie.2024.720p.WEB-DL.DDP5.1.H.264-NTb"
				unknown = "Example.Movie.2024.WEB-DL.DDP5.1.H.264-NTb"
			)
			movieCands, movies := draculaRankedContexts(t, profile, ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
				unknown, hd720, uhdBD, webT1)
			byTitle := map[string]FormatContext{}
			for i, ctx := range movies {
				byTitle[ctx.ReleaseTitle] = ctx
				if i > 0 && ctx.Score > movies[i-1].Score {
					t.Errorf("order no longer follows .Score at #%d", i+1)
				}
			}
			draculaPlaylistLabels(t, payload.ResultNameTemplate, movieCands, movies)
			bd := byTitle[uhdBD]
			if !slices.ContainsFunc(bd.MatchedRules, func(m triage.RuleMatch) bool { return m.Name == "Movies UHD BluRay T1" }) {
				t.Fatalf("premise lost: %s no longer matches Movies UHD BluRay T1", uhdBD)
			}
			if ladder(bd) != ranking.ResolutionScore(rank.Res2160p) {
				t.Errorf("resolution-scoped user rule was removed with the ladder: removed %d", ladder(bd))
			}
			for _, title := range []string{hd720, unknown} {
				if ctx := byTitle[title]; ctx.ScoreWithoutResolution != ctx.Score {
					t.Errorf("%s pays no ladder but ScoreWithoutResolution %d != Score %d", title, ctx.ScoreWithoutResolution, ctx.Score)
				}
			}
		},
	)
}

// draculaProductionProfile compiles the published profile.txt rules against
// the generated Define Library, as a user importing both would.
func draculaProductionProfile(t *testing.T, root string) *ranking.Profile {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, "profile.txt"))
	if err != nil {
		t.Fatalf("read profile.txt: %v", err)
	}
	raw, err := decodeDraculaShareCode(data, "SNZBP1:")
	if err != nil {
		t.Fatalf("decode profile.txt: %v", err)
	}
	var payload struct {
		Preset  string                           `json:"preset"`
		Scoring map[string]*config.ScoringConfig `json:"scoring"`
		Rules   []config.RuleConfig              `json:"rules"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode profile JSON: %v", err)
	}

	defines, err := os.ReadFile(filepath.Join(root, "generated", "streamnzb-defines.txt"))
	if err != nil {
		t.Fatalf("read Define Library: %v", err)
	}
	var library []config.RuleConfig
	for _, line := range strings.Split(string(defines), "\n") {
		line = strings.TrimSpace(line)
		label, when, ok := strings.Cut(line, ": define if ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !ok {
			t.Fatalf("invalid Define line: %q", line)
		}
		scope := ""
		if open := strings.LastIndex(label, " ["); open > 0 && strings.HasSuffix(label, "]") {
			label, scope = label[:open], label[open+2:len(label)-1]
		}
		library = append(library, config.RuleConfig{Name: label, Scope: scope, When: when, Action: config.RuleActionDefine})
	}

	profile, err := ranking.Compile(config.FilterProfileConfig{
		Name: "DraCuLa formatter stars", Preset: payload.Preset, Scoring: payload.Scoring, Rules: payload.Rules,
	}, library...)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}
	return profile
}

// draculaRankedContexts ranks titles through the profile, maps survivors to
// candidates as handlers_playlist.go does, and builds each FormatContext the
// way facade.go does, so field-level assertions can read it directly.
func draculaRankedContexts(t *testing.T, profile *ranking.Profile, req ranking.Request, titles ...string) ([]triage.Candidate, []FormatContext) {
	t.Helper()

	in := make([]triage.Candidate, 0, len(titles))
	for _, title := range titles {
		in = append(in, triage.Candidate{Release: &release.Release{Title: title}})
	}
	results, rejected := profile.ApplyWithRejected(req, in, rank.RankOptions{})
	if len(rejected) > 0 {
		t.Fatalf("premise lost: %d candidate(s) rejected: %v", len(rejected), rejected[0].Torrent.Rejections)
	}

	cands := make([]triage.Candidate, 0, len(results))
	top, topWithout := 0, 0
	for _, r := range results {
		cand := r.Candidate
		cand.Metadata = parser.FromResult(r.Torrent.Raw, r.Torrent.Data)
		cand.Group = cand.Metadata.ResolutionGroup()
		cand.Score = r.Torrent.Rank
		cands = append(cands, cand)
		top = max(top, cand.Score)
		topWithout = max(topWithout, scoreWithoutResolution(cand))
	}

	out := make([]FormatContext, 0, len(cands))
	for i, cand := range cands {
		out = append(out, newFormatContext(cand, i+1, len(cands), top, topWithout, "", "", "", capsSummaryLine(cand.Release), false, 0))
	}
	return cands, out
}

// draculaPlaylistLabels renders the name template through StreamNZB's own
// playlist aggregation, which computes TopScore/TopScoreWithoutResolution
// itself, and fails if any label differs from the helper-built context's.
func draculaPlaylistLabels(t *testing.T, nameTemplate string, cands []triage.Candidate, ctxs []FormatContext) []string {
	t.Helper()

	view := (&Server{}).playlistViewOf(
		&playlistResult{Candidates: cands},
		StreamSlotKey{},
		&auth.Stream{ResultNameTemplate: nameTemplate},
	)
	if len(view.Entries) != len(ctxs) {
		t.Fatalf("playlist has %d entries, want %d", len(view.Entries), len(ctxs))
	}
	labels := make([]string, len(view.Entries))
	for i, entry := range view.Entries {
		labels[i] = entry.DisplayTitle
		if labels[i] == "" {
			t.Fatalf("%s: production playlist label fell back to the release title", entry.Title)
		}
		if want := singleLineLabel(renderDraculaFormatter(t, nameTemplate, ctxs[i])); labels[i] != want {
			t.Errorf("%s: production aggregation renders %q, helper context renders %q", entry.Title, labels[i], want)
		}
	}
	return labels
}

func draculaContextTitles(ctxs []FormatContext) []string {
	out := make([]string, 0, len(ctxs))
	for _, ctx := range ctxs {
		out = append(out, ctx.ReleaseTitle)
	}
	return out
}

func draculaSubscript(n int) string {
	return strings.NewReplacer(
		"0", "₀", "1", "₁", "2", "₂", "3", "₃", "4", "₄",
		"5", "₅", "6", "₆", "7", "₇", "8", "₈", "9", "₉",
	).Replace(fmt.Sprint(n))
}
