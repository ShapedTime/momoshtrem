package subtitle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
)

var (
	// ErrNotConfigured: no OpenSubtitles API key, so nothing can be searched
	// or downloaded. Stored subtitles still list.
	ErrNotConfigured = errors.New("OpenSubtitles is not configured")
	// ErrTorrentSubtitle: the subtitle came with the torrent and goes with
	// it; it can't be deleted on its own.
	ErrTorrentSubtitle = errors.New("subtitle came with the torrent")
	// ErrInvalidLanguage: a language code that isn't "en", "pt-br" and so on.
	ErrInvalidLanguage = errors.New("invalid language code")
	// ErrFetch wraps OpenSubtitles failures (search or download).
	ErrFetch = errors.New("OpenSubtitles request failed")
)

// DefaultLanguage is searched when a request names none.
const DefaultLanguage = "en"

// Service handles subtitle operations including search, download, and storage.
type Service struct {
	fetcher       opensubtitles.SubtitleFetcher
	repo          SubtitleRepository
	subtitlesPath string
}

// NewService creates a new subtitle service. fetcher may be nil when
// OpenSubtitles isn't configured: stored and torrent subtitles still work.
func NewService(fetcher opensubtitles.SubtitleFetcher, repo SubtitleRepository, subtitlesPath string) *Service {
	return &Service{
		fetcher:       fetcher,
		repo:          repo,
		subtitlesPath: subtitlesPath,
	}
}

// IsConfigured returns true if the service has a configured fetcher.
func (s *Service) IsConfigured() bool {
	return s.fetcher != nil && s.fetcher.IsConfigured()
}

// ItemContext is what the service needs to know about a library item.
type ItemContext struct {
	Item Item
	// Search finds the item on OpenSubtitles; Languages is set per request.
	Search opensubtitles.SearchParams
	// InfoHash is the active assignment's torrent; "" when nothing is assigned.
	InfoHash string
	// Release names the assigned release (see AssignedRelease); "" when
	// nothing is assigned.
	Release string
}

// Stored is a stored subtitle with its match against the item's current
// release.
type Stored struct {
	*Subtitle
	Match Match
}

// Current keeps the subtitles that go with the item's current file:
// downloaded ones, and those that came with the assigned torrent. Torrent
// subtitles from an earlier torrent, or with nothing assigned, are left out.
func Current(subs []*Subtitle, infoHash string) []*Subtitle {
	out := make([]*Subtitle, 0, len(subs))
	for _, sub := range subs {
		if sub.Source == SourceTorrent && (infoHash == "" || sub.InfoHash != infoHash) {
			continue
		}
		out = append(out, sub)
	}
	return out
}

// List returns the item's current subtitles (see Current) with their match.
func (s *Service) List(ctx context.Context, item ItemContext) ([]Stored, error) {
	subs, err := s.repo.GetByItem(ctx, item.Item.Type, item.Item.ID)
	if err != nil {
		return nil, err
	}
	current := Current(subs, item.InfoHash)
	out := make([]Stored, len(current))
	for i, sub := range current {
		out[i] = s.stored(item, sub)
	}
	return out, nil
}

func (s *Service) stored(item ItemContext, sub *Subtitle) Stored {
	// Torrent subtitles saved before release names were kept are still the
	// assigned torrent's.
	if sub.Source == SourceTorrent && sub.ReleaseName == "" {
		sub.ReleaseName = item.Release
	}
	return Stored{Subtitle: sub, Match: MatchStored(item.Release, sub)}
}

// ParseLanguages reads a comma-separated language list ("en,pt-br"):
// lowercased, deduplicated and sorted. Empty means DefaultLanguage.
func ParseLanguages(raw string) ([]string, error) {
	var langs []string
	for _, part := range strings.Split(raw, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		code, ok := NormalizeLanguageCode(part)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidLanguage, strings.TrimSpace(part))
		}
		if !slices.Contains(langs, code) {
			langs = append(langs, code)
		}
	}
	if len(langs) == 0 {
		return []string{DefaultLanguage}, nil
	}
	slices.Sort(langs)
	return langs, nil
}

// SearchRanked searches OpenSubtitles for the item and ranks the results
// against its assigned release, best first.
func (s *Service) SearchRanked(ctx context.Context, item ItemContext, languages []string) ([]Candidate, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	params := item.Search
	params.Languages = languages
	resp, err := s.fetcher.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetch, err)
	}
	return Rank(item.Release, candidatesFrom(resp)), nil
}

// candidatesFrom turns search results into candidates, one per subtitle
// (its first file; multi-CD subtitles are rare and Infuse wants one file).
func candidatesFrom(resp *opensubtitles.SearchResponse) []Candidate {
	if resp == nil {
		return nil
	}
	out := make([]Candidate, 0, len(resp.Data))
	seen := make(map[int]bool, len(resp.Data))
	for _, r := range resp.Data {
		a := r.Attributes
		if len(a.Files) == 0 || seen[a.Files[0].FileID] {
			continue
		}
		file := a.Files[0]
		seen[file.FileID] = true
		code := strings.ToLower(a.Language)
		out = append(out, Candidate{
			FileID:            file.FileID,
			LanguageCode:      code,
			LanguageName:      LanguageName(code),
			ReleaseName:       strings.TrimSpace(a.Release),
			FileName:          file.FileName,
			Downloads:         a.DownloadCount,
			HearingImpaired:   a.HearingImpaired,
			MachineTranslated: a.MachineTranslated || a.AITranslated,
		})
	}
	return out
}

