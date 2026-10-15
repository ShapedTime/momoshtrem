package service

import (
	"context"
	"fmt"
	"log/slog"
	"path"

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
	opts ...AssignmentServiceOption,
) *ShowAssignmentService {
	s := &ShowAssignmentService{
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		torrentAdder:   torrentAdder,
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
	TotalFiles     int `json:"total_files"` // videos and subtitles
	Matched        int `json:"matched"`     // assignment rows: one per episode
	Unmatched      int `json:"unmatched"`
	Skipped        int `json:"skipped"` // extras: samples, openings, trailers, older versions...
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

// UnmatchedAssignment represents a file that couldn't be matched. Reason is
// the classifier's (or a conflict's) explanation; Season and Episode are -1.
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

// episodeRef is a library episode and its season number.
type episodeRef struct {
	episode *library.Episode
	season  int
}

// torrentSubtitle is a subtitle file for one episode.
type torrentSubtitle struct {
	ref  episodeRef
	path string
	size int64
}

// AssignTorrent assigns a torrent to a show synchronously (the legacy
// path): it classifies the torrent's files and assigns every file the
// classifier gives a target, whatever the confidence. Files in conflict
// (two files on one episode) are both left out and reported unmatched.
// Specials go to season 0; a multi-episode file gets one row per episode.
func (s *ShowAssignmentService) AssignTorrent(
	ctx context.Context,
	showID int64,
	magnetURI string,
) (*ShowAssignmentResult, error) {
	infoHash := torrent.ExtractInfoHash(magnetURI)
	if infoHash == "" {
		return nil, library.ErrInvalidMagnet
	}

	show, err := s.showRepo.GetWithSeasonsAndEpisodesContext(ctx, showID)
	if err != nil {
		return nil, fmt.Errorf("failed to load show: %w", err)
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}
	if s.torrentAdder == nil {
		return nil, library.ErrTorrentServiceUnavailable
	}
	torrentInfo, err := s.torrentAdder.AddTorrent(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	res := identify.Classify(identify.Input{
		TorrentName: torrentInfo.Name,
		Files:       torrentInfo.Files,
		Show:        ShowCatalog(show),
	})
	refs := episodeRefs(show)
	conflicts := identify.Conflicts(res.Proposals)
	byPath := make(map[string]int, len(res.Proposals))
	for i, p := range res.Proposals {
		byPath[p.Path] = i
	}

	result := &ShowAssignmentResult{
		Matched:   make([]MatchedAssignment, 0),
		Unmatched: make([]UnmatchedAssignment, 0),
	}
	unmatched := func(p identify.Proposal, reason string) {
		result.Unmatched = append(result.Unmatched, UnmatchedAssignment{FilePath: p.Path, Reason: reason, Season: -1, Episode: -1})
		s.log.Warn("Unmatched file in torrent",
			"show_id", showID, "show_title", show.Title, "info_hash", infoHash,
			"file_path", p.Path, "reason", reason)
	}

	var episodesForTree []vfs.EpisodeWithContext
	var subs []torrentSubtitle
	// An episode moving to another torrent leaves the old torrent's
	// subtitles in the tree (and its cache) until a rebuild.
	torrentChanged := false

	for i, p := range res.Proposals {
		class := identify.ClassOf(p.Path)
		if class != identify.ClassVideo && class != identify.ClassSubtitle {
			continue // fonts, images, info files: not counted, as before
		}
		result.Summary.TotalFiles++

		switch {
		case p.Kind == identify.KindExtra || p.Kind == identify.KindSample:
			result.Summary.Skipped++
			continue
		case !p.HasTarget():
			unmatched(p, p.Reason)
			continue
		}

		if p.Kind == identify.KindSubtitle {
			if v, ok := byPath[p.LinkedVideo]; ok && len(conflicts[v]) > 0 {
				unmatched(p, "Its video conflicts with another file")
				continue
			}
			for _, t := range p.Targets {
				if ref, ok := refs[t.EpisodeID]; ok {
					subs = append(subs, torrentSubtitle{ref: ref, path: p.Path, size: p.Size})
				}
			}
			continue
		}

		if others := conflicts[i]; len(others) > 0 {
			unmatched(p, fmt.Sprintf("Conflicts with %s: both map to %s", clipName(path.Base(res.Proposals[others[0]].Path)), targetLabel(p.Targets)))
			continue
		}

		quality := identify.ParseQuality(p.Path)
		for _, t := range p.Targets {
			ref, ok := refs[t.EpisodeID]
			if !ok {
				continue // the catalog came from this show, so this doesn't happen
			}
			if prev := ref.episode.Assignment; prev != nil && prev.InfoHash != infoHash {
				torrentChanged = true
			}
			assignment := &library.TorrentAssignment{
				ItemType:   library.ItemTypeEpisode,
				ItemID:     ref.episode.ID,
				InfoHash:   infoHash,
				MagnetURI:  magnetURI,
				FilePath:   p.Path,
				FileSize:   p.Size,
				Resolution: quality.Resolution,
				Source:     quality.Source,
			}
			if err := s.assignmentRepo.Create(assignment); err != nil {
				s.log.Error("Failed to create assignment", "episode_id", ref.episode.ID, "error", err)
				continue
			}
			result.Matched = append(result.Matched, MatchedAssignment{
				EpisodeID:  ref.episode.ID,
				Season:     ref.season,
				Episode:    ref.episode.EpisodeNumber,
				FilePath:   p.Path,
				FileSize:   p.Size,
				Resolution: quality.Resolution,
				Confidence: string(p.Confidence),
			})
			episodesForTree = append(episodesForTree, vfs.EpisodeWithContext{
				ShowTitle:    show.Title,
				ShowYear:     show.Year,
				SeasonNumber: ref.season,
				Episode:      ref.episode,
				Assignment:   assignment,
			})
		}
	}

	if s.treeUpdater != nil && len(episodesForTree) > 0 {
		s.treeUpdater.AddEpisodesToTree(episodesForTree)
	}

	subtitlesCreated := 0
	if s.subtitleCreator != nil && len(subs) > 0 {
		subtitlesCreated = s.createSubtitles(ctx, subs, infoHash, magnetURI)
	}

	// Targeted updates don't touch subtitle files: rebuild when there are
	// new ones, or an earlier torrent's must go.
	if s.treeUpdater != nil && (subtitlesCreated > 0 || torrentChanged) {
		s.treeUpdater.InvalidateTree()
	}

	result.Summary.Matched = len(result.Matched)
	result.Summary.Unmatched = len(result.Unmatched)
	result.Summary.SubtitlesFound = subtitlesCreated
	return result, nil
}

// episodeRefs indexes a show's episodes by ID.
func episodeRefs(show *library.Show) map[int64]episodeRef {
	refs := make(map[int64]episodeRef)
	for i := range show.Seasons {
		season := &show.Seasons[i]
		for j := range season.Episodes {
			ep := &season.Episodes[j]
			refs[ep.ID] = episodeRef{episode: ep, season: season.SeasonNumber}
		}
	}
	return refs
}

// clipName shortens a file name to 40 characters for a reason, which must
// stay within identify.MaxReasonRunes.
func clipName(name string) string {
	const n = 40
	if r := []rune(name); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return name
}

// targetLabel writes targets as "S01E05" or "S01E05-E06".
func targetLabel(targets []identify.Target) string {
	if len(targets) == 0 {
		return ""
	}
	first, last := targets[0], targets[len(targets)-1]
	label := fmt.Sprintf("S%02dE%02d", first.Season, first.Episode)
	if len(targets) > 1 {
		label += fmt.Sprintf("-E%02d", last.Episode)
	}
	return label
}

// createSubtitles creates subtitle records for matched subtitles. Each
// replaces the episode's subtitles from any earlier torrent.
func (s *ShowAssignmentService) createSubtitles(
	ctx context.Context,
	subs []torrentSubtitle,
	infoHash, magnetURI string,
) int {
	created := 0

	for _, ts := range subs {
		langCode, langName, _ := subtitle.DetectLanguage(ts.path)
		sub := &subtitle.Subtitle{
			ItemType:     subtitle.ItemTypeEpisode,
			ItemID:       ts.ref.episode.ID,
			LanguageCode: langCode,
			LanguageName: langName,
			Format:       subtitle.ParseFormat(ts.path),
			FilePath:     ts.path,
			FileSize:     ts.size,
			Source:       subtitle.SourceTorrent,
			InfoHash:     infoHash,
			ReleaseName:  subtitle.AssignedRelease(magnetURI, ts.path),
		}

		if err := s.subtitleCreator.CreateTorrentSubtitle(ctx, sub); err != nil {
			s.log.Error("Failed to create torrent subtitle",
				"episode_id", ts.ref.episode.ID,
				"file_path", ts.path,
				"error", err,
			)
			continue
		}

		created++
		s.log.Info("Torrent subtitle assigned",
			"episode_id", ts.ref.episode.ID,
			"season", ts.ref.season,
			"episode", ts.ref.episode.EpisodeNumber,
			"language", langCode,
			"file_path", ts.path,
		)
	}

	return created
}
