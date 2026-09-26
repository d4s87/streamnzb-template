package streamnzb_compat

// Permanent regression for the Anime REMUX source-authority decision
// (backlog-roadmap.md "Anime REMUX source authority vs tier authority").
// At equal resolution, StreamNZB's native remux score is intentionally strong
// enough to outrank Anime release-group tier differences; DraCuLa does not
// compensate it. TestAdjacentTierCeilingMatrix only orders tiers inside one
// source family, so nothing else pins this cross-source precedence: a pin
// move that changed the native remux score would shift every REMUX family
// uniformly and pass. Ordering only, no margins. This is a REMUX decision,
// not a claim that every source distinction outranks every tier.

import (
	"fmt"
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

func TestAnimeRemuxSourceAuthority(t *testing.T) {
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "anime-remux-source-authority", Preset: "4k", Rules: loadProductionRules(t)},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}
	defines := loadCeilingDefines(t)

	// Resolution, codec and audio are identical; only source and group vary.
	const (
		remux  = "1080p.BluRay.REMUX.FLAC.x264"
		encode = "1080p.BluRay.FLAC.x264"
		web    = "1080p.WEB-DL.FLAC.x264"
	)

	kinds := []struct {
		kind, prefix, stem string
	}{
		{ranking.KindAnimeShow, "Anime Shows", "Example.Anime.S01E01"},
		{ranking.KindAnimeMovie, "Anime Movies", "Example.Anime.Movie.2025"},
	}

	for _, k := range kinds {
		k := k
		t.Run(k.kind, func(t *testing.T) {
			// score returns the rank and fails unless exactly wantTier (or, when
			// empty, no Anime tier rule at all) matched.
			score := func(source, group, wantTier string) int {
				t.Helper()
				title := k.stem + "." + source + "-" + group
				req := ranking.Request{Kind: k.kind, Title: "Example", IsAnime: true}
				if k.kind == ranking.KindAnimeShow {
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
				var tiers []string
				for _, m := range kept[0].Matched {
					for _, fam := range []string{" BluRay T", " WEB T"} {
						if strings.HasPrefix(m.Name, k.prefix) && strings.Contains(m.Name, fam) {
							tiers = append(tiers, m.Name)
						}
					}
				}
				if (wantTier == "" && len(tiers) != 0) || (wantTier != "" && (len(tiers) != 1 || tiers[0] != wantTier)) {
					t.Fatalf("%q: matched tier rules %v; want %q", title, tiers, wantTier)
				}
				return kept[0].Torrent.Rank
			}
			group := func(family string, tier int) (string, string) {
				name := fmt.Sprintf("%s %s T%d", k.prefix, family, tier)
				toks := defines[name+" Groups"]
				if len(toks) == 0 {
					t.Fatalf("missing/empty Define %q", name+" Groups")
				}
				return toks[0], name
			}
			above := func(label string, a, b int) {
				t.Helper()
				if a <= b {
					t.Errorf("%s: %d does not outrank %d", label, a, b)
				}
			}

			untieredRemux := score(remux, "ZZZREM", "")
			g1, bd1 := group("BluRay", 1)
			bd1Encode := score(encode, g1, bd1)
			w1, web1 := group("WEB", 1)
			above("untiered REMUX > BluRay T1 encode", untieredRemux, bd1Encode)
			above("untiered REMUX > WEB T1", untieredRemux, score(web, w1, web1))

			for _, tier := range []int{1, 3, 8} {
				g, name := group("BluRay", tier)
				tierRemux := score(remux, g, name)
				above(fmt.Sprintf("BluRay T%d REMUX > BluRay T%d encode", tier, tier), tierRemux, score(encode, g, name))
				if tier == 8 {
					above("BluRay T8 REMUX > BluRay T1 encode", tierRemux, bd1Encode)
				}
			}
		})
	}
}
