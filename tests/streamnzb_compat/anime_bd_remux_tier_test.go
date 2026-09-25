package streamnzb_compat

// Permanent regression for the Anime BD remux tier fix (backlog-roadmap.md
// "Anime BD-tier remux source-gating audit"). Jhin never gives a remux the
// `bluray` trait, so the old "bluray and not remux" tier predicate left every
// BD-tier group's remux untiered. The fix keeps Vidhin's Anime BD source gate,
// per-record case flags and PMR/NAN0/-ZR- conditions inside the generated
// "Anime ... BluRay Tn Groups" Defines (sync_vidhin.py anime_bd mode), so the
// tier rules can accept `"bluray" or "remux"` and the stale LazyRemux/
// UltraRemux group bypass is gone. Runs the published profile and the real
// generated Define library through the pinned engine.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

func TestAnimeBDRemuxTierClassification(t *testing.T) {
	productionRules := loadProductionRules(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "anime-bd-remux-tier", Preset: "4k", Rules: productionRules},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	wantPoints := map[int]int{1: 560, 3: 400, 4: 320, 5: 240}

	kinds := []struct {
		label  string
		kind   string
		prefix string // rule-name prefix, e.g. "Anime Shows"
		title  string // title stem before the source tokens
	}{
		{"Anime Show", ranking.KindAnimeShow, "Anime Shows", "Example.Anime.S01E01.1080p"},
		{"Anime Movie", ranking.KindAnimeMovie, "Anime Movies", "Example.Anime.Movie.2025.1080p"},
	}

	// tier is the expected Anime BluRay tier, 0 = no Anime BluRay tier rule.
	// untiered, when set, is the same title with an unclassified group; the
	// score difference must be exactly the tier's points.
	cases := []struct {
		name     string
		suffix   string
		tier     int
		untiered string
	}{
		// Valid BD-sourced remuxes.
		{"ZR BluRay REMUX", ".BluRay.REMUX.FLAC.x264-ZR", 3, ".BluRay.REMUX.FLAC.x264-ZZZREM"},
		{"ZR BD REMUX", ".BD.REMUX-ZR", 3, ".BD.REMUX-ZZZREM"},
		{"ZR BDMux REMUX", ".BDMux.REMUX-ZR", 3, ".BDMux.REMUX-ZZZREM"},
		{"ZR DVD REMUX", ".DVD.REMUX-ZR", 3, ".DVD.REMUX-ZZZREM"},
		{"ZR NTSC REMUX", ".NTSC.REMUX-ZR", 3, ".NTSC.REMUX-ZZZREM"},
		{"ZR mid-name -ZR- form", ".BluRay.REMUX-ZR-v2", 3, ""},
		{"PMR BluRay REMUX", ".BluRay.REMUX-PMR", 3, ".BluRay.REMUX-ZZZREM"},
		{"NAN0 BluRay REMUX", ".BluRay.REMUX-NAN0", 3, ".BluRay.REMUX-ZZZREM"},
		{"LazyRemux BluRay", ".BluRay-LazyRemux", 4, ""},
		{"UltraRemux BluRay REMUX", ".BluRay.REMUX-UltraRemux", 5, ".BluRay.REMUX-ZZZREM"},
		{"sam BluRay REMUX (T1 case-sensitive record)", ".BluRay.REMUX-sam", 1, ".BluRay.REMUX-ZZZREM"},

		// Unchanged non-remux control.
		{"ZR BluRay encode", ".BluRay.x264-ZR", 3, ".BluRay.x264-ZZZENC"},

		// Upstream rejects: no BD source evidence.
		{"bare REMUX-ZR", ".REMUX-ZR", 0, ""},
		{"WEB-DL REMUX-ZR", ".WEB-DL.REMUX-ZR", 0, ""},
		{"WEBRip REMUX-ZR", ".WEBRip.REMUX-ZR", 0, ""},
		{"bare LazyRemux", "-LazyRemux", 0, ""},
		{"WEB-DL LazyRemux", ".WEB-DL-LazyRemux", 0, ""},
		{"bare UltraRemux", "-UltraRemux", 0, ""},
		{"WEB-DL UltraRemux", ".WEB-DL-UltraRemux", 0, ""},
		{"WEB-DL REMUX-NAN0", ".WEB-DL.REMUX-NAN0", 0, ""},

		// Upstream rejects: PMR needs a standalone Remux word, NAN0 needs
		// remux before it.
		{"PMR BDRemux (no \\bRemux\\b)", ".BDRemux-PMR", 0, ""},
		{"PMR BluRay encode", ".BluRay.x265-PMR", 0, ""},
		{"NAN0 BluRay encode", ".BluRay.x265-NAN0", 0, ""},

		// Upstream rejects: T1 sam record is case-sensitive.
		{"lowercase bluray remux-sam", ".bluray.remux-sam", 0, ""},
		{"lowercase bluray encode-sam", ".bluray.x264-sam", 0, ""},
	}

	score := func(t *testing.T, kind, title string) (int, []string) {
		t.Helper()
		req := ranking.Request{Kind: kind, Title: "Example", IsAnime: true}
		if kind == ranking.KindAnimeShow {
			req.Season, req.Episode = 1, 1
		}
		kept, rejected := profile.ApplyWithRejected(
			req,
			[]triage.Candidate{{Release: &release.Release{Title: title}}},
			jhinrank.RankOptions{},
		)
		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf("%q: kept=%d rejected=%d; want a single kept result", title, len(kept), len(rejected))
		}
		var names []string
		for _, m := range kept[0].Matched {
			names = append(names, m.Name)
		}
		return kept[0].Torrent.Rank, names
	}

	// The production formatter fixture for this case renders its tier remark
	// purely from context.MatchedRules. Pin that context to what the real
	// engine produces for the same title, so the rendered remark is proven
	// against production scoring rather than a hand-picked rule list.
	t.Run("formatter fixture MatchedRules come from the engine", func(t *testing.T) {
		const fixtureName = "Anime BluRay T3 remark on ZR remux"
		data, err := os.ReadFile("fixtures/formatter.json")
		if err != nil {
			t.Fatalf("read formatter fixtures: %v", err)
		}
		var fixtures struct {
			Cases []struct {
				Name    string `json:"name"`
				Context struct {
					ReleaseTitle string
					Kind         string
					MatchedRules []struct {
						Name  string
						Score int
					}
				} `json:"context"`
			} `json:"cases"`
		}
		if err := json.Unmarshal(data, &fixtures); err != nil {
			t.Fatalf("decode formatter fixtures: %v", err)
		}
		for _, c := range fixtures.Cases {
			if c.Name != fixtureName {
				continue
			}
			req := ranking.Request{Kind: c.Context.Kind, Title: "Example", IsAnime: true, Season: 1, Episode: 2}
			kept, _ := profile.ApplyWithRejected(
				req,
				[]triage.Candidate{{Release: &release.Release{Title: c.Context.ReleaseTitle}}},
				jhinrank.RankOptions{},
			)
			if len(kept) != 1 {
				t.Fatalf("%q not kept", c.Context.ReleaseTitle)
			}
			var got, want []string
			for _, m := range kept[0].Matched {
				got = append(got, fmt.Sprintf("%s=%d", m.Name, m.Score))
			}
			for _, m := range c.Context.MatchedRules {
				want = append(want, fmt.Sprintf("%s=%d", m.Name, m.Score))
			}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("fixture MatchedRules %v != engine %v", want, got)
			}
			return
		}
		t.Fatalf("formatter fixture %q not found", fixtureName)
	})

	// "Reject bad 4K Anime" exempts a 2160p pool when any 2160p candidate
	// matches an Anime tier Define. With the source gate in the BD Defines, a
	// WEB-DL from a BD-only group no longer counts as trusted 4K (upstream
	// does not classify it either); a BD-sourced release from it still does.
	t.Run("Reject bad 4K Anime follows the source-gated BD Defines", func(t *testing.T) {
		defines := loadCeilingDefines(t)
		web := map[string]bool{}
		for tier := 1; tier <= 6; tier++ {
			for _, tok := range defines[fmt.Sprintf("Anime Shows WEB T%d Groups", tier)] {
				web[strings.ToLower(tok)] = true
			}
		}
		bdOnly := ""
		for _, tok := range defines["Anime Shows BluRay T3 Groups"] {
			if !web[strings.ToLower(tok)] && tok != "PMR" && tok != "NAN0" {
				bdOnly = tok
				break
			}
		}
		if bdOnly == "" {
			t.Fatal("no BD-only Anime BluRay T3 group found")
		}

		pool := func(first string) map[string]bool {
			titles := []string{
				first,
				"Example.Anime.S01E01.2160p.WEB-DL.x265-ZZZOTHER",
				"Example.Anime.S01E01.1080p.WEB-DL.x264-ZZZOTHER",
			}
			var cands []triage.Candidate
			for _, title := range titles {
				cands = append(cands, triage.Candidate{Release: &release.Release{Title: title}})
			}
			req := ranking.Request{Kind: ranking.KindAnimeShow, Title: "Example", IsAnime: true, Season: 1, Episode: 1}
			_, rejected := profile.ApplyWithRejected(req, cands, jhinrank.RankOptions{})
			out := map[string]bool{}
			for _, r := range rejected {
				if strings.Contains(strings.Join(r.Torrent.Rejections, "|"), "Reject bad 4K Anime") {
					out[r.Candidate.Release.Title] = true
				}
			}
			return out
		}

		webTitle := "Example.Anime.S01E01.2160p.WEB-DL.x265-" + bdOnly
		if got := pool(webTitle); !got[webTitle] || len(got) != 2 {
			t.Fatalf("2160p WEB-DL from BD-only group %s should not exempt the pool; rejected=%v", bdOnly, got)
		}
		bdTitle := "Example.Anime.S01E01.2160p.BluRay.x265-" + bdOnly
		if got := pool(bdTitle); len(got) != 0 {
			t.Fatalf("2160p BluRay from BD-only group %s should exempt the pool; rejected=%v", bdOnly, got)
		}
	})

	for _, k := range kinds {
		t.Run(k.label, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					title := k.title + tc.suffix
					got, matched := score(t, k.kind, title)

					var tiers []string
					for _, name := range matched {
						if strings.HasPrefix(name, k.prefix+" BluRay T") {
							tiers = append(tiers, name)
						}
					}

					if tc.tier == 0 {
						if len(tiers) != 0 {
							t.Fatalf("%s: unexpected Anime BluRay tier %v", title, tiers)
						}
						return
					}

					want := fmt.Sprintf("%s BluRay T%d", k.prefix, tc.tier)
					if len(tiers) != 1 || tiers[0] != want {
						t.Fatalf("%s: tier rules %v; want [%s]", title, tiers, want)
					}
					if p := findProductionRule(t, productionRules, want).Points; p != wantPoints[tc.tier] {
						t.Fatalf("%s points = %d; want %d", want, p, wantPoints[tc.tier])
					}

					if tc.untiered == "" {
						return
					}
					base, baseMatched := score(t, k.kind, k.title+tc.untiered)
					if got-base != wantPoints[tc.tier] {
						t.Fatalf(
							"%s: tiered %d - untiered %d = %+d; want exactly %+d\n  untiered matched: %v",
							title, got, base, got-base, wantPoints[tc.tier], baseMatched,
						)
					}
				})
			}
		})
	}
}
