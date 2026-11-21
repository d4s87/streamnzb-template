package stremio

// Regression tests for the DraCuLa Jellyfin formatter (formatter-jellyfin.txt).
//
// StreamNZB's Jellyfin endpoint renders only the result NAME template, flattens
// it to one line and labels a media source with it. These tests render the
// formatter through the real pinned engine: exact expected labels for direct
// FormatContext fixtures, structural invariants over those fixtures and a seeded
// random sweep, and real ranking-pipeline pools (production profile and Define
// Library, real newFormatContext and playlistViewOf). The companion test in
// pkg/server/jellyfin (dracula_jellyfin_media_source_test.go) sends the pool
// labels through StreamNZB's own PlaybackInfo handler.
//
// It reuses the helpers of dracula_formatter_compat_test.go, which
// scripts/test_formatter.sh always injects with it.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/auth"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/parser"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

const (
	// The formatter's own ceiling; StreamNZB itself would cut at 160 runes.
	draculaJellyfinBudget = 72

	// The production star expression, which the Jellyfin formatter must contain unchanged.
	draculaJellyfinStars = `{{$filledStars := replace (stars 5 .TopScoreWithoutResolution .ScoreWithoutResolution) "☆" ""}}{{if $filledStars}} {{$filledStars}}{{end}}`
)

// draculaReadFormatterFile reads a formatter source JSON or published share code.
func draculaReadFormatterFile(t *testing.T, path string) draculaFormatterPayload {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), draculaFormatterPrefix) {
		if data, err = decodeDraculaShareCode(data, draculaFormatterPrefix); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	var p draculaFormatterPayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return p
}

// draculaJellyfinLabel renders a name template and flattens it as StreamNZB does
// for a Jellyfin media-source label. A fallback to the built-in format fails the test.
func draculaJellyfinLabel(t *testing.T, tpl string, ctx FormatContext) (raw, label string) {
	t.Helper()
	raw = renderDraculaFormatter(t, tpl, ctx)
	return raw, singleLineLabel(raw)
}

// draculaJellyfinInvariants returns what must hold for every rendering, whatever
// its exact content: non-empty, within the budget, one line, well-formed
// separators, production stars and deterministic output.
func draculaJellyfinInvariants(t *testing.T, tpl string, ctx FormatContext) []string {
	t.Helper()
	var problems []string
	raw, label := draculaJellyfinLabel(t, tpl, ctx)

	if strings.ContainsAny(raw, "\n\r\u2028\u2029") {
		problems = append(problems, "raw name contains a line break")
	}
	if label == "" {
		return append(problems, "empty label")
	}
	if n := utf8.RuneCountInString(label); n > draculaJellyfinBudget {
		problems = append(problems, fmt.Sprintf("%d code points, budget %d", n, draculaJellyfinBudget))
	}
	for _, seg := range strings.Split(label, " · ") {
		if strings.TrimSpace(seg) == "" || strings.Contains(seg, "·") {
			problems = append(problems, fmt.Sprintf("malformed separator around %q", seg))
		}
	}

	// The stars end the essential head, so they are checked there: a group or other
	// optional field containing a star glyph can neither satisfy nor break this.
	head, _, _ := strings.Cut(label, " · ")
	starsTpl, err := compileFormatTemplate(draculaJellyfinStars)
	if err != nil {
		t.Fatal(err)
	}
	stars := renderResultTemplate(starsTpl, ctx, "")
	if strings.Contains(head, "☆") {
		problems = append(problems, "empty star glyph")
	}
	// The measured-language mark belongs to the language field only: never in the head,
	// and only for measured, fully probed languages (a group cannot carry it).
	if strings.Contains(head, "✓") {
		problems = append(problems, "measured mark in the essential head")
	}
	if verified := ctx.LanguageSource == "measured" && len(ctx.Languages) > 0 && len(withoutStrings(ctx.Probed.AudioLanguages, ctx.Languages)) == 0; !verified && strings.Contains(label, "✓") {
		problems = append(problems, "measured mark without measured, fully probed languages")
	}
	if stars != "" && !strings.HasSuffix(head, " "+stars) {
		problems = append(problems, fmt.Sprintf("head %q does not end with the production stars %q", head, stars))
	} else if stars == "" && strings.Contains(head, "★") {
		problems = append(problems, "stars rendered where production renders none")
	}

	if _, again := draculaJellyfinLabel(t, tpl, ctx); again != label {
		problems = append(problems, fmt.Sprintf("not deterministic: %q then %q", label, again))
	}
	return problems
}

