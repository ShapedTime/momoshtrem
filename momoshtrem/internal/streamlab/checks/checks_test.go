package checks_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// Matching bytes pass; each wrong byte counts, the first is reported, and
// bytes past the file's end are wrong.
func TestIntegrity(t *testing.T) {
	l := media.MustNew(media.EpisodePack("Show", 1, 2, 5, 16<<10, 50_000, media.Raw))
	f := l.Files()[1]
	good := make([]byte, 1000)
	if _, err := l.ReadAt(good, f.Offset+300); err != nil {
		t.Fatal(err)
	}
	c := checks.NewIntegrity(l)
	c.Check(1, 300, good)
	if r := c.Result(); r.Err() != nil || r.Checked != 1000 || r.Mismatched != 0 {
		t.Fatalf("matching bytes: %+v, %v", r, r.Err())
	}

	bad := append([]byte(nil), good...)
	bad[10] ^= 0xff
	bad[20] ^= 0xff
	c.Check(1, 300, bad)
	r := c.Result()
	if r.Mismatched != 2 || r.First == nil || r.First.Offset != 310 || r.First.Torrent != f.Offset+310 ||
		r.First.Piece != int((f.Offset+310)/l.PieceLength()) || r.First.Want != good[10] || r.First.Got != bad[10] {
		t.Fatalf("result = %+v, first = %+v; want 2 mismatches from file offset 310", r, r.First)
	}
	if err := r.Err(); err == nil || !strings.Contains(err.Error(), "2 of 2000 bytes") {
		t.Errorf("Err = %v", err)
	}

	tail := make([]byte, 10)
	if _, err := l.ReadAt(tail[:5], f.Offset+f.Size-5); err != nil {
		t.Fatal(err)
	}
	c.Check(1, f.Size-5, tail)
	if r := c.Result(); r.Mismatched != 2+5 {
		t.Errorf("past the end: %d mismatches, want 7", r.Mismatched)
	}
}

func TestRangeResponse(t *testing.T) {
	resp := func(status int, cr, cl string) *http.Response {
		h := http.Header{}
		h.Set("Content-Range", cr)
		h.Set("Content-Length", cl)
		return &http.Response{StatusCode: status, Header: h}
	}
	tests := []struct {
		name string
		resp *http.Response
		ok   bool
	}{
		{"right", resp(206, "bytes 100-199/1000", "100"), true},
		{"whole file", resp(200, "", "1000"), false},
		{"wrong range", resp(206, "bytes 0-199/1000", "200"), false},
		{"wrong length", resp(206, "bytes 100-199/1000", "99"), false},
		{"no response", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checks.RangeResponse(tt.resp, 100, 199, 1000); (err == nil) != tt.ok {
				t.Errorf("RangeResponse = %v, want ok %v", err, tt.ok)
			}
		})
	}
}

// A torrent loaded before the PROPFIND makes the probe inconclusive, and
// it says so rather than "no torrent loaded".
func TestF3Result_String(t *testing.T) {
	tests := []struct {
		r    checks.F3Result
		want string
	}{
		{checks.F3Result{Dir: "/d", Entries: 2, Loaded: []string{"aa"}}, "torrent loaded by PROPFIND (Depth 1 of /d, 2 entries): aa"},
		{checks.F3Result{Dir: "/d", Entries: 2, Before: []string{"aa"}}, "already loaded before the PROPFIND (Depth 1 of /d, 2 entries): aa"},
		{checks.F3Result{Dir: "/d", Entries: 2}, "no torrent loaded by PROPFIND (Depth 1 of /d, 2 entries)"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("String = %q, want %q", got, tt.want)
		}
	}
}

func TestPanics(t *testing.T) {
	logged := []string{
		"http: superfluous response.WriteHeader call from x",
		"http: panic serving 127.0.0.1:1234: boom\ngoroutine 7 [running]:",
	}
	if got := checks.Panics(logged); len(got) != 1 || !strings.Contains(got[0], "boom") {
		t.Errorf("Panics = %q, want the panic", got)
	}
}
