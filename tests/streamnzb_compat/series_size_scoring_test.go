package streamnzb_compat

import (
	"fmt"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/release"
	"streamnzb/pkg/search/ranking"
	"streamnzb/pkg/search/triage"
)

// seriesSizeScoringProfile compiles the exact published production profile
// (profile.txt, decoded, including its real Scoring map) against the real
// Define library -- the production-regression half of CLAUDE.md's two-layer
// validation philosophy, for the Series/Anime Show size-weight reduction
// (500 -> 150, target unchanged at 6GB) decided by the 2026-09-22
// size-scoring ranking-influence audit (backlog-roadmap.md). Exercised
// against the real pinned StreamNZB/Jhin engine, no synthetic scoring
// reimplementation.
func seriesSizeScoringProfile(t *testing.T) *ranking.Profile {
	t.Helper()

	payload := loadProfilePayload(t, "../../profile.txt", "production")

	profile, err := ranking.Compile(
		config.FilterProfileConfig{
			Name:    "Series/Anime Show size scoring",
			Preset:  payload.Preset,
			Scoring: payload.Scoring,
			Rules:   payload.Rules,
		},
		loadDefineLibrary(t)...,
	)
	if err != nil {
		t.Fatalf("compile production profile: %v", err)
	}
	return profile
}

// seriesSizeScore scores one candidate release for the given content kind
// against the real engine and returns its final Torrent.Rank.
func seriesSizeScore(t *testing.T, profile *ranking.Profile, kind, title string, size int64) int {
	t.Helper()

	req := ranking.Request{Kind: kind, Title: "Slow Horses"}
	if kind == ranking.KindSeries || kind == ranking.KindAnimeShow {
		req.Season = 2
		req.Episode = 4
	}
	if kind == ranking.KindAnimeShow || kind == ranking.KindAnimeMovie {
		req.IsAnime = true
	}

	kept, rejected := profile.ApplyWithRejected(
		req,
		[]triage.Candidate{{
			Release: &release.Release{Title: title, Size: size},
		}},
		jhinrank.RankOptions{},
	)

	if len(rejected) != 0 {
		t.Fatalf("%q unexpectedly rejected: %+v", title, rejected)
	}
	if len(kept) != 1 {
		t.Fatalf("%q: kept %d, want 1", title, len(kept))
	}
	return kept[0].Torrent.Rank
}

func seriesSizeGB(n float64) int64 { return int64(n * 1e9) }

