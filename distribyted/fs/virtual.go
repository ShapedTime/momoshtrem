package fs

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/distribyted/distribyted/episode"
	tloader "github.com/distribyted/distribyted/torrent/loader"
)

// VirtualPathMapper provides bidirectional mapping between virtual paths
// (Infuse-friendly names) and real torrent file paths
type VirtualPathMapper struct {
	mu            sync.RWMutex
	virtualToReal map[string]string // "/Show (2020)/Season 01/..." → "/original/path.mkv"
	realToVirtual map[string]string // reverse mapping
	virtualDirs   map[string]bool   // track virtual directories
	children      map[string][]string // parent path → child names
}

// NewVirtualPathMapper creates a new virtual path mapper
func NewVirtualPathMapper() *VirtualPathMapper {
	return &VirtualPathMapper{
		virtualToReal: make(map[string]string),
		realToVirtual: make(map[string]string),
		virtualDirs:   make(map[string]bool),
		children:      make(map[string][]string),
	}
}

// AddMapping adds a bidirectional mapping between virtual and real paths
func (m *VirtualPathMapper) AddMapping(virtualPath, realPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Normalize paths
	virtualPath = cleanPath(virtualPath)
	realPath = cleanPath(realPath)

	m.virtualToReal[virtualPath] = realPath
	m.realToVirtual[realPath] = virtualPath

	// Create parent directories
	m.createParentDirs(virtualPath)

	log.Debug().
		Str("virtualPath", virtualPath).
		Str("realPath", realPath).
		Msg("virtual: mapping added")
}

// AddMappingWithConflictResolution adds a mapping, resolving conflicts by appending
// quality info or hash suffix if the path already exists
func (m *VirtualPathMapper) AddMappingWithConflictResolution(
	virtualPath, realPath string,
	quality episode.QualityInfo,
	hash string,
) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Normalize paths
	virtualPath = cleanPath(virtualPath)
	realPath = cleanPath(realPath)

	originalPath := virtualPath

	// Check if path already exists
	if _, exists := m.virtualToReal[virtualPath]; exists {
		// Try with quality suffix
		ext := filepath.Ext(virtualPath)
		base := strings.TrimSuffix(virtualPath, ext)
		qualitySuffix := formatQualitySuffix(quality)
		if qualitySuffix != "" {
			virtualPath = fmt.Sprintf("%s %s%s", base, qualitySuffix, ext)
		}

		if _, stillExists := m.virtualToReal[virtualPath]; stillExists {
			// Still exists, append truncated hash
			hashSuffix := hash
			if len(hashSuffix) > 6 {
				hashSuffix = hashSuffix[:6]
			}
			virtualPath = fmt.Sprintf("%s [%s]%s", strings.TrimSuffix(virtualPath, ext), hashSuffix, ext)

			log.Debug().
				Str("original", originalPath).
				Str("resolved", virtualPath).
				Str("method", "hash").
				Msg("virtual: conflict resolved")
		} else {
			log.Debug().
				Str("original", originalPath).
				Str("resolved", virtualPath).
				Str("method", "quality").
				Msg("virtual: conflict resolved")
		}
	}

	m.virtualToReal[virtualPath] = realPath
	m.realToVirtual[realPath] = virtualPath

	// Create parent directories
	m.createParentDirs(virtualPath)

	log.Debug().
		Str("virtualPath", virtualPath).
		Str("realPath", realPath).
		Msg("virtual: mapping added")

	return virtualPath
}

