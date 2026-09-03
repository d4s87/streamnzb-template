package streamnzb_compat

import (
	"strings"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/rules"
	"streamnzb/pkg/search/triage"
)

func TestSeasonPackProductionLimits(t *testing.T) {
	productionRules := loadProductionRules(t)
	defineLibrary := loadDefineLibrary(t)

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:   "Season-Pack Intelligence production regression",
			Preset: "4k",
			Rules:  productionRules,
		},
		defineLibrary...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}

	titles := []string{
		"Example.Show.S02E04.2160p.WEB-DL.H264-GRP1",
		"Example.Show.S02E04.2160p.WEB-DL.H264-GRP2",
		"Example.Show.S02E04.2160p.WEB-DL.H264-GRP3",
		"Example.Show.S02E04.2160p.WEB-DL.H264-GRP4",
		"Example.Show.S02.2160p.WEB-DL.H264-GRP5",
		"Example.Show.S02.COMPLETE.2160p.WEB-DL.H264-GRP6",
	}

	candidates := make([]triage.Candidate, 0, len(titles))

	for _, title := range titles {
		candidates = append(
			candidates,
			triage.Candidate{
				Release: &release.Release{
					Title: title,
				},
			},
		)
	}

	kept, rejected := profile.ApplyWithRejected(
		ranking.Request{
			Kind:    ranking.KindSeries,
			IsAnime: false,
			Season:  2,
			Episode: 4,
			Title:   "Example Show",
		},
		candidates,
		jhinrank.RankOptions{},
	)

	if len(kept) != 4 {
		t.Fatalf(
			"kept %d candidates, want 4 (3 episodes + 1 season pack)",
			len(kept),
		)
	}

	if len(rejected) != 2 {
		t.Fatalf(
			"rejected %d candidates, want 2 (1 episode + 1 season pack)",
			len(rejected),
		)
	}

	keptNonPacks := 0
	keptPacks := 0
	keptCompletePacks := 0
	ordinaryPackKept := false

	for _, result := range kept {
		env := rules.BuildEnv(
			result.Candidate,
			result.Torrent.Data,
			rules.Context{
				Kind:    ranking.KindSeries,
				Season:  2,
				Episode: 4,
				Title:   "Example Show",
			},
		)

		switch {
		case env.SeasonPack:
			keptPacks++

			if env.Complete {
				keptCompletePacks++
			} else {
				ordinaryPackKept = true
			}

		default:
			keptNonPacks++
		}

		t.Logf(
			"KEPT rank=%d seasonPack=%v complete=%v title=%s",
			result.Torrent.Rank,
			env.SeasonPack,
			env.Complete,
			result.Candidate.Release.Title,
		)
	}

	if keptNonPacks != 3 {
		t.Errorf(
			"kept non-pack candidates = %d, want 3",
			keptNonPacks,
		)
	}

	if keptPacks != 1 {
		t.Errorf(
			"kept season packs = %d, want 1",
			keptPacks,
		)
	}

	if keptCompletePacks != 1 {
		t.Errorf(
			"kept complete season packs = %d, want 1",
			keptCompletePacks,
		)
	}

	if ordinaryPackKept {
		t.Error(
			"ordinary season pack beat the COMPLETE pack; " +
				"Complete Season Pack Preference should win the single pack slot",
		)
	}

	nonPackCapRejects := 0
	packCapRejects := 0
	ordinaryPackRejected := false

	for _, result := range rejected {
		title := result.Candidate.Release.Title
		reasons := strings.Join(result.Torrent.Rejections, "\n")

		env := rules.BuildEnv(
			result.Candidate,
			result.Torrent.Data,
			rules.Context{
				Kind:    ranking.KindSeries,
				Season:  2,
				Episode: 4,
				Title:   "Example Show",
			},
		)

		if strings.Contains(
			reasons,
			"Best 3 per R/Q (over the limit of 3 for 2160p WEB-DL)",
		) {
			nonPackCapRejects++
		}

		if strings.Contains(
			reasons,
			"Best 1 Season Pack per R/Q (over the limit of 1 for 2160p WEB-DL)",
		) {
			packCapRejects++
		}

		if env.SeasonPack && !env.Complete {
			ordinaryPackRejected = true
		}

		t.Logf(
			"REJECTED seasonPack=%v complete=%v title=%s rejections=%v",
			env.SeasonPack,
			env.Complete,
			title,
			result.Torrent.Rejections,
		)
	}

	if nonPackCapRejects != 1 {
		t.Errorf(
			"non-pack cap rejected %d candidates, want 1",
			nonPackCapRejects,
		)
	}

	if packCapRejects != 1 {
		t.Errorf(
			"season-pack cap rejected %d candidates, want 1",
			packCapRejects,
		)
	}

	if !ordinaryPackRejected {
		t.Error(
			"ordinary season pack was not the pack rejected by the 1-slot ceiling",
		)
	}
}
