package vfs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

// Over WebDAV, end to end: listings, HEADs and sniffs make no session; a
// playback's Range GETs make one per client.
func TestWebDAV_StreamingSessions(t *testing.T) {
	ref := library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		TranslatorID: 238, Quality: "1080p"}
	data := make([]byte, 3<<20)
	src := &recordingSource{files: map[string][]byte{ref.FilePath(): data}}
	row := hdrezkaRow(ref, int64(len(data)))
	row.ItemType, row.ItemID = library.ItemTypeMovie, 7
	fs := vfs.NewTestFS([]vfs.TestMovie{{Title: "Interstellar", Year: 2014, Assignment: row}}, nil)
	fs.SetRemoteSource(src, 0)
	reg := sessions.NewRegistry()
	fs.SetSessions(reg)
	srv := httptest.NewServer(webdav.NewServer(fs, config.WebDAVAuthConfig{}).Handler())
	defer srv.Close()

	infuse := map[string]string{"User-Agent": "Infuse-Direct/7.7 (AppleTV)"}
	with := func(h map[string]string, k, v string) map[string]string {
		out := map[string]string{k: v}
		for hk, hv := range h {
			out[hk] = hv
		}
		return out
	}

	davRequest(t, "PROPFIND", srv.URL+escape("/Movies/Interstellar (2014)/"), with(infuse, "Depth", "1"))
	davRequest(t, http.MethodHead, srv.URL+escape(moviePath), infuse)
	// Sniffs: the header and the index at the end, a few KB each.
	davRequest(t, http.MethodGet, srv.URL+escape(moviePath), with(infuse, "Range", "bytes=0-4095"))
	davRequest(t, http.MethodGet, srv.URL+escape(moviePath), with(infuse, "Range", "bytes="+strconv.Itoa(len(data)-5000)+"-"))
	if n := reg.Count(); n != 0 {
		t.Fatalf("sessions after a listing, a HEAD and two sniffs = %d, want 0", n)
	}

	// Playback: two Range GETs, one session.
	davRequest(t, http.MethodGet, srv.URL+escape(moviePath), with(infuse, "Range", "bytes=0-1048575"))
	resp, _ := davRequest(t, http.MethodGet, srv.URL+escape(moviePath), with(infuse, "Range", "bytes=1048576-"))
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	got := reg.Sessions(context.Background())
	if len(got) != 1 {
		t.Fatalf("sessions = %+v, want one", got)
	}
	s := got[0]
	if s.Source != sessions.SourceHDRezka || s.FileName != "Interstellar (2014).mp4" || s.OffsetBytes != int64(len(data)) ||
		s.SizeBytes != int64(len(data)) || s.Client == nil || *s.Client != "Infuse" || s.Device == nil || *s.Device != "Apple TV" {
		t.Errorf("session = %+v", s)
	}

	// VLC on the same file is its own session.
	davRequest(t, http.MethodGet, srv.URL+escape(moviePath), map[string]string{"User-Agent": "VLC/3.0.20 LibVLC/3.0.20"})
	if n := reg.Count(); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
	// Opening outside a request (no context) tracks nothing.
	f, err := fs.Open(moviePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.ReadAt(make([]byte, 1<<20), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if n := reg.Count(); n != 2 {
		t.Errorf("sessions = %d after a read outside WebDAV, want 2", n)
	}
}
