package streamnzb_compat

// Permanent regression for the 2026-11 Vidhin GHOSTS sync (upstream
// Vidhin05/Releases-Regex 19c54e0, "match GHOSTS only in capitals").
//
// Upstream dropped GHOSTS from Radarr LQ (it is a Sonarr-only group, as in
// TRaSH) and made its Sonarr LQ match case-sensitive, because Vidhin's
// unanchored \bGHOSTS\b/i regex was matching episode titles like "Full of
// Ghosts". DraCuLa matches LQ groups against the parsed release group, which
// never had that false positive, so "Shows LQ Groups" keeps GHOSTS
// case-insensitive through the mapping's case_insensitive_group_overrides
// (scripts/vidhin_mapping.json). Locked in here, for both published profiles
// with the generated Define Library:
//
//   - Movies: GHOSTS/ghosts/Ghosts are no longer LQ (no -10000, no pruning).
//   - Series (episode, season pack, Anime series): all three stay LQ.
//   - iVy stays case-sensitive; near misses and title-only "Ghosts" never match.

import (
	"fmt"
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/search/ranking"
)

const (
	ghostsLQPenalty    = -10000
	ghostsCleanGroup   = "ZXQGRP" // classified by no Define
	ghostsAdaptiveRule = "Adaptive Low-Score Filtering"
	ghostsProbePrefix  = "probe: " // a rule may not share its Define's name
)

var ghostsLQDefines = []string{
	"Movies LQ Groups", "Movies LQ Release Title", "Movies Bad Dual Groups",
	"Shows LQ Groups", "Shows LQ Release Title", "Shows Bad Dual Groups",
	"Anime LQ Groups",
}

var ghostsSpellings = []string{"GHOSTS", "ghosts", "Ghosts"}

type ghostsKind struct {
	name      string
	req       ranking.Request
	lqDefine  string
	lqPenalty string
	otherKind string // Define-name prefix that must never match this kind
	shapes    []string
	dense     []string // six clean candidates far above a 720p LQ release
	denseTail string
}

var ghostsMovie = ghostsKind{
	name:      "movie",
	req:       ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"},
	lqDefine:  "Movies LQ Groups",
	lqPenalty: "Movies LQ Penalty",
	otherKind: "Shows ",
	shapes: []string{
		"Example.Movie.2020.1080p.AMZN.WEB-DL.DDP5.1.H264-%s",
		"Example.Movie.2020.1080p.BluRay.DD5.1.x264-%s",
		"Example.Movie.2020.2160p.WEB-DL.DDP5.1.HDR.H265-%s",
	},
	dense: []string{
		"Example.Movie.2020.2160p.WEB-DL.H265-FLUX",
		"Example.Movie.2020.2160p.WEB-DL.H265-NTb",
		"Example.Movie.2020.1080p.BluRay.REMUX.AVC-HiFi",
		"Example.Movie.2020.1080p.WEB-DL.H264-FLUX",
		"Example.Movie.2020.1080p.WEB-DL.H264-NTb",
		"Example.Movie.2020.720p.WEB-DL.H264-FLUX",
	},
	denseTail: "Example.Movie.2020.720p.WEB-DL.H264-%s",
}

var ghostsSeries = ghostsKind{
	name:      "series",
	req:       ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 1, Title: "Example Show"},
	lqDefine:  "Shows LQ Groups",
	lqPenalty: "Shows LQ Penalty",
	otherKind: "Movies ",
	shapes: []string{
		"Example.Show.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H264-%s",
		"Example.Show.S01E01.1080p.BluRay.DD5.1.x264-%s",
		"Example.Show.S01E01.2160p.WEB-DL.DDP5.1.HDR.H265-%s",
	},
	dense: []string{
		"Example.Show.S01E01.2160p.WEB-DL.H265-FLUX",
		"Example.Show.S01E01.2160p.WEB-DL.H265-NTb",
		"Example.Show.S01E01.1080p.BluRay.REMUX.AVC-BLURANiUM",
		"Example.Show.S01E01.1080p.WEB-DL.H264-FLUX",
		"Example.Show.S01E01.1080p.WEB-DL.H264-NTb",
		"Example.Show.S01E01.720p.WEB-DL.H264-FLUX",
	},
	denseTail: "Example.Show.S01E01.720p.WEB-DL.H264-%s",
}

