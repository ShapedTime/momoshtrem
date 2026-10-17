package catalog

// The availability rules of PLAN §6. library.libraryItemsSQL computes the
// same stages in SQL for /library; keep the two in step.
//
// A file is downloading only while a download (#77) wants it and it isn't
// all on disk, and not while that download is paused (downloads.
// FileStatus.Downloading). A file nothing keeps streams on demand, so it's
// ready as soon as it's assigned, whatever is cached.

// MovieStage is a movie's availability:
//   - none: not in the library;
//   - needs_torrent: no active assignment;
//   - downloading: its file is downloading;
//   - ready: otherwise.
func MovieStage(inLibrary, assigned, downloading bool) Stage {
	switch {
	case !inLibrary:
		return StageNone
	case !assigned:
		return StageNeedsTorrent
	case downloading:
		return StageDownloading
	}
	return StageReady
}

// ShowStage is a show's availability:
//   - none: not in the library;
//   - needs_torrent: nothing assigned, or the frontier isn't empty;
//   - downloading: an assigned episode is downloading;
//   - ready: otherwise.
func ShowStage(inLibrary, anyAssigned, frontier, downloading bool) Stage {
	switch {
	case !inLibrary:
		return StageNone
	case !anyAssigned || frontier:
		return StageNeedsTorrent
	case downloading:
		return StageDownloading
	}
	return StageReady
}

// Aired reports whether an episode with this air date (YYYY-MM-DD, "" when
// unknown) has aired by today. ISO dates compare as strings.
func Aired(airDate, today string) bool {
	return airDate != "" && airDate <= today
}

// EpisodeStateOf is an episode's state: watched when Trakt has a play of it,
// whatever else is true (B5); else ready or downloading with a file;
// without one, needs_review while a file under review targets it, missing
// when it aired, upcoming when it hasn't aired or has no date. A file wins
// over review.
func EpisodeStateOf(watched, hasFile, downloading, underReview bool, airDate, today string) EpisodeState {
	switch {
	case watched:
		return EpisodeWatched
	case hasFile && downloading:
		return EpisodeDownloading
	case hasFile:
		return EpisodeReady
	case underReview:
		return EpisodeNeedsReview
	case Aired(airDate, today):
		return EpisodeMissing
	}
	return EpisodeUpcoming
}
