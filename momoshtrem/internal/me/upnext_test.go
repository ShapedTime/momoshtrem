package me

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
)

var (
	now   = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	today = now.Format(time.DateOnly)
)

func daysAgo(n int) *time.Time {
	t := now.AddDate(0, 0, -n)
	return &t
}

func hoursFromNow(n int) *time.Time {
	t := now.Add(time.Duration(n) * time.Hour)
	return &t
}

// progress is a Trakt progress row naming S{season}E{episode} next.
func progress(tmdbID int, lastWatched *time.Time, season, episode int) library.UpNextCandidate {
	return library.UpNextCandidate{
		Source: library.UpNextFromProgress, ShowTMDBID: tmdbID, Title: fmt.Sprintf("Show %d", tmdbID),
		Completed: 3, Aired: 10, LastWatchedAt: lastWatched,
		Next: &library.TraktNextEpisode{
			TraktEpisodeKey: library.TraktEpisodeKey{ShowTMDBID: tmdbID, Season: season, Episode: episode},
			Title:           "Trakt's name",
		},
	}
}

// start is a watchlisted library show nobody has started.
func start(tmdbID int, listed *time.Time) library.UpNextCandidate {
	return library.UpNextCandidate{
		Source: library.UpNextFromWatchlist, ShowTMDBID: tmdbID, Title: fmt.Sprintf("Show %d", tmdbID), Aired: 8,
		ListedAt: listed, Next: &library.TraktNextEpisode{TraktEpisodeKey: library.TraktEpisodeKey{ShowTMDBID: tmdbID, Season: 1, Episode: 1}},
	}
}

// libraryShows says what the library knows about the next episodes, by
// show; a show it doesn't list isn't in the library.
type libraryShows map[int]library.KeyedEpisode

// listed: the show is in the library and lists the episode, aired on
// airDate, with a (setup) file or review.
func listed(airDate string, setup ...func(*library.KeyedEpisode)) library.KeyedEpisode {
	k := library.KeyedEpisode{
		Show:         &library.Show{ID: 1, Title: "Library Title", Year: 2020, PosterPath: "/library.jpg"},
		SeasonNumber: 1,
		Episode:      &library.Episode{ID: 100, EpisodeNumber: 1, Name: "Library name", AirDate: airDate},
	}
	for _, f := range setup {
		f(&k)
	}
	return k
}

func withFile(hash string) func(*library.KeyedEpisode) {
	return func(k *library.KeyedEpisode) {
		k.Episode.Assignment = &library.TorrentAssignment{InfoHash: hash, FilePath: "Show/E01.mkv", IsActive: true}
	}
}

func underReview(job int64) func(*library.KeyedEpisode) {
	return func(k *library.KeyedEpisode) { k.ReviewJobID = &job }
}

// inLibraryUnlisted: the show is in the library, the episode isn't.
var inLibraryUnlisted = library.KeyedEpisode{Show: &library.Show{ID: 2, Title: "Library Title"}}

// runUpNext applies the rules the way UpNext does, against lib.
func runUpNext(candidates []library.UpNextCandidate, lib libraryShows, settings library.TraktSettings) []UpNextItem {
	shows := upNextShows(candidates, settings, now)
	episodes := make([]library.KeyedEpisode, len(shows))
	for i, c := range shows {
		episodes[i] = lib[c.ShowTMDBID]
		episodes[i].Key = c.Next.TraktEpisodeKey
	}
	c := cards{
		progress: func(a *library.TorrentAssignment) (bool, int) {
			return a.InfoHash == "fetching", 42
		},
		publicURL: "https://webdav.local:36911",
	}
	return upNextItems(shows, episodes, c, now, today)
}

// summary is "tmdb_id:availability" per item, in order.
func summary(items []UpNextItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprintf("%d:%s", it.Show.TMDBID, it.Availability)
	}
	return strings.Join(parts, " ")
}

var defaults = library.DefaultTraktSettings()