// createParentDirs creates all parent directories for a path (must be called with lock held)
func (m *VirtualPathMapper) createParentDirs(filePath string) {
	dir := path.Dir(filePath)
	fileName := path.Base(filePath)

	// Add to children map
	if _, exists := m.children[dir]; !exists {
		m.children[dir] = []string{}
	}
	// Check if child already exists
	found := false
	for _, child := range m.children[dir] {
		if child == fileName {
			found = true
			break
		}
	}
	if !found {
		m.children[dir] = append(m.children[dir], fileName)
	}

	// Create parent directories up to root
	for dir != "/" && dir != "." && dir != "" {
		if !m.virtualDirs[dir] {
			m.virtualDirs[dir] = true
			log.Debug().
				Str("dir", dir).
				Msg("virtual: directory created")
		}

		parent := path.Dir(dir)
		dirName := path.Base(dir)

		// Add this dir as child of parent
		if _, exists := m.children[parent]; !exists {
			m.children[parent] = []string{}
		}
		found := false
		for _, child := range m.children[parent] {
			if child == dirName {
				found = true
				break
			}
		}
		if !found {
			m.children[parent] = append(m.children[parent], dirName)
		}

		dir = parent
	}
}

// ToReal translates a virtual path to its real path
func (m *VirtualPathMapper) ToReal(virtualPath string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	virtualPath = cleanPath(virtualPath)
	realPath, found := m.virtualToReal[virtualPath]

	if found {
		log.Debug().
			Str("virtual", virtualPath).
			Str("real", realPath).
			Msg("virtual: translated to real")
	}

	return realPath, found
}

// ToVirtual translates a real path to its virtual path
func (m *VirtualPathMapper) ToVirtual(realPath string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realPath = cleanPath(realPath)
	virtualPath, found := m.realToVirtual[realPath]

	if found {
		log.Debug().
			Str("real", realPath).
			Str("virtual", virtualPath).
			Msg("virtual: translated to virtual")
	}

	return virtualPath, found
}

// VirtualChildren returns the child entries at a virtual path
func (m *VirtualPathMapper) VirtualChildren(dirPath string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dirPath = cleanPath(dirPath)
	children := m.children[dirPath]

	log.Debug().
		Str("path", dirPath).
		Int("count", len(children)).
		Msg("virtual: listing children")

	// Return a sorted copy
	result := make([]string, len(children))
	copy(result, children)
	sort.Strings(result)
	return result
}

// IsVirtualDir checks if a path is a virtual directory
func (m *VirtualPathMapper) IsVirtualDir(dirPath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dirPath = cleanPath(dirPath)
	return m.virtualDirs[dirPath]
}

// IsVirtualPath checks if a path exists as a virtual file or directory
func (m *VirtualPathMapper) IsVirtualPath(p string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p = cleanPath(p)
	if _, exists := m.virtualToReal[p]; exists {
		return true
	}
	return m.virtualDirs[p]
}

// Clear resets all mappings
func (m *VirtualPathMapper) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.virtualToReal = make(map[string]string)
	m.realToVirtual = make(map[string]string)
	m.virtualDirs = make(map[string]bool)
	m.children = make(map[string][]string)

	log.Debug().Msg("virtual: mappings cleared")
}

// Count returns the number of file mappings
func (m *VirtualPathMapper) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.virtualToReal)
}

// GenerateVirtualPath creates a standardized path for Infuse
// TV: "Show Name (Year)/Season XX/Show Name - SXXEXX [Quality].ext"
// Movie: "Movie Name (Year)/Movie Name (Year) [Quality].ext"
func GenerateVirtualPath(metadata *tloader.TMDBMetadata, identified *episode.IdentifiedFile) string {
	if metadata == nil || identified == nil {
		return ""
	}

	ext := filepath.Ext(identified.FilePath)
	title := SanitizeFilename(metadata.Title)
	qualitySuffix := formatQualitySuffix(identified.Quality)

	switch metadata.Type {
	case tloader.MediaTypeTV:
		return generateTVPath(title, metadata.Year, identified, qualitySuffix, ext)
	case tloader.MediaTypeMovie:
		return generateMoviePath(title, metadata.Year, qualitySuffix, ext)
	default:
		log.Debug().
			Str("type", string(metadata.Type)).
			Msg("virtual: unknown media type, skipping")
		return ""
	}
}