type ghostsProfiles struct {
	defines *ranking.Profile
	full    map[string]*ranking.Profile
}

func ghostsLoadProfiles(t *testing.T) ghostsProfiles {
	t.Helper()
	library := loadDefineLibrary(t)

	var probes []config.RuleConfig
	for _, name := range ghostsLQDefines {
		probes = append(probes, config.RuleConfig{Name: ghostsProbePrefix + name, When: fmt.Sprintf("matched(%q)", name), Points: 1})
	}
	definesProfile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "ghosts-lq-define-probe", Preset: "4k", Rules: probes},
		library...,
	)
	if err != nil {
		t.Fatalf("compile Define probe profile: %v", err)
	}

	out := ghostsProfiles{defines: definesProfile, full: map[string]*ranking.Profile{}}
	for variant, rules := range map[string][]config.RuleConfig{
		"samsung": loadProductionRules(t),
		"neutral": loadNeutralRules(t),
	} {
		p, err := ranking.Compile(
			config.FilterProfileConfig{Name: "ghosts-lq-sync-" + variant, Preset: "4k", Rules: rules},
			library...,
		)
		if err != nil {
			t.Fatalf("compile %s profile: %v", variant, err)
		}
		out.full[variant] = p
	}
	return out
}

// ghostsMatchedDefines returns the LQ-family Defines a lone release matches.
func ghostsMatchedDefines(t *testing.T, p ghostsProfiles, req ranking.Request, title string) map[string]bool {
	t.Helper()
	kept, rejected := p.defines.ApplyWithRejected(req, hhwebCandidates([]hhwebCand{{title: title}}), jhinrank.RankOptions{})
	r, ok := hhwebResultByTitle(kept, rejected, title)
	if !ok {
		t.Fatalf("%s: missing from Define probe results", title)
	}
	out := map[string]bool{}
	for _, m := range r.Matched {
		out[strings.TrimPrefix(m.Name, ghostsProbePrefix)] = true
	}
	return out
}

func ghostsSole(t *testing.T, p *ranking.Profile, req ranking.Request, title string) ranking.Result {
	t.Helper()
	kept, rejected := p.ApplyWithRejected(req, hhwebCandidates([]hhwebCand{{title: title}}), jhinrank.RankOptions{})
	if len(kept) != 1 || len(rejected) != 0 {
		t.Fatalf("%s: sole result kept=%d rejected=%d; want kept=1 rejected=0", title, len(kept), len(rejected))
	}
	return kept[0]
}

func ghostsParsedGroup(r ranking.Result) string {
	if r.Torrent.Data == nil {
		return ""
	}
	return r.Torrent.Data.Group
}

func ghostsMatchedScore(r ranking.Result, name string) (int, bool) {
	for _, m := range r.Matched {
		if m.Name == name {
			return m.Score, true
		}
	}
	return 0, false
}

// ghostsAssertClassification checks one release against both published
// profiles: Define membership, the exact LQ penalty, the score delta against a
// clean same-shape release, and sole-result preservation.
func ghostsAssertClassification(t *testing.T, p ghostsProfiles, k ghostsKind, req ranking.Request, title, clean, group string, wantLQ bool) {
	t.Helper()
	defines := ghostsMatchedDefines(t, p, req, title)
	if defines[k.lqDefine] != wantLQ {
		t.Errorf("%s: %s matched=%v, want %v", title, k.lqDefine, defines[k.lqDefine], wantLQ)
	}
	for name := range defines {
		if strings.HasPrefix(name, k.otherKind) {
			t.Errorf("%s: content-kind leak into %s", title, name)
		}
		if name != k.lqDefine && name != "Anime LQ Groups" {
			t.Errorf("%s: unexpected LQ-family Define %s", title, name)
		}
	}

	for variant, profile := range p.full {
		r := ghostsSole(t, profile, req, title)
		if got := ghostsParsedGroup(r); got != group {
			t.Errorf("%s/%s: parsed group=%q, want %q", variant, title, got, group)
		}
		score, penalized := ghostsMatchedScore(r, k.lqPenalty)
		if penalized != wantLQ || (penalized && score != ghostsLQPenalty) {
			t.Errorf("%s/%s: %s matched=%v score=%d, want matched=%v score=%d",
				variant, title, k.lqPenalty, penalized, score, wantLQ, ghostsLQPenalty)
		}
		cleanRank := ghostsSole(t, profile, req, clean).Torrent.Rank
		wantRank := cleanRank
		if wantLQ {
			wantRank += ghostsLQPenalty
		}
		if r.Torrent.Rank != wantRank {
			t.Errorf("%s/%s: rank=%d, want %d (clean %s rank %d)", variant, title, r.Torrent.Rank, wantRank, clean, cleanRank)
		}
	}
}

