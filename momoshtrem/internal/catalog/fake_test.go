package catalog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// fakeStore is an in-memory Store that counts calls.
type fakeStore struct {
	mu      sync.Mutex
	movies  map[int]*library.Movie     // by TMDB ID
	shows   map[int]*library.Show      // by TMDB ID
	seasons map[int64][]library.Season // by show ID, full episode rows
	// library answers LibraryItems; OnlyTMDB filters it.
	library    []library.LibraryItem
	libTotal   int
	libCounts  library.LibraryCounts
	libSummary library.LibrarySummary
	lastQuery  library.LibraryQuery
	// reviews answers ReviewState by "tv:<id>" or "movie:<id>"; none is
	// an empty review.
	reviews map[string]*library.ReviewState
	calls   map[string]int
	err     error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		movies:  map[int]*library.Movie{},
		shows:   map[int]*library.Show{},
		seasons: map[int64][]library.Season{},
		reviews: map[string]*library.ReviewState{},
		calls:   map[string]int{},
	}
}

func (f *fakeStore) ReviewState(_ context.Context, mediaType string, id int64) (*library.ReviewState, error) {
	if err := f.count("review"); err != nil {
		return nil, err
	}
	if r, ok := f.reviews[fmt.Sprintf("%s:%d", mediaType, id)]; ok {
		c := *r
		return &c, nil
	}
	return &library.ReviewState{}, nil
}

var _ Store = (*fakeStore)(nil)

func (f *fakeStore) count(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[name]++
	return f.err
}

func (f *fakeStore) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *fakeStore) MovieByTMDBID(_ context.Context, tmdbID int) (*library.Movie, error) {
	if err := f.count("movie"); err != nil {
		return nil, err
	}
	if m, ok := f.movies[tmdbID]; ok {
		c := *m
		return &c, nil
	}
	return nil, nil
}

func (f *fakeStore) ShowByTMDBID(_ context.Context, tmdbID int) (*library.Show, error) {
	if err := f.count("show"); err != nil {
		return nil, err
	}
	if s, ok := f.shows[tmdbID]; ok {
		c := *s
		return &c, nil
	}
	return nil, nil
}

