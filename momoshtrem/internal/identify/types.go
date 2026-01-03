package identify

// Confidence represents the confidence level of episode identification
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
	ConfidenceNone   Confidence = "none"
)

// FileType represents the type of media file
type FileType string

const (
	FileTypeVideo    FileType = "video"
	FileTypeSubtitle FileType = "subtitle"
)

// QualityInfo contains quality metadata extracted from filenames
type QualityInfo struct {
	Resolution string `json:"resolution"` // 2160p, 1080p, 720p, 480p
	Source     string `json:"source"`     // BluRay, WEB-DL, HDTV
	Codec      string `json:"codec"`      // x264, x265, HEVC
	HDR        bool   `json:"hdr"`
}

// IdentifiedFile represents a file with identified episode information
type IdentifiedFile struct {
	FilePath         string      `json:"file_path"`
	FileSize         int64       `json:"file_size"`
	FileType         FileType    `json:"file_type"`
	Season           int         `json:"season"`
	Episodes         []int       `json:"episodes"`           // Expanded list, e.g., [1,2,3,4,5] for E01-E05
	IsSpecial        bool        `json:"is_special"`
	Quality          QualityInfo `json:"quality"`
	Confidence       Confidence  `json:"confidence"`
	PatternUsed      string      `json:"pattern_used"`
	NeedsReview      bool        `json:"needs_review"`
	SeasonFromFolder bool        `json:"season_from_folder"` // true if season extracted from folder path
}

// IdentificationResult is the result of identifying episodes in a torrent
type IdentificationResult struct {
	TorrentName       string           `json:"torrent_name"`
	IdentifiedFiles   []IdentifiedFile `json:"identified_files"`
	UnidentifiedFiles []string         `json:"unidentified_files"`
	TotalFiles        int              `json:"total_files"`
	IdentifiedCount   int              `json:"identified_count"`
}

// TorrentFile represents a file in a torrent (input to identifier)
type TorrentFile struct {
	Path string
	Size int64
}
