package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

func codeOf(err error) string {
	var he *HDRezkaError
	if errors.As(err, &he) {
		return he.Code
	}
	return ""
}

func boolPtr(b bool) *bool { return &b }

func intRef(n int) *int { return &n }

func TestHDRezkaSearch(t *testing.T) {
	interstellar := hdrezka.SearchResult{URL: "https://rezka.ag" + interstellarPath, Path: interstellarPath, PostID: 2259,
		Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014, Category: "films", Kind: hdrezka.KindMovie, Rating: 8.6}
	wars := hdrezka.SearchResult{Path: "/films/fiction/22194-mezhzvezdnye-voyny-2016.html", Title: "Межзвездные войны",
		OriginalTitle: "Interstellar Wars", Year: 2016, Category: "films", Kind: hdrezka.KindMovie}
	cartoon := hdrezka.SearchResult{Path: "/cartoons/fiction/92343-kosmicheskaya-ella-2022.html", Title: "Космическая Элла",
		Category: "cartoons"}

	e := newHDRezkaEnv()
	e.site.quick = map[string][]hdrezka.SearchResult{"Interstellar": {wars, cartoon, interstellar}}
	got, err := e.svc.Search(context.Background(), "movie", interstellarTMDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != "Interstellar" || len(got.Items) != 3 || e.site.called("quick:") != 1 || e.site.called("full:") != 0 {
		t.Fatalf("search = %+v, calls %v", got, e.site.calls)
	}
	first := got.Items[0]
	want := HDRezkaSearchItem{URL: "https://rezka.example" + interstellarPath, Path: interstellarPath, Title: "Интерстеллар",
		OriginalTitle: nonEmpty("Interstellar"), Year: intRef(2014), Kind: nonEmpty("movie"), Category: "films",
		Rating: positiveFloat(8.6), Match: "exact"}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("best = %+v, want %+v", first, want)
	}
	if last := got.Items[2]; last.Kind != nil || last.Year != nil || last.OriginalTitle != nil || last.Rating != nil || last.Match != "weak" {
		t.Errorf("a hit with nothing known = %+v", last)
	}

	// The original title finds nothing, quick or full: the title is next.
	e = newHDRezkaEnv()
	e.tmdb.shows[breakingBadTMDB].OriginalName = "Breaking Bad (US)"
	e.site.full = map[string][]hdrezka.SearchResult{"Breaking Bad": {{Path: breakingBadPath, Title: "Во все тяжкие",
		Year: 2008, Category: "series", Kind: hdrezka.KindSeries}}}
	got, err = e.svc.Search(context.Background(), "tv", breakingBadTMDB, "")
	if err != nil || got.Query != "Breaking Bad" || len(got.Items) != 1 {
		t.Fatalf("search = %+v, %v", got, err)
	}
	if calls := strings.Join(e.site.calls, ","); calls != "quick:Breaking Bad (US),full:Breaking Bad (US),quick:Breaking Bad,full:Breaking Bad" {
		t.Errorf("calls = %s", calls)
	}

	// q replaces the TMDB queries; nothing found is an empty list.
	e = newHDRezkaEnv()
	got, err = e.svc.Search(context.Background(), "movie", interstellarTMDB, " Интерстеллар ")
	if err != nil || got.Query != "Интерстеллар" || got.Items == nil || len(got.Items) != 0 {
		t.Errorf("search with q = %+v, %v", got, err)
	}
}

func TestHDRezkaSearch_Errors(t *testing.T) {
	down := fmt.Errorf("%w: %w", library.ErrTMDBFetch, &tmdb.StatusError{StatusCode: 503})
	tests := []struct {
		name      string
		mediaType string
		tmdbID    int
		q         string
		tmdbErr   error
		siteErr   error
		wantCode  string
	}{
		{"unknown media type", "show", 1396, "", nil, nil, HDRezkaBadRequest},
		{"no tmdb id", "tv", 0, "", nil, nil, HDRezkaBadRequest},
		{"a long query", "tv", 1396, strings.Repeat("я", 201), nil, nil, HDRezkaBadRequest},
		{"TMDB down", "tv", 1396, "", down, nil, HDRezkaTMDBDown},
		{"no TMDB key", "tv", 1396, "", tmdb.ErrNoAPIKey, nil, HDRezkaOff},
		{"unknown to TMDB", "tv", 4242, "", nil, nil, HDRezkaNotFound},
		{"anti-bot page", "tv", 1396, "", nil, hdrezka.ErrChallenge, HDRezkaBlocked},
		{"site down", "tv", 1396, "", nil, &hdrezka.StatusError{StatusCode: 503}, HDRezkaUnavailable},
		{"site 403", "tv", 1396, "", nil, &hdrezka.StatusError{StatusCode: 403}, HDRezkaUnavailable},
		{"site timeout", "tv", 1396, "", nil, context.DeadlineExceeded, HDRezkaTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			e.tmdb.err, e.site.searchErr = tt.tmdbErr, tt.siteErr
			_, err := e.svc.Search(context.Background(), tt.mediaType, tt.tmdbID, tt.q)
			if got := codeOf(err); got != tt.wantCode {
				t.Errorf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
		})
	}

	// With q, a TMDB outage only costs the ranking.
	e := newHDRezkaEnv()
	e.tmdb.err = down
	e.site.quick = map[string][]hdrezka.SearchResult{"breaking bad": {{Path: breakingBadPath, Title: "Во все тяжкие",
		OriginalTitle: "Breaking Bad", Category: "series", Kind: hdrezka.KindSeries}}}
	got, err := e.svc.Search(context.Background(), "tv", breakingBadTMDB, "breaking bad")
	if err != nil || len(got.Items) != 1 || got.Items[0].Match != "exact" {
		t.Errorf("search with q while TMDB is down = %+v, %v", got, err)
	}
}

