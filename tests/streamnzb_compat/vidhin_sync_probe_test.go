package streamnzb_compat

// Real-engine evidence for the advisory Vidhin sync assessment
// (scripts/assess_vidhin_sync.py, docs/VIDHIN_SYNC_ASSESSMENT.md).
//
// Opt-in: skipped unless VIDHIN_PROBE_PLAN names a probe plan. Every probe is
// ranked through each published profile twice -- once with the accepted (old)
// Define Library and once with the proposed (new) one -- and the raw outcomes
// are written to VIDHIN_PROBE_RESULTS. The test asserts nothing about expected
// scores: the assessment compares old against new, so the evidence never
// depends on what the sync generator intended.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"testing"

	jhinrank "github.com/dreulavelle/jhin/rank"

	"streamnzb/pkg/core/config"
	"streamnzb/pkg/search/ranking"
)

type vidhinProbePlan struct {
	SchemaVersion    int               `json:"schema_version"`
	OldLibrary       string            `json:"old_library"`
	NewLibrary       string            `json:"new_library"`
	OldLibrarySHA256 string            `json:"old_library_sha256"`
	NewLibrarySHA256 string            `json:"new_library_sha256"`
	Profiles         map[string]string `json:"profiles"`
	Probes           []vidhinProbe     `json:"probes"`
}

type vidhinProbe struct {
	ID      string `json:"id"`
	Request struct {
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Anime   bool   `json:"anime"`
		Season  int    `json:"season"`
		Episode int    `json:"episode"`
	} `json:"request"`
	Pool   []string `json:"pool"`
	Target string   `json:"target"`
}

type vidhinProbeResult struct {
	ID         string         `json:"id"`
	Variant    string         `json:"variant"`
	Library    string         `json:"library"`
	Found      bool           `json:"found"`
	Fetch      bool           `json:"fetch"`
	Rank       int            `json:"rank"`
	Matched    map[string]int `json:"matched"`
	Rejections []string       `json:"rejections"`
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVidhinSyncProbe(t *testing.T) {
	planPath := os.Getenv("VIDHIN_PROBE_PLAN")
	if planPath == "" {
		t.Skip("VIDHIN_PROBE_PLAN not set; Vidhin sync probes are opt-in")
	}
	resultsPath := os.Getenv("VIDHIN_PROBE_RESULTS")
	if resultsPath == "" {
		t.Fatal("VIDHIN_PROBE_RESULTS must be set with VIDHIN_PROBE_PLAN")
	}

	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read probe plan: %v", err)
	}
	var plan vidhinProbePlan
	if err := json.Unmarshal(planBytes, &plan); err != nil {
		t.Fatalf("decode probe plan: %v", err)
	}
	if plan.SchemaVersion != 1 {
		t.Fatalf("probe plan schema_version = %d, want 1", plan.SchemaVersion)
	}
	if got := fileSHA256(t, plan.OldLibrary); got != plan.OldLibrarySHA256 {
		t.Fatalf("old library sha256 = %s, plan expects %s", got, plan.OldLibrarySHA256)
	}
	if got := fileSHA256(t, plan.NewLibrary); got != plan.NewLibrarySHA256 {
		t.Fatalf("new library sha256 = %s, plan expects %s", got, plan.NewLibrarySHA256)
	}

	libraries := map[string][]config.RuleConfig{
		"old": loadDefineLibraryFile(t, plan.OldLibrary),
		"new": loadDefineLibraryFile(t, plan.NewLibrary),
	}

	variants := make([]string, 0, len(plan.Profiles))
	for variant := range plan.Profiles {
		variants = append(variants, variant)
	}
	sort.Strings(variants)

	var results []vidhinProbeResult
	for _, variant := range variants {
		rules := loadProfileRules(t, plan.Profiles[variant], variant)
		for _, library := range []string{"old", "new"} {
			profile, err := ranking.Compile(
				config.FilterProfileConfig{Name: "vidhin-sync-probe-" + variant, Preset: "4k", Rules: rules},
				libraries[library]...,
			)
			if err != nil {
				t.Fatalf("compile %s profile with %s library: %v", variant, library, err)
			}
			for _, probe := range plan.Probes {
				req := ranking.Request{
					Kind:    probe.Request.Kind,
					IsAnime: probe.Request.Anime,
					Season:  probe.Request.Season,
					Episode: probe.Request.Episode,
					Title:   probe.Request.Title,
				}
				pool := make([]hhwebCand, 0, len(probe.Pool))
				for _, title := range probe.Pool {
					pool = append(pool, hhwebCand{title: title})
				}
				kept, rejected := profile.ApplyWithRejected(req, hhwebCandidates(pool), jhinrank.RankOptions{})
				r, found := hhwebResultByTitle(kept, rejected, probe.Target)

				out := vidhinProbeResult{ID: probe.ID, Variant: variant, Library: library, Found: found, Matched: map[string]int{}, Rejections: []string{}}
				if found {
					out.Fetch = r.Torrent.Fetch
					out.Rank = r.Torrent.Rank
					for _, m := range r.Matched {
						out.Matched[m.Name] = m.Score
					}
					out.Rejections = append(out.Rejections, r.Torrent.Rejections...)
					sort.Strings(out.Rejections)
				}
				results = append(results, out)
			}
		}
	}

	planSum := sha256.Sum256(planBytes)
	payload := map[string]any{
		"schema_version":     1,
		"plan_sha256":        hex.EncodeToString(planSum[:]),
		"old_library_sha256": plan.OldLibrarySHA256,
		"new_library_sha256": plan.NewLibrarySHA256,
		"results":            results,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("encode probe results: %v", err)
	}
	if err := os.WriteFile(resultsPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write probe results: %v", err)
	}
	t.Logf("wrote %d probe results for %d probes to %s", len(results), len(plan.Probes), resultsPath)
}
