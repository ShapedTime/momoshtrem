package stack_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/streamlab/cdn"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// An HDRezka movie end to end: the fake CDN behind production's streamer
// and HDRezkaSource, over real WebDAV. Browsing makes no CDN request,
// every byte played is right, and once the player is done no CDN body or
// streamer slot is left.
func TestLab_HDRezka(t *testing.T) {
	layout := media.MustNew(media.SingleFile("Interstellar.2014.1080p.mp4", 51, 256<<10, 6<<20+99, media.MP4))
	ref := library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		Title: "Interstellar", TranslatorID: 238, Quality: "1080p"}
	c := cdn.Start()
	defer c.Close()
	sf := service.StreamFile(ref)
	c.Add(layout, 0, sf.Key, sf.Quality)
	links := cdn.NewLinks(c)

	m := stack.Movie{Title: "Interstellar", Year: 2014, Source: stack.Source{Layout: layout, Stream: &ref}}
	s, err := stack.Start(stack.Options{Movies: []stack.Movie{m}, HDRezka: links, HDRezkaReadTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.HDRezka == nil {
		t.Fatal("the stack has no HDRezka streamer")
	}
	ctx := context.Background()
	f := layout.Files()[0]
	integrity := checks.NewIntegrity(layout)
	dev := client.Device{BaseURL: s.URL, UserAgent: "Infuse-Direct/7.7 (streamlab hdrezka)"}
	conn := dev.Conn()
	defer conn.Close()

	entries, err := conn.Propfind(ctx, m.Dir(), 1)
	if err != nil || len(entries) != 2 {
		t.Fatalf("PROPFIND %s = %+v, %v", m.Dir(), entries, err)
	}
	resp, err := conn.Head(ctx, m.Path())
	if err != nil || resp.StatusCode != http.StatusOK || resp.ContentLength != f.Size {
		t.Fatalf("HEAD = %v, %v", resp, err)
	}
	if st := c.Stats(); st.Requests != 0 {
		t.Errorf("browsing made %d CDN requests, want none", st.Requests)
	}

	r, body, err := conn.Probe(ctx, m.Path(), f.Size-64<<10, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if err := checks.RangeResponse(r, f.Size-64<<10, f.Size-1, f.Size); err != nil {
		t.Error(err)
	}
	integrity.Check(0, f.Size-64<<10, body)

	st, err := conn.Play(ctx, m.Path(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := checks.RangeResponse(st.Response, 1<<20, f.Size-1, f.Size); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32<<10)
	for off := int64(1 << 20); ; {
		n, err := st.Read(buf)
		integrity.Check(0, off, buf[:n])
		off += int64(n)
		if err == io.EOF {
			if off != f.Size {
				t.Fatalf("the play ended at %d of %d", off, f.Size)
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	conn.Close()

	if err := integrity.Result().Err(); err != nil {
		t.Error(err)
	}
	if q := checks.Quiescence(ctx, s, 10*time.Second); q.Err() != nil {
		t.Error(q.Err())
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Open() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := c.Open(); n != 0 {
		t.Errorf("%d CDN bodies left open", n)
	}
	if n := links.FreeSlots(s.HDRezka, cdn.StreamerSlots, 2*time.Second); n != cdn.StreamerSlots {
		t.Errorf("%d of the streamer's %d slots free", n, cdn.StreamerSlots)
	}
	if st := c.Stats(); st.Bodies != 2 {
		t.Errorf("CDN stats %+v: want one body per GET", st)
	}
	if st := links.Stats(); st.Resolves != 1 || st.Invalidations != 0 {
		t.Errorf("link stats %+v", st)
	}
	var _ hdrezka.LinkResolver = links
}
