package search

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// testKey is the fake Jackett's API key. No response may contain it.
const testKey = "s5-test-key-0123456789abcdef"

// testNow is the tests' "now": Frieren's second cour has aired.
var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// frierenContext is Frieren's search context as TitleContexts builds it
// for a show outside the library: TMDB S01 has 38 episodes, and releases
// call E29–E38 "S02".
func frierenContext(t *testing.T) *service.TitleContext {
	return showContext(fixture(t, "frieren"), 209867, "tt22248376", 424536, "Sousou no Frieren")
}

func perfectDaysContext() *service.TitleContext {
	return &service.TitleContext{
		MediaType: library.JobMovie, TMDBID: 976893, Title: "Perfect Days", Year: 2023,
		IMDbID: "tt27503384", Runtime: 124,
	}
}

func mustPlan(t *testing.T, tc *service.TitleContext, req Request) *QueryPlan {
	t.Helper()
	p, err := Plan(tc, req)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// textQuery and idQuery are a plan's primary text query and its ID query.
func textQuery(p *QueryPlan) Query { return p.Queries[0] }

func idQuery(t *testing.T, p *QueryPlan) Query {
	t.Helper()
	for _, q := range p.Queries {
		if q.IsID() {
			return q
		}
	}
	t.Fatal("the plan has no ID query")
	return Query{}
}

// hashOf is a fake info hash for a title.
func hashOf(title string) string {
	sum := sha1.Sum([]byte(title))
	return hex.EncodeToString(sum[:])
}

// result is a Torznab result with a magnet, 100 seeders and 1.5 GB, from
// indexer "alpha"; opts change it.
func result(title string, opts ...func(*torznab.Result)) torznab.Result {
	h := hashOf(title)
	r := torznab.Result{
		Title: title, GUID: "https://alpha.test/t/" + h[:8],
		Link:      "http://jackett.test/dl/alpha/?jackett_apikey=" + testKey + "&file=" + url.QueryEscape(title),
		MagnetURI: "magnet:?xt=urn:btih:" + h + "&dn=" + url.QueryEscape(title),
		InfoHash:  h, Size: 1_500_000_000, Seeders: 100, Leechers: 5,
		IndexerID: "alpha", IndexerName: "Alpha",
	}
	for _, o := range opts {
		o(&r)
	}
	return r
}

func seeders(n int) func(*torznab.Result) { return func(r *torznab.Result) { r.Seeders = n } }
func size(n int64) func(*torznab.Result)  { return func(r *torznab.Result) { r.Size = n } }
func guid(g string) func(*torznab.Result) { return func(r *torznab.Result) { r.GUID = g } }
func noMagnet() func(*torznab.Result) {
	return func(r *torznab.Result) { r.MagnetURI, r.InfoHash = "", "" }
}
func hashOnly() func(*torznab.Result) { return func(r *torznab.Result) { r.MagnetURI = "" } }
func infoHash(h string) func(*torznab.Result) {
	return func(r *torznab.Result) { r.InfoHash, r.MagnetURI = h, "magnet:?xt=urn:btih:"+h }
}
func indexer(id, name string) func(*torznab.Result) {
	return func(r *torznab.Result) { r.IndexerID, r.IndexerName = id, name }
}
func published(t time.Time) func(*torznab.Result) {
	return func(r *torznab.Result) { r.PublishedAt = t }
}

// mustIngest ingests r for q and fails when it's dropped.
func mustIngest(t *testing.T, in *ingester, r torznab.Result, q Query) *item {
	t.Helper()
	it, ok := in.ingest(r, q)
	if !ok {
		t.Fatalf("%q was dropped", r.Title)
	}
	return it
}

// fakeClock is a settable Now.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: testNow} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeTitles serves one title context.
type fakeTitles struct{ tc *service.TitleContext }

func (f fakeTitles) Get(_ context.Context, mt library.JobMediaType, id int) (*service.TitleContext, error) {
	if mt != f.tc.MediaType || id != f.tc.TMDBID {
		return nil, fmt.Errorf("%w: no fixture for %s %d", service.ErrTitleNotFound, mt, id)
	}
	return f.tc, nil
}

type fakeProfiles struct {
	p   library.QualityProfile
	err error
}

func (f fakeProfiles) QualityProfile(context.Context) (library.QualityProfile, error) {
	return f.p, f.err
}

// fakeIndexer is one indexer of the fake Jackett; serve answers its
// searches.
type fakeIndexer struct {
	id, name string
	serve    http.HandlerFunc
}

// fakeJackett serves the Torznab indexer list, each indexer's searches
// and /dl/ downloads, counting requests.
type fakeJackett struct {
	srv      *httptest.Server
	indexers []fakeIndexer

	mu       sync.Mutex
	list     http.HandlerFunc // replaces the indexer list when set
	dl       http.HandlerFunc
	searches map[string]int
	dlHits   int
}

func newFakeJackett(t *testing.T, indexers ...fakeIndexer) *fakeJackett {
	t.Helper()
	j := &fakeJackett{indexers: indexers, searches: map[string]int{}}
	j.srv = httptest.NewServer(http.HandlerFunc(j.serveHTTP))
	t.Cleanup(j.srv.Close)
	return j
}

const torznabPrefix = "/api/v2.0/indexers/"

func (j *fakeJackett) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/dl/") {
		j.mu.Lock()
		j.dlHits++
		dl := j.dl
		j.mu.Unlock()
		if dl == nil {
			http.NotFound(w, r)
			return
		}
		dl(w, r)
		return
	}
	if r.URL.Query().Get("apikey") != testKey {
		_, _ = io.WriteString(w, `<error code="100" description="Invalid API Key"/>`)
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, torznabPrefix), "/results/torznab/api")
	if id == "all" {
		j.mu.Lock()
		list := j.list
		j.mu.Unlock()
		if list != nil {
			list(w, r)
			return
		}
		_, _ = io.WriteString(w, indexersXML(j.indexers))
		return
	}
	for _, ix := range j.indexers {
		if ix.id == id {
			j.mu.Lock()
			j.searches[id]++
			j.mu.Unlock()
			ix.serve(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, `{"result":"error","error":"Unknown indexer"}`)
}

func (j *fakeJackett) setList(h http.HandlerFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.list = h
}

func (j *fakeJackett) setDL(h http.HandlerFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.dl = h
}

// total is the search requests so far, over every indexer.
func (j *fakeJackett) total() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, c := range j.searches {
		n += c
	}
	return n
}

