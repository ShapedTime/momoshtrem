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

// SearchParams contains parameters for subtitle search
type SearchParams struct {
	TMDBID        int      // TMDB ID of the movie or TV show
	Type          string   // "movie" or "episode"
	SeasonNumber  int      // Season number (for episodes)
	EpisodeNumber int      // Episode number (for episodes)
	Languages     []string // ISO 639-1 language codes (en, ru, tr, az)
}

// SearchResponse is the API response for subtitle search
type SearchResponse struct {
	TotalPages int               `json:"total_pages"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	Data       []SubtitleResult `json:"data"`
}

// SubtitleResult represents a single subtitle from search results
type SubtitleResult struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Attributes SubtitleAttributes `json:"attributes"`
}

// SubtitleAttributes contains the subtitle metadata
type SubtitleAttributes struct {
	SubtitleID      string  `json:"subtitle_id"`
	Language        string  `json:"language"`        // ISO 639-1 code
	DownloadCount   int     `json:"download_count"`
	NewDownloadCount int    `json:"new_download_count"`
	HearingImpaired bool    `json:"hearing_impaired"`
	HD              bool    `json:"hd"`
	FPS             float64 `json:"fps"`
	Votes           int     `json:"votes"`
	Ratings         float64 `json:"ratings"`
	FromTrusted     bool    `json:"from_trusted"`
	ForeignPartsOnly bool   `json:"foreign_parts_only"`
	AITranslated    bool    `json:"ai_translated"`
	MachineTranslated bool  `json:"machine_translated"`
	Release         string  `json:"release"`
	Comments        string  `json:"comments"`
	LegacySubtitleID int   `json:"legacy_subtitle_id"`
	UploadDate      string  `json:"upload_date"`
	Files           []SubtitleFile `json:"files"`
	FeatureDetails  FeatureDetails `json:"feature_details"`
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
	User  LoginUser `json:"user"`
	Token string    `json:"token"`
	Status int      `json:"status"`
}

// LoginUser contains user info from login
type LoginUser struct {
	AllowedDownloads   int    `json:"allowed_downloads"`
	Level              string `json:"level"`
	UserID             int    `json:"user_id"`
	ExtInstalled       bool   `json:"ext_installed"`
	VIP                bool   `json:"vip"`
}

// DownloadRequest is the request body for download
type DownloadRequest struct {
	FileID int `json:"file_id"`
}

// DownloadResponse is the API response for download
type DownloadResponse struct {
	Link          string `json:"link"`
	FileName      string `json:"file_name"`
	Requests      int    `json:"requests"`
	Remaining     int    `json:"remaining"`
	Message       string `json:"message"`
	ResetTime     string `json:"reset_time"`
	ResetTimeUTC  string `json:"reset_time_utc"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// LanguageName maps ISO 639-1 codes to display names
var LanguageNames = map[string]string{
	"en": "English",
	"ru": "Russian",
	"tr": "Turkish",
	"az": "Azerbaijani",
}

// GetLanguageName returns the display name for a language code
func GetLanguageName(code string) string {
	if name, ok := LanguageNames[code]; ok {
		return name
	}
	return code
}
