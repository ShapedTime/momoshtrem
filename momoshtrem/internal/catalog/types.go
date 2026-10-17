package catalog

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/downloads"
)

// The JSON shapes of PLAN §6 "Titles and library", field for field with
// web/src/lib/api/contract/titles.ts and library.ts. Nullable fields are
// pointers; lists are never null.

// Stage is a title's availability_stage.
type Stage string

const (
	StageNone         Stage = "none"
	StageNeedsTorrent Stage = "needs_torrent"
	StageDownloading  Stage = "downloading"
	StageReady        Stage = "ready"
)

// EpisodeState is an episode's state on the title page.
type EpisodeState string

const (
	EpisodeWatched     EpisodeState = "watched"      // B5
	EpisodeReady       EpisodeState = "ready"        // has a file
	EpisodeDownloading EpisodeState = "downloading"  // has a file a download is fetching (#77)
	EpisodeMissing     EpisodeState = "missing"      // aired, no file
	EpisodeUpcoming    EpisodeState = "upcoming"     // not aired, or no air date
	EpisodeNeedsReview EpisodeState = "needs_review" // B2
)

// TorrentState is a movie torrent's state in the client.
type TorrentState string

const (
	TorrentActive    TorrentState = "active"
	TorrentIdle      TorrentState = "idle"
	TorrentNotLoaded TorrentState = "not_loaded"
	// TorrentPaused: the user paused it (B6), loaded or not. It wins over
	// the others.
	TorrentPaused TorrentState = "paused"
)

// WatchState is what Trakt (B5) knows about a movie; a title's is null
// when it knows nothing (PLAN §6 "Watch state rules").
type WatchState struct {
	WatchedAt *time.Time `json:"watched_at"` // the last play; a show: its last watched episode
	Plays     int        `json:"plays"`      // a show: the sum of its episodes' plays
	Rating    *int       `json:"rating"`
}

// ShowWatchState is a show's: a movie's, plus Trakt's progress and the
// episode Up next names.
type ShowWatchState struct {
	WatchState
	// Progress is left out until Trakt's progress for the show is in.
	Progress    *WatchProgress `json:"progress,omitempty"`
	NextEpisode *NextEpisode   `json:"next_episode"` // null when there's none
}

// WatchProgress is a show's regular episodes watched of aired.
type WatchProgress struct {
	Completed int `json:"completed"`
	Aired     int `json:"aired"`
}

// NextEpisode is an episode to watch next: in the library's numbering and
// with its absolute number, name and air date when the library lists it,
// else as Trakt names it.
type NextEpisode struct {
	Season         int     `json:"season"`
	Episode        int     `json:"episode"`
	AbsoluteNumber *int    `json:"absolute_number"`
	Name           *string `json:"name"`
	AirDate        *string `json:"air_date"`
}

// DownloadProgress is a title's downloading files (#77): those a download
// wants that aren't all on disk.
type DownloadProgress struct {
	Pct      int   `json:"pct"`       // 0–100: what's on disk of them, from the piece cache
	SpeedBps int64 `json:"speed_bps"` // smoothed; shows: summed over their torrents, each once
	// ETASeconds is set while they download with a speed (every torrent of
	// them is in a download slot); null otherwise.
	ETASeconds *int `json:"eta_seconds"`
	Seeders    int  `json:"seeders"`
}

// TitleTorrent is a movie's assigned torrent.
type TitleTorrent struct {
	InfoHash    string       `json:"info_hash"`
	ReleaseName string       `json:"release_name"`
	FileName    string       `json:"file_name"`
	SizeBytes   int64        `json:"size_bytes"`
	Resolution  *string      `json:"resolution"`
	Source      *string      `json:"source"`
	AddedAt     time.Time    `json:"added_at"`
	State       TorrentState `json:"state"`
	Seeders     *int         `json:"seeders"` // null unless loaded
}

// MissingRun is the first run of the frontier: aired, unassigned regular
// episodes after the last one with a file or a play, within one season.
type MissingRun struct {
	SeasonNumber int  `json:"season_number"`
	From         int  `json:"from"` // in-season episode numbers
	To           int  `json:"to"`
	AbsoluteFrom *int `json:"absolute_from"`
	AbsoluteTo   *int `json:"absolute_to"`
	Count        int  `json:"count"`
}

// TitleLibraryState is where a title is: not in the library, needs a
// torrent, downloading or ready.
type TitleLibraryState struct {
	InLibrary         bool              `json:"in_library"`
	LibraryID         *int64            `json:"library_id"`
	AddedAt           *time.Time        `json:"added_at"`
	AvailabilityStage Stage             `json:"availability_stage"`
	WebDAVPath        *string           `json:"webdav_path"`
	WebDAVURL         *string           `json:"webdav_url"`
	Download          *DownloadProgress `json:"download"`
	NeedsReview       int               `json:"needs_review"`
	ReviewJobID       *int64            `json:"review_job_id"`
	EpisodesTotal     *int              `json:"episodes_total"`
	EpisodesReady     *int              `json:"episodes_ready"`
	Missing           *MissingRun       `json:"missing"`
	Torrent           *TitleTorrent     `json:"torrent"`
	Stream            *TitleStream      `json:"stream"` // movies playing from HDRezka; Torrent is null then
	// Keep is a movie's download, or a show's show-scope one (#77); null
	// when there's none.
	Keep *downloads.Keep `json:"keep"`
}