func TestHDRezkaTitle_Movie(t *testing.T) {
	e := newHDRezkaEnv()
	got, err := e.svc.Title(context.Background(), HDRezkaTitleRequest{
		URL: "https://evil.example" + interstellarPath + "?x=1#t", MovieID: 7})
	if err != nil {
		t.Fatal(err)
	}
	// Only the path is kept: the configured mirror is what's fetched.
	if e.site.calls[0] != "title:"+interstellarPath {
		t.Errorf("fetched %v", e.site.calls)
	}
	if got.URL != "https://rezka.example"+interstellarPath || got.PostID != 2259 || got.Kind != "movie" ||
		got.TranslatorID != 56 || len(got.Translators) != 4 || got.Seasons == nil || len(got.Seasons) != 0 ||
		*got.OriginalTitle != "Interstellar" || *got.Year != 2014 {
		t.Errorf("title = %+v", got)
	}
	if cut := got.Translators[1]; cut.ID != 56 || !cut.Director || cut.Default {
		t.Errorf("the director's cut = %+v", cut)
	}
	raw, _ := json.Marshal(got)
	for _, field := range []string{`"translator_id":56`, `"seasons":[]`, `"post_id":2259`, `"camrip":false`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("JSON lacks %s: %s", field, raw)
		}
	}
}