func (f *fakeStore) ShowByID(_ context.Context, id int64) (*library.Show, error) {
	if err := f.count("show_by_id"); err != nil {
		return nil, err
	}
	for _, s := range f.shows {
		if s.ID == id {
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) ShowEpisodes(_ context.Context, showID int64) ([]library.Season, error) {
	if err := f.count("show_episodes"); err != nil {
		return nil, err
	}
	return f.seasons[showID], nil
}

func (f *fakeStore) SeasonByTMDBID(_ context.Context, tmdbID, n int) (*library.SeasonView, error) {
	if err := f.count("season"); err != nil {
		return nil, err
	}
	show, ok := f.shows[tmdbID]
	if !ok {
		return nil, nil
	}
	view := &library.SeasonView{ShowID: show.ID, ShowTitle: show.Title, ShowYear: show.Year, IncludeSpecials: show.IncludeSpecials}
	for _, s := range f.seasons[show.ID] {
		if s.SeasonNumber == n {
			c := s
			view.Season = &c
		}
	}
	return view, nil
}

func (f *fakeStore) LibraryItems(_ context.Context, q library.LibraryQuery) ([]library.LibraryItem, int, error) {
	if err := f.count("library"); err != nil {
		return nil, 0, err
	}
	f.mu.Lock()
	f.lastQuery = q
	f.mu.Unlock()
	if q.OnlyTMDB == nil {
		return f.library, f.libTotal, nil
	}
	var out []library.LibraryItem
	for _, item := range f.library {
		ids := q.OnlyTMDB.Shows
		if item.MediaType == "movie" {
			ids = q.OnlyTMDB.Movies
		}
		for _, id := range ids {
			if id == item.TMDBID {
				out = append(out, item)
			}
		}
	}
	return out, len(out), nil
}

// ListLibrary answers like LibraryItems, with libCounts and libSummary.
func (f *fakeStore) ListLibrary(ctx context.Context, q library.LibraryQuery) (*library.LibraryListing, error) {
	items, total, err := f.LibraryItems(ctx, q)
	if err != nil {
		return nil, err
	}
	return &library.LibraryListing{Items: items, Total: total, Counts: f.libCounts, Summary: f.libSummary}, nil
}

// fakeTMDB serves canned details and counts calls; errs forces errors.
type fakeTMDB struct {
	mu         sync.Mutex
	movies     map[int]*tmdb.MovieDetails
	shows      map[int]*tmdb.ShowDetails
	groupLists map[int][]tmdb.EpisodeGroupSummary
	groups     map[string]*tmdb.EpisodeGroup
	err        error // returned by every call when set
	calls      map[string]int
}

func newFakeTMDB() *fakeTMDB {
	return &fakeTMDB{
		movies:     map[int]*tmdb.MovieDetails{},
		shows:      map[int]*tmdb.ShowDetails{},
		groupLists: map[int][]tmdb.EpisodeGroupSummary{},
		groups:     map[string]*tmdb.EpisodeGroup{},
		calls:      map[string]int{},
	}
}

var _ TMDB = (*fakeTMDB)(nil)

func (f *fakeTMDB) count(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[name]++
	return f.err
}

func (f *fakeTMDB) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if err := f.count("movie"); err != nil {
		return nil, err
	}
	if m, ok := f.movies[id]; ok {
		return m, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	if err := f.count("show"); err != nil {
		return nil, err
	}
	if s, ok := f.shows[id]; ok {
		return s, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) EpisodeGroups(_ context.Context, showID int) ([]tmdb.EpisodeGroupSummary, error) {
	if err := f.count("groups"); err != nil {
		return nil, err
	}
	return f.groupLists[showID], nil
}

func (f *fakeTMDB) EpisodeGroup(_ context.Context, id string) (*tmdb.EpisodeGroup, error) {
	if err := f.count("group"); err != nil {
		return nil, err
	}
	if g, ok := f.groups[id]; ok {
		return g, nil
	}
	return nil, tmdb.ErrNotFound
}

// fakeTorrents is a torrent client with a few loaded torrents, and the
// torrents the user paused (loaded or not).
type fakeTorrents struct {
	loaded map[string]fakeTorrent
	paused map[string]bool
}

type fakeTorrent struct {
	active   bool
	seeders  int
	speedBps int64
	files    map[string]torrent.FileProgress
}

var _ Torrents = (*fakeTorrents)(nil)

func (f *fakeTorrents) ListTorrents() ([]torrent.TorrentStatus, error) {
	var out []torrent.TorrentStatus
	for hash, t := range f.loaded {
		out = append(out, torrent.TorrentStatus{InfoHash: hash, Seeders: t.seeders})
	}
	return out, nil
}

func (f *fakeTorrents) GetStatus(hash string) (*torrent.TorrentStatus, error) {
	t, ok := f.loaded[hash]
	if !ok {
		return nil, torrent.ErrTorrentNotFound
	}
	return &torrent.TorrentStatus{InfoHash: hash, Seeders: t.seeders, DownloadSpeed: t.speedBps}, nil
}

func (f *fakeTorrents) PausedAt(hash string) (time.Time, bool) {
	if f.paused[hash] {
		return time.Unix(1_700_000_000, 0), true
	}
	return time.Time{}, false
}

func (f *fakeTorrents) FilesProgress(hash string) (map[string]torrent.FileProgress, error) {
	t, ok := f.loaded[hash]
	if !ok {
		return nil, torrent.ErrTorrentNotFound
	}
	return t.files, nil
}

func (f *fakeTorrents) IsActive(hash string) bool {
	t, ok := f.loaded[hash]
	return ok && t.active
}

// fakeColors answers a fixed colour per backdrop.
type fakeColors map[string]string

func (f fakeColors) Color(_ context.Context, backdropPath string) *string {
	if c, ok := f[backdropPath]; ok {
		return &c
	}
	return nil
}

// fakeSettings records the patch and returns the show.
type fakeSettings struct {
	store *fakeStore
	patch metadata.SettingsPatch
	err   error
}

func (f *fakeSettings) ApplySettings(_ context.Context, showID int64, patch metadata.SettingsPatch) (*library.Show, error) {
	f.patch = patch
	if f.err != nil {
		return nil, f.err
	}
	for _, s := range f.store.shows {
		if s.ID == showID {
			if patch.IncludeSpecials != nil {
				s.IncludeSpecials = *patch.IncludeSpecials
			}
			return s, nil
		}
	}
	return nil, library.ErrShowNotFound
}

// Fixture: Dune: Part Two (ready), Frieren (S1 29–38 missing, E28
// downloading), One Piece (in the library, recommended by Frieren).

var fixedNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

var addedAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

const (
	duneTMDB    = 693134
	frierenTMDB = 209867
	duneHash    = "4f1a2b3c4d5e6f708192a3b4c5d6e7f8091abe90"
	frierenHash = "aa11bb22cc33dd44ee55ff66aa77bb88cc99dd00"
)

type fixture struct {
	store    *fakeStore
	tmdb     *fakeTMDB
	torrents *fakeTorrents
	settings *fakeSettings
	svc      *Service
}

func newFixture() *fixture {
	store := newFakeStore()
	tm := newFakeTMDB()
	torrents := &fakeTorrents{loaded: map[string]fakeTorrent{}}

	store.movies[duneTMDB] = &library.Movie{
		ID: 7, TMDBID: duneTMDB, Title: "Dune: Part Two", Year: 2024, CreatedAt: addedAt,
		PosterPath: "/dune-poster.jpg", BackdropPath: "/dune-backdrop.jpg", Runtime: 167, Genres: []string{"Science Fiction"},
		Assignment: &library.TorrentAssignment{
			ID: 70, ItemType: library.ItemTypeMovie, ItemID: 7, InfoHash: duneHash,
			MagnetURI:  "magnet:?xt=urn:btih:" + duneHash + "&dn=Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX",
			FilePath:   "Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv",
			FileSize:   18_400_000_000,
			Resolution: "2160p", Source: "WEB-DL", IsActive: true, CreatedAt: addedAt.Add(time.Hour),
		},
	}
	tm.movies[duneTMDB] = &tmdb.MovieDetails{
		ID: duneTMDB, Title: "Dune: Part Two", OriginalTitle: "Dune: Part Two", OriginalLanguage: "en",
		Overview: "Paul Atreides unites with the Fremen.", ReleaseDate: "2024-02-27", Runtime: 167,
		PosterPath: "/dune-poster.jpg", BackdropPath: "/dune-backdrop.jpg",
		Genres:      []tmdb.Genre{{ID: 878, Name: "Science Fiction"}, {ID: 12, Name: "Adventure"}},
		VoteAverage: 8.1, VoteCount: 6000,
		Credits: tmdb.Credits{
			Cast: []tmdb.CastMember{{Name: "Timothée Chalamet", Character: "Paul Atreides", ProfilePath: "/tc.jpg"}, {Name: "Zendaya", Character: "Chani"}},
			Crew: []tmdb.CrewMember{{Name: "Denis Villeneuve", Job: "Director", Department: "Directing"}},
		},
		Videos: tmdb.Videos{Results: []tmdb.Video{{Key: "Way9Dexny3w", Site: "YouTube", Type: "Trailer", Official: true}}},
		Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{
			{ID: 157336, MediaType: "movie", Title: "Interstellar", ReleaseDate: "2014-11-05", PosterPath: "/interstellar.jpg"},
			{ID: 438631, MediaType: "movie", Title: "Dune", ReleaseDate: "2021-09-15"},
		}},
		Similar: tmdb.ResultsPage{Results: []tmdb.ListItem{{ID: 1, Title: "Ignored when there are recommendations"}}},
	}

	frieren := &library.Show{
		ID: 3, TMDBID: frierenTMDB, Title: "Frieren: Beyond Journey's End", Year: 2023, CreatedAt: addedAt,
		OriginalName: "葬送のフリーレン", OriginalLanguage: "ja", IsAnime: true, Status: "Returning Series",
		PosterPath: "/frieren-poster.jpg", BackdropPath: "/frieren-backdrop.jpg", Genres: []string{"Animation"},
		EpisodeOrdering: library.OrderingAbsolute, IncludeSpecials: true, ClassifierMode: library.ClassifierAuto,
	}
	store.shows[frierenTMDB] = frieren
	store.seasons[frieren.ID] = frierenSeasons(38, 28)

	tm.shows[frierenTMDB] = &tmdb.ShowDetails{
		ID: frierenTMDB, Name: "Frieren: Beyond Journey's End", OriginalName: "葬送のフリーレン", OriginalLanguage: "ja",
		OriginCountry: []string{"JP"}, Overview: "An elf mage outlives her party.", Status: "Returning Series",
		FirstAirDate: "2023-09-29", LastAirDate: "2024-03-22",
		PosterPath: "/frieren-poster.jpg", BackdropPath: "/frieren-backdrop.jpg",
		Genres:   []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 10765, Name: "Sci-Fi & Fantasy"}},
		Networks: []tmdb.Company{{Name: "Nippon TV"}}, CreatedBy: []tmdb.Creator{{Name: "Kanehito Yamada"}},
		EpisodeRunTime: []int{24}, VoteAverage: 8.8, VoteCount: 900,
		Seasons: []tmdb.SeasonSummary{{SeasonNumber: 0, Name: "Specials", EpisodeCount: 2}, {SeasonNumber: 1, Name: "Season 1", EpisodeCount: 38}},
		AggregateCredits: tmdb.AggregateCredits{Cast: []tmdb.AggregateCastMember{
			{Name: "Atsumi Tanezaki", ProfilePath: "/at.jpg", Roles: []tmdb.AggregateRole{{Character: "Frieren"}}},
		}},
		Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{
			{ID: 37854, MediaType: "tv", Name: "One Piece", FirstAirDate: "1999-10-20", PosterPath: "/op.jpg"},
			{ID: 66573, MediaType: "tv", Name: "The Good Place", FirstAirDate: "2016-09-19"},
		}},
	}
	store.library = []library.LibraryItem{
		{MediaType: "tv", ID: 4, TMDBID: 37854, Title: "One Piece", Stage: "ready", EpisodesTotal: intPtr(1124), EpisodesReady: intPtr(1124)},
		{MediaType: "movie", ID: 8, TMDBID: 157336, Title: "Interstellar", Stage: "downloading"},
	}

	// Frieren's E28 is still being fetched; Dune's torrent is loaded but idle.
	torrents.loaded[frierenHash] = fakeTorrent{active: true, seeders: 41, files: map[string]torrent.FileProgress{
		"Frieren S01/Frieren.S01E27.mkv": {Length: 1_000, BytesCompleted: 1_000},
		"Frieren S01/Frieren.S01E28.mkv": {Length: 1_000, BytesCompleted: 420},
	}}
	torrents.loaded[duneHash] = fakeTorrent{active: false, seeders: 12, files: map[string]torrent.FileProgress{
		"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv": {Length: 18_400_000_000, BytesCompleted: 9_000_000_000},
	}}

	settings := &fakeSettings{store: store}
	svc := New(Deps{
		Store: store, TMDB: tm, Torrents: torrents, Settings: settings,
		Colors:          fakeColors{"/dune-backdrop.jpg": "#7a4a22", "/frieren-backdrop.jpg": "#2d5a4f"},
		WebDAVPublicURL: "https://momoshtrem.local:36911/",
		Now:             func() time.Time { return fixedNow },
	})
	return &fixture{store: store, tmdb: tm, torrents: torrents, settings: settings, svc: svc}
}