// TestSeriesSizeWeightReduction_TriggeringCaseFlips is the production
// regression for the real-world Slow Horses S2E4 report that motivated the
// audit: same 2160p WEB-DL/WEB-T1 tier, candidate A (7.79GB, SDR, DDP5.1)
// vs candidate B (8.69GB, DV+HDR10+, Atmos+DDP5.1). At the pre-fix 6GB/+500
// weight the real engine scored B - A = -25 (SDR/smaller file wins despite
// carrying no HDR10+/Atmos). At 6GB/+150 the engine must now score B ahead.
func TestSeriesSizeWeightReduction_TriggeringCaseFlips(t *testing.T) {
	profile := seriesSizeScoringProfile(t)

	const base = "Slow.Horses.S02E04.2160p.WEB-DL"
	a := fmt.Sprintf("%s.DDP5.1.H.265-GRP", base)
	b := fmt.Sprintf("%s.DV.HDR10Plus.Atmos.DDP5.1.H.265-GRP2", base)

	rankA := seriesSizeScore(t, profile, ranking.KindSeries, a, seriesSizeGB(7.79))
	rankB := seriesSizeScore(t, profile, ranking.KindSeries, b, seriesSizeGB(8.69))
	delta := rankB - rankA

	if rankB <= rankA {
		t.Fatalf(
			"HDR10+/Atmos candidate B (8.69GB, rank=%d) must outrank plain SDR candidate A (7.79GB, rank=%d) at the 6GB/+150 weight; delta=%d",
			rankB, rankA, delta,
		)
	}

	// The audit's algebra predicts roughly +27/+28 at this exact byte pair
	// (weight scaled 500->150 shrinks the -75 size-term delta to -22.5,
	// against the unchanged +50 HDR10+/Atmos technical delta), subject to
	// exact byte rounding -- assert a tight tolerance band around it rather
	// than pinning one brittle exact integer, since the underlying bytes
	// behind a real indexer's reported size are not observable from the
	// rounded GB the report captured.
	if delta < 20 || delta > 35 {
		t.Fatalf(
			"triggering-case delta (B-A) = %+d, want roughly +27/+28 (tolerance [20,35]); audit predicted the -75 size penalty at weight 500 to shrink to about -22/-23 at weight 150 against the unchanged +50 HDR10+/Atmos technical bonus",
			delta,
		)
	}

	t.Logf("triggering case: A(7.79GB SDR)=%d B(8.69GB HDR10+/Atmos)=%d delta=%+d (pre-fix weight-500 delta was -25/-26)", rankA, rankB, delta)

	// Isolate size-term-only delta vs. technical-only delta, the same
	// decomposition the audit used, so a future change to either the size
	// formula or the HDR10+/Atmos residuals fails this test at the
	// component level, not just via a final-total coincidence.
	plainA := fmt.Sprintf("%s.H.265-GRP3", base)
	plainB := fmt.Sprintf("%s.H.265-GRP4", base)
	sizeOnlyA := seriesSizeScore(t, profile, ranking.KindSeries, plainA, seriesSizeGB(7.79))
	sizeOnlyB := seriesSizeScore(t, profile, ranking.KindSeries, plainB, seriesSizeGB(8.69))
	sizeTermDelta := sizeOnlyB - sizeOnlyA
	technicalDelta := delta - sizeTermDelta

	if sizeTermDelta < -30 || sizeTermDelta > -15 {
		t.Fatalf(
			"isolated size-term delta (7.79GB vs 8.69GB) = %+d, want roughly -22/-23 (weight 150 scaling of the pre-fix -75 at weight 500); the size formula's slope must have scaled proportionally with the weight change",
			sizeTermDelta,
		)
	}
	if technicalDelta != 50 {
		t.Fatalf(
			"isolated technical (HDR10+ + Atmos, DDP cancels) delta = %+d, want exactly +50 -- must be unaffected by the Series/Anime Show size-weight change",
			technicalDelta,
		)
	}
	t.Logf("decomposition: size-term delta=%+d, technical delta=%+d", sizeTermDelta, technicalDelta)
}

// TestSeriesSizeWeightReduction_ThresholdsHold proves the two headline
// threshold claims from the audit's decision: at weight 150 (slope 25
// pts/GB), a lone HDR10+ +25 preference survives just under 1GB of size
// disadvantage, and HDR10+ + Atmos +50 survives just under 2GB, holding the
// SDR candidate fixed at the 6GB peak (its own maximum size score) in both
// cases -- the worst case for the decorated candidate. The decorated
// candidate's size is placed above the 6GB peak (the realistic direction:
// HDR10+/Atmos/DV genuinely costs bitrate, as in the triggering Slow Horses
// case where the decorated candidate was the larger file) -- sizeFactor's
// linear falloff is symmetric around the target, so a disadvantage placed
// below the peak instead would produce byte-identical scores, but above
// matches the real-world shape this test exists to protect. The exact
// algebraic break-even points (bonusPoints * target / weight) are precisely
// 1.0GB and 2.0GB respectively -- real ties, not margins -- so this test
// asserts "still ahead" at 0.9GB/1.9GB (comfortably inside the intended
// "roughly <=1GB"/"roughly <=2GB" survival claim) and separately confirms
// the exact tie at the 1.0GB/2.0GB boundary itself, matching the audit's
// derived thresholds byte-for-byte.
func TestSeriesSizeWeightReduction_ThresholdsHold(t *testing.T) {
	profile := seriesSizeScoringProfile(t)
	const base = "Slow.Horses.S02E04.2160p.WEB-DL"

	sdrAt6GB := seriesSizeScore(
		t, profile, ranking.KindSeries,
		fmt.Sprintf("%s.DDP5.1.H.265-SDRFIX", base),
		seriesSizeGB(6),
	)

	cases := []struct {
		label             string
		tag               string
		aheadDisadvantage float64 // GB above the SDR candidate's 6GB peak, expected strictly ahead
		tieDisadvantage   float64 // GB above the peak, expected exact algebraic tie
	}{
		{"HDR10+ only (+25 bonus)", "HDR10Plus.DDP5.1", 0.9, 1.0},
		{"HDR10+ + Atmos (+50 bonus)", "HDR10Plus.Atmos.DDP5.1", 1.9, 2.0},
	}

	for i, c := range cases {
		aheadSize := 6.0 + c.aheadDisadvantage
		aheadTitle := fmt.Sprintf("%s.%s.H.265-DEC%dA", base, c.tag, i)
		aheadRank := seriesSizeScore(t, profile, ranking.KindSeries, aheadTitle, seriesSizeGB(aheadSize))

		if aheadRank <= sdrAt6GB {
			t.Fatalf(
				"[%s] decorated candidate at %.1fGB (rank=%d) must still outrank plain SDR fixed at the 6GB peak (rank=%d)",
				c.label, aheadSize, aheadRank, sdrAt6GB,
			)
		}
		t.Logf("[%s] decorated@%.1fGB=%d vs SDR@6GB(peak)=%d, margin=%+d",
			c.label, aheadSize, aheadRank, sdrAt6GB, aheadRank-sdrAt6GB)

		tieSize := 6.0 + c.tieDisadvantage
		tieTitle := fmt.Sprintf("%s.%s.H.265-DEC%dT", base, c.tag, i)
		tieRank := seriesSizeScore(t, profile, ranking.KindSeries, tieTitle, seriesSizeGB(tieSize))
		if tieRank != sdrAt6GB {
			t.Fatalf(
				"[%s] exact algebraic break-even at %.1fGB disadvantage: decorated rank=%d, want exactly %d (SDR@6GB peak)",
				c.label, c.tieDisadvantage, tieRank, sdrAt6GB,
			)
		}
		t.Logf("[%s] confirmed exact break-even at %.1fGB disadvantage (both rank=%d)", c.label, c.tieDisadvantage, tieRank)
	}
}