func TestHDRezkaTitle_SeriesAnnotated(t *testing.T) {
	e := newHDRezkaEnv()
	got, err := e.svc.Title(context.Background(), HDRezkaTitleRequest{URL: breakingBadPath, ShowID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.TranslatorID != 56 || len(got.Seasons) != 2 || len(got.Seasons[0].Episodes) != 7 || e.site.called("episodes:") != 0 {
		t.Fatalf("title = %+v (calls %v)", got, e.site.calls)
	}
	e1, e2, e3, e4 := got.Seasons[0].Episodes[0], got.Seasons[0].Episodes[1], got.Seasons[0].Episodes[2], got.Seasons[0].Episodes[3]
	hdrezkaSource, torrentSource := "hdrezka", "torrent"
	if l := e1.Library; l == nil || l.EpisodeID != 501 || l.Name != "Pilot" || *l.AirDate != "2008-01-20" || !l.HasFile ||
		*l.Source != hdrezkaSource || l.Stream == nil || l.Stream.Translator != "LostFilm" || l.Stream.Quality != "720p" {
		t.Errorf("E1 = %+v", e1.Library)
	}
	if l := e2.Library; l == nil || *l.Source != torrentSource || l.Stream != nil || l.AirDate != nil {
		t.Errorf("E2 = %+v", e2.Library)
	}
	if l := e3.Library; l == nil || l.HasFile || l.Source != nil {
		t.Errorf("E3 = %+v", e3.Library)
	}
	if e4.Library != nil || got.Seasons[1].Episodes[0].Library != nil {
		t.Errorf("episodes not in the library = %+v, %+v", e4.Library, got.Seasons[1].Episodes[0].Library)
	}

	// Another translator's seasons come from the CDN.
	got, err = e.svc.Title(context.Background(), HDRezkaTitleRequest{URL: breakingBadPath, TranslatorID: 238})
	if err != nil || got.TranslatorID != 238 || len(got.Seasons) != 2 || len(got.Seasons[0].Episodes) != 3 ||
		got.Seasons[0].Episodes[0].Library != nil || e.site.called("episodes:238") != 1 {
		t.Errorf("translator 238 = %+v, %v", got, err)
	}
}

func TestHDRezkaTitle_PremiumDefault(t *testing.T) {
	e := newHDRezkaEnv()
	movie := e.site.titles[interstellarPath]
	movie.Translators[0].Premium = true
	got, err := e.svc.Title(context.Background(), HDRezkaTitleRequest{URL: interstellarPath})
	if err != nil || got.TranslatorID != 56 || got.Translators[0].Premium != true {
		t.Fatalf("title = %+v, %v", got, err)
	}
	// The default's director's cut isn't premium: it's the pick.
	if tr, _ := pickTranslator(movie, 0, TranslatorCut{}); !tr.Director {
		t.Errorf("pick = %+v, want the first that isn't premium", tr)
	}
}

func TestHDRezkaTitle_Errors(t *testing.T) {
	tests := []struct {
		name     string
		req      HDRezkaTitleRequest
		siteErr  error
		wantCode string
		network  bool
	}{
		{"no url", HDRezkaTitleRequest{}, nil, HDRezkaBadRequest, false},
		{"not a title link", HDRezkaTitleRequest{URL: "https://rezka.ag/search/?q=x"}, nil, HDRezkaInvalid, false},
		{"show and movie", HDRezkaTitleRequest{URL: breakingBadPath, ShowID: 5, MovieID: 7}, nil, HDRezkaBadRequest, false},
		{"unknown show", HDRezkaTitleRequest{URL: breakingBadPath, ShowID: 77}, nil, HDRezkaNotFound, false},
		{"unknown movie", HDRezkaTitleRequest{URL: interstellarPath, MovieID: 77}, nil, HDRezkaNotFound, false},
		{"unknown translator", HDRezkaTitleRequest{URL: breakingBadPath, TranslatorID: 4}, nil, HDRezkaInvalid, true},
		{"title gone", HDRezkaTitleRequest{URL: "/films/x/1-gone.html"}, nil, HDRezkaNotFound, true},
		{"markup changed", HDRezkaTitleRequest{URL: breakingBadPath}, &hdrezka.ParseError{What: "player init call"}, HDRezkaUnavailable, true},
		{"anti-bot", HDRezkaTitleRequest{URL: breakingBadPath}, hdrezka.ErrChallenge, HDRezkaBlocked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			e.site.titleErr = tt.siteErr
			_, err := e.svc.Title(context.Background(), tt.req)
			if got := codeOf(err); got != tt.wantCode {
				t.Errorf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
			if (len(e.site.calls) > 0) != tt.network {
				t.Errorf("site calls = %v", e.site.calls)
			}
		})
	}
}

func TestHDRezkaStreams(t *testing.T) {
	e := newHDRezkaEnv()
	got, err := e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238})
	if err != nil {
		t.Fatal(err)
	}
	want := []HDRezkaQuality{
		{Label: "360p", Resolution: 360, Available: true, SizeBytes: positiveInt64(100_000)},
		{Label: "720p", Resolution: 720, Available: true, SizeBytes: positiveInt64(200_000)},
		{Label: "1080p", Resolution: 1080, Available: true, SizeBytes: positiveInt64(300_000)},
		{Label: "1080p Ultra", Resolution: 1080, Premium: true},
		{Label: "4K", Resolution: 2160, Premium: true},
	}
	if !reflect.DeepEqual(got.Qualities, want) {
		t.Errorf("qualities = %+v", got.Qualities)
	}
	if len(got.Subtitles) != 2 || got.Subtitles[0].LanguageCode != "en" || got.Subtitles[0].LanguageName != "English" ||
		got.Subtitles[1].LanguageCode != "" {
		t.Errorf("subtitles = %+v", got.Subtitles)
	}
	if e.site.called("size:") != 3 {
		t.Errorf("measured %d qualities, want the 3 that aren't premium", e.site.called("size:"))
	}

	// A series without an episode: the translator's first.
	e = newHDRezkaEnv()
	if _, err := e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: breakingBadPath, TranslatorID: 238}); err != nil {
		t.Fatal(err)
	}
	if k := e.streams.keys[0]; k.Season != 1 || k.Episode != 1 || k.TranslatorID != 238 || k.Ads {
		t.Errorf("key = %+v", k)
	}
	// With one, that one; the cut flags don't apply to a series.
	e = newHDRezkaEnv()
	_, err = e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: breakingBadPath, TranslatorID: 238,
		Cut: TranslatorCut{Ads: boolPtr(true)}, Season: intRef(2), Episode: intRef(1)})
	if k := e.streams.keys[0]; err != nil || k.Season != 2 || k.Episode != 1 || k.Ads || e.site.called("episodes:") != 0 {
		t.Errorf("key = %+v, %v", k, err)
	}
}

