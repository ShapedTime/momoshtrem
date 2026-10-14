package opensubtitles

import "context"

// SubtitleFetcher defines the interface for fetching subtitles from external sources.
type SubtitleFetcher interface {
	// IsConfigured returns true if the fetcher is properly configured
	IsConfigured() bool
	// Search searches for subtitles matching the given parameters
	Search(ctx context.Context, params SearchParams) (*SearchResponse, error)
	// Download downloads a subtitle file by file ID, returns content and filename
	Download(ctx context.Context, fileID int) ([]byte, string, error)
}

// SearchParams contains parameters for subtitle search.
//
// Movies are found by their own TMDB ID. Episodes are found by the show's
// TMDB ID (sent as parent_tmdb_id) with the season and episode number: in
// the OpenSubtitles API tmdb_id is the ID "of the movie or episode", and the
// docs recommend "the parent ID, along with the episode and season number"
// for episodes.
// https://opensubtitles.stoplight.io/docs/opensubtitles-api/a172317bd5ccc-search-for-subtitles
type SearchParams struct {
	TMDBID        int      // Movies: the movie's TMDB ID
	ParentTMDBID  int      // Episodes: the show's TMDB ID
	Type          string   // "movie" or "episode"
	SeasonNumber  int      // Season number (for episodes; 0 is specials)
	EpisodeNumber int      // Episode number (for episodes)
	Languages     []string // OpenSubtitles language codes (en, pt-br, ...)
}

// SearchResponse is the API response for subtitle search
type SearchResponse struct {
	TotalPages int              `json:"total_pages"`
	TotalCount int              `json:"total_count"`
	Page       int              `json:"page"`
	Data       []SubtitleResult `json:"data"`
}

// SubtitleResult represents a single subtitle from search results
type SubtitleResult struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Attributes SubtitleAttributes `json:"attributes"`
}

// SubtitleAttributes contains the subtitle metadata
type SubtitleAttributes struct {
	SubtitleID        string         `json:"subtitle_id"`
	Language          string         `json:"language"` // ISO 639-1 code
	DownloadCount     int            `json:"download_count"`
	NewDownloadCount  int            `json:"new_download_count"`
	HearingImpaired   bool           `json:"hearing_impaired"`
	HD                bool           `json:"hd"`
	FPS               float64        `json:"fps"`
	Votes             int            `json:"votes"`
	Ratings           float64        `json:"ratings"`
	FromTrusted       bool           `json:"from_trusted"`
	ForeignPartsOnly  bool           `json:"foreign_parts_only"`
	AITranslated      bool           `json:"ai_translated"`
	MachineTranslated bool           `json:"machine_translated"`
	Release           string         `json:"release"`
	Comments          string         `json:"comments"`
	LegacySubtitleID  int            `json:"legacy_subtitle_id"`
	UploadDate        string         `json:"upload_date"`
	Files             []SubtitleFile `json:"files"`
	FeatureDetails    FeatureDetails `json:"feature_details"`
}

// SubtitleFile represents a file within a subtitle entry
type SubtitleFile struct {
	FileID   int    `json:"file_id"`
	CDNumber int    `json:"cd_number"`
	FileName string `json:"file_name"`
}

// FeatureDetails contains movie/episode info
type FeatureDetails struct {
	FeatureID     int    `json:"feature_id"`
	FeatureType   string `json:"feature_type"`
	Year          int    `json:"year"`
	Title         string `json:"title"`
	MovieName     string `json:"movie_name"`
	IMDBID        int    `json:"imdb_id"`
	TMDBID        int    `json:"tmdb_id"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	ParentIMDBID  int    `json:"parent_imdb_id"`
	ParentTitle   string `json:"parent_title"`
}

// LoginRequest is the request body for login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the API response for login
type LoginResponse struct {
	User   LoginUser `json:"user"`
	Token  string    `json:"token"`
	Status int       `json:"status"`
}

// LoginUser contains user info from login
type LoginUser struct {
	AllowedDownloads int    `json:"allowed_downloads"`
	Level            string `json:"level"`
	UserID           int    `json:"user_id"`
	ExtInstalled     bool   `json:"ext_installed"`
	VIP              bool   `json:"vip"`
}

// DownloadRequest is the request body for download
type DownloadRequest struct {
	FileID int `json:"file_id"`
}

// DownloadResponse is the API response for download
type DownloadResponse struct {
	Link         string `json:"link"`
	FileName     string `json:"file_name"`
	Requests     int    `json:"requests"`
	Remaining    int    `json:"remaining"`
	Message      string `json:"message"`
	ResetTime    string `json:"reset_time"`
	ResetTimeUTC string `json:"reset_time_utc"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}