// TitleStream is a movie's HDRezka stream (PLAN §6 "HDRezka (#63)").
type TitleStream struct {
	Provider   string    `json:"provider"` // "hdrezka"
	Title      string    `json:"title"`    // the title on HDRezka
	URL        string    `json:"url"`      // its page on the configured mirror
	Translator string    `json:"translator"`
	Quality    string    `json:"quality"` // e.g. "1080p"
	SizeBytes  int64     `json:"size_bytes"`
	AddedAt    time.Time `json:"added_at"`
}

// EpisodeStream is an episode's HDRezka stream.
type EpisodeStream struct {
	Translator string `json:"translator"`
	Quality    string `json:"quality"`
}

// Sources of an episode's file (EpisodeSummary.Source).
const (
	SourceTorrent = "torrent"
	SourceHDRezka = "hdrezka"
)

// CastMember is one credited actor.
type CastMember struct {
	Name        string  `json:"name"`
	Character   string  `json:"character"`
	ProfilePath *string `json:"profile_path"`
}

// SimilarLibrary is a similar title's place in the library.
type SimilarLibrary struct {
	LibraryID         int64 `json:"library_id"`
	AvailabilityStage Stage `json:"availability_stage"`
	EpisodesTotal     *int  `json:"episodes_total"`
	EpisodesReady     *int  `json:"episodes_ready"`
}

// SimilarTitle is a "More like this" entry.
type SimilarTitle struct {
	MediaType  string          `json:"media_type"`
	TMDBID     int             `json:"tmdb_id"`
	Title      string          `json:"title"`
	Year       *int            `json:"year"`
	PosterPath *string         `json:"poster_path"`
	Library    *SimilarLibrary `json:"library"`
}

// TitleBase is what movie and show titles share.
type TitleBase struct {
	MediaType             string            `json:"media_type"`
	TMDBID                int               `json:"tmdb_id"`
	Title                 string            `json:"title"`
	OriginalTitle         *string           `json:"original_title"`
	Year                  *int              `json:"year"`
	Overview              string            `json:"overview"`
	PosterPath            *string           `json:"poster_path"`
	BackdropPath          *string           `json:"backdrop_path"`
	BackdropDominantColor *string           `json:"backdrop_dominant_color"`
	Genres                []string          `json:"genres"`
	VoteAverage           *float64          `json:"vote_average"`
	VoteCount             int               `json:"vote_count"`
	OriginalLanguage      *string           `json:"original_language"`
	Cast                  []CastMember      `json:"cast"`
	TrailerYouTubeKey     *string           `json:"trailer_youtube_key"`
	Similar               []SimilarTitle    `json:"similar"`
	Library               TitleLibraryState `json:"library"`
	RecommendationReason  *string           `json:"recommendation_reason"`
}

// MovieTitle is GET /titles/movie/{tmdb_id}.
type MovieTitle struct {
	TitleBase
	WatchState *WatchState `json:"watch_state"`
	Runtime    *int        `json:"runtime"`
	Directors  []string    `json:"directors"`
}

// SeasonSummary is one season on a show's title page. For a show that isn't
// in the library only EpisodesTotal is known.
type SeasonSummary struct {
	SeasonNumber  int    `json:"season_number"`
	Name          string `json:"name"`
	EpisodesTotal int    `json:"episodes_total"` // upcoming included
	Ready         int    `json:"ready"`          // with a file
	Missing       int    `json:"missing"`
	NeedsReview   int    `json:"needs_review"`
	Upcoming      int    `json:"upcoming"`
	AbsoluteFrom  *int   `json:"absolute_from"`
	AbsoluteTo    *int   `json:"absolute_to"`
}

// ShowTitle is GET /titles/tv/{tmdb_id}.
type ShowTitle struct {
	TitleBase
	WatchState      *ShowWatchState `json:"watch_state"`
	Status          *string         `json:"status"`
	IsAnime         bool            `json:"is_anime"`
	EpisodeOrdering string          `json:"episode_ordering"`
	EpisodeGroupID  *string         `json:"episode_group_id"`
	IncludeSpecials bool            `json:"include_specials"`
	ClassifierMode  string          `json:"classifier_mode"`
	Creators        []string        `json:"creators"`
	Networks        []string        `json:"networks"`
	EndYear         *int            `json:"end_year"`
	EpisodeRunTime  *int            `json:"episode_run_time"`
	Seasons         []SeasonSummary `json:"seasons"` // specials last
	DefaultSeason   *int            `json:"default_season"`
}