func TestHDRezkaStreams_Cuts(t *testing.T) {
	tests := []struct {
		name         string
		cut          TranslatorCut
		wantDirector bool
		wantCode     string
	}{
		{"no flags: the id's first entry", TranslatorCut{}, false, ""},
		{"the director's cut", TranslatorCut{Director: boolPtr(true)}, true, ""},
		{"all flags, plain", TranslatorCut{Camrip: boolPtr(false), Ads: boolPtr(false), Director: boolPtr(false)}, false, ""},
		{"no such cut", TranslatorCut{Camrip: boolPtr(true)}, false, HDRezkaInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			_, err := e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 56, Cut: tt.cut})
			if codeOf(err) != tt.wantCode {
				t.Fatalf("err = %v, want code %q", err, tt.wantCode)
			}
			if tt.wantCode == "" && e.streams.keys[0].Director != tt.wantDirector {
				t.Errorf("key = %+v", e.streams.keys[0])
			}
		})
	}
}

func TestHDRezkaStreams_Sizes(t *testing.T) {
	// A refused link (from an earlier resolve) is resolved again.
	e := newHDRezkaEnv()
	e.site.size = func(_ context.Context, link string) (int64, error) {
		if strings.HasSuffix(link, "v0.mp4") {
			return 0, hdrezka.ErrLinkExpired
		}
		return sizeOf(link), nil
	}
	got, err := e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238})
	if err != nil || e.streams.invalidated != 1 || got.Qualities[2].SizeBytes == nil || *got.Qualities[2].SizeBytes != 300_000 {
		t.Errorf("after a refusal: %+v, %v, invalidated %d", got, err, e.streams.invalidated)
	}

	// A size not in within the budget is null; the quality is still available.
	e = newHDRezkaEnv()
	e.svc.sizeBudget = 50 * time.Millisecond
	e.site.size = func(ctx context.Context, link string) (int64, error) {
		if strings.Contains(link, "/1080p/") {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		return sizeOf(link), nil
	}
	start := time.Now()
	got, err = e.svc.Streams(context.Background(), HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238})
	if err != nil || time.Since(start) > 2*time.Second {
		t.Fatalf("streams = %v after %v", err, time.Since(start))
	}
	if q := got.Qualities[2]; q.SizeBytes != nil || !q.Available || got.Qualities[1].SizeBytes == nil {
		t.Errorf("qualities = %+v", got.Qualities)
	}
}

func TestHDRezkaStreams_Errors(t *testing.T) {
	tests := []struct {
		name     string
		req      HDRezkaStreamsRequest
		resolve  error
		wantCode string
	}{
		{"no translator", HDRezkaStreamsRequest{URL: interstellarPath}, nil, HDRezkaBadRequest},
		{"season without episode", HDRezkaStreamsRequest{URL: breakingBadPath, TranslatorID: 238, Season: intRef(1)}, nil, HDRezkaBadRequest},
		{"episode 0", HDRezkaStreamsRequest{URL: breakingBadPath, TranslatorID: 238, Season: intRef(1), Episode: intRef(0)}, nil, HDRezkaBadRequest},
		{"premium translator", HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 111}, nil, HDRezkaPremiumOnly},
		{"premium content", HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238}, hdrezka.ErrPremium, HDRezkaPremiumOnly},
		{"HLS only", HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238}, hdrezka.ErrNoMP4, HDRezkaInvalid},
		{"CDN refusal", HDRezkaStreamsRequest{URL: interstellarPath, TranslatorID: 238}, &hdrezka.CDNError{Message: "Ошибка"}, HDRezkaUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			e.streams.errs = map[[2]int]error{{0, 0}: tt.resolve}
			_, err := e.svc.Streams(context.Background(), tt.req)
			if got := codeOf(err); got != tt.wantCode {
				t.Errorf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
		})
	}
}

func TestPickQuality(t *testing.T) {
	stream := testStream(hdrezka.StreamKey{PostID: 1}, 0)
	tests := []struct {
		ask  string
		want string
		err  error
	}{
		{"1080p", "1080p", nil},
		{"720p", "720p", nil},
		{"1080p Ultra", "1080p", nil}, // premium: the best one at its resolution
		{"4K", "1080p", nil},
		{"2K", "1080p", nil},
		{"480p", "360p", nil},
		{"240p", "", hdrezka.ErrNoQuality},
		{"best", "", hdrezka.ErrNoQuality},
	}
	for _, tt := range tests {
		q, err := pickQuality(stream, tt.ask)
		if q.Label != tt.want || !errors.Is(err, tt.err) || (tt.err == nil && err != nil) {
			t.Errorf("pickQuality(%q) = %q, %v; want %q, %v", tt.ask, q.Label, err, tt.want, tt.err)
		}
	}
	premiumOnly := &hdrezka.Stream{Qualities: []hdrezka.Quality{{Label: "4K", Resolution: 2160, Premium: true, MP4: "x"}}}
	if _, err := pickQuality(premiumOnly, "4K"); !errors.Is(err, hdrezka.ErrPremium) {
		t.Errorf("only premium = %v", err)
	}
}

