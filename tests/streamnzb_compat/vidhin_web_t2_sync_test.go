package streamnzb_compat

// Permanent regression for the 2026-10-23 Vidhin sync (PR #81): upstream
// added monkee to Radarr Web T2 and CMRG to Sonarr Web T2, so they now map
// into DraCuLa's "Movies WEB T2 Groups" and "Shows WEB T2 Groups". Locks in
// the +300 tier score, case-insensitive whole-group matching, content-kind
// and WEB-DL scoping, and the trusted-group consequences ("Trusted Release
// Groups", "Reject suspicious 4K upscale", "Adaptive HD x265") against the
// exact published profiles and the real synced Define library.
//
// Requires PR #81's generated/streamnzb-defines.txt; against the earlier
// library every positive case fails.

import (
	"fmt"
	"strings"
	"testing"

	jhin "github.com/dreulavelle/jhin/parser"
	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

const (
	webT2MovieTitle  = "Example.Movie.2020.%s.%s.DDP5.1.%s-%s"
	webT2SeriesTitle = "Example.Show.S01E01.%s.%s.DDP5.1.%s-%s"
)

func webT2Movie(res, source, codec, group string) string {
	return fmt.Sprintf(webT2MovieTitle, res, source, codec, group)
}

func webT2Series(res, source, codec, group string) string {
	return fmt.Sprintf(webT2SeriesTitle, res, source, codec, group)
}

var (
	webT2MovieReq  = ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}
	webT2SeriesReq = ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 1, Title: "Example Show"}
)