// EpisodeSummary is one episode of GET /titles/tv/{id}/seasons/{n}.
type EpisodeSummary struct {
	ID             int64          `json:"id"`
	SeasonNumber   int            `json:"season_number"`
	EpisodeNumber  int            `json:"episode_number"`
	AbsoluteNumber *int           `json:"absolute_number"`
	Name           string         `json:"name"`
	Overview       string         `json:"overview"`
	AirDate        *string        `json:"air_date"`
	StillPath      *string        `json:"still_path"`
	Runtime        *int           `json:"runtime"`
	EpisodeType    *string        `json:"episode_type"`
	State          EpisodeState   `json:"state"`       // watched wins; the fields below still describe the file
	Pct            *int           `json:"pct"`         // while downloading
	WebDAVPath     *string        `json:"webdav_path"` // while it has a file
	Source         *string        `json:"source"`      // the file's: torrent or hdrezka; null without one
	Stream         *EpisodeStream `json:"stream"`      // an HDRezka file's stream
	// Keep is what holds the episode (#77): its own download, its season's,
	// its show's or a smart pick (kept wins, then the narrowest), with the
	// state and pct of its own file; null when nothing does, or an
	// until-watched show or season keep released it as watched.
	Keep *downloads.Keep `json:"keep"`
}

// EpisodeRange is a jump target in a long season.
type EpisodeRange struct {
	Label        string `json:"label"`
	Kind         string `json:"kind"` // "arc" or "block"
	From         int    `json:"from"` // in-season episode numbers
	To           int    `json:"to"`
	AbsoluteFrom *int   `json:"absolute_from"`
	AbsoluteTo   *int   `json:"absolute_to"`
	Missing      int    `json:"missing"`
}

// SeasonDetail is GET /titles/tv/{tmdb_id}/seasons/{n}.
type SeasonDetail struct {
	SeasonNumber int              `json:"season_number"`
	Name         string           `json:"name"`
	Episodes     []EpisodeSummary `json:"episodes"`
	Ranges       []EpisodeRange   `json:"ranges"`
	Keep         *downloads.Keep  `json:"keep"` // the season-scope download (#77); null when there's none
}

// LibraryItem is one row of GET /library.
type LibraryItem struct {
	ID                int64      `json:"id"`
	MediaType         string     `json:"media_type"`
	TMDBID            int        `json:"tmdb_id"`
	Title             string     `json:"title"`
	Year              *int       `json:"year"`
	PosterPath        *string    `json:"poster_path"`
	AvailabilityStage Stage      `json:"availability_stage"`
	EpisodesTotal     *int       `json:"episodes_total"`
	EpisodesReady     *int       `json:"episodes_ready"`
	EpisodesWatched   *int       `json:"episodes_watched"`
	NeedsReview       int        `json:"needs_review"`
	SizeBytes         int64      `json:"size_bytes"`
	AddedAt           time.Time  `json:"added_at"`
	LastWatchedAt     *time.Time `json:"last_watched_at"`
	Runtime           *int       `json:"runtime"`    // movies: minutes; null for shows and when unknown
	Resolution        *string    `json:"resolution"` // movies: the assigned file's
	// WebDAVPath and WebDAVURL are the title page's: null until something
	// is assigned.
	WebDAVPath  *string          `json:"webdav_path"`
	WebDAVURL   *string          `json:"webdav_url"`
	ReviewJobID *int64           `json:"review_job_id"` // the mapping job to review (review rules)
	Download    *LibraryDownload `json:"download"`      // while a file is downloading (a download wants it, #77)
	Missing     *MissingRun      `json:"missing"`       // shows: the first frontier run
	Streaming   bool             `json:"streaming"`     // a streaming session plays it (a show: any episode)
}

// LibraryDownload is how far along a library title's downloading files
// are: those a download wants that aren't all on disk (#77).
type LibraryDownload struct {
	Pct int `json:"pct"` // on disk, from the piece cache; shows: combined over the downloading files
	// InfoHash is one of the torrents being fetched, for "See download".
	InfoHash string `json:"info_hash"`
}

// LibraryCounts are /library's filter counts. They honour q and ids, never
// type or status.
type LibraryCounts struct {
	All         int `json:"all"`
	Movie       int `json:"movie"`
	TV          int `json:"tv"`
	Watching    int `json:"watching"` // shows started on Trakt, not finished, dropped or hidden
	Ready       int `json:"ready"`
	Incomplete  int `json:"incomplete"`
	NeedsReview int `json:"needs_review"`
}

// LibrarySummary is the whole library, whatever the parameters.
type LibrarySummary struct {
	Total     int   `json:"total"`
	SizeBytes int64 `json:"size_bytes"`
	Ready     int   `json:"ready"`
}

// LibraryPage is GET /library: a Paginated<LibraryItem>, plus the filter
// counts and the library's summary.
type LibraryPage struct {
	Items    []LibraryItem  `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Counts   LibraryCounts  `json:"counts"`
	Summary  LibrarySummary `json:"summary"`
}

// EpisodeGroup is one entry of GET /shows/{id}/episode-groups.
type EpisodeGroup struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	GroupCount   int    `json:"group_count"`
	EpisodeCount int    `json:"episode_count"`
}