// TestSeriesSizeWeightReduction_SizeStillMeaningfulNearTarget proves size
// remains a real, positive, secondary preference near the 6GB target after
// the weight reduction -- this is not a "zero size scoring" change.
func TestSeriesSizeWeightReduction_SizeStillMeaningfulNearTarget(t *testing.T) {
	profile := seriesSizeScoringProfile(t)
	const base = "Slow.Horses.S02E04.2160p.WEB-DL.DDP5.1.H.265"

	far := seriesSizeScore(t, profile, ranking.KindSeries, fmt.Sprintf("%s-FAR", base), seriesSizeGB(1))
	near := seriesSizeScore(t, profile, ranking.KindSeries, fmt.Sprintf("%s-NEAR", base), seriesSizeGB(5.5))
	atTarget := seriesSizeScore(t, profile, ranking.KindSeries, fmt.Sprintf("%s-TARGET", base), seriesSizeGB(6))

	if !(atTarget > near && near > far) {
		t.Fatalf(
			"size preference must still be monotonically increasing toward the 6GB target: far(1GB)=%d, near(5.5GB)=%d, atTarget(6GB)=%d",
			far, near, atTarget,
		)
	}

	// At weight 150, exactly at the target the size contribution is
	// exactly +150 (factor 1.0); far from it (1GB, factor (1-5/6)=1/6)
	// contributes exactly round(150/6)=25. The gap between the extremes
	// should be a meaningful two-digit-plus swing, not negligible/rounded
	// to noise -- proving "secondary" does not mean "irrelevant."
	if gap := atTarget - far; gap < 100 || gap > 150 {
		t.Fatalf(
			"size-term span from 1GB to 6GB = %d, want roughly 100-150 (weight 150's full range), too small would mean size stopped mattering at all",
			gap,
		)
	}
}

// TestSeriesSizeWeightReduction_ExcessiveSizeNotRewarded proves a clearly
// oversized episode (well beyond 2x the 6GB target, where sizeFactor
// clamps to zero) never outranks a well-targeted one, at the new weight.
func TestSeriesSizeWeightReduction_ExcessiveSizeNotRewarded(t *testing.T) {
	profile := seriesSizeScoringProfile(t)
	const base = "Slow.Horses.S02E04.2160p.WEB-DL.DDP5.1.H.265"

	huge := seriesSizeScore(t, profile, ranking.KindSeries, fmt.Sprintf("%s-HUGE", base), seriesSizeGB(25))
	targetSized := seriesSizeScore(t, profile, ranking.KindSeries, fmt.Sprintf("%s-TGT", base), seriesSizeGB(6))

	if huge >= targetSized {
		t.Fatalf(
			"a 25GB episode (rank=%d) must not outrank a 6GB-target-sized one (rank=%d) merely due to size",
			huge, targetSized,
		)
	}
}