func TestGhostsLQClassificationByCase(t *testing.T) {
	p := ghostsLoadProfiles(t)
	for _, tc := range []struct {
		kind ghostsKind
		want bool
	}{{ghostsMovie, false}, {ghostsSeries, true}} {
		for _, shape := range tc.kind.shapes {
			clean := fmt.Sprintf(shape, ghostsCleanGroup)
			for _, g := range ghostsSpellings {
				t.Run(tc.kind.name+"/"+fmt.Sprintf(shape, g), func(t *testing.T) {
					ghostsAssertClassification(t, p, tc.kind, tc.kind.req, fmt.Sprintf(shape, g), clean, g, tc.want)
				})
			}
		}
	}
}

func TestGhostsLQSeasonPackAndAnime(t *testing.T) {
	p := ghostsLoadProfiles(t)
	pack := ranking.Request{Kind: ranking.KindSeries, Season: 1, Title: "Example Show"}
	animeSeries := ranking.Request{Kind: ranking.KindSeries, IsAnime: true, Season: 1, Episode: 1, Title: "Example Anime"}
	animeMovie := ranking.Request{Kind: ranking.KindMovie, IsAnime: true, Title: "Example Anime"}
	for _, g := range ghostsSpellings {
		ghostsAssertClassification(t, p, ghostsSeries, pack,
			"Example.Show.S01.1080p.WEB-DL.DDP5.1.H264-"+g, "Example.Show.S01.1080p.WEB-DL.DDP5.1.H264-"+ghostsCleanGroup, g, true)
		ghostsAssertClassification(t, p, ghostsSeries, animeSeries,
			"Example.Anime.S01E01.1080p.WEB-DL.H264-"+g, "Example.Anime.S01E01.1080p.WEB-DL.H264-"+ghostsCleanGroup, g, true)
		ghostsAssertClassification(t, p, ghostsMovie, animeMovie,
			"Example.Anime.2020.1080p.WEB-DL.H264-"+g, "Example.Anime.2020.1080p.WEB-DL.H264-"+ghostsCleanGroup, g, false)
	}
}

func TestGhostsLQControlsAndNearMisses(t *testing.T) {
	p := ghostsLoadProfiles(t)
	for _, k := range []ghostsKind{ghostsMovie, ghostsSeries} {
		shape := k.shapes[0]
		clean := fmt.Sprintf(shape, ghostsCleanGroup)
		for g, want := range map[string]bool{
			"iVy": true, "ivy": false, "IVY": false, // upstream case-sensitive, unchanged
			"GHOST": false, "GHOSTSS": false, "xGHOSTS": false, "GHOSTS2": false, "GH0STS": false,
		} {
			t.Run(k.name+"/"+g, func(t *testing.T) {
				ghostsAssertClassification(t, p, k, k.req, fmt.Sprintf(shape, g), clean, g, want)
			})
		}
	}
}

