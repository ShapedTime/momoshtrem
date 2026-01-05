package subtitle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
)

// Service handles subtitle operations including search, download, and storage.
type Service struct {
	fetcher       opensubtitles.SubtitleFetcher
	repo          SubtitleRepository
	subtitlesPath string
}

// NewService creates a new subtitle service.
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

// Search searches for subtitles using the configured fetcher.
func (s *Service) Search(ctx context.Context, params opensubtitles.SearchParams) (*opensubtitles.SearchResponse, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("subtitle fetcher not configured")
	}
	return s.fetcher.Search(ctx, params)
}

// DownloadAndStore downloads a subtitle and stores it locally.
// Returns the created/updated Subtitle record.
func (s *Service) DownloadAndStore(ctx context.Context, itemType ItemType, itemID int64, fileID int, languageCode, languageName string) (*Subtitle, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("subtitle fetcher not configured")
	}

	// Download from fetcher
	content, fileName, err := s.fetcher.Download(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to download subtitle: %w", err)
	}

	// Determine format from filename
	format := ParseFormat(fileName)

	// Create storage directory: {download_path}/{item_type}/{item_id}/
	storageDir := filepath.Join(s.subtitlesPath, string(itemType), strconv.FormatInt(itemID, 10))
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create subtitle directory: %w", err)
	}

	// Final file path: {language_code}.{format}
	storedFileName := fmt.Sprintf("%s.%s", languageCode, format)
	storedFilePath := filepath.Join(storageDir, storedFileName)

	// Write atomically: write to temp file, then rename
	if err := s.writeFileAtomic(storedFilePath, content); err != nil {
		return nil, fmt.Errorf("failed to save subtitle file: %w", err)
	}

	// Create database record
	sub := &Subtitle{
		ItemType:     itemType,
		ItemID:       itemID,
		LanguageCode: languageCode,
		LanguageName: languageName,
		Format:       format,
		FilePath:     storedFilePath,
		FileSize:     int64(len(content)),
	}

	if err := s.repo.Create(ctx, sub); err != nil {
		// Clean up file on database failure
		os.Remove(storedFilePath)
		return nil, fmt.Errorf("failed to save subtitle record: %w", err)
	}

	slog.Info("Subtitle downloaded and stored",
		"item_type", itemType,
		"item_id", itemID,
		"language", languageCode,
		"format", format,
		"file", storedFilePath,
	)

	return sub, nil
}

// Delete removes a subtitle by ID, including its file.
func (s *Service) Delete(ctx context.Context, id int64) error {
	// Get subtitle to find file path
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get subtitle: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("subtitle not found")
	}

	// Delete from database first
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete subtitle record: %w", err)
	}

	// Delete file from disk (best effort, log errors)
	if err := os.Remove(sub.FilePath); err != nil && !os.IsNotExist(err) {
		slog.Error("Failed to delete subtitle file", "path", sub.FilePath, "error", err)
	}

	slog.Info("Subtitle deleted",
		"id", id,
		"item_type", sub.ItemType,
		"item_id", sub.ItemID,
		"language", sub.LanguageCode,
	)

	return nil
}

// GetByItem retrieves all subtitles for a library item.
func (s *Service) GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error) {
	return s.repo.GetByItem(ctx, itemType, itemID)
}

// writeFileAtomic writes content to a file atomically using a temp file and rename.
func (s *Service) writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)

	// Create temp file in the same directory (for atomic rename)
	tmpFile, err := os.CreateTemp(dir, ".subtitle-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any error
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	// Write content
	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Clear tmpPath so defer doesn't try to remove it
	tmpPath = ""
	return nil
}