func TestUpNext_Rules(t *testing.T) {
	paused := defaults
	paused.IncludePaused = true
	noWatchlist := defaults
	noWatchlist.ShowWatchlist = false

	calendarNext := progress(5, daysAgo(2), 2, 1) // caught up: the calendar names S2E1
	calendarNext.Next.FromCalendar = true
	calendarNext.Next.FirstAired = hoursFromNow(-3)
	unaired := progress(6, daysAgo(1), 2, 1)
	unaired.Next.FromCalendar = true
	unaired.Next.FirstAired = hoursFromNow(30)
	hidden := progress(7, daysAgo(1), 1, 4)
	hidden.Hidden = true
	noNext := progress(8, daysAgo(1), 0, 0)
	noNext.Next = nil
	hiddenStart := start(9, daysAgo(1))
	hiddenStart.Hidden = true

	tests := []struct {
		name       string
		candidates []library.UpNextCandidate
		lib        libraryShows
		settings   library.TraktSettings
		want       string
	}{
		{"each availability",
			[]library.UpNextCandidate{
				progress(1, daysAgo(1), 1, 1), progress(2, daysAgo(2), 1, 1), progress(3, daysAgo(3), 1, 1),
				progress(4, daysAgo(4), 1, 1), progress(10, daysAgo(5), 1, 1), progress(11, daysAgo(6), 1, 1),
			},
			libraryShows{1: listed("2020-01-01", withFile("idle")), 2: listed("2020-01-01", withFile("fetching")),
				3: listed("2020-01-01", underReview(77)), 4: listed("2020-01-01"), 10: inLibraryUnlisted},
			defaults, "1:ready 2:downloading 3:needs_review 4:needs_torrent 10:needs_torrent 11:not_in_library"},
		{"a file wins over review, review over airing",
			[]library.UpNextCandidate{progress(1, daysAgo(1), 1, 1), progress(2, daysAgo(2), 1, 1)},
			libraryShows{1: listed("2099-01-01", withFile("idle"), underReview(5)), 2: listed("2099-01-01", underReview(5))},
			defaults, "1:ready 2:needs_review"},
		{"next episode not aired: by the library's date",
			[]library.UpNextCandidate{progress(1, daysAgo(1), 1, 1), progress(2, daysAgo(2), 1, 1)},
			libraryShows{1: listed("2026-10-17"), 2: listed(today)},
			defaults, "2:needs_torrent 1:airs_on"},
		{"next episode Trakt names without a date has aired",
			[]library.UpNextCandidate{progress(1, daysAgo(1), 1, 1), progress(2, daysAgo(2), 1, 1)},
			libraryShows{2: listed("")},
			defaults, "1:not_in_library 2:needs_torrent"},
		{"caught up, then a calendar episode airs",
			[]library.UpNextCandidate{calendarNext, unaired},
			nil, defaults, "5:not_in_library 6:airs_on"},
		{"dropped and hidden shows, and shows without a next episode, are left out",
			[]library.UpNextCandidate{hidden, noNext, progress(1, daysAgo(1), 1, 1)},
			nil, defaults, "1:not_in_library"},
		{"paused shows are left out",
			[]library.UpNextCandidate{progress(1, daysAgo(181), 1, 1), progress(2, daysAgo(179), 1, 1)},
			nil, defaults, "2:not_in_library"},
		{"paused shows kept with include_paused",
			[]library.UpNextCandidate{progress(1, daysAgo(181), 1, 1), progress(2, daysAgo(179), 1, 1)},
			nil, paused, "2:not_in_library 1:not_in_library"},
		{"one progress row per show: the last watched",
			[]library.UpNextCandidate{progress(1, daysAgo(9), 1, 1), progress(1, daysAgo(1), 3, 1), progress(2, daysAgo(5), 1, 1)},
			nil, defaults, "1:not_in_library 2:not_in_library"},
		{"watchlist starts: at most three, newest listed first, once aired",
			[]library.UpNextCandidate{
				start(21, daysAgo(5)), start(22, daysAgo(1)), start(23, daysAgo(3)), start(24, daysAgo(2)), start(25, daysAgo(4)),
				progress(1, daysAgo(1), 1, 1),
			},
			libraryShows{21: listed("2020-01-01"), 22: listed("2020-01-01", withFile("idle")), 23: listed("2099-01-01"),
				24: listed("2020-01-01"), 25: listed("2020-01-01")},
			defaults, "1:not_in_library 22:ready 24:needs_torrent 25:needs_torrent"},
		{"watchlist starts need the library to list S01E01",
			[]library.UpNextCandidate{start(21, daysAgo(1))}, libraryShows{21: inLibraryUnlisted}, defaults, ""},
		{"no watchlist starts without show_watchlist",
			[]library.UpNextCandidate{start(21, daysAgo(1)), progress(1, daysAgo(1), 1, 1)},
			libraryShows{21: listed("2020-01-01")}, noWatchlist, "1:not_in_library"},
		{"a started or hidden show is no watchlist start",
			[]library.UpNextCandidate{start(1, daysAgo(1)), hiddenStart, progress(1, daysAgo(400), 1, 1)},
			libraryShows{1: listed("2020-01-01"), 9: listed("2020-01-01")}, defaults, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summary(runUpNext(tt.candidates, tt.lib, tt.settings)); got != tt.want {
				t.Errorf("up next = %q, want %q", got, tt.want)
			}
		})
	}
}