// Title-only "Ghosts" (a show or movie named Ghosts, an episode title) must
// never classify: only the parsed group is matched.
func TestGhostsLQTitleOnlyNeverMatches(t *testing.T) {
	p := ghostsLoadProfiles(t)
	episode := ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 5, Title: "Example Show"}
	showGhosts := ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 1, Title: "Ghosts"}
	movieGhosts := ranking.Request{Kind: ranking.KindMovie, Title: "Full of Ghosts"}
	for _, tc := range []struct {
		kind  ghostsKind
		req   ranking.Request
		title string
		group string
	}{
		{ghostsSeries, episode, "Example.Show.S01E05.Full.of.Ghosts.1080p.WEB-DL.H264-" + ghostsCleanGroup, ghostsCleanGroup},
		{ghostsSeries, episode, "Example.Show.S01E05.1080p.WEB-DL.H264.Full.of.Ghosts", ""},
		{ghostsSeries, episode, "Example.Show.S01E05.1080p.WEB-DL.H264-Full.of.Ghosts", ""},
		{ghostsSeries, showGhosts, "Ghosts.2021.S01E01.1080p.WEB.H264-" + ghostsCleanGroup, ghostsCleanGroup},
		{ghostsSeries, showGhosts, "Ghosts.2021.S01E01.1080p.WEB.H264", ""},
		{ghostsMovie, movieGhosts, "Full.of.Ghosts.2020.1080p.WEB-DL.H264-" + ghostsCleanGroup, ghostsCleanGroup},
	} {
		t.Run(tc.title, func(t *testing.T) {
			if defines := ghostsMatchedDefines(t, p, tc.req, tc.title); len(defines) != 0 {
				t.Errorf("%s: matched LQ-family Defines %v, want none", tc.title, defines)
			}
			for variant, profile := range p.full {
				r := ghostsSole(t, profile, tc.req, tc.title)
				if got := ghostsParsedGroup(r); got != tc.group {
					t.Errorf("%s/%s: parsed group=%q, want %q", variant, tc.title, got, tc.group)
				}
				if _, ok := ghostsMatchedScore(r, tc.kind.lqPenalty); ok {
					t.Errorf("%s/%s: %s applied to a title-only Ghosts", variant, tc.title, tc.kind.lqPenalty)
				}
			}
		})
	}
}

// A 720p GHOSTS release with six clean candidates >=5000 points above it:
// Series LQ is pruned by Adaptive Low-Score Filtering, a Movie is kept.
func TestGhostsLQAdaptiveLowScoreFiltering(t *testing.T) {
	p := ghostsLoadProfiles(t)
	for _, tc := range []struct {
		kind       ghostsKind
		wantPruned bool
	}{{ghostsMovie, false}, {ghostsSeries, true}} {
		for _, g := range append(append([]string{}, ghostsSpellings...), ghostsCleanGroup) {
			title := fmt.Sprintf(tc.kind.denseTail, g)
			want := tc.wantPruned && g != ghostsCleanGroup
			pool := []hhwebCand{{title: title}}
			for _, d := range tc.kind.dense {
				pool = append(pool, hhwebCand{title: d})
			}
			for variant, profile := range p.full {
				kept, rejected := profile.ApplyWithRejected(tc.kind.req, hhwebCandidates(pool), jhinrank.RankOptions{})
				if len(kept)+len(rejected) != len(pool) {
					t.Fatalf("%s/%s: kept=%d rejected=%d, want %d total", variant, title, len(kept), len(rejected), len(pool))
				}
				var pruned []string
				for _, r := range rejected {
					pruned = append(pruned, r.Candidate.Release.Title)
				}
				wantPruned := []string{}
				if want {
					wantPruned = []string{title}
				}
				if fmt.Sprint(pruned) != fmt.Sprint(wantPruned) {
					t.Errorf("%s/%s: pruned=%v, want %v", variant, title, pruned, wantPruned)
					continue
				}
				if want && fmt.Sprint(rejected[0].Torrent.Rejections) != fmt.Sprint([]string{"rule: " + ghostsAdaptiveRule}) {
					t.Errorf("%s/%s: rejections=%v, want only %s", variant, title, rejected[0].Torrent.Rejections, ghostsAdaptiveRule)
				}
			}
		}
	}
}

// The generated library must express the override as a whole-group match and
// never as a release-name match, and GHOSTS must be gone from every Movies Define.
func TestGhostsLQDefineLibraryShape(t *testing.T) {
	for _, d := range loadDefineLibrary(t) {
		if !strings.Contains(strings.ToUpper(d.When), "GHOSTS") {
			continue
		}
		if d.Name != "Shows LQ Groups" {
			t.Errorf("GHOSTS appears in %q; want only Shows LQ Groups", d.Name)
			continue
		}
		if !strings.Contains(d.When, `group matches "(?i)^(GHOSTS)$"`) || strings.Count(d.When, "GHOSTS") != 1 {
			t.Errorf("Shows LQ Groups GHOSTS condition = %q; want exactly one case-insensitive whole-group match", d.When)
		}
		if !strings.Contains(d.When, `group == "iVy"`) {
			t.Errorf("Shows LQ Groups lost its case-sensitive iVy match: %q", d.When)
		}
	}
}
