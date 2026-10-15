package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestParseProbeFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOK   bool
		wantCode int
		want     probeFlags
	}{
		{"query only", []string{"interstellar"}, true, 0, probeFlags{query: "interstellar"}},
		{"flags first", []string{"-season", "1", "-episode", "2", "breaking bad"}, true, 0, probeFlags{query: "breaking bad", season: 1, episode: 2}},
		{"flags after the query", []string{"breaking bad", "-season", "1", "-episode", "1"}, true, 0, probeFlags{query: "breaking bad", season: 1, episode: 1}},
		{"unquoted words join", []string{"breaking", "bad", "-quality", "720p"}, true, 0, probeFlags{query: "breaking bad", quality: "720p"}},
		{
			"everything",
			[]string{"--translator=238", "--quality", "1080p", "--show-urls", "interstellar"},
			true, 0,
			probeFlags{query: "interstellar", translator: 238, quality: "1080p", showURLs: true},
		},
		{"help", []string{"-h"}, false, 0, probeFlags{}},
		{"no query", []string{"-season", "1"}, false, 2, probeFlags{}},
		{"blank query", []string{"  "}, false, 2, probeFlags{}},
		{"unknown flag", []string{"--everything", "x"}, false, 2, probeFlags{}},
		{"negative episode", []string{"-episode", "-1", "x"}, false, 2, probeFlags{}},
		{"not a number", []string{"-translator", "original", "x"}, false, 2, probeFlags{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, code, ok := parseProbeFlags(tt.args)
			if ok != tt.wantOK || code != tt.wantCode {
				t.Fatalf("ok %v code %d, want ok %v code %d", ok, code, tt.wantOK, tt.wantCode)
			}
			if ok && got != tt.want {
				t.Errorf("flags = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// box builds a top-level MP4 box of size bytes (header included).
func box(typ string, size int) []byte {
	b := make([]byte, size)
	binary.BigEndian.PutUint32(b, uint32(size))
	copy(b[4:8], typ)
	return b
}

// largeBox builds the header of a box with a 64-bit size.
func largeBox(typ string, size int64) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b, 1)
	copy(b[4:8], typ)
	binary.BigEndian.PutUint64(b[8:], uint64(size))
	return b
}

// readerOver serves readAt from data, reporting a file of size total.
func readerOver(data []byte) func(off, n int64) ([]byte, error) {
	return func(off, n int64) ([]byte, error) {
		end := min(off+n, int64(len(data)))
		return data[off:end], nil
	}
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestScanBoxes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		total    int64 // 0: len(data)
		wantFast bool
		wantDesc string
		wantErr  string
	}{
		{
			"faststart",
			join(box("ftyp", 32), box("moov", 100), box("mdat", 500)), 0,
			true, "moov before mdat: ftyp at 0, moov at 32", "",
		},
		{
			"moov at the end",
			join(box("ftyp", 24), box("free", 8), box("mdat", 200), box("moov", 64)), 0,
			false, "mdat before moov: ftyp at 0, free at 24, mdat at 32", "",
		},
		{
			"64-bit mdat size",
			join(box("ftyp", 24), largeBox("mdat", 5_000_000_000)), 5_000_000_024,
			false, "mdat before moov: ftyp at 0, mdat at 24", "",
		},
		{
			"size 0 runs to the end",
			join(box("ftyp", 24), box("moov", 40), []byte{0, 0, 0, 0, 'm', 'd', 'a', 't'}), 0,
			true, "moov before mdat: ftyp at 0, moov at 24", "",
		},
		{"not an MP4", []byte("<html><body>teaser</body></html>"), 0, false, "", "not an MP4"},
		{"box larger than the file", join(box("ftyp", 24), largeBox("mdat", 1000)), 100, false, "", "invalid size"},
		{"no moov or mdat", join(box("ftyp", 16), box("free", 16)), 0, false, "", "no moov or mdat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := tt.total
			if total == 0 {
				total = int64(len(tt.data))
			}
			boxes, err := scanBoxes(readerOver(tt.data), total)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one about %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanBoxes: %v", err)
			}
			fast, desc := faststart(boxes)
			if fast != tt.wantFast || desc != tt.wantDesc {
				t.Errorf("faststart = %v, %q; want %v, %q", fast, desc, tt.wantFast, tt.wantDesc)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	link := "https://stream.voidboost.one/89d0832:2026101618:dlFOU2k3VzR6==/5/8/4/2/6/g3l26.mp4"
	if got := redact(link, false); got != "https://stream.voidboost.one/…/g3l26.mp4" {
		t.Errorf("redact = %q", got)
	}
	if got := redact(link, true); got != link {
		t.Errorf("redact with -show-urls = %q", got)
	}
	if got := redact("::", false); got != "(link hidden)" {
		t.Errorf("redact of a bad link = %q", got)
	}
}

func TestFormatSize(t *testing.T) {
	tests := map[int64]string{
		3322740623: "3,322,740,623 bytes (3.09 GiB)",
		7949309:    "7,949,309 bytes (7.58 MiB)",
		512:        "512 bytes (512.00 B)",
	}
	for n, want := range tests {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}