func TestHDRezkaAssignMovie(t *testing.T) {
	e := newHDRezkaEnv()
	got, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: "https://hdrezka.me" + interstellarPath,
		TranslatorID: 56, Director: boolPtr(true), Quality: "4K"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Quality != "1080p" || got.SizeBytes != 300_000 {
		t.Errorf("assigned = %+v, want 1080p for the premium 4K", got)
	}
	if len(e.writer.writes) != 1 || e.tree.count() != 1 {
		t.Fatalf("writes %d, rebuilds %d", len(e.writer.writes), e.tree.count())
	}
	w := e.writer.writes[0]
	ref := library.StreamRef{Provider: "hdrezka", Path: interstellarPath, PostID: 2259, Title: "Interstellar",
		TranslatorID: 56, TranslatorName: "Дубляж", Director: true, Quality: "1080p"}
	if w.ItemType != library.ItemTypeMovie || w.MovieID != 7 || len(w.Rows) != 1 || w.Rows[0].ItemID != 7 ||
		*w.Rows[0].Stream != ref || w.Rows[0].FileSize != 300_000 || w.Rows[0].Resolution != "1080p" {
		t.Errorf("write = %+v, row %+v, stream %+v", w, w.Rows[0], w.Rows[0].Stream)
	}

	// A refused link is resolved again once.
	e = newHDRezkaEnv()
	e.site.size = func(_ context.Context, link string) (int64, error) {
		if strings.HasSuffix(link, "v0.mp4") {
			return 0, hdrezka.ErrLinkExpired
		}
		return sizeOf(link), nil
	}
	if _, err := e.svc.AssignMovie(context.Background(), 7, HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "720p"}); err != nil ||
		e.streams.invalidated != 1 || e.writer.writes[0].Rows[0].FileSize != 200_000 {
		t.Errorf("after a refused link: %v, invalidated %d", err, e.streams.invalidated)
	}
}

func TestHDRezkaAssignMovie_Errors(t *testing.T) {
	ok := HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"}
	with := func(change func(*HDRezkaAssign)) HDRezkaAssign {
		r := ok
		change(&r)
		return r
	}
	tests := []struct {
		name     string
		movieID  int64
		req      HDRezkaAssign
		size     error
		resolve  error
		write    error
		wantCode string
		network  bool
	}{
		{"no url", 7, with(func(r *HDRezkaAssign) { r.URL = "" }), nil, nil, nil, HDRezkaBadRequest, false},
		{"no translator", 7, with(func(r *HDRezkaAssign) { r.TranslatorID = 0 }), nil, nil, nil, HDRezkaBadRequest, false},
		{"no quality", 7, with(func(r *HDRezkaAssign) { r.Quality = " " }), nil, nil, nil, HDRezkaBadRequest, false},
		{"another site", 7, with(func(r *HDRezkaAssign) { r.URL = "https://evil.example/x" }), nil, nil, nil, HDRezkaInvalid, false},
		{"unknown movie", 77, ok, nil, nil, nil, HDRezkaNotFound, false},
		{"a series link", 7, with(func(r *HDRezkaAssign) { r.URL = breakingBadPath }), nil, nil, nil, HDRezkaInvalid, true},
		{"unknown translator", 7, with(func(r *HDRezkaAssign) { r.TranslatorID = 9 }), nil, nil, nil, HDRezkaInvalid, true},
		{"no such cut", 7, with(func(r *HDRezkaAssign) { r.Ads = boolPtr(true) }), nil, nil, nil, HDRezkaInvalid, true},
		{"premium translator", 7, with(func(r *HDRezkaAssign) { r.TranslatorID = 111 }), nil, nil, nil, HDRezkaPremiumOnly, true},
		{"no such quality", 7, with(func(r *HDRezkaAssign) { r.Quality = "240p" }), nil, nil, nil, HDRezkaInvalid, true},
		{"anti-bot", 7, ok, nil, hdrezka.ErrChallenge, nil, HDRezkaBlocked, true},
		{"CDN refuses twice", 7, ok, hdrezka.ErrLinkExpired, nil, nil, HDRezkaUnavailable, true},
		{"CDN 403 stays a 502", 7, ok, &hdrezka.StatusError{StatusCode: 403}, nil, nil, HDRezkaUnavailable, true},
		{"CDN slow", 7, ok, context.DeadlineExceeded, nil, nil, HDRezkaTimeout, true},
		{"the movie left", 7, ok, nil, nil, library.ErrMovieNotFound, HDRezkaNotFound, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			if tt.size != nil {
				e.site.size = func(context.Context, string) (int64, error) { return 0, tt.size }
			}
			e.streams.errs = map[[2]int]error{{0, 0}: tt.resolve}
			e.writer.err = tt.write
			_, err := e.svc.AssignMovie(context.Background(), tt.movieID, tt.req)
			if got := codeOf(err); got != tt.wantCode {
				t.Errorf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
			if (len(e.site.calls) > 0) != tt.network {
				t.Errorf("site calls = %v", e.site.calls)
			}
			if e.tree.count() != 0 {
				t.Error("the tree was rebuilt after a failure")
			}
		})
	}
}

