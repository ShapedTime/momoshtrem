package service

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// loadedTorrents is fakeTorrents that also has torrents loaded in the
// client (GetTorrent), like torrent.Service after a restart's first read.
type loadedTorrents struct {
	*fakeTorrents
	mu     sync.Mutex
	loaded map[string]*torrent.TorrentInfo
}

func (l *loadedTorrents) GetTorrent(hash string) (*torrent.TorrentInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if info := l.loaded[hash]; info != nil {
		return info, nil
	}
	return nil, torrent.ErrTorrentNotFound
}

// withLoaded makes the env's service see infos as loaded torrents.
func (e *jobEnv) withLoaded(infos ...*torrent.TorrentInfo) {
	l := &loadedTorrents{fakeTorrents: e.torrents, loaded: map[string]*torrent.TorrentInfo{}}
	for _, info := range infos {
		l.loaded[info.InfoHash] = info
	}
	e.svc.torrents = l
}

func addsOf(f *fakeTorrents) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adds
}

func (e *jobEnv) legacyRow(itemType library.ItemType, itemID int64, hash, path string, active bool) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ($1, $2, $3, $4, $5, 1, $6)`, itemType, itemID, hash, magnet(hash)+"&dn=Legacy", path, active); err != nil {
		e.t.Fatal(err)
	}
}

func assignmentRow(t *testing.T, e *jobEnv, itemType library.ItemType, itemID int64, hash, path string) (id int64, active bool) {
	t.Helper()
	err := e.db.QueryRow(`SELECT id, is_active FROM torrent_assignments
		WHERE item_type = $1 AND item_id = $2 AND info_hash = $3 AND file_path = $4`, itemType, itemID, hash, path).Scan(&id, &active)
	if err != nil {
		t.Fatalf("row %s %d %s: %v", itemType, itemID, path, err)
	}
	return id, active
}

// A manual override writes one row through Apply's writer: the episode's
// other row goes, the torrent's other episodes keep theirs, and moving it
// back reactivates the inactive pair instead of inserting a duplicate.
func TestOverride_Episode(t *testing.T) {
	hash := strings.Repeat("8", 40)
	root := "Swap.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t, info(hash, root,
		file("Swap.Show.S01E01.1080p.WEB-GRP.mkv", gb),
		file("Swap.Show.S01E02.1080p.WEB-GRP.mkv", 2*gb),
		file("Swap.Show.S01.nfo", 1<<10)))
	showID, eps := e.show("Swap Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	if id := e.assignShow(showID, hash, nil); e.job(id).Status != library.JobApplied {
		t.Fatalf("job = %s", e.job(id).Status)
	}
	e1Path, e2Path := root+"/Swap.Show.S01E01.1080p.WEB-GRP.mkv", root+"/Swap.Show.S01E02.1080p.WEB-GRP.mkv"
	original, _ := assignmentRow(t, e, library.ItemTypeEpisode, eps["S01E01"], hash, e1Path)
	rebuilds := e.tree.count()

	// E01 plays E02's file now; E02 keeps it too.
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E01"], strings.ToUpper(hash), e2Path); err != nil {
		t.Fatal(err)
	}
	a := e.active(library.ItemTypeEpisode, eps["S01E01"])
	if a == nil || a.FilePath != e2Path || a.MatchSource != "manual" || a.Confidence != "" || a.FileSize != 2*gb ||
		a.Resolution != "1080p" || !strings.Contains(a.MagnetURI, hash) {
		t.Errorf("E01 = %+v", a)
	}
	if b := e.active(library.ItemTypeEpisode, eps["S01E02"]); b == nil || b.FilePath != e2Path || b.MatchSource != "rules" {
		t.Errorf("E02 = %+v (a partial write keeps the torrent's other rows)", b)
	}
	if _, active := assignmentRow(t, e, library.ItemTypeEpisode, eps["S01E01"], hash, e1Path); active {
		t.Error("E01's earlier row is still active")
	}
	if e.tree.count() != rebuilds+1 {
		t.Errorf("tree rebuilt %d times, want once", e.tree.count()-rebuilds)
	}

	// Back to its own file: the same row, reactivated, now manual.
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E01"], hash, e1Path); err != nil {
		t.Fatal(err)
	}
	id, active := assignmentRow(t, e, library.ItemTypeEpisode, eps["S01E01"], hash, e1Path)
	if id != original || !active {
		t.Errorf("row %d active %v, want %d reactivated", id, active, original)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || a.ID != original || a.MatchSource != "manual" {
		t.Errorf("E01 = %+v", a)
	}

	// What can't be done.
	cases := []struct {
		name    string
		episode int64
		hash    string
		path    string
		check   func(error) bool
	}{
		{"unknown episode", 999_999, hash, e1Path, func(err error) bool { return errors.Is(err, library.ErrEpisodeNotFound) }},
		{"unknown torrent", eps["S01E03"], strings.Repeat("9", 40), e1Path, func(err error) bool { return errors.Is(err, ErrUnknownTorrent) }},
		{"not a hash", eps["S01E03"], "nope", e1Path, func(err error) bool { return errors.Is(err, ErrUnknownTorrent) }},
		{"no such file", eps["S01E03"], hash, root + "/Swap.Show.S01E03.mkv", func(err error) bool { return errors.Is(err, ErrFileNotInTorrent) }},
		{"not a video", eps["S01E03"], hash, root + "/Swap.Show.S01.nfo", func(err error) bool { return validationMessage(err) != "" }},
	}
	for _, c := range cases {
		if err := e.svc.OverrideEpisode(e.ctx, c.episode, c.hash, c.path); !c.check(err) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if e.active(library.ItemTypeEpisode, eps["S01E03"]) != nil {
		t.Error("a refused override wrote a row")
	}
}

// Without a job that stored the file list, it comes from the loaded
// torrent, else from adding the torrent again with a magnet the database
// has; when no peers answer, the override times out.
func TestOverride_FileListSources(t *testing.T) {
	loaded, legacy, silent := strings.Repeat("a1", 20), strings.Repeat("b2", 20), strings.Repeat("c3", 20)
	e := newJobEnv(t, info(legacy, "Legacy.Pack", file("Old.Show.S01E02.mkv", gb)))
	showID, eps := e.show("Old Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	_ = showID
	e.withLoaded(info(loaded, "Loaded Pack", file("Old.Show.S01E01.mkv", gb)))

	// Loaded only: the magnet is made from the hash and the torrent's name.
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E01"], loaded, "Loaded Pack/Old.Show.S01E01.mkv"); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E01"]); a == nil || a.MagnetURI != "magnet:?xt=urn:btih:"+loaded+"&dn=Loaded+Pack" {
		t.Errorf("E01 = %+v", a)
	}
	if addsOf(e.torrents) != 0 {
		t.Errorf("adds = %d, want none for a loaded torrent", addsOf(e.torrents))
	}

	// Known from an old assignment: added again with its magnet.
	e.legacyRow(library.ItemTypeEpisode, eps["S01E03"], legacy, "Legacy.Pack/Old.Show.S01E03.mkv", false)
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E02"], legacy, "Legacy.Pack/Old.Show.S01E02.mkv"); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeEpisode, eps["S01E02"]); a == nil || a.MagnetURI != magnet(legacy)+"&dn=Legacy" {
		t.Errorf("E02 = %+v", a)
	}
	if addsOf(e.torrents) != 1 || len(e.torrents.removals()) != 0 {
		t.Errorf("adds %d, removals %v: want it added and kept (it's in use now)", addsOf(e.torrents), e.torrents.removals())
	}

	// Known, but no peers send the file list.
	e.legacyRow(library.ItemTypeEpisode, eps["S01E03"], silent, "x.mkv", false)
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E03"], silent, "x.mkv"); !errors.Is(err, ErrFileListTimeout) {
		t.Errorf("silent torrent: %v", err)
	}

	// Without a torrent client nothing can be added.
	e.svc.torrents = nil
	if err := e.svc.OverrideEpisode(e.ctx, eps["S01E03"], silent, "x.mkv"); !errors.Is(err, library.ErrTorrentServiceUnavailable) {
		t.Errorf("no client: %v", err)
	}
}

func TestOverride_Movie(t *testing.T) {
	hash := strings.Repeat("d4", 20)
	e := newJobEnv(t, info(hash, "Two Films",
		file("Heat.1995.1080p.mkv", 9*gb), file("Ronin.1998.1080p.mkv", 8*gb)))
	movieID := e.movie("Heat", 1995)
	e.legacyRow(library.ItemTypeMovie, movieID, hash, "Two Films/Ronin.1998.1080p.mkv", true)

	if err := e.svc.OverrideMovie(e.ctx, movieID, hash, "Two Films/Heat.1995.1080p.mkv"); err != nil {
		t.Fatal(err)
	}
	if a := e.active(library.ItemTypeMovie, movieID); a == nil || a.FilePath != "Two Films/Heat.1995.1080p.mkv" || a.MatchSource != "manual" || a.FileSize != 9*gb {
		t.Errorf("movie = %+v", a)
	}
	if err := e.svc.OverrideMovie(e.ctx, 999_999, hash, "Two Films/Heat.1995.1080p.mkv"); !errors.Is(err, library.ErrMovieNotFound) {
		t.Errorf("unknown movie: %v", err)
	}
}

// GET /torrents/{hash}/files: the files in the torrent's order, each with
// what its active assignments map it to.
func TestTorrentFiles(t *testing.T) {
	hash, film := strings.Repeat("e5", 20), strings.Repeat("f6", 20)
	root := "Files.Show.S01.1080p.WEB-GRP"
	e := newJobEnv(t,
		info(hash, root,
			file("Files.Show.S01E01E02.1080p.WEB-GRP.mkv", 2*gb),
			file("Files.Show.S01E03.1080p.WEB-GRP.mkv", gb),
			file("Files.Show.S01.nfo", 1<<10)),
		info(film, "Heat.1995.1080p", file("Heat.1995.1080p.mkv", 9*gb)))
	showID, eps := e.show("Files Show", [][3]any{{1, 1, "One"}, {1, 2, "Two"}, {1, 3, "Three"}})
	e.assignShow(showID, hash, nil)
	movieID := e.movie("Heat", 1995)
	id, _, err := e.svc.Assign(e.ctx, library.JobMovie, movieID, AssignRequest{MagnetURI: magnet(film)})
	if err != nil {
		t.Fatal(err)
	}
	e.run(id)

	files, err := e.svc.TorrentFiles(e.ctx, strings.ToUpper(hash))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0].Path != root+"/Files.Show.S01E01E02.1080p.WEB-GRP.mkv" || files[0].Size != 2*gb {
		t.Fatalf("files = %+v", files)
	}
	want := &FileMapping{Episodes: []ProposalTarget{
		{EpisodeID: eps["S01E01"], Season: 1, Episode: 1, Name: "One"},
		{EpisodeID: eps["S01E02"], Season: 1, Episode: 2, Name: "Two"},
	}}
	if !reflect.DeepEqual(files[0].MappedTo, want) {
		t.Errorf("multi-episode file = %+v", files[0].MappedTo)
	}
	if m := files[1].MappedTo; m == nil || len(m.Episodes) != 1 || m.Episodes[0].Episode != 3 {
		t.Errorf("E03 file = %+v", m)
	}
	if files[2].MappedTo != nil {
		t.Errorf("nfo = %+v", files[2].MappedTo)
	}

	films, err := e.svc.TorrentFiles(e.ctx, film)
	if err != nil || len(films) != 1 || films[0].MappedTo == nil || films[0].MappedTo.MovieID == nil ||
		*films[0].MappedTo.MovieID != movieID || films[0].MappedTo.Episodes != nil {
		t.Errorf("film = %+v, %v", films, err)
	}

	for _, unknown := range []string{strings.Repeat("0", 40), "not-a-hash"} {
		if _, err := e.svc.TorrentFiles(e.ctx, unknown); !errors.Is(err, ErrUnknownTorrent) {
			t.Errorf("%s: %v", unknown, err)
		}
	}
}

// A torrent only an inactive row remembers is added to list its files,
// then dropped again: nothing uses it.
func TestTorrentFiles_AddsAndReleases(t *testing.T) {
	hash := strings.Repeat("a7", 20)
	e := newJobEnv(t, info(hash, "Forgotten", file("Forgotten.S01E01.mkv", gb)))
	_, eps := e.show("Forgotten", [][3]any{{1, 1, "One"}})
	e.legacyRow(library.ItemTypeEpisode, eps["S01E01"], hash, "Forgotten/Forgotten.S01E01.mkv", false)

	files, err := e.svc.TorrentFiles(e.ctx, hash)
	if err != nil || len(files) != 1 || files[0].MappedTo != nil {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if addsOf(e.torrents) != 1 || !reflect.DeepEqual(e.torrents.removals(), []string{hash}) {
		t.Errorf("adds %d, removals %v", addsOf(e.torrents), e.torrents.removals())
	}
}

func TestShowEpisodes(t *testing.T) {
	e := newJobEnv(t)
	showID, eps := e.show("Picker Show", [][3]any{{1, 2, "Two"}, {1, 1, "One"}, {0, 1, "Special"}, {2, 1, "Next"}})
	if _, err := e.db.Exec(`UPDATE shows SET include_specials = FALSE WHERE id = $1`, showID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE episodes SET absolute_number = 7, air_date = NULL WHERE id = $1`, eps["S02E01"]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO seasons (show_id, season_number, name) VALUES ($1, 3, 'The Return')`, showID); err != nil {
		t.Fatal(err)
	}
	e.legacyRow(library.ItemTypeEpisode, eps["S01E02"], strings.Repeat("b8", 20), "two.mkv", true)

	seasons, err := e.svc.ShowEpisodes(e.ctx, showID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range seasons {
		got = append(got, s.Name)
	}
	if !reflect.DeepEqual(got, []string{"Specials", "Season 1", "Season 2", "The Return"}) {
		t.Fatalf("seasons = %v", got)
	}
	s1 := seasons[1].Episodes
	if len(s1) != 2 || s1[0].EpisodeNumber != 1 || s1[0].HasFile || !s1[1].HasFile || s1[1].ID != eps["S01E02"] ||
		s1[0].AirDate == nil || *s1[0].AirDate != "2020-01-01" {
		t.Errorf("season 1 = %+v", s1)
	}
	if next := seasons[2].Episodes[0]; next.AbsoluteNumber == nil || *next.AbsoluteNumber != 7 || next.AirDate != nil {
		t.Errorf("S02E01 = %+v", next)
	}
	if seasons[3].Episodes == nil || len(seasons[3].Episodes) != 0 {
		t.Errorf("an empty season = %+v", seasons[3])
	}

	if _, err := e.svc.ShowEpisodes(e.ctx, 999_999); !errors.Is(err, library.ErrShowNotFound) {
		t.Errorf("unknown show: %v", err)
	}
	empty, _ := e.show("Empty Show", nil)
	if seasons, err := e.svc.ShowEpisodes(e.ctx, empty); err != nil || seasons == nil || len(seasons) != 0 {
		t.Errorf("a show without seasons = %+v, %v", seasons, err)
	}
}