// Order: shows to carry on with by last watched, newest first (never
// watched last); then watchlist starts; then what airs next by when. At
// most 40.
func TestUpNext_Order(t *testing.T) {
	soon, later := progress(30, daysAgo(1), 1, 1), progress(31, daysAgo(9), 1, 1)
	soon.Next.FirstAired, later.Next.FirstAired = hoursFromNow(5), hoursFromNow(50)
	farOff := progress(32, daysAgo(1), 1, 1) // airs on the library's date, far off
	candidates := []library.UpNextCandidate{
		later, start(20, daysAgo(3)), farOff, progress(2, daysAgo(5), 1, 1), soon,
		progress(1, daysAgo(1), 1, 1), progress(3, nil, 1, 1), start(21, daysAgo(1)),
	}
	lib := libraryShows{20: listed("2020-01-01"), 21: listed("2020-01-01"), 32: listed("2099-01-01")}
	items := runUpNext(candidates, lib, defaults)
	if got, want := summary(items), "1:not_in_library 2:not_in_library 3:not_in_library 21:needs_torrent 20:needs_torrent 30:airs_on 31:airs_on 32:airs_on"; got != want {
		t.Errorf("order = %q\nwant      %q", got, want)
	}
	for _, it := range items {
		switch {
		case it.Availability == AvailabilityAirsOn && it.AirsAt == nil:
			t.Errorf("%d: airs_at is null", it.Show.TMDBID)
		case it.Availability != AvailabilityAirsOn && it.AirsAt != nil:
			t.Errorf("%d: airs_at set while %s", it.Show.TMDBID, it.Availability)
		}
	}
	if at := items[7].AirsAt; at == nil || !at.Equal(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("airs_at from the library's date = %v, want 2099-01-01 at 00:00 UTC", at)
	}

	var many []library.UpNextCandidate
	for i := 1; i <= 50; i++ {
		many = append(many, progress(1000+i, daysAgo(i), 1, 1))
	}
	if items := runUpNext(many, nil, defaults); len(items) != MaxUpNext || items[0].Show.TMDBID != 1001 {
		t.Errorf("50 shows = %d items, first %d; want %d, the last watched first", len(items), items[0].Show.TMDBID, MaxUpNext)
	}
	if items := runUpNext(nil, nil, defaults); items == nil || len(items) != 0 {
		t.Errorf("nothing = %#v, want an empty list", items)
	}
}

