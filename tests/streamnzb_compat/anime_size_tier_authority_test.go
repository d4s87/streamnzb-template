package streamnzb_compat

import (
	"fmt"
	"testing"
	"time"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

// TestAnimeSizeTierAuthority locks in the removal of Anime size scoring.
// anime_show (6GB/+150) and anime_movie (20GB/+500) used to carry the same
// bounded size term as Series/Movie, 25 points per GB. The smallest adjacent
// Anime tier gap is 80 points and the ordinary bonus stack the tier-authority
// matrix already applies consumes most of it, so a lower tier won with only
// ~0.5-1.3GB more size, and a clean Anime Movie tier flipped at ~3.2GB. The
// largest weight that kept every pair ordered was 12 (one point of headroom),
// so both Anime kinds were dropped from the published scoring map instead.
//
// Compiled from the exact published profile.txt payload, including its real
// Scoring map. Ordering only; no hardcoded margins.
func TestAnimeSizeTierAuthority(t *testing.T) {
	payload := loadProfilePayload(t, "../../profile.txt", "production")

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:    "Anime size tier authority",
			Preset:  payload.Preset,
			Scoring: payload.Scoring,
			Rules:   payload.Rules,
		},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	defines := loadCeilingDefines(t)

	fullAvail := &triage.AvailState{
		Status:       triage.AvailAvailable,
		OnMyBackbone: true,
		CheckedAt:    time.Now().Add(-3 * 24 * time.Hour),
	}

	score := func(kind, title string, size int64, avail *triage.AvailState) int {
		t.Helper()

		candidate := triage.Candidate{
			Release: &release.Release{Title: title, Size: size},
		}
		if avail != nil {
			candidate.Verdict.Avail = *avail
		}

		req := ranking.Request{Kind: kind, Title: "Example"}
		switch kind {
		case ranking.KindSeries:
			req.Season, req.Episode = 1, 1
		case ranking.KindAnimeShow:
			req.Season, req.Episode = 1, 1
			req.IsAnime = true
		case ranking.KindAnimeMovie:
			req.IsAnime = true
		}

		kept, rejected := profile.ApplyWithRejected(
			req, []triage.Candidate{candidate}, jhinrank.RankOptions{},
		)
		if len(rejected) != 0 || len(kept) != 1 {
			t.Fatalf("%q @%d bytes: kept=%d rejected=%+v", title, size, len(kept), rejected)
		}
		return kept[0].Torrent.Rank
	}

	tok := func(name string) string {
		t.Helper()
		toks := defines[name]
		if len(toks) == 0 {
			t.Fatalf("missing/empty Define %q", name)
		}
		return toks[0]
	}

	gb := func(n float64) int64 { return int64(n * 1e9) }

	showBD := []string{"S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "TrueHD", "Atmos", "7.1"}
	showWEB := []string{"CR", "S01.COMPLETE", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "DDP5.1", "Atmos"}
	movieBD := []string{"Dual", "Audio", "Uncensored", "v4", "REPACK3", "TrueHD", "Atmos", "7.1"}
	movieWEB := []string{"CR", "Dual", "Audio", "Uncensored", "v4", "REPACK3", "DDP5.1", "Atmos"}

	// Same ordinary positive decorations as TestAdjacentTierCeilingMatrix.
	// oldTarget is where the removed size term peaked (full weight); the
	// clean higher tier is scored at a size that earned ~0 (tiny) and one
	// that earned exactly 0 (beyond twice the old target).
	families := []struct {
		label, kind, prefix, stem string
		tiers                     int
		source                    []string
		codec                     string
		decorations               []string
		oldTarget                 float64
	}{
		{"Anime Show BluRay", ranking.KindAnimeShow, "Anime Shows BluRay", "Example.Anime.S01E01", 8,
			[]string{"1080p", "BluRay"}, "x264", showBD, 6},
		{"Anime Show BluRay REMUX", ranking.KindAnimeShow, "Anime Shows BluRay", "Example.Anime.S01E01", 8,
			[]string{"1080p", "BluRay", "REMUX"}, "AVC", showBD, 6},
		{"Anime Show WEB", ranking.KindAnimeShow, "Anime Shows WEB", "Example.Anime.S01E01", 6,
			[]string{"1080p", "WEB-DL"}, "x264", showWEB, 6},
		{"Anime Movie BluRay", ranking.KindAnimeMovie, "Anime Movies BluRay", "Example.Anime.Movie.2025", 8,
			[]string{"1080p", "BluRay"}, "x264", movieBD, 20},
		{"Anime Movie BluRay REMUX", ranking.KindAnimeMovie, "Anime Movies BluRay", "Example.Anime.Movie.2025", 8,
			[]string{"1080p", "BluRay", "REMUX"}, "AVC", movieBD, 20},
		{"Anime Movie WEB", ranking.KindAnimeMovie, "Anime Movies WEB", "Example.Anime.Movie.2025", 6,
			[]string{"1080p", "WEB-DL"}, "x264", movieWEB, 20},
	}
	build := func(stem string, parts []string, group string) string {
		return joinCeilingParts(append([]string{stem}, parts...)) + "-" + group
	}

	for _, f := range families {
		f := f
		t.Run(f.label, func(t *testing.T) {
			for lower := 2; lower <= f.tiers; lower++ {
				higher := lower - 1
				lowerGroup := tok(fmt.Sprintf("%s T%d Groups", f.prefix, lower))
				higherGroup := tok(fmt.Sprintf("%s T%d Groups", f.prefix, higher))

				lowerParts := append(append(append([]string{}, f.source...), "AV1"), f.decorations...)
				higherParts := append(append([]string{}, f.source...), f.codec)
				lowerTitle := build(f.stem, lowerParts, lowerGroup)
				higherTitle := build(f.stem, higherParts, higherGroup)

				decorated := score(f.kind, lowerTitle, gb(f.oldTarget), fullAvail)

				for _, higherSize := range []float64{0.05, 2*f.oldTarget + 1} {
					clean := score(f.kind, higherTitle, gb(higherSize), nil)
					if decorated >= clean {
						t.Errorf(
							"T%d decorated @%gGB (%d) does not stay below clean T%d @%gGB (%d); margin=%+d\n"+
								"  decorated: %s\n  clean:     %s",
							lower, f.oldTarget, decorated, higher, higherSize, clean, decorated-clean,
							lowerTitle, higherTitle,
						)
					}
				}
			}
		})
	}

	// Anime size contributes exactly nothing: a sized release ranks exactly
	// like the same release with no size at all, below, at and above the old
	// target and at/beyond twice it. This also proves the absent map entry
	// does not fall back to the preset's native size weight.
	zeroCases := []struct {
		kind, title string
		sizes       []float64
	}{
		{ranking.KindAnimeShow, build("Example.Anime.S01E01", []string{"1080p", "BluRay", "x264"}, tok("Anime Shows BluRay T1 Groups")),
			[]float64{0.5, 3, 6, 9, 12, 30}},
		// Multi-episode: 12GB over two episodes was the old 6GB/episode peak.
		{ranking.KindAnimeShow, build("Example.Anime.S01E01E02", []string{"1080p", "BluRay", "x264"}, tok("Anime Shows BluRay T1 Groups")),
			[]float64{1, 6, 12, 24}},
		{ranking.KindAnimeMovie, build("Example.Anime.Movie.2025", []string{"1080p", "BluRay", "x264"}, tok("Anime Movies BluRay T1 Groups")),
			[]float64{4, 12, 20, 30, 40, 60}},
	}
	for _, c := range zeroCases {
		unsized := score(c.kind, c.title, 0, nil)
		for _, s := range c.sizes {
			if got := score(c.kind, c.title, gb(s), nil); got != unsized {
				t.Errorf("%s %q @%gGB: rank %d, want %d (Anime size must contribute 0; delta %+d)",
					c.kind, c.title, s, got, unsized, got-unsized)
			}
		}
	}

	// Non-vacuity control: the same harness does feed size into scoring for
	// Series, so the zero result above is not an artifact of the fixture.
	seriesTitle := "Example.S01E01.1080p.WEB-DL.x264-GRP"
	if score(ranking.KindSeries, seriesTitle, gb(6), nil) == score(ranking.KindSeries, seriesTitle, 0, nil) {
		t.Fatalf("Series size scoring inactive in this harness; Anime zero-size assertions would be vacuous")
	}
}
