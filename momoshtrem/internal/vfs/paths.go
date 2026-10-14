package vfs

import (
	"net/url"
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
