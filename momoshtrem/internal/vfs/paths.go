package vfs

import (
	"net/url"
	"sort"
	"strings"
)

// Paths as Infuse sees them over WebDAV. The tree builder uses these too, so
// what the API reports is exactly what the tree serves.

// MovieDir is a movie's folder: "/Movies/Title (Year)".
func MovieDir(title string, year int) string {
	return MoviesPath + "/" + makeMediaFolderName(title, year)
}

// MovieFilePath is a movie's video file: "/Movies/Title (Year)/Title
// (Year).ext", with the extension of sourcePath (the file inside the
// torrent), .mkv when it has none.
func MovieFilePath(title string, year int, sourcePath string) string {
	return MovieDir(title, year) + "/" + makeMediaFolderName(title, year) + getVideoExt(sourcePath)
}

// ShowDir is a show's folder: "/TV Shows/Title (Year)".
func ShowDir(title string, year int) string {
	return TVShowsPath + "/" + makeMediaFolderName(title, year)
}

// SeasonDir is a season's folder: "/TV Shows/Title (Year)/Season 01".
// Season 0 holds specials.
func SeasonDir(showTitle string, showYear, season int) string {
	return ShowDir(showTitle, showYear) + "/" + makeSeasonFolderName(season)
}

// EpisodeFilePath is an episode's video file: "/TV Shows/Title (Year)/Season
// 01/Title - S01E05 - Name.ext". An empty name becomes "Episode N"; the
// extension comes from sourcePath like MovieFilePath's.
func EpisodeFilePath(showTitle string, showYear, season, episode int, name, sourcePath string) string {
	return SeasonDir(showTitle, showYear, season) + "/" +
		makeEpisodeFileName(showTitle, season, episode, name, getVideoExt(sourcePath))
}

// SeasonEpisode is an assigned episode of a season folder, for naming its
// video.
type SeasonEpisode struct {
	Number   int
	Name     string
	InfoHash string
	FilePath string // the file inside the torrent
	// Grouped is set for torrent rows written by assignment jobs
	// (TorrentAssignment.Groupable): episodes that share a file become one
	// entry. Older rows and HDRezka streams keep one entry per episode, so
	// their paths never change.
	Grouped bool
}

// SeasonEntry is one video in a season folder and the episodes it plays.
type SeasonEntry struct {
	FileName string
	Episodes []int // indexes into SeasonEntries' input, in episode order
}

// SeasonEntries names a season folder's videos. Grouped episodes that
// share a file (info hash and path) and follow each other are one entry,
// "Title - S01E01-E02 - FirstName.ext"; a file mapped to episodes with a
// gap (E07 and E09) gets one entry per run, so a name never claims an
// episode it doesn't play. Every other episode is its own "Title - S01E05 -
// Name.ext", exactly as EpisodeFilePath names it. Entries come in episode
// order.
func SeasonEntries(showTitle string, season int, eps []SeasonEpisode) []SeasonEntry {
	order := make([]int, len(eps))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return eps[order[a]].Number < eps[order[b]].Number })

	type fileKey struct{ hash, path string }
	group := map[fileKey]int{} // file → its latest entry
	var entries []SeasonEntry
	for _, i := range order {
		ep := eps[i]
		key := fileKey{ep.InfoHash, ep.FilePath}
		if at, ok := group[key]; ok && ep.Grouped {
			run := entries[at].Episodes
			if eps[run[len(run)-1]].Number+1 == ep.Number {
				entries[at].Episodes = append(run, i)
				continue
			}
		}
		if ep.Grouped {
			group[key] = len(entries)
		}
		entries = append(entries, SeasonEntry{Episodes: []int{i}})
	}
	for n := range entries {
		e := &entries[n]
		first := eps[e.Episodes[0]]
		ext := getVideoExt(first.FilePath)
		if len(e.Episodes) == 1 {
			e.FileName = makeEpisodeFileName(showTitle, season, first.Number, first.Name, ext)
		} else {
			last := eps[e.Episodes[len(e.Episodes)-1]]
			e.FileName = makeMultiEpisodeFileName(showTitle, season, first.Number, last.Number, first.Name, ext)
		}
	}
	return entries
}

// SeasonEpisodePaths returns the WebDAV path of each episode in eps, as
// SeasonEntries names them: episodes grouped into one entry share its path.
func SeasonEpisodePaths(showTitle string, showYear, season int, eps []SeasonEpisode) []string {
	dir := SeasonDir(showTitle, showYear, season)
	paths := make([]string, len(eps))
	for _, e := range SeasonEntries(showTitle, season, eps) {
		for _, i := range e.Episodes {
			paths[i] = dir + "/" + e.FileName
		}
	}
	return paths
}

// PublicURL joins the public WebDAV address (WEBDAV_PUBLIC_URL) and a VFS
// path, escaping each segment: "/TV Shows/Frieren (2023)/" becomes
// "<base>/TV%20Shows/Frieren%20%282023%29/".
func PublicURL(base, vfsPath string) string {
	if !strings.HasPrefix(vfsPath, "/") {
		vfsPath = "/" + vfsPath
	}
	segments := strings.Split(vfsPath, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.TrimRight(base, "/") + strings.Join(segments, "/")
}