func items(ids ...int64) []HDRezkaAssignItem {
	var out []HDRezkaAssignItem
	for _, id := range ids {
		out = append(out, HDRezkaAssignItem{EpisodeID: id, Season: 1, Episode: int(id % 100)})
	}
	return out
}

func TestHDRezkaAssignShow(t *testing.T) {
	e := newHDRezkaEnv()
	e.streams.errs = map[[2]int]error{{1, 2}: &hdrezka.CDNError{Message: "Видео не найдено"}}
	got, err := e.svc.AssignShow(context.Background(), 5, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Ads: boolPtr(true), Quality: "1080p"},
		Items:         items(501, 502, 503),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantAssigned := []HDRezkaAssigned{{EpisodeID: 501, Quality: "1080p", SizeBytes: 300_001}, {EpisodeID: 503, Quality: "1080p", SizeBytes: 300_003}}
	if !reflect.DeepEqual(got.Assigned, wantAssigned) {
		t.Errorf("assigned = %+v", got.Assigned)
	}
	if len(got.Failed) != 1 || got.Failed[0].EpisodeID != 502 || got.Failed[0].Code != HDRezkaUnavailable ||
		!strings.Contains(got.Failed[0].Message, "Видео не найдено") {
		t.Errorf("failed = %+v", got.Failed)
	}
	if len(e.writer.writes) != 1 || e.tree.count() != 1 || e.site.called("episodes:238") != 1 {
		t.Fatalf("writes %d, rebuilds %d, calls %v", len(e.writer.writes), e.tree.count(), e.site.calls)
	}
	w := e.writer.writes[0]
	if w.ItemType != library.ItemTypeEpisode || w.ShowID != 5 || len(w.Rows) != 2 || w.Rows[1].ItemID != 503 {
		t.Fatalf("write = %+v", w)
	}
	// A series ignores the cut: the id's first entry (no ads).
	if s := w.Rows[1].Stream; s.Season != 1 || s.Episode != 3 || s.TranslatorID != 238 || s.Ads || s.Title != "Breaking Bad" ||
		s.Path != breakingBadPath || s.PostID != 646 {
		t.Errorf("stream = %+v", s)
	}
}

func TestHDRezkaAssignShow_NothingAssigned(t *testing.T) {
	e := newHDRezkaEnv()
	e.streams.errs = map[[2]int]error{{1, 1}: hdrezka.ErrPremium, {1, 2}: hdrezka.ErrChallenge}
	_, err := e.svc.AssignShow(context.Background(), 5, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "1080p"},
		Items:         items(501, 502),
	})
	if codeOf(err) != HDRezkaPremiumOnly {
		t.Errorf("err = %v, want the first failure's premium_only", err)
	}
	if len(e.writer.writes) != 0 || e.tree.count() != 0 {
		t.Error("wrote or rebuilt with nothing assigned")
	}
}

func TestHDRezkaAssignShow_ThirtyAtFourAtATime(t *testing.T) {
	e := newHDRezkaEnv()
	e.streams.delay = 10 * time.Millisecond
	var its []HDRezkaAssignItem
	for n := 1; n <= MaxHDRezkaItems; n++ {
		its = append(its, HDRezkaAssignItem{EpisodeID: int64(9000 + n), Season: 1, Episode: n})
	}
	got, err := e.svc.AssignShow(context.Background(), 9, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: bigShowPath, TranslatorID: 1, Quality: "720p"}, Items: its})
	if err != nil || len(got.Assigned) != MaxHDRezkaItems || len(got.Failed) != 0 {
		t.Fatalf("assign = %d assigned, %v", len(got.Assigned), err)
	}
	if e.streams.maxRunning > hdrezkaAssignWorkers || e.streams.maxRunning < 2 {
		t.Errorf("resolves at once = %d, want up to %d", e.streams.maxRunning, hdrezkaAssignWorkers)
	}
	if len(e.writer.writes) != 1 || len(e.writer.writes[0].Rows) != MaxHDRezkaItems || e.tree.count() != 1 {
		t.Errorf("writes %d, rebuilds %d", len(e.writer.writes), e.tree.count())
	}
}

