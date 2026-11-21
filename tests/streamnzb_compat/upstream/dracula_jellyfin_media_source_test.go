package jellyfin

// Second stage of the DraCuLa Jellyfin formatter regression. The stremio-package
// test (dracula_jellyfin_compat_test.go) renders real ranked candidates through
// StreamNZB's own playlist aggregation and writes the resulting entries to
// DRACULA_JELLYFIN_ENTRIES. This test sends them through StreamNZB's real
// Jellyfin PlaybackInfo handler, using the engine's own test fixture, and checks
// the MediaSourceInfo.Name a Jellyfin client would show in its version picker.
// scripts/test_formatter.sh runs both stages and injects both files.

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"streamnzb/pkg/server/stremio"
)

func TestDraculaJellyfinMediaSourceNames(t *testing.T) {
	path := os.Getenv("DRACULA_JELLYFIN_ENTRIES")
	if path == "" {
		t.Fatal("DRACULA_JELLYFIN_ENTRIES is not set; run scripts/test_formatter.sh --jellyfin-production")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var pools map[string][]stremio.PlaylistEntry
	if err := json.Unmarshal(data, &pools); err != nil {
		t.Fatal(err)
	}
	if len(pools) < 4 {
		t.Fatalf("expected at least 4 release pools, got %d", len(pools))
	}

	checked := 0
	for name, entries := range pools {
		f := newFixture()
		movie, _ := itemIDFor("movie", "tt0111161")
		f.catalog.playlist = &stremio.PlaylistView{RuntimeSeconds: 7200, Entries: entries}

		var info playbackInfoResponse
		decodeInto(t, f.do(http.MethodGet, "/jellyfin/Items/"+movie.encode()+"/PlaybackInfo", ""), &info)
		if len(info.MediaSources) != len(entries) {
			t.Fatalf("%s: %d media sources for %d entries", name, len(info.MediaSources), len(entries))
		}
		for i, source := range info.MediaSources {
			checked++
			label, raw := source.Name, entries[i].Title
			switch {
			case label == "":
				t.Errorf("%s #%d: empty MediaSourceInfo.Name", name, i)
			case label == raw || strings.Contains(label, raw):
				t.Errorf("%s #%d: Name fell back to the raw release title: %q", name, i, label)
			case label != entries[i].DisplayTitle:
				t.Errorf("%s #%d: Name %q differs from the rendered label %q", name, i, label, entries[i].DisplayTitle)
			case strings.ContainsAny(label, "\r\n"):
				t.Errorf("%s #%d: Name contains a newline: %q", name, i, label)
			case utf8.RuneCountInString(label) > 72:
				t.Errorf("%s #%d: Name has %d code points, budget 72: %q", name, i, utf8.RuneCountInString(label), label)
			}
			// The label is display only: stream capabilities still come from the raw title.
			if streams := source.MediaStreams; strings.Contains(raw, "2160p") && (len(streams) == 0 || streams[0].Height == nil || *streams[0].Height != 2160) {
				t.Errorf("%s #%d: capabilities no longer come from the raw title: %+v", name, i, streams)
			}
		}
	}
	t.Logf("%d media-source names checked through the real PlaybackInfo handler in %d pools", checked, len(pools))
}