// TestVidhinWebT2SyncTrustedDefine is the Define layer: "Trusted Release
// Groups" (generated from every tier Define) recognizes each new group in
// its own content kind only, by whole case-insensitive group name.
func TestVidhinWebT2SyncTrustedDefine(t *testing.T) {
	const probe = "probe: Trusted Release Groups"
	set, err := rules.Compile(
		[]config.RuleConfig{{Name: probe, When: `matched("Trusted Release Groups")`, Points: 1}},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile trusted probe: %v", err)
	}
	trusted := func(kind, title string) bool {
		t.Helper()
		cand := triage.Candidate{Release: &release.Release{Title: title}}
		env := rules.BuildEnv(cand, jhin.Parse(title), rules.Context{Kind: kind})
		return ruleMatched(set.Evaluate(env, kind), probe)
	}

	cases := []struct {
		name  string
		kind  string
		title string
		want  bool
	}{
		{"movie monkee", ranking.KindMovie, webT2Movie("1080p", "AMZN.WEB-DL", "H264", "monkee"), true},
		{"movie MONKEE", ranking.KindMovie, webT2Movie("1080p", "AMZN.WEB-DL", "H264", "MONKEE"), true},
		{"movie monkeez", ranking.KindMovie, webT2Movie("1080p", "AMZN.WEB-DL", "H264", "monkeez"), false},
		{"movie xmonkee", ranking.KindMovie, webT2Movie("1080p", "AMZN.WEB-DL", "H264", "xmonkee"), false},
		{"series CMRG", ranking.KindSeries, webT2Series("1080p", "AMZN.WEB-DL", "H264", "CMRG"), true},
		{"series cmrg", ranking.KindSeries, webT2Series("1080p", "AMZN.WEB-DL", "H264", "cmrg"), true},
		{"series CMRGX", ranking.KindSeries, webT2Series("1080p", "AMZN.WEB-DL", "H264", "CMRGX"), false},
		{"series XCMRG", ranking.KindSeries, webT2Series("1080p", "AMZN.WEB-DL", "H264", "XCMRG"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trusted(tc.kind, tc.title); got != tc.want {
				t.Errorf("%q trusted = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}

func webT2Profiles(t *testing.T) map[string]*ranking.Profile {
	t.Helper()
	defines := loadDefineLibrary(t)
	out := map[string]*ranking.Profile{}
	for name, ruleSet := range map[string][]config.RuleConfig{
		"samsung": loadProductionRules(t),
		"neutral": loadNeutralRules(t),
	} {
		profile, err := ranking.Compile(
			config.FilterProfileConfig{Name: "vidhin-web-t2-sync-" + name, Preset: "4k", Rules: ruleSet},
			defines...,
		)
		if err != nil {
			t.Fatalf("compile %s production profile: %v", name, err)
		}
		out[name] = profile
	}
	return out
}

func webT2Apply(profile *ranking.Profile, req ranking.Request, pool []hhwebCand, title string) (ranking.Result, bool) {
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates(pool), jhinrank.RankOptions{})
	return hhwebResultByTitle(kept, rejected, title)
}

func webT2Payouts(r ranking.Result) map[string]int {
	out := make(map[string]int, len(r.Matched))
	for _, m := range r.Matched {
		out[m.Name] = m.Score
	}
	return out
}

// TestVidhinWebT2SyncTierScore: each new group earns exactly the +300 T2
// payout, ranks identically to an existing T2 member (MiU) and +300 above an
// unclassified control. Non-WEB-DL sources, near-miss group names and the
// other content kind (where each group keeps its existing T1 tier) get no T2
// payout.
func TestVidhinWebT2SyncTierScore(t *testing.T) {
	cases := []struct {
		name      string
		req       ranking.Request
		title     func(source, group string) string
		group     string
		tier      string
		otherTier string
	}{
		{"movie monkee", webT2MovieReq, func(s, g string) string { return webT2Movie("1080p", s, "H264", g) }, "monkee", "Movies WEB T2", "Shows WEB T2"},
		{"series CMRG", webT2SeriesReq, func(s, g string) string { return webT2Series("1080p", s, "H264", g) }, "CMRG", "Shows WEB T2", "Movies WEB T2"},
	}

	for variant, profile := range webT2Profiles(t) {
		score := func(t *testing.T, req ranking.Request, title string) (int, map[string]int) {
			t.Helper()
			r, ok := webT2Apply(profile, req, []hhwebCand{{title: title}}, title)
			if !ok || !r.Torrent.Fetch {
				t.Fatalf("%s: %q not kept (found=%v rejections=%v)", variant, title, ok, r.Torrent.Rejections)
			}
			return r.Torrent.Rank, webT2Payouts(r)
		}

		for _, tc := range cases {
			t.Run(variant+"/"+tc.name, func(t *testing.T) {
				rank, payouts := score(t, tc.req, tc.title("AMZN.WEB-DL", tc.group))
				if got, ok := payouts[tc.tier]; !ok || got != 300 {
					t.Fatalf("%s = %d (matched=%v), want +300; payouts=%v", tc.tier, got, ok, payouts)
				}
				peerRank, _ := score(t, tc.req, tc.title("AMZN.WEB-DL", "MiU"))
				if rank != peerRank {
					t.Errorf("rank %d != existing T2 member MiU rank %d", rank, peerRank)
				}
				controlRank, _ := score(t, tc.req, tc.title("AMZN.WEB-DL", "ZZZCONTROL"))
				if delta := rank - controlRank; delta != 300 {
					t.Errorf("rank - unclassified control = %+d, want +300", delta)
				}

				upper, _ := score(t, tc.req, tc.title("AMZN.WEB-DL", strings.ToUpper(tc.group)))
				lower, _ := score(t, tc.req, tc.title("AMZN.WEB-DL", strings.ToLower(tc.group)))
				if upper != rank || lower != rank {
					t.Errorf("case variants rank upper=%d lower=%d, want %d", upper, lower, rank)
				}

				for _, neg := range []struct{ source, group string }{
					{"BluRay", tc.group},
					{"WEBRip", tc.group},
					{"AMZN.WEB-DL", tc.group + "X"},
					{"AMZN.WEB-DL", "X" + tc.group},
				} {
					_, p := score(t, tc.req, tc.title(neg.source, neg.group))
					if _, ok := p[tc.tier]; ok {
						t.Errorf("%s/%s unexpectedly matched %s; payouts=%v", neg.source, neg.group, tc.tier, p)
					}
				}
			})
		}

		t.Run(variant+"/other content kind keeps T1, not T2", func(t *testing.T) {
			_, movieCMRG := score(t, webT2MovieReq, webT2Movie("1080p", "AMZN.WEB-DL", "H264", "CMRG"))
			if _, ok := movieCMRG["Movies WEB T2"]; ok {
				t.Errorf("movie CMRG matched Movies WEB T2; payouts=%v", movieCMRG)
			}
			if movieCMRG["Movies WEB T1"] != 500 {
				t.Errorf("movie CMRG Movies WEB T1 = %d, want +500; payouts=%v", movieCMRG["Movies WEB T1"], movieCMRG)
			}
			_, seriesMonkee := score(t, webT2SeriesReq, webT2Series("1080p", "AMZN.WEB-DL", "H264", "monkee"))
			if _, ok := seriesMonkee["Shows WEB T2"]; ok {
				t.Errorf("series monkee matched Shows WEB T2; payouts=%v", seriesMonkee)
			}
			if seriesMonkee["Shows WEB T1"] != 500 {
				t.Errorf("series monkee Shows WEB T1 = %d, want +500; payouts=%v", seriesMonkee["Shows WEB T1"], seriesMonkee)
			}
		})
	}
}

// TestVidhinWebT2SyncUpscaleGuard: a 2160p WEB-DL from either group is now
// exempt from "Reject suspicious 4K upscale" when only a 1080p remux exists,
// while an unclassified 2160p in the same shape is still rejected. The
// control runs in its own pool because the guard's exemption is pool-wide.
func TestVidhinWebT2SyncUpscaleGuard(t *testing.T) {
	cases := []struct {
		name   string
		req    ranking.Request
		anchor string
		title  func(group string) string
		group  string
	}{
		{"movie monkee", webT2MovieReq, "Example.Movie.2020.1080p.BluRay.REMUX-ANCHOR", func(g string) string { return webT2Movie("2160p", "WEB-DL", "H265", g) }, "monkee"},
		{"series CMRG", webT2SeriesReq, "Example.Show.S01E01.1080p.BluRay.REMUX-ANCHOR", func(g string) string { return webT2Series("2160p", "WEB-DL", "H265", g) }, "CMRG"},
	}
	for variant, profile := range webT2Profiles(t) {
		for _, tc := range cases {
			t.Run(variant+"/"+tc.name, func(t *testing.T) {
				title := tc.title(tc.group)
				r, ok := webT2Apply(profile, tc.req, []hhwebCand{{title: tc.anchor}, {title: title}}, title)
				if !ok || !r.Torrent.Fetch {
					t.Errorf("%q rejected, want exempt from Reject suspicious 4K upscale: %v", title, r.Torrent.Rejections)
				}

				control := tc.title("ZZZCONTROL")
				c, ok := webT2Apply(profile, tc.req, []hhwebCand{{title: tc.anchor}, {title: control}}, control)
				if !ok || c.Torrent.Fetch || !strings.Contains(strings.Join(c.Torrent.Rejections, "|"), "Reject suspicious 4K upscale") {
					t.Errorf("unclassified control %q: fetch=%v rejections=%v, want Reject suspicious 4K upscale", control, c.Torrent.Fetch, c.Torrent.Rejections)
				}
			})
		}
	}
}

// TestVidhinWebT2SyncAdaptiveHDx265: as trusted groups, a 1080p x265 WEB-DL
// from either group survives "Adaptive HD x265" in a pool of more than six
// 1080p AVC peers; an unclassified x265 control in the same pool is rejected.
func TestVidhinWebT2SyncAdaptiveHDx265(t *testing.T) {
	cases := []struct {
		name  string
		req   ranking.Request
		title func(codec, group string) string
		group string
	}{
		{"movie monkee", webT2MovieReq, func(c, g string) string { return webT2Movie("1080p", "WEB-DL", c, g) }, "monkee"},
		{"series CMRG", webT2SeriesReq, func(c, g string) string { return webT2Series("1080p", "WEB-DL", c, g) }, "CMRG"},
	}
	for variant, profile := range webT2Profiles(t) {
		for _, tc := range cases {
			t.Run(variant+"/"+tc.name, func(t *testing.T) {
				var pool []hhwebCand
				for i := 0; i < 7; i++ {
					pool = append(pool, hhwebCand{title: tc.title("H264", fmt.Sprintf("PEER%d", i))})
				}
				title := tc.title("x265", tc.group)
				control := tc.title("x265", "ZZZCONTROL")
				pool = append(pool, hhwebCand{title: title}, hhwebCand{title: control})

				r, ok := webT2Apply(profile, tc.req, pool, title)
				if !ok || !r.Torrent.Fetch {
					t.Errorf("%q rejected, want trusted exemption from Adaptive HD x265: %v", title, r.Torrent.Rejections)
				}
				c, ok := webT2Apply(profile, tc.req, pool, control)
				if !ok || c.Torrent.Fetch || !strings.Contains(strings.Join(c.Torrent.Rejections, "|"), "Adaptive HD x265") {
					t.Errorf("unclassified control %q: fetch=%v rejections=%v, want Adaptive HD x265", control, c.Torrent.Fetch, c.Torrent.Rejections)
				}
			})
		}
	}
}