func TestHDRezkaAssignShow_Budget(t *testing.T) {
	e := newHDRezkaEnv()
	e.svc.assignBudget = 30 * time.Millisecond
	e.site.size = func(ctx context.Context, link string) (int64, error) {
		if strings.Contains(link, "s1e3") {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		return sizeOf(link), nil
	}
	got, err := e.svc.AssignShow(context.Background(), 5, HDRezkaShowAssign{
		HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "1080p"}, Items: items(501, 503)})
	if err != nil || len(got.Assigned) != 1 || len(got.Failed) != 1 || got.Failed[0].Code != HDRezkaTimeout {
		t.Fatalf("assign = %+v, %v", got, err)
	}
	if len(e.writer.writes) != 1 {
		t.Error("the episode in time wasn't written")
	}
}

func TestHDRezkaAssignShow_Validation(t *testing.T) {
	base := HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238, Quality: "1080p"}
	var tooMany []HDRezkaAssignItem
	for n := 1; n <= MaxHDRezkaItems+1; n++ {
		tooMany = append(tooMany, HDRezkaAssignItem{EpisodeID: int64(9000 + n), Season: 1, Episode: n})
	}
	tests := []struct {
		name     string
		showID   int64
		req      HDRezkaShowAssign
		wantCode string
		network  bool
	}{
		{"no items", 5, HDRezkaShowAssign{HDRezkaAssign: base}, HDRezkaBadRequest, false},
		{"31 items", 9, HDRezkaShowAssign{HDRezkaAssign: base, Items: tooMany}, HDRezkaBadRequest, false},
		{"twice the same episode", 5, HDRezkaShowAssign{HDRezkaAssign: base, Items: items(501, 503, 501)}, HDRezkaBadRequest, false},
		{"no episode id", 5, HDRezkaShowAssign{HDRezkaAssign: base, Items: []HDRezkaAssignItem{{Season: 1, Episode: 1}}}, HDRezkaBadRequest, false},
		{"episode 0", 5, HDRezkaShowAssign{HDRezkaAssign: base, Items: []HDRezkaAssignItem{{EpisodeID: 501, Season: 1}}}, HDRezkaBadRequest, false},
		{"no quality", 5, HDRezkaShowAssign{HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 238}, Items: items(501)}, HDRezkaBadRequest, false},
		{"unknown show", 77, HDRezkaShowAssign{HDRezkaAssign: base, Items: items(501)}, HDRezkaNotFound, false},
		{"another show's episode", 5, HDRezkaShowAssign{HDRezkaAssign: base, Items: items(501, 9001)}, HDRezkaInvalid, false},
		{"a movie link", 5, HDRezkaShowAssign{HDRezkaAssign: HDRezkaAssign{URL: interstellarPath, TranslatorID: 238, Quality: "1080p"},
			Items: items(501)}, HDRezkaInvalid, true},
		{"HDRezka lacks an episode", 5, HDRezkaShowAssign{HDRezkaAssign: base,
			Items: []HDRezkaAssignItem{{EpisodeID: 501, Season: 1, Episode: 1}, {EpisodeID: 503, Season: 1, Episode: 9}}}, HDRezkaInvalid, true},
		{"premium translator", 5, HDRezkaShowAssign{HDRezkaAssign: HDRezkaAssign{URL: breakingBadPath, TranslatorID: 111, Quality: "1080p"},
			Items: items(501)}, HDRezkaPremiumOnly, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newHDRezkaEnv()
			_, err := e.svc.AssignShow(context.Background(), tt.showID, tt.req)
			if got := codeOf(err); got != tt.wantCode {
				t.Errorf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
			if (len(e.site.calls) > 0) != tt.network {
				t.Errorf("site calls = %v", e.site.calls)
			}
			if len(e.streams.keys) != 0 || len(e.writer.writes) != 0 {
				t.Errorf("resolved %d, wrote %d", len(e.streams.keys), len(e.writer.writes))
			}
		})
	}
}

func TestHDRezkaFailure_Codes(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{hdrezka.ErrChallenge, HDRezkaBlocked},
		{hdrezka.ErrPremium, HDRezkaPremiumOnly},
		{hdrezka.ErrNotFound, HDRezkaNotFound},
		{hdrezka.ErrInvalidPath, HDRezkaInvalid},
		{hdrezka.ErrNoMP4, HDRezkaInvalid},
		{hdrezka.ErrNoQuality, HDRezkaInvalid},
		{hdrezka.ErrLinkExpired, HDRezkaUnavailable},
		{hdrezka.ErrRangeIgnored, HDRezkaUnavailable},
		{hdrezka.ErrSizeMismatch, HDRezkaUnavailable},
		{&hdrezka.StatusError{StatusCode: 401}, HDRezkaUnavailable},
		{&hdrezka.StatusError{StatusCode: 403}, HDRezkaUnavailable},
		{&hdrezka.ParseError{What: "x"}, HDRezkaUnavailable},
		{&hdrezka.CDNError{}, HDRezkaUnavailable},
		{fmt.Errorf("request failed: %w", context.DeadlineExceeded), HDRezkaTimeout},
		{errors.New("request failed: connection reset"), HDRezkaUnavailable},
	}
	for _, tt := range tests {
		f := failureOf(tt.err)
		if f.Code != tt.want || f.Message == "" {
			t.Errorf("failureOf(%v) = %+v, want %s", tt.err, f, tt.want)
		}
	}
	if err := siteFailure(context.Canceled); err != context.Canceled {
		t.Errorf("a canceled request = %v", err)
	}
}