// Download fetches an OpenSubtitles file for the item and stores it,
// replacing the item's downloaded subtitle in that language. releaseName is
// the candidate's, kept to show the match later. It works with nothing
// assigned; the match is then unknown.
func (s *Service) Download(ctx context.Context, item ItemContext, fileID int, languageCode, releaseName string) (*Stored, error) {
	sub, err := s.download(ctx, item.Item, fileID, languageCode, "", strings.TrimSpace(releaseName))
	if err != nil {
		return nil, err
	}
	stored := s.stored(item, sub)
	return &stored, nil
}

// DownloadAndStore is the legacy POST /subtitles/download.
func (s *Service) DownloadAndStore(ctx context.Context, itemType ItemType, itemID int64, fileID int, languageCode, languageName string) (*Subtitle, error) {
	return s.download(ctx, Item{Type: itemType, ID: itemID}, fileID, languageCode, languageName, "")
}

func (s *Service) download(ctx context.Context, item Item, fileID int, languageCode, languageName, releaseName string) (*Subtitle, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	code, ok := NormalizeLanguageCode(languageCode)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidLanguage, languageCode)
	}
	if languageName == "" {
		languageName = LanguageName(code)
	}

	content, fileName, err := s.fetcher.Download(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetch, err)
	}
	format := ParseFormat(fileName)

	previous, err := s.repo.GetDownloaded(ctx, item.Type, item.ID, code)
	if err != nil {
		return nil, err
	}

	// {download_path}/{item_type}/{item_id}/{language_code}.{format}
	storageDir := filepath.Join(s.subtitlesPath, string(item.Type), strconv.FormatInt(item.ID, 10))
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create subtitle directory: %w", err)
	}
	storedFilePath := filepath.Join(storageDir, code+"."+format)

	// Write beside the final path and rename only once the record is saved,
	// so a failed upsert leaves the current subtitle untouched.
	tmpPath, err := writeTemp(storageDir, content)
	if err != nil {
		return nil, fmt.Errorf("failed to save subtitle file: %w", err)
	}
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	sub := &Subtitle{
		ItemType:     item.Type,
		ItemID:       item.ID,
		LanguageCode: code,
		LanguageName: languageName,
		Format:       format,
		FilePath:     storedFilePath,
		FileSize:     int64(len(content)),
		ReleaseName:  releaseName,
	}
	if err := s.repo.UpsertDownloaded(ctx, sub); err != nil {
		return nil, fmt.Errorf("failed to save subtitle record: %w", err)
	}
	if err := os.Rename(tmpPath, storedFilePath); err != nil {
		return nil, fmt.Errorf("failed to save subtitle file: %w", err)
	}
	tmpPath = ""
	// A replacement in another format leaves the old file behind.
	if previous != nil && previous.FilePath != storedFilePath {
		if err := os.Remove(previous.FilePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to remove replaced subtitle file", "path", previous.FilePath, "error", err)
		}
	}

	slog.Info("Subtitle downloaded and stored",
		"item_type", item.Type,
		"item_id", item.ID,
		"language", code,
		"format", format,
		"release", releaseName,
		"file", storedFilePath,
	)

	return sub, nil
}

// DeleteFromItem deletes one of the item's downloaded subtitles and its file.
// ErrNotFound when the subtitle isn't among the item's current ones (see
// Current: another item's, or an earlier torrent's); ErrTorrentSubtitle when
// it came with the assigned torrent.
func (s *Service) DeleteFromItem(ctx context.Context, item ItemContext, id int64) error {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if sub == nil || sub.Item() != item.Item || len(Current([]*Subtitle{sub}, item.InfoHash)) == 0 {
		return ErrNotFound
	}
	return s.remove(ctx, sub)
}

// Delete is the legacy DELETE /subtitles/{id}.
func (s *Service) Delete(ctx context.Context, id int64) error {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if sub == nil {
		return ErrNotFound
	}
	return s.remove(ctx, sub)
}

func (s *Service) remove(ctx context.Context, sub *Subtitle) error {
	// A torrent subtitle's FilePath is a path inside the torrent, not on disk.
	if sub.Source == SourceTorrent {
		return ErrTorrentSubtitle
	}
	if err := s.repo.Delete(ctx, sub.ID); err != nil {
		return err
	}
	if err := os.Remove(sub.FilePath); err != nil && !os.IsNotExist(err) {
		slog.Error("Failed to delete subtitle file", "path", sub.FilePath, "error", err)
	}

	slog.Info("Subtitle deleted",
		"id", sub.ID,
		"item_type", sub.ItemType,
		"item_id", sub.ItemID,
		"language", sub.LanguageCode,
	)
	return nil
}

// CreateTorrentSubtitle records a subtitle file that came with the item's
// torrent. Nothing is downloaded: the VFS streams it from the torrent.
func (s *Service) CreateTorrentSubtitle(ctx context.Context, sub *Subtitle) error {
	if err := s.repo.UpsertTorrent(ctx, sub); err != nil {
		return fmt.Errorf("failed to save torrent subtitle record: %w", err)
	}

	slog.Info("Torrent subtitle created",
		"item_type", sub.ItemType,
		"item_id", sub.ItemID,
		"language", sub.LanguageCode,
		"format", sub.Format,
		"torrent_path", sub.FilePath,
		"info_hash", sub.InfoHash,
	)

	return nil
}

// Search searches for subtitles using the configured fetcher (legacy
// GET /subtitles/search, unranked).
func (s *Service) Search(ctx context.Context, params opensubtitles.SearchParams) (*opensubtitles.SearchResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrNotConfigured
	}
	return s.fetcher.Search(ctx, params)
}

// writeTemp writes content to a synced temp file in dir (the final file's
// directory, so a rename is atomic) and returns its path. The caller renames
// or removes it.
func writeTemp(dir string, content []byte) (string, error) {
	tmpFile, err := os.CreateTemp(dir, ".subtitle-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}
	return tmpPath, nil
}