// TestAnimeShowSizeWeightReduction_ActiveAndNoTechnicalPreferenceIntroduced
// proves the published 6GB/+150 scoring map is genuinely active for
// anime_show (not just series), and that no Anime HDR/Atmos/DDP technical
// preference was accidentally introduced by this change -- Anime dynamic
// range/audio-codec preference must remain exactly 0 (project_context.md's
// hard invariant), so an Anime candidate's rank difference at equal size
// must come from size alone, never from HDR10+/Atmos/DDP tokens.
func TestAnimeShowSizeWeightReduction_ActiveAndNoTechnicalPreferenceIntroduced(t *testing.T) {
	profile := seriesSizeScoringProfile(t)

	animeScore := func(title string, size int64) int {
		return seriesSizeScore(t, profile, ranking.KindAnimeShow, title, size)
	}

	// Scoring map is active: two different sizes must produce two
	// different ranks (weight 0 would make them identical).
	plain5 := animeScore("Anime.Show.S01E01.2160p.WEB-DL.DDP5.1.H.265-A5", seriesSizeGB(5))
	plain6 := animeScore("Anime.Show.S01E01.2160p.WEB-DL.DDP5.1.H.265-A6", seriesSizeGB(6))
	if plain5 == plain6 {
		t.Fatalf("Anime Show size scoring appears inactive: rank(5GB)=%d == rank(6GB)=%d", plain5, plain6)
	}
	// Exactly the weight-150 marginal step from 5GB to 6GB: round(150*(1-1/6)) - round(150*1) = 125-150 = -25.
	if got := plain6 - plain5; got != 25 {
		t.Fatalf("Anime Show 5GB->6GB marginal size delta = %+d, want +25 (150/6 pts-per-GB slope)", got)
	}

	// No Anime technical preference introduced: at a fixed size, adding
	// HDR10+/Atmos/DDP tokens to an Anime Show release must not change its
	// rank at all (Anime HDR10+/Atmos/DDP preference is a hard 0).
	bare := animeScore("Anime.Show.S01E01.2160p.WEB-DL.H.265-B1", seriesSizeGB(6))
	decorated := animeScore("Anime.Show.S01E01.2160p.WEB-DL.DV.HDR10Plus.Atmos.DDP5.1.H.265-B2", seriesSizeGB(6))
	if bare != decorated {
		t.Fatalf(
			"Anime Show technical-attribute neutrality broken by this change: bare@6GB=%d, DV+HDR10+/Atmos/DDP@6GB=%d (want equal)",
			bare, decorated,
		)
	}
}

// TestMovieAnimeMovieSizeScoring_UnchangedControl is the negative control:
// Movie and Anime Movie keep 20GB/+500 untouched. It re-derives the same
// production-equivalent absolute scores TestProductionProfileBoundsPresetSizeScoring
// already locks in for Movie, from the Anime Movie side, and independently
// re-confirms the exact scoring-map values for both.
func TestMovieAnimeMovieSizeScoring_UnchangedControl(t *testing.T) {
	profile := seriesSizeScoringProfile(t)

	payload := loadProfilePayload(t, "../../profile.txt", "production")
	movie := config.ResolveScoring(payload.Scoring, ranking.KindMovie)
	animeMovie := config.ResolveScoring(payload.Scoring, ranking.KindAnimeMovie)

	want := config.ScoringConfig{SizeTargetGB: 20, SizeWeight: 500}
	if movie != want {
		t.Fatalf("movie scoring = %+v, want %+v (must be untouched by the Series/Anime Show fix)", movie, want)
	}
	if animeMovie != want {
		t.Fatalf("anime_movie scoring = %+v, want %+v (must be untouched by the Series/Anime Show fix)", animeMovie, want)
	}

	// Behavioral control: the same relative straddle around the 20GB
	// target that produced -25 pre-fix for Series must still produce the
	// identical -25 for Movie, since Movie's scoring config is untouched.
	a := seriesSizeScore(t, profile, ranking.KindMovie, "Example.Movie.2026.2160p.WEB-DL.DDP5.1.H.265-MA", seriesSizeGB(20*7.79/6))
	b := seriesSizeScore(t, profile, ranking.KindMovie, "Example.Movie.2026.2160p.WEB-DL.HDR10Plus.Atmos.DDP5.1.H.265-MB", seriesSizeGB(20*8.69/6))
	if got := b - a; got != -25 {
		t.Fatalf("Movie control delta (scaled straddle around unchanged 20GB target) = %+d, want exactly -25 (must match the audit's pre-fix measurement byte-for-byte, since Movie config did not change)", got)
	}
}