// frierenSeasons is Frieren's specials and a first season of n weekly
// episodes, the first `withFiles` of them assigned.
func frierenSeasons(n, withFiles int) []library.Season {
	specials := library.Season{ID: 30, ShowID: 3, SeasonNumber: 0, Name: "Specials", Episodes: []library.Episode{
		{ID: 3001, EpisodeNumber: 1, Name: "Frieren Mini Anime 1", AirDate: "2023-10-06", Runtime: 3},
		{ID: 3002, EpisodeNumber: 2, Name: "Frieren Mini Anime 2", AirDate: "2099-01-01", Runtime: 3},
	}}
	s1 := library.Season{ID: 31, ShowID: 3, SeasonNumber: 1, Name: "Season 1"}
	start := time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= n; i++ {
		abs := i
		ep := library.Episode{
			ID: int64(31000 + i), SeasonID: 31, EpisodeNumber: i, Name: fmt.Sprintf("Episode %d", i),
			AirDate: start.AddDate(0, 0, 7*(i-1)).Format(time.DateOnly), AbsoluteNumber: &abs,
			EpisodeType: "standard", Runtime: 24, Overview: "…", StillPath: fmt.Sprintf("/still-%d.jpg", i),
		}
		if i == n {
			ep.EpisodeType = "finale"
		}
		if i <= withFiles {
			ep.Assignment = &library.TorrentAssignment{
				ItemType: library.ItemTypeEpisode, ItemID: ep.ID, InfoHash: frierenHash,
				FilePath: fmt.Sprintf("Frieren S01/Frieren.S01E%02d.mkv", i), FileSize: 1_000, IsActive: true,
			}
		}
		s1.Episodes = append(s1.Episodes, ep)
	}
	return []library.Season{specials, s1}
}
