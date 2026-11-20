package stremio

// Regression tests for the DraCuLa Jellyfin formatter (formatter-jellyfin.txt).
//
// StreamNZB's Jellyfin endpoint renders only the result NAME template, flattens
// it to one line and labels a media source with it. These tests render the
// formatter through the real pinned engine: direct FormatContext fixtures with
// exact expected labels, real ranking-pipeline pools (production profile and
// Define Library, real newFormatContext and playlistViewOf), a bounded seeded
// adversarial suite, and a self-test proving the label oracle can fail. The
// companion test in pkg/server/jellyfin (dracula_jellyfin_media_source_test.go)
// then sends the real playlist labels through StreamNZB's own PlaybackInfo
// handler.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/release"
	"streamnzb/pkg/search/parser"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const (
	// draculaJellyfinBudget is the formatter's own ceiling. StreamNZB would cut
	// a label at 160 runes (maxSourceLabelRunes) without warning; the formatter
	// must stay far below that on its own.
	draculaJellyfinBudget = 72

	// The production star expression, unchanged. The Jellyfin formatter must
	// contain it byte for byte.
	draculaJellyfinStarsFragment = `{{$filledStars := replace (stars 5 .TopScoreWithoutResolution .ScoreWithoutResolution) "☆" ""}}{{if $filledStars}} {{$filledStars}}{{end}}`
)

type draculaJellyfinFixtureFile struct {
	Cases []draculaJellyfinCase `json:"cases"`
}

type draculaJellyfinCase struct {
	Name    string          `json:"name"`
	Context json.RawMessage `json:"context"`
	Label   string          `json:"label"`
	// OutOfVocabulary marks contexts the parser cannot produce (oversized or
	// multi-line essential values). The label oracle then skips the checks that
	// model in-vocabulary rendering (intact head, whitespace); budget, newline,
	// separator, stars and claim checks still apply.
	OutOfVocabulary bool `json:"out_of_vocabulary,omitempty"`
}

// draculaJellyfinRoot is the repository root, derived from the fixture path
// exactly as the other real-pipeline subtests do.
func draculaJellyfinRoot() string {
	return filepath.Join(filepath.Dir(os.Getenv("DRACULA_FORMATTER_CASES_PATH")), "..", "..", "..")
}

// draculaJellyfinRender renders a name template and applies StreamNZB's own
// single-line flattening. A template that falls back to the built-in format
// (and therefore to the raw release title in a Jellyfin picker) fails the test.
func draculaJellyfinRender(t *testing.T, tpl string, ctx FormatContext) (raw, label string) {
	t.Helper()
	compiled, err := compileFormatTemplate(tpl)
	if err != nil {
		t.Fatalf("compile Jellyfin name template through StreamNZB: %v", err)
	}
	const fallback = "DRACULA_JELLYFIN_FALLBACK"
	raw = renderResultTemplate(compiled, ctx, fallback)
	if raw == fallback {
		t.Fatalf("Jellyfin name template fell back to StreamNZB's built-in format for %+v", ctx)
	}
	return raw, singleLineLabel(raw)
}

var draculaStarRun = regexp.MustCompile(`★+`)

// draculaLangSegment matches the language field: up to three upper-case codes,
// an optional measured mark, an optional overflow count and the DUAL claim, or
// DUAL alone. Claims are checked in this segment only, never in arbitrary label
// text such as a release group called DUALITY.
var draculaLangSegment = regexp.MustCompile(`^(?:[A-Z]{2}(?:/[A-Z]{2}){0,2}(✓)?(?: \+\d+)?(?: DUAL)?|DUAL)$`)

// draculaJellyfinGroupSegment is the label segment the release group renders as,
// derived independently of the template: line breaks become spaces, the value is
// trimmed, a blank or separator-only value renders nothing, and the rest is cut
// at 12 characters with an ellipsis.
func draculaJellyfinGroupSegment(group string) string {
	g := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(group))
	if strings.TrimSpace(strings.ReplaceAll(g, "·", "")) == "" {
		return ""
	}
	if r := []rune(g); len(r) > 12 {
		return string(r[:12]) + "…"
	}
	return g
}