// Every JSON shape carries exactly the fields of the web's contract
// (web/src/lib/api/contract/hdrezka.ts).
func TestHDRezkaContractFields(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"HDRezkaSearchResult", HDRezkaSearch{}, "items query"},
		{"HDRezkaCandidate", HDRezkaSearchItem{}, "category kind match original_title path rating title url year"},
		{"HDRezkaTitle", HDRezkaTitle{}, "kind original_title path post_id seasons title translator_id translators url year"},
		{"HDRezkaTranslator", HDRezkaTranslator{}, "ads camrip default director id name premium"},
		{"HDRezkaSeason", HDRezkaSeason{}, "episodes number"},
		{"HDRezkaEpisode", HDRezkaEpisode{}, "library number"},
		{"HDRezkaLibraryEpisode", HDRezkaLibraryEpisode{}, "air_date episode episode_id has_file name season source stream"},
		{"HDRezkaStreams", HDRezkaStreams{}, "qualities subtitles"},
		{"HDRezkaQuality", HDRezkaQuality{}, "available label premium resolution size_bytes"},
		{"HDRezkaSubtitle", HDRezkaSubtitle{}, "language_code language_name"},
		{"AssignShowStreamResult", HDRezkaShowResult{}, "assigned failed"},
		{"AssignShowStreamResult.assigned", HDRezkaAssigned{}, "episode_id quality size_bytes"},
		{"AssignShowStreamResult.failed", HDRezkaFailed{}, "code episode_id message"},
	}
	for _, tt := range tests {
		raw, err := json.Marshal(tt.v)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		var got []string
		for k := range fields {
			got = append(got, k)
		}
		slices.Sort(got)
		if strings.Join(got, " ") != tt.want {
			t.Errorf("%s fields = %v, want %s", tt.name, got, tt.want)
		}
	}
}

func TestHDRezkaPing(t *testing.T) {
	e := newHDRezkaEnv()
	e.site.pingErr = hdrezka.ErrChallenge
	if err := e.svc.Ping(context.Background()); err == nil || err.Error() != "HDRezka asks for a browser check" {
		t.Errorf("Ping = %v", err)
	}
	e.site.pingErr = nil
	if err := e.svc.Ping(context.Background()); err != nil {
		t.Errorf("Ping = %v", err)
	}
}

// fakeStreamer records what the source opened.
type fakeStreamer struct {
	file      hdrezka.File
	off, size int64
	err       error
}

func (f *fakeStreamer) Open(_ context.Context, file hdrezka.File, off, size int64) (io.ReadCloser, error) {
	f.file, f.off, f.size = file, off, size
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader("data")), nil
}

func TestHDRezkaSource(t *testing.T) {
	st := &fakeStreamer{}
	src := NewHDRezkaSource(st)
	ref := library.StreamRef{Provider: "hdrezka", Path: breakingBadPath, PostID: 646, TranslatorID: 238, Ads: true,
		Season: 1, Episode: 3, Quality: "1080p"}
	a := &library.TorrentAssignment{ItemType: library.ItemTypeEpisode, ItemID: 503, Kind: library.AssignmentKindHDRezka,
		Stream: &ref, FileSize: 1_145_043_718}
	body, err := src.OpenRemote(context.Background(), a, 4096)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	want := hdrezka.File{Key: hdrezka.StreamKey{PostID: 646, Path: breakingBadPath, TranslatorID: 238, Ads: true, Season: 1, Episode: 3},
		Quality: "1080p"}
	if st.file != want || st.off != 4096 || st.size != 1_145_043_718 {
		t.Errorf("opened %+v at %d of %d", st.file, st.off, st.size)
	}

	st.err = hdrezka.ErrSizeMismatch
	if _, err := src.OpenRemote(context.Background(), a, 0); !errors.Is(err, hdrezka.ErrSizeMismatch) {
		t.Errorf("err = %v", err)
	}
	if _, err := src.OpenRemote(context.Background(), &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka}, 0); err == nil {
		t.Error("an assignment without a stream opened")
	}
}