func TestDraculaJellyfinFormatter(t *testing.T) {
	root := filepath.Join(filepath.Dir(os.Getenv("DRACULA_FORMATTER_CASES_PATH")), "..", "..", "..")
	jellyfin := draculaReadFormatterFile(t, os.Getenv("DRACULA_FORMATTER_PATH"))
	tpl := jellyfin.ResultNameTemplate
	production := draculaReadFormatterFile(t, filepath.Join(root, "formatter.txt"))

	t.Run("static contract", func(t *testing.T) {
		if jellyfin.StreamNZBFormatProfile != 1 || jellyfin.Name != "DraCuLa Jellyfin" {
			t.Errorf("schema %d, name %q", jellyfin.StreamNZBFormatProfile, jellyfin.Name)
		}
		// Only the name is rendered for Jellyfin; the empty description falls back to the engine's.
		if jellyfin.ResultDescriptionTemplate != "" {
			t.Error("the Jellyfin formatter must have an empty description")
		}
		if err := ValidateResultTemplates(tpl, jellyfin.ResultDescriptionTemplate); err != nil {
			t.Errorf("StreamNZB rejected the formatter: %v", err)
		}
		for name, p := range map[string]draculaFormatterPayload{
			"Jellyfin":   jellyfin,
			"production": production,
		} {
			if !strings.Contains(p.ResultNameTemplate, draculaJellyfinStars) {
				t.Errorf("%s name template no longer contains the shared star fragment", name)
			}
		}
		// The empty description is allowed for this formatter only.
		for _, name := range []string{"formatter.txt", "formatter-debug.txt"} {
			if strings.TrimSpace(draculaReadFormatterFile(t, filepath.Join(root, name)).ResultDescriptionTemplate) == "" {
				t.Errorf("%s must keep a description", name)
			}
		}
	})

	data, err := os.ReadFile(os.Getenv("DRACULA_FORMATTER_CASES_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Name    string          `json:"name"`
			Context json.RawMessage `json:"context"`
			Label   string          `json:"label"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures.Cases) == 0 {
		t.Fatalf("decode Jellyfin fixtures: %v (%d cases)", err, len(fixtures.Cases))
	}

	t.Run("exact labels", func(t *testing.T) {
		for _, c := range fixtures.Cases {
			var ctx FormatContext
			if err := json.Unmarshal(c.Context, &ctx); err != nil {
				t.Fatalf("%s: %v", c.Name, err)
			}
			if _, got := draculaJellyfinLabel(t, tpl, ctx); got != c.Label {
				t.Errorf("%s:\n got  %q\n want %q", c.Name, got, c.Label)
			}
			if problems := draculaJellyfinInvariants(t, tpl, ctx); len(problems) > 0 {
				t.Errorf("%s: %s", c.Name, strings.Join(problems, "; "))
			}
		}
	})

	// Random contexts, including out-of-vocabulary and hostile values, against the
	// invariants only: the exact content is covered by the fixtures above.
	t.Run("seeded invariants", func(t *testing.T) {
		r := rand.New(rand.NewSource(1))
		pick := func(xs ...string) string { return xs[r.Intn(len(xs))] }
		many := func(n int, xs ...string) (out []string) {
			for range r.Intn(n + 1) {
				out = append(out, pick(xs...))
			}
			return out
		}
		long := strings.Repeat("Z", 40)
		for i := range 2000 {
			langs := many(8, "en", "fr", "de", "ja", "es", "it", "pt", "ko")
			ctx := FormatContext{
				Library:                   r.Intn(2) == 0,
				Complete:                  r.Intn(4) == 0,
				Seasons:                   intList{1}[:r.Intn(2)],
				Resolution:                pick("", "2160p", "1440p", "1080p", "720p", long),
				Quality:                   pick("", "WEB-DL", "BluRay REMUX", "BluRay", long),
				HDR:                       many(5, "DV", "HDR", "HDR10+", "HLG", "SDR", long),
				Size:                      []int64{0, 450e6, 999999999, 1e9, 9.95e9, 12.49e9, 123e12}[r.Intn(7)],
				Audio:                     many(4, "Atmos", "TrueHD", "Dolby Digital Plus", "DTS Lossless", long),
				Channels:                  many(1, "7.1", "5.1", "stereo"),
				Languages:                 langs,
				LanguageSource:            pick("", "measured", "reported", "inferred"),
				DualAudio:                 r.Intn(3) == 0,
				Codec:                     pick("", "HEVC", "AVC", long),
				BitDepth:                  pick("", "10bit"),
				Group:                     pick("", "NTb", "  ", "·", "·····Team", "A · B", "EN✓", "✓", "L1\nL2", "u\u2028v", "世界字幕组", long),
				TopScoreWithoutResolution: r.Intn(120) - 10,
				ScoreWithoutResolution:    r.Intn(120) - 10,
			}
			ctx.Probed.AudioLanguages = langs[:r.Intn(len(langs)+1)]
			if problems := draculaJellyfinInvariants(t, tpl, ctx); len(problems) > 0 {
				t.Fatalf("seed context %d: %s\n%+v", i, strings.Join(problems, "; "), ctx)
			}
		}
	})

	t.Run("real ranking pipeline", func(t *testing.T) {
		draculaJellyfinPools(t, tpl, production.ResultNameTemplate)
	})
}

// draculaTrailingStars returns the run of filled stars that ends s.
func draculaTrailingStars(s string) string { return s[len(strings.TrimRight(s, "★")):] }

type draculaJellyfinSpec struct {
	title   string
	sizeGB  float64
	langs   []string
	library bool
	probed  *release.MediaCaps
	tail    string // exact label after the head, for Library releases whose languages were measured
}

// draculaJellyfinPools ranks representative releases through the published
// profile and Define Library, as a user importing both would, and renders each
// pool through StreamNZB's playlist aggregation (the path the Jellyfin endpoint
// reads). It hands the labels to the pkg/server/jellyfin stage.
func draculaJellyfinPools(t *testing.T, jellyfinTpl, productionTpl string) {
	profile := draculaProductionProfile(t, filepath.Join(filepath.Dir(os.Getenv("DRACULA_FORMATTER_CASES_PATH")), "..", "..", ".."))

	// Synthetic probe data attached to Library releases, standing in for ffprobe.
	uhdProbe := &release.MediaCaps{TracksProbed: true, AudioStreams: 2, AudioLanguages: []string{"en", "fr"}, DurationSeconds: 9960}
	sdrProbe := &release.MediaCaps{TracksProbed: true, AudioStreams: 1, AudioLanguages: []string{"en"}, DurationSeconds: 3000}
	jaProbe := &release.MediaCaps{TracksProbed: true, AudioStreams: 1, AudioLanguages: []string{"ja"}, DurationSeconds: 1440}
	en := []string{"en"}

	pools := []struct {
		name  string
		req   ranking.Request
		specs []draculaJellyfinSpec
	}{
		{"Movie", ranking.Request{Kind: ranking.KindMovie, Title: "Dune Part Two"}, []draculaJellyfinSpec{
			{title: "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.Atmos.7.1-FraMeSToR", sizeGB: 68.4, langs: en},
			{title: "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.Atmos.7.1-FraMeSToR", sizeGB: 68.4, langs: []string{"en", "fr"}, library: true, probed: uhdProbe, tail: "68 GB · Atmos TrueHD 7.1 · EN/FR✓"},
			{title: "Dune.Part.Two.2024.2160p.WEB-DL.x265.10bit.HDR10.DDP5.1-GRP", sizeGB: 15.2, langs: en},
			{title: "Dune.Part.Two.2024.1080p.BluRay.x264.DTS-HD.MA.5.1-ExampleGroup", sizeGB: 14.3, langs: en},
			{title: "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 6.2, langs: en},
			{title: "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 6.2, langs: en, library: true, probed: sdrProbe, tail: "6.2 GB · DD+ 5.1 · EN✓ · AVC · NTb"},
			{title: "Dune.Part.Two.2024.1080p.WEBRip.x265.10bit.AAC.2.0-ExtremelyLongReleaseGroupNameTeam", sizeGB: 2.1, langs: en},
			{title: "Dune.Part.Two.2024.1080p.BluRay.MULTi.TrueHD.Atmos.7.1.x265-GRP", sizeGB: 31.0, langs: []string{"en", "fr", "de", "es", "it", "ja"}},
			{title: "Dune.Part.Two.2024.WEB-DL.German.DL.DDP5.1-GRP", sizeGB: 8.0, langs: []string{"de", "en"}},
			{title: "Dune.Part.Two.2024.1080p-GRP", sizeGB: 0.45},
		}},
		{"TV", ranking.Request{Kind: ranking.KindSeries, Title: "The Last of Us", Season: 1, Episode: 3}, []draculaJellyfinSpec{
			{title: "The.Last.of.Us.S01E03.2160p.MAX.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", sizeGB: 9.8, langs: en},
			{title: "The.Last.of.Us.S01E03.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 3.4, langs: en},
			{title: "The.Last.of.Us.S01E03.720p.HDTV.x264-SYNCOPY", sizeGB: 0.9},
		}},
		{"Season pack", ranking.Request{Kind: ranking.KindSeries, Title: "The Last of Us", Season: 1}, []draculaJellyfinSpec{
			{title: "The.Last.of.Us.S01.2160p.BluRay.REMUX.HEVC.TrueHD.Atmos.7.1-GRP", sizeGB: 190, langs: en},
			{title: "The.Last.of.Us.S01.COMPLETE.1080p.BluRay.x265.10bit.DTS-HD.MA.5.1-GRP", sizeGB: 42, langs: en},
			{title: "The.Last.of.Us.S01.1080p.WEB-DL.DDP5.1.H.264-NTb", sizeGB: 42, langs: en, library: true, probed: sdrProbe, tail: "42 GB · DD+ 5.1 · EN✓ · AVC · NTb"},
		}},
		{"Anime", ranking.Request{Kind: ranking.KindAnimeShow, IsAnime: true, Title: "Frieren Beyond Journeys End", Season: 1, Episode: 1}, []draculaJellyfinSpec{
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG", sizeGB: 1.4, langs: []string{"ja"}},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.Dual-Audio.FLAC2.0.x265-FLE", sizeGB: 2.8, langs: []string{"ja", "en"}},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.REMUX.AVC.FLAC2.0.Dual.Audio-ZR", sizeGB: 12.5, langs: []string{"ja", "en"}},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.Multi-Audio.FLAC2.0.x265.10bit-GRP", sizeGB: 3.1, langs: []string{"ja", "en", "de", "fr", "es"}},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.BluRay.x265.10bit.FLAC2.0-LIB", sizeGB: 2.5, langs: []string{"ja"}, library: true, probed: jaProbe, tail: "2.5 GB · FLAC 2.0 · JA✓ · HEVC 10bit · LIB"},
			{title: "Frieren.Beyond.Journeys.End.S01E01.1080p.WEB.H264-SubsPlease", sizeGB: 0.9, langs: []string{"ja"}},
		}},
	}

	entries := map[string][]PlaylistEntry{}
	for _, pool := range pools {
		t.Run(pool.name, func(t *testing.T) {
			in := make([]triage.Candidate, 0, len(pool.specs))
			for i, s := range pool.specs {
				in = append(in, triage.Candidate{Release: &release.Release{
					Title: s.title, Size: int64(s.sizeGB * 1e9), Indexer: "TestIndexer", Languages: s.langs, IsLibrary: s.library,
					PubDate: time.Now().AddDate(0, 0, -30).Format(time.RFC1123Z), DetailsURL: fmt.Sprintf("https://example.invalid/%d", i),
				}})
			}
			results, rejected := profile.ApplyWithRejected(pool.req, in, rank.RankOptions{})
			if len(rejected) > 0 {
				t.Fatalf("premise lost: %d candidate(s) rejected: %v", len(rejected), rejected[0].Torrent.Rejections)
			}

			// Map survivors to candidates and contexts as handlers_playlist.go and facade.go do.
			var cands []triage.Candidate
			var specs []draculaJellyfinSpec
			top, topWithout := 0, 0
			for _, r := range results {
				cand := r.Candidate
				cand.Metadata = parser.FromResult(r.Torrent.Raw, r.Torrent.Data)
				cand.Group = cand.Metadata.ResolutionGroup()
				cand.Score = r.Torrent.Rank
				i := slices.IndexFunc(pool.specs, func(s draculaJellyfinSpec) bool {
					return s.title == cand.Release.Title && s.library == cand.Release.IsLibrary
				})
				if i < 0 {
					t.Fatalf("ranked release %q matches no spec", cand.Release.Title)
				}
				cand.Verdict.Probed = pool.specs[i].probed // a stand-in for a real ffprobe run
				cands, specs = append(cands, cand), append(specs, pool.specs[i])
				top, topWithout = max(top, cand.Score), max(topWithout, scoreWithoutResolution(cand))
			}
			ctxs := make([]FormatContext, len(cands))
			for i, cand := range cands {
				ctxs[i] = newFormatContext(cand, i+1, len(cands), top, topWithout, "", "", pool.req.Title, capsSummaryLine(cand.Release), false, 0)
			}

			jf := draculaPlaylistLabels(t, jellyfinTpl, cands, ctxs)
			// The PlaybackInfo stage gets the real playlist entries, not entries rebuilt from contexts.
			entries[pool.name] = (&Server{}).playlistViewOf(&playlistResult{Candidates: cands}, StreamSlotKey{}, &auth.Stream{ResultNameTemplate: jellyfinTpl}).Entries
			// The PlaybackInfo stage derives stream capabilities (such as 2160p) from the raw
			// entry Title; it must stay the candidate's release title or that check goes vacuous.
			if len(entries[pool.name]) != len(cands) {
				t.Fatalf("%d playlist entries for %d candidates", len(entries[pool.name]), len(cands))
			}
			for i, e := range entries[pool.name] {
				if e.Title != cands[i].Release.Title {
					t.Errorf("entry #%d Title %q differs from its release title %q", i, e.Title, cands[i].Release.Title)
				}
			}
			prod := draculaPlaylistLabels(t, productionTpl, cands, ctxs)
			distinctJF, distinctProd := map[string]bool{}, map[string]bool{}
			for i, ctx := range ctxs {
				if problems := draculaJellyfinInvariants(t, jellyfinTpl, ctx); len(problems) > 0 {
					t.Errorf("%s: %s", jf[i], strings.Join(problems, "; "))
				}
				// The stars end the head: they must equal the production label's, wherever a
				// group might also contain a star. LIB leads the label.
				head, tail, _ := strings.Cut(jf[i], " · ")
				if got, want := draculaTrailingStars(head), draculaTrailingStars(prod[i]); got != want {
					t.Errorf("%q has stars %q, production label %q has %q", jf[i], got, prod[i], want)
				}
				if got := strings.HasPrefix(jf[i], "LIB "); got != specs[i].library {
					t.Errorf("%q: LIB marker %v, Library %v", jf[i], got, specs[i].library)
				}
				// The measured mark appears only on probed Library releases, in this exact tail;
				// no other release in the pool may carry one.
				if want := specs[i].tail; want != "" && tail != want {
					t.Errorf("%q: tail %q, want %q", jf[i], tail, want)
				}
				distinctJF[jf[i]], distinctProd[prod[i]] = true, true
				t.Logf("prod=%q jellyfin=%q", prod[i], jf[i])
			}
			// Better differentiation than production, not a uniqueness guarantee: the Movie
			// and Anime pools hold releases production labels collide on, and the Jellyfin
			// labels must separate more of them; elsewhere they must separate at least as many.
			if len(distinctJF) < len(distinctProd) || ((pool.name == "Movie" || pool.name == "Anime") && len(distinctJF) <= len(distinctProd)) {
				t.Errorf("%d distinct Jellyfin labels do not improve on production's %d over %d releases", len(distinctJF), len(distinctProd), len(ctxs))
			}
		})
	}

	if out := os.Getenv("DRACULA_JELLYFIN_ENTRIES"); out != "" {
		data, err := json.Marshal(entries)
		if err == nil {
			err = os.WriteFile(out, data, 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
