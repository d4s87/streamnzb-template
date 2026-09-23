package streamnzb_compat

// Permanent regression for the 2026-09-23 Vidhin sync (backlog-roadmap.md
// "Vidhin committed-baseline drift audit"): HHWEB was removed from
// upstream Radarr/Sonarr Web T3 and no longer maps into DraCuLa's "Movies
// WEB T3 Groups"/"Shows WEB T3 Groups" Defines. Locks in the real,
// reject-level behavioral consequence the pre-sync audit found: an
// HHWEB-grouped 2160p WEB-DL release loses both its +100 WEB T3 tier score
// and its exemption from "Reject suspicious 4K upscale", against the exact
// published production rule and the real synced Define library -- not a
// synthetic substitute.

import (
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

func hhwebFullProductionProfile(t *testing.T) *ranking.Profile {
	t.Helper()
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)
	profile, err := ranking.Compile(
		config.FilterProfileConfig{Name: "hhweb-web-t3-sync-regression", Preset: "4k", Rules: productionRules},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile full production profile: %v", err)
	}
	return profile
}

type hhwebCand struct {
	title   string
	library bool
}

func hhwebCandidates(ts []hhwebCand) []triage.Candidate {
	out := make([]triage.Candidate, 0, len(ts))
	for _, c := range ts {
		out = append(out, triage.Candidate{Release: &release.Release{Title: c.title, IsLibrary: c.library}})
	}
	return out
}

func hhwebResultByTitle(kept, rejected []ranking.Result, title string) (ranking.Result, bool) {
	for _, r := range append(append([]ranking.Result{}, kept...), rejected...) {
		if r.Candidate.Release.Title == title {
			return r, true
		}
	}
	return ranking.Result{}, false
}

func hhwebMatchedNames(r ranking.Result) []string {
	var out []string
	for _, m := range r.Matched {
		out = append(out, m.Name)
	}
	return out
}

// TestHHWEB_NoLongerMatchesWebT3_Movie: (1) HHWEB no longer matches
// "Movies WEB T3 Groups" (the "Movies WEB T3" +100 scoring rule no longer
// fires for it).
func TestHHWEB_NoLongerMatchesWebT3_Movie(t *testing.T) {
	profile := hhwebFullProductionProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	title := "Example.Movie.2020.2160p.WEB-DL.DDP5.1.H264-HHWEB"
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates([]hhwebCand{{title: title}}), jhinrank.RankOptions{})
	r, ok := hhwebResultByTitle(kept, rejected, title)
	if !ok {
		t.Fatalf("candidate missing from results")
	}
	matched := hhwebMatchedNames(r)
	for _, m := range matched {
		if m == "Movies WEB T3" {
			t.Errorf("expected HHWEB to no longer match 'Movies WEB T3' (Movies WEB T3 Groups), matched=%v", matched)
		}
	}
}

// TestHHWEB_NoLongerMatchesWebT3_Series: (2) same check for Shows WEB T3 Groups.
func TestHHWEB_NoLongerMatchesWebT3_Series(t *testing.T) {
	profile := hhwebFullProductionProfile(t)
	req := ranking.Request{Kind: ranking.KindSeries, Season: 1, Episode: 1, Title: "Example Show"}

	title := "Example.Show.S01E01.2160p.WEB-DL.DDP5.1.H264-HHWEB"
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates([]hhwebCand{{title: title}}), jhinrank.RankOptions{})
	r, ok := hhwebResultByTitle(kept, rejected, title)
	if !ok {
		t.Fatalf("candidate missing from results")
	}
	matched := hhwebMatchedNames(r)
	for _, m := range matched {
		if m == "Shows WEB T3" {
			t.Errorf("expected HHWEB to no longer match 'Shows WEB T3' (Shows WEB T3 Groups), matched=%v", matched)
		}
	}
}

