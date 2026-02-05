package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// TorrentAdder defines the torrent operations needed by ShowAssignmentService.
type TorrentAdder interface {
	AddTorrent(magnetURI string) (*torrent.TorrentInfo, error)
}

// Compile-time verification
var _ TorrentAdder = (torrent.Service)(nil)

// EpisodeIdentifier defines the identification operations.
type EpisodeIdentifier interface {
	Identify(files []identify.TorrentFile, torrentName string) *identify.IdentificationResult
}

// Compile-time verification
var _ EpisodeIdentifier = (*identify.Identifier)(nil)

// SubtitleCreator defines subtitle operations needed for torrent subtitles.
type SubtitleCreator interface {
	CreateTorrentSubtitle(ctx context.Context, sub *subtitle.Subtitle) error
}

// Compile-time verification
var _ SubtitleCreator = (*subtitle.Service)(nil)

// ShowAssignmentService handles torrent-to-show assignment operations.
type ShowAssignmentService struct {
	showRepo        *library.ShowRepository
	assignmentRepo  *library.AssignmentRepository
	torrentAdder    TorrentAdder
	identifier      EpisodeIdentifier
	treeUpdater     vfs.TreeUpdater // Optional
	subtitleCreator SubtitleCreator // Optional
	log             *slog.Logger
}

// AssignmentServiceOption configures optional dependencies.
type AssignmentServiceOption func(*ShowAssignmentService)

// WithTreeUpdater configures VFS tree updates.
func WithTreeUpdater(tu vfs.TreeUpdater) AssignmentServiceOption {
	return func(s *ShowAssignmentService) {
		s.treeUpdater = tu
	}
}

// WithSubtitleCreator configures torrent subtitle creation.
func WithSubtitleCreator(sc SubtitleCreator) AssignmentServiceOption {
	return func(s *ShowAssignmentService) {
		s.subtitleCreator = sc
	}
}