// A card's fields: the file while ready or downloading, the job while
// under review; the library's names and numbers when it lists the episode,
// else Trakt's.
func TestUpNext_Cards(t *testing.T) {
	fetching := progress(1, daysAgo(1), 3, 9) // Trakt's S3E9 is the library's S1E1
	fetching.PosterPath, fetching.BackdropPath = "/trakt-poster.jpg", "/trakt-backdrop.jpg"
	fetching.Next.EpisodeTMDBID, fetching.Next.StillPath = 555, "/trakt-still.jpg"
	review := progress(2, daysAgo(2), 1, 1)
	outside := progress(3, daysAgo(3), 4, 2)
	outside.Next.EpisodeTMDBID, outside.Next.StillPath = 777, "/trakt-still.jpg"
	outside.Next.FirstAired = hoursFromNow(-30)

	mates := listed("2020-01-01", withFile("fetching"))
	mates.Episode.Assignment.MatchSource = "rules" // one file plays E1–E2
	mates.FileMates = []library.Episode{{ID: 101, EpisodeNumber: 2, Name: "Two",
		Assignment: &library.TorrentAssignment{InfoHash: "fetching", FilePath: "Show/E01.mkv", MatchSource: "rules"}}}
	abs := 31
	mates.Episode.AbsoluteNumber = &abs
	mates.Show.EpisodeOrdering = library.OrderingAbsolute
	items := runUpNext([]library.UpNextCandidate{fetching, review, outside},
		libraryShows{1: mates, 2: listed("2020-01-01", underReview(77))}, defaults)
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}

	d := items[0]
	wantPath := "/TV Shows/Library Title (2020)/Season 01/Library Title - S01E01-E02 - Library name.mkv"
	if d.Availability != AvailabilityDownloading || d.WebDAVPath == nil || *d.WebDAVPath != wantPath ||
		*d.WebDAVURL != "https://webdav.local:36911/TV%20Shows/Library%20Title%20%282020%29/Season%2001/Library%20Title%20-%20S01E01-E02%20-%20Library%20name.mkv" ||
		d.Download == nil || d.Download.Pct != 42 || d.Download.InfoHash != "fetching" || d.ReviewJobID != nil {
		t.Errorf("downloading card = %+v (path %v, download %+v)", d.EpisodeCard, deref(d.WebDAVPath), d.Download)
	}
	if e := d.NextEpisode; e.Season != 1 || e.Episode != 1 || *e.AbsoluteNumber != 31 || *e.Name != "Library name" ||
		*e.AirDate != "2020-01-01" || *e.StillPath != "/trakt-still.jpg" || *e.TMDBID != 555 {
		t.Errorf("library episode = %+v", e)
	}
	if s := d.Show; *s.LibraryID != 1 || s.Title != "Library Title" || *s.PosterPath != "/library.jpg" || *s.BackdropPath != "/trakt-backdrop.jpg" {
		t.Errorf("library show = %+v", s)
	}
	if d.Progress.Completed != 3 || d.Progress.Aired != 10 || d.Source != "progress" || !d.LastWatchedAt.Equal(*daysAgo(1)) {
		t.Errorf("progress = %+v, source %s", d.Progress, d.Source)
	}

	if r := items[1]; r.Availability != AvailabilityNeedsReview || *r.ReviewJobID != 77 || r.WebDAVPath != nil || r.Download != nil {
		t.Errorf("review card = %+v", r.EpisodeCard)
	}

	o := items[2]
	if e := o.NextEpisode; e.Season != 4 || e.Episode != 2 || e.AbsoluteNumber != nil || *e.Name != "Trakt's name" ||
		*e.AirDate != "2026-10-15" || *e.TMDBID != 777 || o.Show.LibraryID != nil || o.Show.Title != "Show 3" {
		t.Errorf("Trakt's episode = %+v, show %+v", e, o.Show)
	}
}

// fakeDownloads is a downloads snapshot (#77) that wants these files and
// holds nothing.
type fakeDownloads map[downloads.FileKey]downloads.FileStatus

func (fakeDownloads) Movie(int64) (downloads.Keep, bool)       { return downloads.Keep{}, false }
func (fakeDownloads) Show(int64) (downloads.Keep, bool)        { return downloads.Keep{}, false }
func (fakeDownloads) Season(int64, int) (downloads.Keep, bool) { return downloads.Keep{}, false }
func (fakeDownloads) Episode(int64) (downloads.Keep, bool)     { return downloads.Keep{}, false }
func (fakeDownloads) DownloadingFiles() []downloads.FileKey    { return nil }

func (f fakeDownloads) File(hash, path string) (downloads.FileStatus, bool) {
	s, ok := f[downloads.FileKey{Hash: hash, Path: path}]
	return s, ok
}

// A card is downloading by the title pages' rule (#77): while a download
// wants its file and it isn't all on disk, not while that download is
// paused. A file nothing keeps streams on demand, so its card is ready.
func TestCards_FollowTheDownloads(t *testing.T) {
	snap := fakeDownloads{
		{Hash: "kept", Path: "Show/E01.mkv"}:    {Bytes: 1_000, DoneBytes: 420, State: downloads.StateDownloading},
		{Hash: "waiting", Path: "Show/E01.mkv"}: {Bytes: 1_000, DoneBytes: 0, State: downloads.StateWaitingForWindow},
		{Hash: "paused", Path: "Show/E01.mkv"}:  {Bytes: 1_000, DoneBytes: 420, State: downloads.StatePaused},
		{Hash: "done", Path: "Show/E01.mkv"}:    {Bytes: 1_000, DoneBytes: 1_000, Complete: true, State: downloads.StateDone},
	}
	svc := New(Deps{Catalog: catalog.New(catalog.Deps{Downloads: func() catalog.DownloadsSnapshot { return snap }})})
	c := svc.cards()
	tests := []struct {
		hash string
		want Availability
		pct  int
	}{
		{"kept", AvailabilityDownloading, 42},
		{"waiting", AvailabilityDownloading, 0},
		{"paused", AvailabilityReady, -1},
		{"done", AvailabilityReady, -1},
		{"streamed", AvailabilityReady, -1}, // nothing keeps it
	}
	for _, tt := range tests {
		card := c.card(listed("2020-01-01", withFile(tt.hash)), true)
		if card.Availability != tt.want || card.WebDAVPath == nil {
			t.Errorf("%s: card = %+v, want %s", tt.hash, card, tt.want)
		}
		switch {
		case tt.pct < 0 && card.Download != nil:
			t.Errorf("%s: download = %+v, want nil", tt.hash, card.Download)
		case tt.pct >= 0 && (card.Download == nil || card.Download.Pct != tt.pct || card.Download.InfoHash != tt.hash):
			t.Errorf("%s: download = %+v, want %d%%", tt.hash, card.Download, tt.pct)
		}
	}

	// Without downloads, nothing is downloading.
	c = New(Deps{Catalog: catalog.New(catalog.Deps{})}).cards()
	if card := c.card(listed("2020-01-01", withFile("kept")), true); card.Availability != AvailabilityReady || card.Download != nil {
		t.Errorf("no downloads: card = %+v", card)
	}
}

