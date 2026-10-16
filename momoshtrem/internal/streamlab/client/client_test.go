package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A PROPFIND answer as x/net/webdav writes it: escaped hrefs, the DAV:
// namespace, and properties a resource lacks in a 404 propstat.
func TestParseMultistatus(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:">
<D:response><D:href>/Movies/Film%20%282020%29/</D:href><D:propstat><D:prop>
<D:resourcetype><D:collection xmlns:D="DAV:"/></D:resourcetype><D:displayname>Film (2020)</D:displayname>
</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>
<D:response><D:href>/Movies/Film%20%282020%29/Film%20%282020%29.mkv</D:href><D:propstat><D:prop>
<D:resourcetype></D:resourcetype><D:getcontentlength>16777216</D:getcontentlength>
<D:getcontenttype>video/x-matroska</D:getcontenttype><D:getetag>"abc"</D:getetag>
</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>
<D:propstat><D:prop><D:getcontentlength>1</D:getcontentlength></D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>
</D:response></D:multistatus>`
	got, err := parseMultistatus([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Path: "/Movies/Film (2020)/", IsDir: true},
		{Path: "/Movies/Film (2020)/Film (2020).mkv", Size: 16 << 20, ContentType: "video/x-matroska", ETag: `"abc"`},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("entries = %+v\nwant %+v", got, want)
	}
}

// Play sends an open-ended range with the device's User-Agent; a Stream
// tracks its position, and reads after Abort fail with ErrAborted.
func TestPlay(t *testing.T) {
	const size = 1 << 20
	var gotRange, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange, gotUA = r.Header.Get("Range"), r.UserAgent()
		w.WriteHeader(http.StatusPartialContent)
		w.Write(make([]byte, size)) // more than the client reads
		<-r.Context().Done()        // until the client drops the connection
	}))
	defer srv.Close()

	conn := Device{BaseURL: srv.URL, UserAgent: "streamlab-test/1"}.Conn()
	defer conn.Close()
	st, err := conn.Play(context.Background(), "/Movies/A B.mkv", 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(st, make([]byte, 1000)); err != nil {
		t.Fatal(err)
	}
	if gotRange != "bytes=100-" || gotUA != "streamlab-test/1" || st.Pos() != 1100 {
		t.Errorf("Range %q, User-Agent %q, Pos %d; want bytes=100-, streamlab-test/1, 1100", gotRange, gotUA, st.Pos())
	}
	st.Abort()
	if _, err := st.Read(make([]byte, 10)); !errors.Is(err, ErrAborted) {
		t.Errorf("Read after Abort = %v, want ErrAborted", err)
	}
}

// Probe asks for exactly n bytes.
func TestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.bin", time.Time{}, strings.NewReader(strings.Repeat("0123456789", 100)))
	}))
	defer srv.Close()
	conn := Device{BaseURL: srv.URL}.Conn()
	defer conn.Close()
	resp, body, err := conn.Probe(context.Background(), "/x.bin", 990, 10)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "0123456789" {
		t.Errorf("Probe = %d %q", resp.StatusCode, body)
	}
}