// generateTVPath creates a path for TV shows
func generateTVPath(title string, year int, identified *episode.IdentifiedFile, qualitySuffix, ext string) string {
	// Folder: "Show Name (Year)"
	showFolder := fmt.Sprintf("%s (%d)", title, year)

	// Season folder: "Season XX" or "Season 00" for specials
	season := identified.Season
	if identified.IsSpecial {
		season = 0
	}
	seasonFolder := fmt.Sprintf("Season %02d", season)

	// Filename: "Show Name - SXXEXX [Quality].ext"
	episodeStr := formatSeasonEpisode(season, identified.Episodes)
	var filename string
	if qualitySuffix != "" {
		filename = fmt.Sprintf("%s - %s %s%s", title, episodeStr, qualitySuffix, ext)
	} else {
		filename = fmt.Sprintf("%s - %s%s", title, episodeStr, ext)
	}

	virtualPath := path.Join("/", showFolder, seasonFolder, filename)

	log.Debug().
		Str("title", title).
		Int("season", season).
		Ints("episodes", identified.Episodes).
		Str("virtualPath", virtualPath).
		Msg("virtual: generated TV path")

	return virtualPath
}

// generateMoviePath creates a path for movies
func generateMoviePath(title string, year int, qualitySuffix, ext string) string {
	// Folder: "Movie Name (Year)"
	movieFolder := fmt.Sprintf("%s (%d)", title, year)

	// Filename: "Movie Name (Year) [Quality].ext"
	var filename string
	if qualitySuffix != "" {
		filename = fmt.Sprintf("%s (%d) %s%s", title, year, qualitySuffix, ext)
	} else {
		filename = fmt.Sprintf("%s (%d)%s", title, year, ext)
	}

	virtualPath := path.Join("/", movieFolder, filename)

	log.Debug().
		Str("title", title).
		Int("year", year).
		Str("virtualPath", virtualPath).
		Msg("virtual: generated movie path")

	return virtualPath
}

// formatSeasonEpisode formats season and episode numbers
// Single: S01E05
// Multi: S01E05-E08
func formatSeasonEpisode(season int, episodes []int) string {
	if len(episodes) == 0 {
		return fmt.Sprintf("S%02dE00", season)
	}

	if len(episodes) == 1 {
		return fmt.Sprintf("S%02dE%02d", season, episodes[0])
	}

	// Sort episodes
	sorted := make([]int, len(episodes))
	copy(sorted, episodes)
	sort.Ints(sorted)

	// Check if consecutive
	isConsecutive := true
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1]+1 {
			isConsecutive = false
			break
		}
	}

	if isConsecutive {
		// Range: S01E05-E08
		return fmt.Sprintf("S%02dE%02d-E%02d", season, sorted[0], sorted[len(sorted)-1])
	}

	// Non-consecutive: S01E05E07E09
	result := fmt.Sprintf("S%02dE%02d", season, sorted[0])
	for i := 1; i < len(sorted); i++ {
		result += fmt.Sprintf("E%02d", sorted[i])
	}
	return result
}

// formatQualitySuffix creates a quality suffix like "[1080p WEB-DL]"
func formatQualitySuffix(quality episode.QualityInfo) string {
	var parts []string

	if quality.Resolution != "" {
		parts = append(parts, quality.Resolution)
	}

	if quality.Source != "" {
		parts = append(parts, quality.Source)
	}

	if quality.HDR {
		parts = append(parts, "HDR")
	}

	if len(parts) == 0 {
		return ""
	}

	return "[" + strings.Join(parts, " ") + "]"
}

// SanitizeFilename removes invalid characters from filenames
// Invalid chars: < > : " / \ | ? *
var invalidCharsRegex = regexp.MustCompile(`[<>:"/\\|?*]`)

func SanitizeFilename(name string) string {
	// Replace invalid characters with safe alternatives
	result := invalidCharsRegex.ReplaceAllString(name, "")

	// Replace multiple spaces with single space
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")

	// Trim whitespace
	result = strings.TrimSpace(result)

	return result
}

// cleanPath normalizes a path
func cleanPath(p string) string {
	return path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
}