// draculaJellyfinProblems is the label oracle: it returns every violated
// invariant for one rendered label. It re-derives the essential head from the
// context independently of the template, so a template regression cannot hide
// behind its own output.
func draculaJellyfinProblems(t *testing.T, ctx FormatContext, raw, label string, inVocabulary bool) []string {
	t.Helper()
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if strings.ContainsAny(raw, "\n\r") {
		add("raw name contains a newline")
	}
	if label == "" {
		add("empty label")
		return problems
	}
	if n := utf8.RuneCountInString(label); n > draculaJellyfinBudget {
		add("label has %d code points, budget %d", n, draculaJellyfinBudget)
	}
	if label != strings.TrimSpace(label) || (inVocabulary && strings.Contains(label, "  ")) {
		add("stray whitespace")
	}
	if strings.HasPrefix(label, "·") || strings.HasSuffix(label, "·") || strings.Contains(label, "· ·") || strings.Contains(label, " ·· ") {
		add("malformed separator")
	}
	if ctx.ReleaseTitle != "" && strings.Contains(label, ctx.ReleaseTitle) {
		add("label contains the raw release title")
	}
	if strings.Contains(label, "☆") {
		add("empty star glyph")
	}

	// Stars: exactly what the production fragment renders for this context.
	starsTpl, err := compileFormatTemplate(draculaJellyfinStarsFragment)
	if err != nil {
		t.Fatalf("compile production star fragment: %v", err)
	}
	wantStars := renderResultTemplate(starsTpl, ctx, "")
	if got := strings.Join(draculaStarRun.FindAllString(label, -1), ","); got != wantStars {
		add("stars %q, production renders %q", got, wantStars)
	}

	// Essential head, re-derived from the context.
	var head []string
	if ctx.Library {
		head = append(head, "LIB")
	}
	if (len(ctx.Seasons) > 0 || ctx.Complete) && ctx.Episode == 0 && len(ctx.Episodes) == 0 {
		head = append(head, "PACK")
	}
	if ctx.Resolution != "" {
		head = append(head, strings.NewReplacer("2160p", "4K", "1440p", "2K").Replace(ctx.Resolution))
	}
	if ctx.Quality != "" {
		head = append(head, strings.ReplaceAll(ctx.Quality, "BluRay REMUX", "REMUX"))
	}
	if len(ctx.HDR) > 0 {
		head = append(head, strings.Join(ctx.HDR[:min(len(ctx.HDR), 3)], "+"))
	}
	wantHead := "Unknown"
	if len(head) > 0 {
		wantHead = strings.Join(head, " ")
	}
	if wantStars != "" {
		wantHead += " " + wantStars
	}
	if inVocabulary && label != wantHead && !strings.HasPrefix(label, wantHead+" · ") {
		add("essential head %q is not intact at the start of the label", wantHead)
	}

	// Optional fields: each ` · `-separated segment is non-empty; claims need support.
	segments := strings.Split(label, " · ")
	for _, seg := range segments[1:] {
		if strings.Trim(seg, " ·…") == "" {
			add("meaningless segment %q", seg)
		}
		if m := regexp.MustCompile(`^(\d+)(?:\.(\d))? GB$`).FindStringSubmatch(seg); m != nil {
			gb, _ := strconv.ParseFloat(m[1]+"."+orZero(m[2]), 64)
			tolerance := 0.05 + 1e-9 // one decimal: nearest 0.1 GB
			if m[2] == "" {
				tolerance = 0.5 + 1e-9 // whole GB: nearest 1 GB, rounded once
			}
			if diff := gb - float64(ctx.Size)/1e9; diff > tolerance || diff < -tolerance {
				add("size segment %q does not match %d bytes", seg, ctx.Size)
			}
		}
	}
	// The release group is the last optional field. Attribute a trailing segment that is
	// exactly the rendered group to the group, so a group called DUAL (or EN✓) is not
	// mistaken for a language claim. A claim and a group that are textually identical
	// and the only optional segment cannot be told apart, which is inherent to the label.
	claimSegments := segments[1:]
	if group := draculaJellyfinGroupSegment(ctx.Group); group != "" && len(claimSegments) > 0 && claimSegments[len(claimSegments)-1] == group {
		claimSegments = claimSegments[:len(claimSegments)-1]
	}
	for _, seg := range claimSegments {
		m := draculaLangSegment.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		if m[1] != "" {
			verified := ctx.LanguageSource == "measured" && len(withoutStrings(ctx.Probed.AudioLanguages, ctx.Languages)) == 0 && len(ctx.Languages) > 0
			if !verified {
				add("✓ asserted without measured, fully probed languages")
			}
		}
		if (seg == "DUAL" || strings.HasSuffix(seg, " DUAL")) && !ctx.DualAudio {
			add("DUAL without the release's dual-audio flag")
		}
	}
	if hasLib := label == "LIB" || strings.HasPrefix(label, "LIB "); hasLib != ctx.Library {
		add("LIB marker %v, Library %v", hasLib, ctx.Library)
	}
	return problems
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func draculaJellyfinRequireClean(t *testing.T, ctx FormatContext, raw, label, name string, inVocabulary bool) {
	t.Helper()
	if problems := draculaJellyfinProblems(t, ctx, raw, label, inVocabulary); len(problems) > 0 {
		t.Errorf("%s: %s\nlabel: %q", name, strings.Join(problems, "; "), label)
	}
}

// loadDraculaJellyfinFormatter loads the formatter under test (source JSON or
// published share code) and the production formatter it must stay consistent with.
func loadDraculaJellyfinFormatter(t *testing.T) (jellyfin, production draculaFormatterPayload) {
	t.Helper()
	path := os.Getenv("DRACULA_FORMATTER_PATH")
	if path == "" {
		t.Fatal("DRACULA_FORMATTER_PATH is not set")
	}
	jellyfin = loadDraculaFormatterFile(t, path, draculaDescriptionEmpty)
	production = loadDraculaFormatterFile(t, filepath.Join(draculaJellyfinRoot(), "formatter.txt"), draculaDescriptionRequired)
	return jellyfin, production
}

func TestDraculaJellyfinFormatter(t *testing.T) {
	jellyfin, production := loadDraculaJellyfinFormatter(t)
	tpl := jellyfin.ResultNameTemplate

	t.Run("static contract", func(t *testing.T) {
		if jellyfin.Name != "DraCuLa Jellyfin" {
			t.Errorf("formatter name %q, want %q", jellyfin.Name, "DraCuLa Jellyfin")
		}
		if !strings.Contains(tpl, draculaJellyfinStarsFragment) {
			t.Error("Jellyfin name template no longer contains the production star fragment unchanged")
		}
		if !strings.Contains(production.ResultNameTemplate, draculaJellyfinStarsFragment) {
			t.Error("production name template no longer contains the shared star fragment; update both formatters together")
		}
		for _, forbidden := range []string{".MatchedRules", ".Seadex"} {
			if strings.Contains(tpl, forbidden) {
				t.Errorf("Jellyfin formatter must not duplicate tier/classification logic (%s)", forbidden)
			}
		}
	})

	t.Run("direct fixtures (exact labels)", func(t *testing.T) {
		data, err := os.ReadFile(os.Getenv("DRACULA_FORMATTER_CASES_PATH"))
		if err != nil {
			t.Fatal(err)
		}
		var fixtures draculaJellyfinFixtureFile
		if err := json.Unmarshal(data, &fixtures); err != nil {
			t.Fatal(err)
		}
		if len(fixtures.Cases) < 30 {
			t.Fatalf("only %d Jellyfin fixtures", len(fixtures.Cases))
		}
		seen := map[string]bool{}
		longest := 0
		for _, fx := range fixtures.Cases {
			if seen[fx.Name] {
				t.Fatalf("duplicate fixture name %q", fx.Name)
			}
			seen[fx.Name] = true
			var ctx FormatContext
			if err := json.Unmarshal(fx.Context, &ctx); err != nil {
				t.Fatalf("%s: decode context: %v", fx.Name, err)
			}
			raw, label := draculaJellyfinRender(t, tpl, ctx)
			if label != fx.Label {
				t.Errorf("%s:\n got  %q\n want %q", fx.Name, label, fx.Label)
			}
			draculaJellyfinRequireClean(t, ctx, raw, label, fx.Name, !fx.OutOfVocabulary)
			if _, again := draculaJellyfinRender(t, tpl, ctx); again != label {
				t.Errorf("%s: rendering is not deterministic: %q vs %q", fx.Name, label, again)
			}
			longest = max(longest, utf8.RuneCountInString(fx.Label))
		}
		// The limit is inclusive: a fixture must sit exactly on it, so a `<=` that
		// became `<` (wasting the last code point) cannot pass the exact fixtures.
		if longest != draculaJellyfinBudget {
			t.Errorf("the longest exact-label fixture has %d code points; one must have exactly %d", longest, draculaJellyfinBudget)
		}
	})

	t.Run("oracle detects broken labels", func(t *testing.T) {
		ctx := FormatContext{Resolution: "1080p", Quality: "WEB-DL", Size: 2_800_000_000, TopScoreWithoutResolution: 100, ScoreWithoutResolution: 100}
		if _, label := draculaJellyfinRender(t, tpl, ctx); label != "1080p WEB-DL ★★★★★ · 2.8 GB" {
			t.Fatalf("oracle baseline changed: %q", label)
		}
		cases := []struct {
			name, label, raw, want string
			ctx                    FormatContext
		}{
			{"empty", "", "", "empty label", ctx},
			{"newline", "1080p WEB-DL ★★★★★", "1080p WEB-DL ★★★★★\n2.8 GB", "newline", ctx},
			{"over budget", "1080p WEB-DL ★★★★★ · " + strings.Repeat("x", 60), "", "budget", ctx},
			{"leading separator", "· 1080p WEB-DL ★★★★★", "", "separator", ctx},
			{"trailing separator", "1080p WEB-DL ★★★★★ ·", "", "separator", ctx},
			{"duplicate separator", "1080p WEB-DL ★★★★★ · · 2.8 GB", "", "separator", ctx},
			{"raw title fallback", "Dune.Part.Two.2024.1080p.WEB-DL-GRP", "", "raw release title", func() FormatContext {
				c := ctx
				c.ReleaseTitle = "Dune.Part.Two.2024.1080p.WEB-DL-GRP"
				return c
			}()},
			{"empty star glyph", "1080p WEB-DL ★★★☆☆", "", "empty star glyph", ctx},
			{"wrong stars", "1080p WEB-DL ★★★ · 2.8 GB", "", "production renders", ctx},
			{"stars missing", "1080p WEB-DL · 2.8 GB", "", "production renders", ctx},
			{"truncated quality", "1080p WEB ★★★★★ · 2.8 GB", "", "essential head", ctx},
			{"head reordered", "WEB-DL 1080p ★★★★★ · 2.8 GB", "", "essential head", ctx},
			{"wrong size", "1080p WEB-DL ★★★★★ · 9.9 GB", "", "does not match", ctx},
			{"unverified check mark", "1080p WEB-DL ★★★★★ · EN✓", "", "✓ asserted", ctx},
			{"unflagged DUAL", "1080p WEB-DL ★★★★★ · JA/EN DUAL", "", "DUAL without", ctx},
			{"phantom LIB", "LIB 1080p WEB-DL ★★★★★ · 2.8 GB", "", "essential head", ctx},
			{"unsupported DUAL before a DUAL group", "1080p WEB-DL ★★★★★ · DUAL · DUAL", "", "DUAL without", func() FormatContext {
				c := ctx
				c.Group = "DUAL"
				return c
			}()},
			{"dangling ellipsis group", "1080p WEB-DL ★★★★★ · …", "", "meaningless segment", ctx},
			{"separator-only group", "1080p WEB-DL ★★★★★ · ·", "", "meaningless segment", ctx},
		}
		// A legitimate label must not be rejected because its group resembles a claim,
		// and a genuine dual-audio flag must still be accepted.
		for _, tc := range []struct {
			name  string
			ctx   FormatContext
			label string
		}{
			{"group DUALITY, flag false", FormatContext{Resolution: "1080p", Group: "DUALITY"}, "1080p · DUALITY"},
			{"group DUAL, flag false", FormatContext{Resolution: "1080p", Group: "DUAL"}, "1080p · DUAL"},
			{"dual flag true", FormatContext{Resolution: "1080p", DualAudio: true, Group: "NTb"}, "1080p · DUAL · NTb"},
			{"dual flag true and group DUAL", FormatContext{Resolution: "1080p", DualAudio: true, Group: "DUAL"}, "1080p · DUAL · DUAL"},
			{"group EN✓, flag false", FormatContext{Resolution: "1080p", Group: "EN✓"}, "1080p · EN✓"},
		} {
			raw, label := draculaJellyfinRender(t, tpl, tc.ctx)
			if label != tc.label {
				t.Fatalf("%s: self-test baseline changed: %q, want %q", tc.name, label, tc.label)
			}
			if problems := draculaJellyfinProblems(t, tc.ctx, raw, label, true); len(problems) > 0 {
				t.Errorf("%s: oracle rejected a valid label: %v", tc.name, problems)
			}
		}
		// A deliberately defective formatter that always claims DUAL must still be caught,
		// with and without a group that is literally named DUAL.
		defective := strings.Replace(tpl, "{{- if .DualAudio -}}", "{{- if true -}}", 1)
		if defective == tpl {
			t.Fatal("self-test premise lost: the DUAL claim condition is no longer `{{- if .DualAudio -}}`")
		}
		for _, tc := range []struct {
			name  string
			group string
			label string
		}{
			{"group NTb", "NTb", "1080p · DUAL · NTb"},
			{"group DUAL", "DUAL", "1080p · DUAL · DUAL"},
			{"no group", "", "1080p · DUAL"},
		} {
			ctx := FormatContext{Resolution: "1080p", Group: tc.group}
			raw, label := draculaJellyfinRender(t, defective, ctx)
			if label != tc.label {
				t.Fatalf("defective formatter, %s: rendered %q, want %q", tc.name, label, tc.label)
			}
			if problems := strings.Join(draculaJellyfinProblems(t, ctx, raw, label, true), "; "); !strings.Contains(problems, "DUAL without") {
				t.Errorf("defective formatter, %s: oracle reported %q, want an unsupported-DUAL problem", tc.name, problems)
			}
		}
		for _, tc := range cases {
			problems := strings.Join(draculaJellyfinProblems(t, tc.ctx, tc.raw, tc.label, true), "; ")
			if !strings.Contains(problems, tc.want) {
				t.Errorf("%s: oracle reported %q, want a problem containing %q", tc.name, problems, tc.want)
			}
		}
	})

	t.Run("bounded adversarial suite", func(t *testing.T) {
		rng := rand.New(rand.NewSource(20261120))
		pick := func(xs []string) string { return xs[rng.Intn(len(xs))] }
		list := func(xs []string, n int) stringList {
			out := make(stringList, 0, n)
			for i := rng.Intn(n + 1); i > 0; i-- {
				out = append(out, pick(xs))
			}
			return out
		}
		// Parser vocabularies (jhin v0.8.0 rank/attrs.go): the formatter's head is
		// bounded by them. Out-of-vocabulary junk is exercised separately below.
		resolutions := []string{"", "", "2160p", "1440p", "1080p", "720p", "576p", "480p"}
		qualities := []string{"", "WEB", "WEB-DL", "WEBMux", "BluRay", "DVD", "HDTV", "HDTVRip", "VHS", "BluRay REMUX", "WEBRip", "WEB-DLRip", "UHDRip", "HDRip", "DVDRip", "BDRip", "BRRip", "TeleSync", "WORKPRINT"}
		hdrs := []string{"DV", "HDR", "HDR10+", "HLG", "SDR"}
		audios := []string{"AAC", "Atmos", "Dolby Digital", "Dolby Digital Plus", "DTS Lossy", "DTS Lossless", "DTS:X", "DTS-ES", "FLAC", "OPUS", "PCM", "MP3", "TrueHD", "HQ Clean Audio"}
		channels := []string{"2.0", "5.1", "6.1", "7.1", "stereo", "mono"}
		codecs := []string{"", "AVC", "HEVC", "AV1", "VC-1", "MPEG-2", "XviD"}
		bits := []string{"", "8bit", "10bit", "12bit"}
		langs := []string{"en", "ja", "fr", "de", "es", "it", "pt", "ru", "ko", "zh", "nl", "pl", "sv", "tr", "ar", "he", "hi", "th", "id", "vi"}
		sources := []string{"", "measured", "reported", "inferred"}
		groups := []string{"", "NTb", "FLUX", "FraMeSToR", "世界字幕组", "-", "DUAL", "EN✓", "·", " · · ", strings.Repeat(" ", 13), "  Team Foo  ", "A·B", strings.Repeat("LongGroup", 3), strings.Repeat("LongGroup", 40)}
		sizes := []int64{0, 1, 450_000_000, 999_999_999, 1_000_000_000, 2_849_999_999, 9_949_999_999, 9_950_000_000, 10_000_000_000, 68_400_000_000, 999_999_999_999, 123_456_789_012_345}
		var longest int
		for i := 0; i < 4000; i++ {
			ctx := FormatContext{
				Resolution: pick(resolutions), Quality: pick(qualities), HDR: list(hdrs, 5), Audio: list(audios, 6), Channels: list(channels, 3),
				Languages: list(langs, 20), LanguageSource: pick(sources), DualAudio: rng.Intn(3) == 0,
				Codec: pick(codecs), BitDepth: pick(bits), Group: pick(groups), Size: sizes[rng.Intn(len(sizes))],
				Library: rng.Intn(4) == 0, Complete: rng.Intn(6) == 0, Episode: rng.Intn(3),
				TopScoreWithoutResolution: rng.Intn(5000) - 500, ScoreWithoutResolution: rng.Intn(5000) - 1500,
			}
			if rng.Intn(3) == 0 {
				ctx.Seasons = intList{1 + rng.Intn(3)}
			}
			if ctx.Episode > 0 && rng.Intn(2) == 0 {
				ctx.Episodes = intList{ctx.Episode}
			}
			ctx.Probed.AudioLanguages = list(langs[:6], 4)
			raw, label := draculaJellyfinRender(t, tpl, ctx)
			draculaJellyfinRequireClean(t, ctx, raw, label, fmt.Sprintf("adversarial #%d", i), true)
			if again := singleLineLabel(mustRender(t, tpl, ctx)); again != label {
				t.Fatalf("adversarial #%d is not deterministic", i)
			}
			longest = max(longest, utf8.RuneCountInString(label))
			if t.Failed() {
				t.Fatalf("stopping at first failing adversarial context: %+v", ctx)
			}
		}
		t.Logf("4000 seeded adversarial contexts; longest label %d of %d code points", longest, draculaJellyfinBudget)

		// Outside the parser vocabulary the essential head is length-capped, so the
		// 72 budget still holds: oversized resolution, quality and HDR values, a
		// 400-character group, 1000 languages, line breaks in every free-text field.
		junk := FormatContext{Resolution: strings.Repeat("9", 40) + "p", Quality: strings.Repeat("Q", 90), Group: strings.Repeat("G\n", 200), Library: true,
			Seasons: intList{1}, Languages: slices.Repeat(stringList{"en"}, 1000), Audio: slices.Repeat(stringList{"Atmos"}, 50),
			HDR: slices.Repeat(stringList{strings.Repeat("D", 30)}, 50), TopScoreWithoutResolution: 100, ScoreWithoutResolution: 100,
			Codec: "C\r\nD", BitDepth: "8\nbit", Size: 123_456_789_012_345}
		raw, label := draculaJellyfinRender(t, tpl, junk)
		if strings.ContainsAny(raw, "\r\n") || label == "" || utf8.RuneCountInString(label) > draculaJellyfinBudget {
			t.Errorf("out-of-vocabulary context produced %d code points (budget %d): %q", utf8.RuneCountInString(label), draculaJellyfinBudget, label)
		}
	})

	t.Run("real ranking pipeline pools", func(t *testing.T) {
		draculaJellyfinPools(t, tpl, production.ResultNameTemplate)
	})
}

func mustRender(t *testing.T, tpl string, ctx FormatContext) string {
	t.Helper()
	raw, _ := draculaJellyfinRender(t, tpl, ctx)
	return raw
}

// ---- real pipeline pools ------------------------------------------------

type draculaJellyfinSpec struct {
	title   string
	sizeGB  float64
	langs   []string
	library bool
	probed  *release.MediaCaps
	note    string
}

type draculaJellyfinPool struct {
	name  string
	req   ranking.Request
	specs []draculaJellyfinSpec
}

// draculaJellyfinRank ranks specs through the published profile and builds a
// FormatContext per survivor the way facade.go does, attaching probe data to
// Library releases afterwards (a stand-in for a real ffprobe run).
func draculaJellyfinRank(t *testing.T, profile *ranking.Profile, pool draculaJellyfinPool) ([]triage.Candidate, []FormatContext, []draculaJellyfinSpec) {
	t.Helper()
	in := make([]triage.Candidate, 0, len(pool.specs))
	for i, s := range pool.specs {
		in = append(in, triage.Candidate{Release: &release.Release{
			Title: s.title, Size: int64(s.sizeGB * 1e9), Indexer: "TestIndexer", Languages: s.langs, IsLibrary: s.library,
			PubDate: time.Now().AddDate(0, 0, -30).Format(time.RFC1123Z), DetailsURL: fmt.Sprintf("https://example.invalid/%d", i),
		}})
	}
	results, rejected := profile.ApplyWithRejected(pool.req, in, rank.RankOptions{})
	if len(rejected) > 0 {
		t.Fatalf("%s: premise lost, %d candidate(s) rejected: %v", pool.name, len(rejected), rejected[0].Torrent.Rejections)
	}
	var cands []triage.Candidate
	var specs []draculaJellyfinSpec
	top, topWithout := 0, 0
	for _, r := range results {
		cand := r.Candidate
		cand.Metadata = parser.FromResult(r.Torrent.Raw, r.Torrent.Data)
		cand.Group = cand.Metadata.ResolutionGroup()
		cand.Score = r.Torrent.Rank
		idx := slices.IndexFunc(pool.specs, func(s draculaJellyfinSpec) bool {
			return cand.Release.Title == s.title && cand.Release.IsLibrary == s.library
		})
		if idx < 0 {
			t.Fatalf("%s: ranked release %q matches no spec", pool.name, cand.Release.Title)
		}
		cand.Verdict.Probed = pool.specs[idx].probed
		specs = append(specs, pool.specs[idx])
		cands = append(cands, cand)
		top, topWithout = max(top, cand.Score), max(topWithout, scoreWithoutResolution(cand))
	}
	ctxs := make([]FormatContext, 0, len(cands))
	for i, cand := range cands {
		ctxs = append(ctxs, newFormatContext(cand, i+1, len(cands), top, topWithout, "", "", pool.req.Title, capsSummaryLine(cand.Release), false, 0))
	}
	return cands, ctxs, specs
}

func draculaJellyfinPools(t *testing.T, jellyfinTpl, productionTpl string) {
	profile := draculaProductionProfile(t, draculaJellyfinRoot())

	hdrProbe := &release.MediaCaps{VideoCodec: "hevc", AudioCodec: "truehd", Width: 3840, Height: 2160, BitDepth: 10, HDR: "HDR10", DolbyVision: true,
		TracksProbed: true, AudioStreams: 2, AudioLanguages: []string{"en", "fr"}, DurationSeconds: 9960}
	sdrProbe := &release.MediaCaps{VideoCodec: "h264", AudioCodec: "eac3", Width: 1920, Height: 1080, BitDepth: 8,
		TracksProbed: true, AudioStreams: 1, AudioLanguages: []string{"en"}, DurationSeconds: 3000}
	jaProbe := &release.MediaCaps{VideoCodec: "hevc", AudioCodec: "flac", Width: 1920, Height: 1080, BitDepth: 10,
		TracksProbed: true, AudioStreams: 1, AudioLanguages: []string{"ja"}, DurationSeconds: 1440}
	en := []string{"en"}

	pools := []draculaJellyfinPool{
		{"Movie", ranking.Request{Kind: ranking.KindMovie, Title: "Dune Part Two"}, []draculaJellyfinSpec{
			{title: "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.Atmos.7.1-FraMeSToR", sizeGB: 68.4, langs: en, note: "4K REMUX DV/HDR"},
			{title: "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.Atmos.7.1-FraMeSToR", sizeGB: 68.4, langs: []string{"en", "fr"}, library: true, probed: hdrProbe, note: "Library 4K REMUX"},
			{title: "Dune.Part.Two.2024.2160p.WEB-DL.x265.10bit.HDR10.DDP5.1-GRP", sizeGB: 15.2, langs: en, note: "4K HDR10 encode"},
			{title: "Dune.Part.Two.2024.1080p.BluRay.x264.DTS-HD.MA.5.1-ExampleGroup", sizeGB: 14.3, langs: en, note: "1080p BluRay SDR"},
			{title: "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 6.2, langs: en, note: "1080p WEB SDR"},
			{title: "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 6.2, langs: en, library: true, probed: sdrProbe, note: "Library 1080p WEB"},
			{title: "Dune.Part.Two.2024.1080p.WEBRip.x265.10bit.AAC.2.0-ExtremelyLongReleaseGroupNameTeam", sizeGB: 2.1, langs: en, note: "compressed encode, long group"},
			{title: "Dune.Part.Two.2024.1080p.BluRay.MULTi.TrueHD.Atmos.7.1.x265-GRP", sizeGB: 31.0, langs: []string{"en", "fr", "de", "es", "it", "ja"}, note: "MULTi, 6 languages"},
			{title: "Dune.Part.Two.2024.WEB-DL.German.DL.DDP5.1-GRP", sizeGB: 8.0, langs: []string{"de", "en"}, note: "unknown resolution"},
			{title: "Dune.Part.Two.2024.1080p-GRP", sizeGB: 0.45, note: "sparse metadata"},
		}},
		{"TV", ranking.Request{Kind: ranking.KindSeries, Title: "The Last of Us", Season: 1, Episode: 3}, []draculaJellyfinSpec{
			{title: "The.Last.of.Us.S01E03.2160p.MAX.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", sizeGB: 9.8, langs: en, note: "episode 4K WEB DV"},
			{title: "The.Last.of.Us.S01E03.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 3.4, langs: en, note: "episode 1080p WEB"},
			{title: "The.Last.of.Us.S01E03.720p.HDTV.x264-SYNCOPY", sizeGB: 0.9, note: "episode 720p HDTV"},
		}},
		{"Season pack", ranking.Request{Kind: ranking.KindSeries, Title: "The Last of Us", Season: 1}, []draculaJellyfinSpec{
			{title: "The.Last.of.Us.S01.2160p.BluRay.REMUX.HEVC.TrueHD.Atmos.7.1-GRP", sizeGB: 190, langs: en, note: "pack REMUX"},
			{title: "The.Last.of.Us.S01.COMPLETE.1080p.BluRay.x265.10bit.DTS-HD.MA.5.1-GRP", sizeGB: 42, langs: en, note: "pack x265"},
			{title: "The.Last.of.Us.S01.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 42, langs: en, library: true, probed: sdrProbe, note: "Library pack"},
		}},
		{"Anime", ranking.Request{Kind: ranking.KindAnimeShow, IsAnime: true, Title: "Frieren Beyond Journeys End", Season: 1, Episode: 1}, []draculaJellyfinSpec{
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG", sizeGB: 1.4, langs: []string{"ja"}, note: "Anime WEB"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.Dual-Audio.FLAC2.0.x265-FLE", sizeGB: 2.8, langs: []string{"ja", "en"}, note: "Anime BluRay dual audio"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.REMUX.AVC.FLAC2.0.Dual.Audio-ZR", sizeGB: 12.5, langs: []string{"ja", "en"}, note: "Anime BluRay REMUX"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.Multi-Audio.FLAC2.0.x265.10bit-GRP", sizeGB: 3.1, langs: []string{"ja", "en", "de", "fr", "es"}, note: "Anime multi-audio"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.x265.10bit.FLAC2.0-LIB", sizeGB: 2.5, langs: []string{"ja"}, library: true, probed: jaProbe, note: "Anime Library"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.WEB.H264-SubsPlease", sizeGB: 0.9, langs: []string{"ja"}, note: "Anime WEB untiered"},
		}},
	}

	entries := map[string][]map[string]any{}
	for _, pool := range pools {
		t.Run(pool.name, func(t *testing.T) {
			cands, ctxs, specs := draculaJellyfinRank(t, profile, pool)
			// Labels come from StreamNZB's own playlist aggregation, the path the
			// Jellyfin endpoint reads; the helper also checks them against a
			// helper-built context and fails on an empty (fallback) label.
			jf := draculaPlaylistLabels(t, jellyfinTpl, cands, ctxs)
			prod := draculaPlaylistLabels(t, productionTpl, cands, ctxs)

			distinctJF, distinctProd := map[string]bool{}, map[string]bool{}
			for i, ctx := range ctxs {
				spec := specs[i]
				raw, _ := draculaJellyfinRender(t, jellyfinTpl, ctx)
				draculaJellyfinRequireClean(t, ctx, raw, jf[i], spec.note, true)
				distinctJF[jf[i]], distinctProd[prod[i]] = true, true

				if got, want := strings.Join(draculaStarRun.FindAllString(jf[i], -1), ","), strings.Join(draculaStarRun.FindAllString(prod[i], -1), ","); got != want {
					t.Errorf("%s: stars %q differ from the production label's %q", spec.note, got, want)
				}
				if strings.HasPrefix(jf[i], "LIB ") != spec.library {
					t.Errorf("%s: LIB marker does not follow Library (%v): %q", spec.note, spec.library, jf[i])
				}
				if strings.Contains(jf[i], "✓") {
					if !spec.library || spec.probed == nil {
						t.Errorf("%s: ✓ on a release that was never probed: %q", spec.note, jf[i])
					}
				} else if spec.library && spec.probed != nil && len(ctx.Languages) > 0 && !strings.Contains(jf[i], "DUAL") && ctx.LanguageSource == "measured" {
					if len(withoutStrings(ctx.Probed.AudioLanguages, ctx.Languages)) == 0 {
						t.Errorf("%s: measured languages not marked ✓: %q", spec.note, jf[i])
					}
				}
				t.Logf("%-26s prod=%q jellyfin=%q (%d)", spec.note, prod[i], jf[i], utf8.RuneCountInString(jf[i]))
				entries[pool.name] = append(entries[pool.name], map[string]any{"Index": i, "Title": ctx.ReleaseTitle, "DisplayTitle": jf[i], "Size": ctx.Size})
			}
			// Better differentiation, not universal uniqueness.
			if len(distinctJF) < len(distinctProd) {
				t.Errorf("Jellyfin labels (%d distinct) are less differentiated than production (%d) over %d releases", len(distinctJF), len(distinctProd), len(ctxs))
			}
			if len(distinctJF) != len(ctxs) {
				t.Errorf("expected every release in this pool to get its own Jellyfin label, got %d distinct for %d releases", len(distinctJF), len(ctxs))
			}
			if (pool.name == "Movie" || pool.name == "Anime") && len(distinctProd) >= len(ctxs) {
				t.Errorf("premise lost: production no longer collides in the %s pool", pool.name)
			}
		})
	}

	// Hand the real labels to the pkg/server/jellyfin handler test.
	if out := os.Getenv("DRACULA_JELLYFIN_ENTRIES"); out != "" {
		data, err := json.MarshalIndent(entries, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
	}
}

// TestDraculaFormatterDescriptionPolicy proves the empty-description allowance
// is exclusive to the Jellyfin formatter: emptying the production or debug
// description (source or published) still fails validation, and the Jellyfin
// formatter may not grow a copied description.
func TestDraculaFormatterDescriptionPolicy(t *testing.T) {
	root := draculaJellyfinRoot()
	read := func(name string) draculaFormatterPayload {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		p, err := parseDraculaFormatter(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return p
	}

	production := read("formatter.txt")
	for _, name := range []string{
		"formatter.txt", "formatter-debug.txt",
		"tests/streamnzb_compat/formatter.source.json", "tests/streamnzb_compat/formatter-debug.source.json",
	} {
		p := read(name)
		if err := validateDraculaFormatterPayload(p, draculaDescriptionRequired); err != nil {
			t.Errorf("%s no longer validates: %v", name, err)
		}
		if err := validateDraculaFormatterPayload(p, draculaDescriptionEmpty); err == nil {
			t.Errorf("%s: the Jellyfin empty-description policy must not accept a formatter with a description", name)
		}
		for _, emptied := range []string{"", "  \n\t "} {
			broken := p
			broken.ResultDescriptionTemplate = emptied
			if err := validateDraculaFormatterPayload(broken, draculaDescriptionRequired); err == nil {
				t.Errorf("%s with description %q was accepted", name, emptied)
			}
		}
	}

	for _, name := range []string{"formatter-jellyfin.txt", "tests/streamnzb_compat/formatter-jellyfin.source.json"} {
		p := read(name)
		if err := validateDraculaFormatterPayload(p, draculaDescriptionEmpty); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if err := validateDraculaFormatterPayload(p, draculaDescriptionRequired); err == nil {
			t.Errorf("%s: required policy accepted an empty description", name)
		}
		copied := p
		copied.ResultDescriptionTemplate = production.ResultDescriptionTemplate
		if err := validateDraculaFormatterPayload(copied, draculaDescriptionEmpty); err == nil {
			t.Errorf("%s: a copied production description was accepted", name)
		}
	}
}