func (j *fakeJackett) downloads() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.dlHits
}

// indexersXML is a t=indexers answer: every indexer searches by text, TV
// takes season, episode, IMDb and TVDB ids, and movies IMDb ids.
func indexersXML(ixs []fakeIndexer) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><indexers>`)
	for _, ix := range ixs {
		fmt.Fprintf(&b, `<indexer id="%s" configured="true"><title>%s</title><type>public</type><caps>`+
			`<limits default="100" max="100"/><searching>`+
			`<search available="yes" supportedParams="q"/>`+
			`<tv-search available="yes" supportedParams="q,season,ep,imdbid,tvdbid"/>`+
			`<movie-search available="yes" supportedParams="q,imdbid"/></searching>`+
			`<categories><category id="2000" name="Movies"/><category id="5000" name="TV"><subcat id="5070" name="TV/Anime"/></category></categories>`+
			`</caps></indexer>`, ix.id, esc(ix.name))
	}
	b.WriteString(`</indexers>`)
	return b.String()
}

// feedItem is one item of a fake Torznab feed. Empty fields are left out.
type feedItem struct {
	title, guid, link, magnet, hash, cover string
	size                                   int64
	seeders                                int
	category                               int
	pub                                    time.Time
}

// feed is a Torznab search answer.
func feed(items ...feedItem) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><title>fake</title>`)
	tag := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "<%s>%s</%s>", name, esc(value), name)
		}
	}
	attr := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, `<torznab:attr name="%s" value="%s"/>`, name, esc(value))
		}
	}
	for _, it := range items {
		b.WriteString("<item>")
		tag("title", it.title)
		tag("guid", it.guid)
		tag("link", it.link)
		if it.size > 0 {
			tag("size", strconv.FormatInt(it.size, 10))
		}
		if !it.pub.IsZero() {
			tag("pubDate", it.pub.Format(time.RFC1123Z))
		}
		cat := it.category
		if cat == 0 {
			cat = 5070
		}
		tag("category", strconv.Itoa(cat))
		attr("seeders", strconv.Itoa(it.seeders))
		attr("peers", strconv.Itoa(it.seeders+3))
		attr("infohash", it.hash)
		attr("magneturl", it.magnet)
		attr("coverurl", it.cover)
		b.WriteString("</item>")
	}
	b.WriteString("</channel></rss>")
	return b.String()
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// serveFeed answers every search with items.
func serveFeed(items ...feedItem) http.HandlerFunc {
	body := feed(items...)
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }
}

// torrentFile is a valid single-file .torrent named name, announcing to
// tracker; its info hash is the second value.
func torrentFile(t *testing.T, name, tracker string) ([]byte, string) {
	t.Helper()
	info := metainfo.Info{Name: name, PieceLength: 16384, Pieces: make([]byte, 20), Length: 1000}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: infoBytes, Announce: tracker}
	var b bytes.Buffer
	if err := mi.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), mi.HashInfoBytes().HexString()
}

// newTestService is a Service on the fake Jackett for one title, with
// clock as its Now and the default quality profile.
func newTestService(t *testing.T, j *fakeJackett, tc *service.TitleContext, clock *fakeClock, wait time.Duration, opts ...torznab.Option) *Service {
	t.Helper()
	opts = append([]torznab.Option{torznab.WithLogger(discard)}, opts...)
	return New(Deps{
		Titles:   fakeTitles{tc: tc},
		Jackett:  torznab.New(j.srv.URL, testKey, opts...),
		Profiles: fakeProfiles{p: library.DefaultQualityProfile()},
		Now:      clock.Now,
		Log:      discard,
		Wait:     wait,
	})
}

// searchUntilComplete polls like the client until the search completes.
func searchUntilComplete(t *testing.T, s *Service, req SearchRequest) *SearchResponse {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := s.Search(context.Background(), req)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if resp.Complete {
			return resp
		}
		if time.Now().After(deadline) {
			t.Fatalf("the search never completed: %+v", resp.Indexers)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// releaseNamed is the response's release whose raw title is title.
func releaseNamed(t *testing.T, resp *SearchResponse, title string) Release {
	t.Helper()
	for _, r := range resp.Items {
		if r.RawTitle == title {
			return r
		}
	}
	t.Fatalf("no release %q", title)
	return Release{}
}

func indexerNamed(t *testing.T, resp *SearchResponse, name string) IndexerReport {
	t.Helper()
	for _, r := range resp.Indexers {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no indexer %q in %+v", name, resp.Indexers)
	return IndexerReport{}
}