// Absolute numbers follow the title page's rule: only a library show with
// absolute ordering has them, so Doctor Who's S03E04 never reads "#30".
func TestEpisodeRef_AbsoluteNumber(t *testing.T) {
	ordered := func(ordering library.EpisodeOrdering) library.KeyedEpisode {
		k := listed("2020-01-01")
		abs := 30
		k.Episode.AbsoluteNumber = &abs
		k.Show.EpisodeOrdering = ordering
		return k
	}
	for _, tc := range []struct {
		name string
		k    library.KeyedEpisode
		want *int
	}{
		{"absolute ordering", ordered(library.OrderingAbsolute), intPtr(30)},
		{"aired ordering", ordered(library.OrderingAired), nil},
		{"episode group ordering", ordered(library.OrderingEpisodeGroup), nil},
		{"not in the library", library.KeyedEpisode{}, nil},
	} {
		got := episodeRef(tc.k, traktEpisode{name: "Trakt's name"}).AbsoluteNumber
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: absolute_number = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func intPtr(n int) *int { return &n }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestNewEpisodeItems(t *testing.T) {
	from := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	key := func(show, season, episode int) library.TraktEpisodeKey {
		return library.TraktEpisodeKey{ShowTMDBID: show, Season: season, Episode: episode}
	}
	calendar := func(k library.TraktEpisodeKey, title string, aired *time.Time) library.NewEpisodeCandidate {
		return library.NewEpisodeCandidate{TraktEpisodeKey: k, FromCalendar: true, ShowTitle: fmt.Sprintf("Calendar %d", k.ShowTMDBID),
			EpisodeTitle: title, FirstAired: aired}
	}
	candidates := []library.NewEpisodeCandidate{
		{TraktEpisodeKey: key(1, 1, 5), AirDate: "2026-10-14"},  // the library's
		calendar(key(1, 1, 5), "Five", hoursFromNow(-50)),       // the same episode, on the calendar
		{TraktEpisodeKey: key(1, 1, 6), AirDate: "2026-10-16"},  // aired today
		calendar(key(2, 3, 1), "Premiere", hoursFromNow(-20)),   // not in the library
		calendar(key(2, 3, 2), "Before the window", daysAgo(9)), // too early
		{TraktEpisodeKey: key(3, 1, 1), AirDate: "2026-10-15"},  // a file
	}
	episodes := []library.KeyedEpisode{
		listed("2026-10-14"), listed("2026-10-14"), listed("2026-10-16"), {}, {}, listed("2026-10-15", withFile("idle")),
	}
	episodes[0].Episode.EpisodeNumber, episodes[1].Episode.EpisodeNumber = 5, 5
	episodes[2].Episode.ID, episodes[2].Episode.EpisodeNumber = 106, 6
	episodes[5].Episode.ID = 300
	for i := range episodes {
		episodes[i].Key = candidates[i].TraktEpisodeKey
	}
	c := cards{progress: func(*library.TorrentAssignment) (bool, int) { return false, 0 }}
	items := newEpisodeItems(candidates, episodes, c, from, now)

	var got []string
	for _, it := range items {
		got = append(got, fmt.Sprintf("%d S%dE%d %s %s", it.Show.TMDBID, it.Episode.Season, it.Episode.Episode,
			it.AiredAt.Format(time.RFC3339), it.Availability))
	}
	want := []string{
		"1 S1E6 2026-10-16T00:00:00Z needs_torrent",
		"2 S3E1 2026-10-15T16:00:00Z not_in_library",
		"3 S1E1 2026-10-15T00:00:00Z ready",
		"1 S1E5 2026-10-14T10:00:00Z needs_torrent", // once, aired when the calendar says
	}
	if !slices.Equal(got, want) {
		t.Errorf("new episodes =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