// TestHHWEB_2160pWithNoTrustFallback_RejectedByUpscaleGuard: (3) a 2160p
// HHWEB WEB-DL candidate with no qualifying trust/fallback condition (no
// 2160p remux/UHD-tier alternative in the pool, not Library) is now
// subject to "Reject suspicious 4K upscale" when a 1080p remux alternative
// exists -- and (4) a same-shape unclassified control behaves identically,
// proving the change is driven purely by HHWEB's lost tier membership, not
// a group-specific carve-out.
func TestHHWEB_2160pWithNoTrustFallback_RejectedByUpscaleGuard(t *testing.T) {
	profile := hhwebFullProductionProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	anchor1080pRemux := "Example.Movie.2020.1080p.BluRay.REMUX-ANCHOR"
	hhweb2160p := "Example.Movie.2020.2160p.WEB-DL.DDP5.1.H264-HHWEB"
	control2160p := "Example.Movie.2020.2160p.WEB-DL.DDP5.1.H264-ZZZCONTROL"

	ts := []hhwebCand{{title: anchor1080pRemux}, {title: hhweb2160p}, {title: control2160p}}
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates(ts), jhinrank.RankOptions{})

	hhwebResult, ok := hhwebResultByTitle(kept, rejected, hhweb2160p)
	if !ok {
		t.Fatalf("HHWEB candidate missing from results")
	}
	if hhwebResult.Torrent.Fetch {
		t.Errorf("expected the 2160p HHWEB candidate to be REJECTED by 'Reject suspicious 4K upscale' now that it carries no WEB T3 trust, matched=%v", hhwebMatchedNames(hhwebResult))
	}
	if !strings.Contains(strings.Join(hhwebResult.Torrent.Rejections, "|"), "Reject suspicious 4K upscale") {
		t.Errorf("expected rejection specifically by 'Reject suspicious 4K upscale', got %v", hhwebResult.Torrent.Rejections)
	}

	controlResult, ok := hhwebResultByTitle(kept, rejected, control2160p)
	if !ok {
		t.Fatalf("control candidate missing from results")
	}
	if controlResult.Torrent.Fetch != hhwebResult.Torrent.Fetch {
		t.Errorf("expected the unclassified control to behave identically to post-sync HHWEB (both rejected), control fetch=%v hhweb fetch=%v", controlResult.Torrent.Fetch, hhwebResult.Torrent.Fetch)
	}
	if hhwebResult.Torrent.Rank != controlResult.Torrent.Rank {
		t.Errorf("expected identical scores between post-sync HHWEB and the unclassified control, got hhweb=%d control=%d", hhwebResult.Torrent.Rank, controlResult.Torrent.Rank)
	}
}

// TestHHWEB_LibraryExemptionUnaffected: (5) an existing, explicitly
// accepted trust path -- the rule's own "not library" clause -- remains
// unaffected by HHWEB's WEB T3 removal: a Library HHWEB candidate in the
// exact same otherwise-triggering shape still survives.
func TestHHWEB_LibraryExemptionUnaffected(t *testing.T) {
	profile := hhwebFullProductionProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	anchor1080pRemux := "Example.Movie.2020.1080p.BluRay.REMUX-ANCHOR"
	hhwebLibrary2160p := "Example.Movie.2020.2160p.WEB-DL.DDP5.1.H264-HHWEB"

	ts := []hhwebCand{{title: anchor1080pRemux}, {title: hhwebLibrary2160p, library: true}}
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates(ts), jhinrank.RankOptions{})

	r, ok := hhwebResultByTitle(kept, rejected, hhwebLibrary2160p)
	if !ok {
		t.Fatalf("Library HHWEB candidate missing from results")
	}
	if !r.Torrent.Fetch {
		t.Errorf("expected the Library HHWEB candidate to survive via the rule's own 'not library' exemption, unaffected by the WEB T3 removal, rejected=%v", r.Torrent.Rejections)
	}
	if strings.Contains(strings.Join(r.Torrent.Rejections, "|"), "Reject suspicious 4K upscale") {
		t.Error("Library candidate must never be rejected by 'Reject suspicious 4K upscale'")
	}
}

// TestHHWEB_1080pNotRejected_ChangeIsResolutionMediated: (6) a 1080p HHWEB
// candidate is not rejected merely because HHWEB left WEB T3 -- proving
// the retention change is specifically mediated through the 2160p-scoped
// "Reject suspicious 4K upscale" rule, not a blanket HHWEB rejection.
func TestHHWEB_1080pNotRejected_ChangeIsResolutionMediated(t *testing.T) {
	profile := hhwebFullProductionProfile(t)
	req := ranking.Request{Kind: ranking.KindMovie, Title: "Example Movie"}

	anchor1080pRemux := "Example.Movie.2020.1080p.BluRay.REMUX-ANCHOR"
	hhweb1080p := "Example.Movie.2020.1080p.WEB-DL.DDP5.1.H264-HHWEB"

	ts := []hhwebCand{{title: anchor1080pRemux}, {title: hhweb1080p}}
	kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates(ts), jhinrank.RankOptions{})

	r, ok := hhwebResultByTitle(kept, rejected, hhweb1080p)
	if !ok {
		t.Fatalf("1080p HHWEB candidate missing from results")
	}
	if !r.Torrent.Fetch {
		t.Errorf("expected the 1080p HHWEB candidate to survive -- 'Reject suspicious 4K upscale' only ever scopes resolution==2160p, rejected=%v", r.Torrent.Rejections)
	}
}