// NewShowAssignmentService creates a new ShowAssignmentService.
func NewShowAssignmentService(
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
	torrentAdder TorrentAdder,
	identifier EpisodeIdentifier,
	opts ...AssignmentServiceOption,
) *ShowAssignmentService {
	s := &ShowAssignmentService{
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		torrentAdder:   torrentAdder,
		identifier:     identifier,
		log:            slog.With("component", "show-assignment-service"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetSubtitleCreator configures the subtitle creator after construction.
// This is useful when the subtitle service is optional and configured later.
func (s *ShowAssignmentService) SetSubtitleCreator(sc SubtitleCreator) {
	s.subtitleCreator = sc
}

// AssignmentSummary contains counts of the assignment operation.
type AssignmentSummary struct {
	TotalFiles     int `json:"total_files"`
	Matched        int `json:"matched"`
	Unmatched      int `json:"unmatched"`
	Skipped        int `json:"skipped"`
	SubtitlesFound int `json:"subtitles_found"`
}

// MatchedAssignment represents a successful episode-to-file match.
type MatchedAssignment struct {
	EpisodeID  int64  `json:"episode_id"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Resolution string `json:"resolution"`
	Confidence string `json:"confidence"`
}

// UnmatchedAssignment represents a file that couldn't be matched.
type UnmatchedAssignment struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
}

// ShowAssignmentResult contains the result of a torrent assignment.
type ShowAssignmentResult struct {
	Matched   []MatchedAssignment
	Unmatched []UnmatchedAssignment
	Summary   AssignmentSummary
}

// AssignTorrent assigns a torrent to a show, auto-detecting episodes.
func (s *ShowAssignmentService) AssignTorrent(
	ctx context.Context,
	showID int64,
	magnetURI string,
) (*ShowAssignmentResult, error) {
	// 1. Validate magnet URI and extract info hash
	infoHash := torrent.ExtractInfoHash(magnetURI)
	if infoHash == "" {
		return nil, library.ErrInvalidMagnet
	}

	// 2. Load show with all seasons/episodes
	show, err := s.showRepo.GetWithSeasonsAndEpisodes(showID)
	if err != nil {
		return nil, fmt.Errorf("failed to load show: %w", err)
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}

	// 3. Check torrent service availability
	if s.torrentAdder == nil {
		return nil, library.ErrTorrentServiceUnavailable
	}

	// 4. Add torrent and get file list
	torrentInfo, err := s.torrentAdder.AddTorrent(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	// 5. Identify episodes in the torrent
	identResult := s.identifier.Identify(torrentInfo.Files, torrentInfo.Name)

	// 6. Match identified files to library episodes
	matchResult := identify.MatchToShow(show, identResult)

	// 7. Create assignments for matched episodes
	result := &ShowAssignmentResult{
		Matched:   make([]MatchedAssignment, 0, len(matchResult.Matched)),
		Unmatched: make([]UnmatchedAssignment, 0, len(matchResult.Unmatched)),
	}

	episodesForTree := make([]vfs.EpisodeWithContext, 0, len(matchResult.Matched))

	for _, m := range matchResult.Matched {
		assignment := &library.TorrentAssignment{
			ItemType:   library.ItemTypeEpisode,
			ItemID:     m.Episode.ID,
			InfoHash:   infoHash,
			MagnetURI:  magnetURI,
			FilePath:   m.FilePath,
			FileSize:   m.FileSize,
			Resolution: m.Quality.Resolution,
			Source:     m.Quality.Source,
		}

		if err := s.assignmentRepo.Create(assignment); err != nil {
			s.log.Error("Failed to create assignment",
				"episode_id", m.Episode.ID,
				"error", err,
			)
			continue
		}

		result.Matched = append(result.Matched, MatchedAssignment{
			EpisodeID:  m.Episode.ID,
			Season:     m.Season.SeasonNumber,
			Episode:    m.Episode.EpisodeNumber,
			FilePath:   m.FilePath,
			FileSize:   m.FileSize,
			Resolution: m.Quality.Resolution,
			Confidence: string(m.Confidence),
		})

		episodesForTree = append(episodesForTree, vfs.EpisodeWithContext{
			ShowTitle:    show.Title,
			ShowYear:     show.Year,
			SeasonNumber: m.Season.SeasonNumber,
			Episode:      m.Episode,
			Assignment:   assignment,
		})
	}

	// 8. Update VFS tree
	if s.treeUpdater != nil && len(episodesForTree) > 0 {
		s.treeUpdater.AddEpisodesToTree(episodesForTree)
	}

	// 9. Process matched subtitles
	subtitlesCreated := 0
	if s.subtitleCreator != nil && len(matchResult.MatchedSubtitles) > 0 {
		subtitlesCreated = s.createSubtitles(ctx, matchResult.MatchedSubtitles, infoHash)

		if subtitlesCreated > 0 && s.treeUpdater != nil {
			s.treeUpdater.InvalidateTree()
		}
	}

	// 10. Build unmatched response
	for _, u := range matchResult.Unmatched {
		result.Unmatched = append(result.Unmatched, UnmatchedAssignment{
			FilePath: u.FilePath,
			Reason:   string(u.Reason),
			Season:   u.Season,
			Episode:  u.Episode,
		})

		s.log.Warn("Unmatched file in torrent",
			"show_id", showID,
			"show_title", show.Title,
			"info_hash", infoHash,
			"file_path", u.FilePath,
			"reason", u.Reason,
			"parsed_season", u.Season,
			"parsed_episode", u.Episode,
		)
	}

	// 11. Calculate summary
	skipped := identResult.TotalFiles - len(identResult.IdentifiedFiles) - len(identResult.UnidentifiedFiles)
	if skipped < 0 {
		skipped = 0
	}

	result.Summary = AssignmentSummary{
		TotalFiles:     identResult.TotalFiles,
		Matched:        len(result.Matched),
		Unmatched:      len(result.Unmatched),
		Skipped:        skipped,
		SubtitlesFound: subtitlesCreated,
	}

	return result, nil
}

// createSubtitles creates subtitle records for matched subtitles.
func (s *ShowAssignmentService) createSubtitles(
	ctx context.Context,
	matched []identify.MatchedSubtitle,
	infoHash string,
) int {
	created := 0

	for _, ms := range matched {
		sub := &subtitle.Subtitle{
			ItemType:     subtitle.ItemTypeEpisode,
			ItemID:       ms.Episode.ID,
			LanguageCode: ms.LanguageCode,
			LanguageName: ms.LanguageName,
			Format:       ms.Format,
			FilePath:     ms.FilePath,
			FileSize:     ms.FileSize,
			Source:       subtitle.SourceTorrent,
			InfoHash:     infoHash,
		}

		if err := s.subtitleCreator.CreateTorrentSubtitle(ctx, sub); err != nil {
			s.log.Error("Failed to create torrent subtitle",
				"episode_id", ms.Episode.ID,
				"file_path", ms.FilePath,
				"error", err,
			)
			continue
		}

		created++
		s.log.Info("Torrent subtitle assigned",
			"episode_id", ms.Episode.ID,
			"season", ms.Season.SeasonNumber,
			"episode", ms.Episode.EpisodeNumber,
			"language", ms.LanguageCode,
			"file_path", ms.FilePath,
		)
	}

	return created
}
