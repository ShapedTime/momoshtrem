package hdrezka

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// fixtureURL returns the url field of a recorded get_cdn_series answer.
func fixtureURL(t *testing.T, name string) string {
	t.Helper()
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(fixture(t, name), &resp); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return resp.URL
}

func TestDecodeStreamURL(t *testing.T) {
	plain := fixtureURL(t, "cdn_get_movie.json")
	// testdata/legacy_stream.txt is that url field encoded the old way: "#h",
	// base64, and five junk strings after "//_//" markers at offsets that
	// aren't multiples of four.
	legacy := strings.TrimSpace(string(fixture(t, "legacy_stream.txt")))

	tests := []struct {
		name       string
		in         string
		want       string
		wantLegacy bool
		wantErr    bool
	}{
		{"plain", plain, plain, false, false},
		{"plain with spaces around", "  " + plain + "\n", plain, false, false},
		{"bare link", "https://cdn.example/stream/a.mp4", "https://cdn.example/stream/a.mp4", false, false},
		{"legacy", legacy, plain, true, false},
		{"legacy without junk", "#h" + "WzQ4MHBdaHR0cHM6Ly9jZG4uZXhhbXBsZS9hLm1wNA==", "[480p]https://cdn.example/a.mp4", true, false},
		{"legacy, not base64", "#h!!not base64!!", "", true, true},
		{"unknown format", "data:abc", "", false, true},
		{"empty", "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, legacy, err := decodeStreamURL(tt.in)
			if tt.wantErr {
				var perr *ParseError
				if !errors.As(err, &perr) {
					t.Fatalf("error = %v, want a ParseError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeStreamURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("plain = %.80q…, want %.80q…", got, tt.want)
			}
			if legacy != tt.wantLegacy {
				t.Errorf("legacy = %v, want %v", legacy, tt.wantLegacy)
			}
		})
	}
}

func TestLegacyJunk(t *testing.T) {
	// 5² two-character and 5³ three-character combinations.
	if len(legacyJunk) != 25+125 {
		t.Errorf("legacyJunk has %d entries, want 150", len(legacyJunk))
	}
	for _, chunk := range []string{"IyM=", "QEBA", "JCQh", "Xl5e"} { // "##", "@@@", "$$!", "^^^"
		if !legacyJunk[chunk] {
			t.Errorf("legacyJunk lacks %q", chunk)
		}
	}
}

func TestParseQualities(t *testing.T) {
	got := parseQualities(fixtureURL(t, "cdn_get_stream.json"))
	want := []Quality{
		{Label: "360p", Resolution: 360, MP4: "https://cdn.example/stream/47zhg.mp4", HLS: "https://cdn.example/stream/47zhg.mp4:hls:manifest.m3u8"},
		{Label: "480p", Resolution: 480, MP4: "https://cdn.example/stream/gr10m.mp4", HLS: "https://cdn.example/stream/gr10m.mp4:hls:manifest.m3u8"},
		{Label: "720p", Resolution: 720, MP4: "https://cdn.example/stream/1hid4.mp4", HLS: "https://cdn.example/stream/1hid4.mp4:hls:manifest.m3u8"},
		{Label: "1080p", Resolution: 1080, MP4: "https://cdn.example/stream/4oiv2.mp4", HLS: "https://cdn.example/stream/4oiv2.mp4:hls:manifest.m3u8"},
		{Label: "1080p Ultra", Resolution: 1080, Premium: true, MP4: "https://cdn.example/stream/zrkms.mp4", HLS: "https://cdn.example/stream/zrkms.mp4:hls:manifest.m3u8"},
		{Label: "2K", Resolution: 1440, Premium: true, MP4: "https://cdn.example/stream/zrkms.mp4", HLS: "https://cdn.example/stream/zrkms.mp4:hls:manifest.m3u8"},
		{Label: "4K", Resolution: 2160, Premium: true, MP4: "https://cdn.example/stream/zrkms.mp4", HLS: "https://cdn.example/stream/zrkms.mp4:hls:manifest.m3u8"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseQualities =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseQualities_Links(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantMP4 string
		wantHLS string
	}{
		{"mp4 after hls", "[720p]https://c/a.mp4:hls:manifest.m3u8 or https://c/a.mp4", "https://c/a.mp4", "https://c/a.mp4:hls:manifest.m3u8"},
		{"mp4 first", "[720p]https://c/a.mp4 or https://c/a.mp4:hls:manifest.m3u8", "https://c/a.mp4", "https://c/a.mp4:hls:manifest.m3u8"},
		{"hls suffix only: mp4 derived", "[720p]https://c/a.mp4:hls:manifest.m3u8", "https://c/a.mp4", "https://c/a.mp4:hls:manifest.m3u8"},
		{"plain manifest only: no mp4", "[720p]https://c/a/index.m3u8", "", "https://c/a/index.m3u8"},
		{"mp4 only", "[720p]https://c/a.mp4", "https://c/a.mp4", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQualities(tt.in)
			if len(got) != 1 {
				t.Fatalf("got %d qualities, want 1", len(got))
			}
			if got[0].MP4 != tt.wantMP4 || got[0].HLS != tt.wantHLS {
				t.Errorf("MP4 %q HLS %q, want %q and %q", got[0].MP4, got[0].HLS, tt.wantMP4, tt.wantHLS)
			}
		})
	}
}

func TestStream_BestAndFind(t *testing.T) {
	s := &Stream{Qualities: parseQualities(fixtureURL(t, "cdn_get_stream.json"))}
	best, ok := s.Best()
	if !ok || best.Label != "1080p" {
		t.Errorf("Best = %q, %v; want 1080p (premium 2K and 4K skipped)", best.Label, ok)
	}
	if q, ok := s.Find("1080P ULTRA"); !ok || !q.Premium {
		t.Errorf("Find(1080P ULTRA) = %+v, %v; want the premium quality", q, ok)
	}
	if _, ok := s.Find("8K"); ok {
		t.Error("Find(8K) found a quality")
	}

	premiumOnly := &Stream{Qualities: []Quality{{Label: "4K", Resolution: 2160, Premium: true, MP4: "https://c/a.mp4"}}}
	if _, ok := premiumOnly.Best(); ok {
		t.Error("Best picked a premium quality")
	}
	hlsOnly := &Stream{Qualities: []Quality{{Label: "720p", Resolution: 720, HLS: "https://c/a/index.m3u8"}}}
	if _, ok := hlsOnly.Best(); ok {
		t.Error("Best picked a quality without an MP4")
	}
}

func TestResolution(t *testing.T) {
	tests := map[string]int{
		"360p": 360, "1080p": 1080, "1080p Ultra": 1080, "2K": 1440, "4K": 2160, "4k": 2160,
		"HD": 0, "1080": 0, "": 0, "p": 0,
	}
	for label, want := range tests {
		if got := Resolution(label); got != want {
			t.Errorf("Resolution(%q) = %d, want %d", label, got, want)
		}
	}
}

func TestStripTags(t *testing.T) {
	in := `<span class="pjs-prem-quality">1080p Ultra<img src="https://static.hdrezka.ac/prem-icon.svg" alt=""></span>`
	if got := stripTags(in); got != "1080p Ultra" {
		t.Errorf("stripTags = %q", got)
	}
}

func TestParseSubtitles(t *testing.T) {
	codes := map[string]string{"откл.": "", "Русский": "ru", "Українська": "ua", "English": "en"}
	tests := []struct {
		name  string
		raw   string
		codes map[string]string
		want  []Subtitle
	}{
		{
			"with codes; ua becomes uk",
			"[Русский]https://cdn.example/sub/ru.vtt,[Українська]https://cdn.example/sub/uk.vtt,[English]https://cdn.example/sub/en.vtt",
			codes,
			[]Subtitle{
				{Language: "Русский", Code: "ru", URL: "https://cdn.example/sub/ru.vtt"},
				{Language: "Українська", Code: "uk", URL: "https://cdn.example/sub/uk.vtt"},
				{Language: "English", Code: "en", URL: "https://cdn.example/sub/en.vtt"},
			},
		},
		{
			"subtitle_lns false",
			"[English]https://cdn.example/sub/en.vtt",
			nil,
			[]Subtitle{{Language: "English", URL: "https://cdn.example/sub/en.vtt"}},
		},
		{
			"alternatives: the last wins",
			"[English]https://a.example/en.vtt or https://b.example/en.vtt",
			codes,
			[]Subtitle{{Language: "English", Code: "en", URL: "https://b.example/en.vtt"}},
		},
		{"none", "", codes, nil},
		{"entry without a link", "[English]", codes, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubtitles(tt.raw, tt.codes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSubtitles =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}
