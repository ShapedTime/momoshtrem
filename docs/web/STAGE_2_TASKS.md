# Stage 2: Infuse Media Identification - Implementation Tasks

Solve Infuse's inability to identify TV shows/movies from distribyted's WebDAV by adding TMDB metadata support and virtual file renaming.

---

## Problem Statement

Infuse fails to identify content because:
1. Distribyted exposes files exactly as they appear in torrent metadata (no organization)
2. Torrents have wildly inconsistent naming (`S01E01`, `1x01`, `101`, scene tags, etc.)
3. No metadata is stored alongside torrents

## Solution Overview

1. **Phase 1:** Store TMDB metadata when adding torrents
2. **Phase 2:** Identify episodes within torrents using regex patterns
3. **Phase 3:** Present files via WebDAV with standardized naming

USE sequential thinking and think critically when doing tasks. Some of the details might be wrong or unsitable for our case.

---

## Task 1: TMDB Metadata Data Structures (distribyted)

**Goal:** Add data structures for TMDB metadata storage.

### Steps

1. Create new file `distribyted/torrent/model.go` with:
   ```go
   type MediaType string
   const (
       MediaTypeMovie MediaType = "movie"
       MediaTypeTV    MediaType = "tv"
   )

   type TMDBMetadata struct {
       Type      MediaType `json:"type"`
       TMDBID    int       `json:"tmdb_id"`
       Title     string    `json:"title"`
       Year      int       `json:"year"`
       Season    *int      `json:"season,omitempty"`    // For season packs
       Episode   *int      `json:"episode,omitempty"`   // For single episodes
   }

   type TorrentWithMetadata struct {
       MagnetURI string        `json:"magnet_uri"`
       Metadata  *TMDBMetadata `json:"metadata,omitempty"`
   }
   ```

2. Update `distribyted/torrent/loader/loader.go` interface:
   ```go
   type LoaderAdder interface {
       AddMagnet(route, magnet string, metadata *TMDBMetadata) error
       ListMagnets() (map[string][]TorrentWithMetadata, error)
       // ... existing methods
   }
   ```

### Deliverables
- [ ] `TMDBMetadata` struct defined in `torrent/model.go`
- [ ] `TorrentWithMetadata` struct defined
- [ ] Interface updated (compilation will fail until Task 2)

---

## Task 2: Database Layer for Metadata (distribyted)

**Goal:** Store TMDB metadata in BadgerDB alongside magnet URIs.

### Steps

1. Modify `distribyted/torrent/loader/db.go`:
   - Update `AddMagnet(route, magnet string, metadata *TMDBMetadata)`:
     ```go
     // Store as JSON: {"magnet_uri": "...", "metadata": {...}}
     data := TorrentWithMetadata{MagnetURI: magnet, Metadata: metadata}
     jsonBytes, _ := json.Marshal(data)
     txn.Set([]byte(rp), jsonBytes)
     ```
   - Update `ListMagnets()` to return `map[string][]TorrentWithMetadata`:
     - Try JSON unmarshal first
     - Fall back to plain string for backward compatibility
   - Add `GetTorrentInfo(route, hash string) (*TorrentWithMetadata, error)`
   - Add `UpdateMetadata(route, hash string, metadata *TMDBMetadata) error`

2. Test backward compatibility:
   - Existing plain-string entries should still load
   - New entries store as JSON

### Deliverables
- [ ] `AddMagnet` stores JSON with optional metadata
- [ ] `ListMagnets` returns metadata when available
- [ ] Existing torrents without metadata still work
- [ ] `UpdateMetadata` allows updating existing torrents

---

## Task 3: Service Layer Updates (distribyted)

**Goal:** Pass metadata through the service layer.

### Steps

1. Update `distribyted/torrent/service.go`:
   - Modify `AddMagnet(route, magnet string, metadata *TMDBMetadata) error`
   - Store metadata in memory map: `metadataMap map[string]*TMDBMetadata` (keyed by infohash)
   - Pass metadata to `loader.AddMagnet()`

2. Add method to retrieve metadata:
   ```go
   func (s *Service) GetTorrentMetadata(hash string) *TMDBMetadata
   ```

### Deliverables
- [ ] `AddMagnet` accepts metadata parameter
- [ ] Metadata stored in memory for runtime access
- [ ] Metadata retrievable by torrent hash

---

## Task 4: HTTP API Updates (distribyted)

**Goal:** API endpoints accept and return TMDB metadata.

### Steps

1. Update `distribyted/http/model.go`:
   ```go
   type RouteAdd struct {
       Magnet   string              `json:"magnet" binding:"required"`
       Metadata *torrent.TMDBMetadata `json:"metadata,omitempty"`
   }

   type TorrentInfo struct {
       Hash     string                `json:"hash"`
       Name     string                `json:"name"`
       Metadata *torrent.TMDBMetadata `json:"metadata,omitempty"`
       Files    []FileInfo            `json:"files"`
   }

   type FileInfo struct {
       Path string `json:"path"`
       Size int64  `json:"size"`
   }
   ```

2. Update `distribyted/http/api.go`:
   - Modify `apiAddTorrentHandler` to extract and pass metadata
   - Add `apiGetTorrentsHandler` for `GET /api/routes/{route}/torrents`
   - Add `apiUpdateMetadataHandler` for `PATCH /api/routes/{route}/torrent/{hash}/metadata`

3. Register new routes in `distribyted/http/http.go`

### Deliverables
- [ ] `POST /api/routes/{route}/torrent` accepts `metadata` field
- [ ] `GET /api/routes/{route}/torrents` returns list with metadata
- [ ] `PATCH /api/routes/{route}/torrent/{hash}/metadata` updates metadata
- [ ] Backward compatible (metadata is optional)

---

## Task 5: PrettyTVCatalog Integration

**Goal:** Pass TMDB metadata when adding torrents from the frontend.

### Steps

1. Update types `PrettyTVCatalog/src/types/distribyted.ts`:
   ```typescript
   export type MediaType = 'movie' | 'tv';

   export interface TMDBMetadata {
     type: MediaType;
     tmdb_id: number;
     title: string;
     year: number;
     season?: number;
     episode?: number;
   }

   export interface AddTorrentRequest {
     magnet: string;
     metadata?: TMDBMetadata;
   }

   export interface TorrentInfo {
     hash: string;
     name: string;
     metadata?: TMDBMetadata;
     files: FileInfo[];
   }
   ```

2. Update API client `PrettyTVCatalog/src/lib/api/distribyted.ts`:
   ```typescript
   async addTorrent(magnet: string, metadata?: TMDBMetadata): Promise<void>
   async getLibrary(): Promise<TorrentInfo[]>
   ```

3. Update `PrettyTVCatalog/src/config/distribyted.ts`:
   - Add endpoint for `getTorrents`

4. Update API route `PrettyTVCatalog/src/app/api/distribyted/add/route.ts`:
   - Accept metadata in request body
   - Pass to distribyted API

5. Update `PrettyTVCatalog/src/components/torrent/TorrentCard.tsx`:
   - Accept TMDB context (mediaType, tmdbId, title, year, season)
   - Pass metadata when calling addTorrent

### Deliverables
- [ ] `TMDBMetadata` type defined in TypeScript
- [ ] `addTorrent()` accepts optional metadata
- [ ] TorrentCard passes metadata when adding
- [ ] `getLibrary()` method available

---

## Task 6: Episode Identification - Core Types (distribyted)

**Goal:** Create data structures for episode identification results.

### Steps

1. Create new package `distribyted/episode/`

2. Create `distribyted/episode/types.go`:
   ```go
   type Confidence string
   const (
       ConfidenceHigh   Confidence = "high"
       ConfidenceMedium Confidence = "medium"
       ConfidenceLow    Confidence = "low"
       ConfidenceNone   Confidence = "none"
   )

   type IdentifiedFile struct {
       FilePath         string      `json:"file_path"`
       FileSize         int64       `json:"file_size"`
       FileType         FileType    `json:"file_type"`        // video, subtitle
       Season           int         `json:"season"`
       Episodes         []int       `json:"episodes"`
       EpisodeRange     *Range      `json:"episode_range,omitempty"`
       IsSpecial        bool        `json:"is_special"`
       Quality          QualityInfo `json:"quality"`
       Confidence       Confidence  `json:"confidence"`
       PatternUsed      string      `json:"pattern_used"`
       NeedsReview      bool        `json:"needs_review"`
       SeasonFromFolder bool        `json:"season_from_folder"`
   }

   type FileType string
   const (
       FileTypeVideo    FileType = "video"
       FileTypeSubtitle FileType = "subtitle"
   )

   type QualityInfo struct {
       Resolution string `json:"resolution"` // 2160p, 1080p, 720p, 480p
       Source     string `json:"source"`     // BluRay, WEB-DL, HDTV
       Codec      string `json:"codec"`      // x264, x265, HEVC
       HDR        bool   `json:"hdr"`
   }

   type Range struct {
       Start int `json:"start"`
       End   int `json:"end"`
   }

   type IdentificationResult struct {
       TorrentName       string           `json:"torrent_name"`
       IdentifiedFiles   []IdentifiedFile `json:"identified_files"`
       UnidentifiedFiles []string         `json:"unidentified_files"`
       TotalFiles        int              `json:"total_files"`
       IdentifiedCount   int              `json:"identified_count"`
   }
   ```

### Deliverables
- [ ] Episode identification types defined
- [ ] Confidence levels defined
- [ ] Quality info struct defined
- [ ] FileType includes video and subtitle

---

## Task 7: Episode Identification - Regex Patterns (distribyted)

**Goal:** Create comprehensive regex patterns for episode detection.

### Steps

1. Create `distribyted/episode/patterns.go`:
   ```go
   type CompiledPatterns struct {
       // Primary (High confidence)
       SxxExx      *regexp.Regexp // S01E01, s01e01, S1E1
       SxxExxRange *regexp.Regexp // S01E01-E03, S01E01-03
       SxxExxMulti *regexp.Regexp // S01E01E02E03
       XxYY        *regexp.Regexp // 1x01, 01x01

       // Secondary (Medium confidence)
       SeasonEpisode *regexp.Regexp // Season 1 Episode 1
       EpNumber      *regexp.Regexp // Ep 1, Episode 1 (needs context)
       AnimeEpisode  *regexp.Regexp // [Group] Show - 01
       DateYMD       *regexp.Regexp // 2024.01.15 (daily shows)

       // Low confidence
       Concatenated4 *regexp.Regexp // 0101
       Concatenated3 *regexp.Regexp // 101

       // Folder context
       SeasonFolder *regexp.Regexp // Season 01/, S01/

       // Quality
       Resolution *regexp.Regexp // 2160p, 4K, 1080p, 720p
       Source     *regexp.Regexp // BluRay, WEB-DL, HDTV
       Codec      *regexp.Regexp // x264, x265, HEVC
       HDR        *regexp.Regexp // HDR, HDR10, Dolby Vision

       // Specials
       Special *regexp.Regexp // S00E01, Special, OVA
   }

   func NewCompiledPatterns() *CompiledPatterns
   ```

2. Implement all regex patterns as specified in the plan

### Deliverables
- [ ] All primary patterns compile and work
- [ ] All secondary patterns compile and work
- [ ] Quality extraction patterns work
- [ ] Folder context patterns work

---

## Task 8: Episode Identification - Core Algorithm (distribyted)

**Goal:** Implement the main identification algorithm with fallback extensibility.

### Steps

1. Create `distribyted/episode/identifier.go`:
   ```go
   // Identifier is the main episode identification engine
   type Identifier struct {
       patterns *CompiledPatterns
       fallback FallbackHandler // For future LLM integration
   }

   // FallbackHandler is an interface for handling unidentified files
   // This allows future integration with local LLMs
   type FallbackHandler interface {
       // IdentifyBatch processes a batch of unidentified files
       // Returns a map of file path to identified episode info
       IdentifyBatch(files []UnidentifiedFile, metadata *TMDBMetadata) (map[string]*IdentifiedFile, error)
   }

   type UnidentifiedFile struct {
       Path      string
       Size      int64
       Context   *Context // folder context, torrent name hints
   }

   // NoOpFallback is the default fallback that does nothing
   type NoOpFallback struct{}
   func (n *NoOpFallback) IdentifyBatch(files []UnidentifiedFile, metadata *TMDBMetadata) (map[string]*IdentifiedFile, error) {
       return nil, nil // Returns nothing, files remain unidentified
   }

   func NewIdentifier(fallback FallbackHandler) *Identifier
   func (i *Identifier) Identify(metadata *TMDBMetadata, files []TorrentFile, torrentName string) *IdentificationResult
   ```

2. Implement the algorithm flow:
   - Extract torrent context (season hints, "complete" indicator)
   - Filter to media files (video + subtitles)
   - Skip samples/trailers/extras
   - For each file: try patterns in confidence order
   - Call fallback handler for unidentified files
   - Post-process and validate

### Deliverables
- [ ] `Identifier` struct with fallback interface
- [ ] `FallbackHandler` interface defined for future LLM use
- [ ] `NoOpFallback` default implementation
- [ ] Main `Identify()` method works

---

## Task 9: Episode Identification - File Filtering (distribyted)

**Goal:** Filter and categorize media files (video + subtitles).

### Steps

1. Create `distribyted/episode/filter.go`:
   ```go
   var videoExtensions = map[string]bool{
       ".mkv": true, ".mp4": true, ".avi": true, ".wmv": true,
       ".mov": true, ".m4v": true, ".webm": true, ".ts": true,
       ".m2ts": true, ".vob": true,
   }

   var subtitleExtensions = map[string]bool{
       ".srt": true, ".sub": true, ".ass": true, ".ssa": true,
       ".vtt": true, ".idx": true,
   }

   var skipPatterns = []*regexp.Regexp{
       regexp.MustCompile(`(?i)sample`),
       regexp.MustCompile(`(?i)trailer`),
       regexp.MustCompile(`(?i)preview`),
       regexp.MustCompile(`(?i)extra[s]?[\\/]`),
       regexp.MustCompile(`(?i)featurette`),
       regexp.MustCompile(`(?i)deleted.?scenes?`),
       regexp.MustCompile(`(?i)behind.?the.?scenes`),
   }

   type MediaFile struct {
       Path     string
       Size     int64
       FileType FileType // video or subtitle
   }

   func FilterMediaFiles(files []TorrentFile) []MediaFile
   func IsVideoFile(path string) bool
   func IsSubtitleFile(path string) bool
   func ShouldSkip(path string) bool
   ```

2. Subtitle handling:
   - Identify subtitles that match video files
   - Parse subtitle language from filename if present
   - Associate subtitle with parent video

### Deliverables
- [ ] Video files identified correctly
- [ ] Subtitle files identified correctly
- [ ] Samples/trailers/extras filtered out
- [ ] Subtitles associated with videos

---

## Task 10: Episode Identification - Quality Extraction (distribyted)

**Goal:** Extract quality information from filenames.

### Steps

1. Create `distribyted/episode/quality.go`:
   ```go
   func ExtractQuality(filename string, patterns *CompiledPatterns) QualityInfo
   func NormalizeResolution(match string) string  // "4K" -> "2160p"
   func NormalizeSource(match string) string      // "Blu-Ray" -> "BluRay"
   func NormalizeCodec(match string) string       // "H.265" -> "HEVC"
   ```

### Deliverables
- [ ] Resolution extraction works (2160p, 1080p, 720p, 480p)
- [ ] Source extraction works (BluRay, WEB-DL, HDTV)
- [ ] Codec extraction works (x264, x265, HEVC)
- [ ] HDR detection works

---

## Task 11: Episode Identification - Integration (distribyted)

**Goal:** Integrate episode identification with torrent loading.

### Steps

1. Modify `distribyted/torrent/service.go`:
   - After torrent metadata is received, run identification
   - Store identification results alongside torrent
   - Add method: `GetIdentifiedFiles(hash string) *IdentificationResult`

2. Modify `distribyted/fs/torrent.go`:
   - Store identification results for each torrent
   - Make results available to virtual path layer

### Deliverables
- [ ] Identification runs automatically when torrent info received
- [ ] Results stored and retrievable
- [ ] Ready for Phase 3 (virtual renaming)

---

## Task 12: Virtual Path Mapper (distribyted)

**Goal:** Create bidirectional path mapping for virtual renaming.

### Steps

1. Create `distribyted/fs/virtual.go`:
   ```go
   type VirtualPathMapper struct {
       mu            sync.RWMutex
       virtualToReal map[string]string
       realToVirtual map[string]string
   }

   func NewVirtualPathMapper() *VirtualPathMapper
   func (m *VirtualPathMapper) AddMapping(virtualPath, realPath string)
   func (m *VirtualPathMapper) ToReal(virtualPath string) string
   func (m *VirtualPathMapper) ToVirtual(realPath string) string
   func (m *VirtualPathMapper) VirtualChildren(path string) []string
   ```

2. Create path generation functions:
   ```go
   // GenerateVirtualPath creates standardized media paths
   func GenerateVirtualPath(metadata *TMDBMetadata, identified *IdentifiedFile) string

   // TV: "Show Name (Year)/Season XX/Show Name - SXXEXX [Quality].ext"
   // Movie: "Movie Name (Year)/Movie Name (Year) [Quality].ext"

   func SanitizeFilename(name string) string  // Remove invalid chars
   ```

3. Handle naming conflicts:
   - If path exists, append quality info
   - If still exists, append truncated hash

### Deliverables
- [ ] Bidirectional path mapping works
- [ ] Virtual paths generated correctly for TV shows
- [ ] Virtual paths generated correctly for movies
- [ ] Naming conflicts resolved with quality suffix

---

## Task 13: WebDAV Virtual Path Integration (distribyted)

**Goal:** Modify WebDAV layer to serve virtual paths.

### Steps

1. Modify `distribyted/webdav/fs.go`:
   - Inject `VirtualPathMapper`
   - Modify `OpenFile()`: translate virtual to real before opening
   - Modify `Stat()`: handle virtual paths
   - Modify `listDir()`: return virtual structure

2. Modify `distribyted/fs/torrent.go`:
   - Build virtual mappings when torrent files loaded
   - Use identification results + TMDB metadata

3. Ensure backward compatibility:
   - Torrents without metadata use original paths
   - Both virtual and real paths work for access

### Deliverables
- [ ] WebDAV shows virtual folder structure for identified content
- [ ] Files accessible via virtual paths
- [ ] Files still accessible via original paths (backward compat)
- [ ] Subtitles appear alongside videos in virtual structure

---

## Task 14: End-to-End Testing

**Goal:** Verify the complete flow works with Infuse.

### Steps

1. Test Phase 1 (Metadata Storage):
   - Add torrent with metadata via API
   - Add torrent without metadata (backward compat)
   - Query library returns metadata
   - Update metadata for existing torrent

2. Test Phase 2 (Episode Identification):
   - Test with various torrent naming patterns
   - Verify confidence scoring
   - Check subtitle association

3. Test Phase 3 (Virtual Renaming):
   - Mount WebDAV in Finder/Explorer
   - Verify virtual folder structure
   - Add to Infuse and verify identification
   - Stream content successfully

### Deliverables
- [ ] API tests pass
- [ ] Episode identification works for common patterns
- [ ] Infuse correctly identifies TV shows
- [ ] Infuse correctly identifies movies
- [ ] Streaming works via WebDAV

---

## Files Summary

### distribyted (Go) - New Files
| File | Task | Purpose |
|------|------|---------|
| `torrent/model.go` | 1 | TMDB metadata structs |
| `episode/types.go` | 6 | Identification result types |
| `episode/patterns.go` | 7 | Compiled regex patterns |
| `episode/identifier.go` | 8 | Main identification algorithm |
| `episode/filter.go` | 9 | Media file filtering |
| `episode/quality.go` | 10 | Quality extraction |
| `fs/virtual.go` | 12 | Virtual path mapper |

### distribyted (Go) - Modified Files
| File | Task | Changes |
|------|------|---------|
| `torrent/loader/loader.go` | 1 | Interface signature update |
| `torrent/loader/db.go` | 2 | JSON storage with backward compat |
| `torrent/service.go` | 3, 11 | Metadata passing, identification integration |
| `http/model.go` | 4 | API request/response types |
| `http/api.go` | 4 | New handlers |
| `http/http.go` | 4 | Route registration |
| `fs/torrent.go` | 11, 13 | Virtual mapping integration |
| `webdav/fs.go` | 13 | Virtual path resolution |

### PrettyTVCatalog (TypeScript)
| File | Task | Changes |
|------|------|---------|
| `src/types/distribyted.ts` | 5 | TMDBMetadata type |
| `src/lib/api/distribyted.ts` | 5 | addTorrent with metadata |
| `src/config/distribyted.ts` | 5 | New endpoints |
| `src/app/api/distribyted/add/route.ts` | 5 | Pass metadata |
| `src/components/torrent/TorrentCard.tsx` | 5 | Include metadata on add |

---

## Task Dependency Order

```
Task 1 (Data Structures)
    ↓
Task 2 (Database Layer)
    ↓
Task 3 (Service Layer)
    ↓
Task 4 (HTTP API)
    ↓
Task 5 (PrettyTVCatalog)
    ↓
[Phase 1 Complete - Metadata Storage Working]
    ↓
Task 6 (Episode Types)
    ↓
Task 7 (Regex Patterns)
    ↓
Task 8 (Core Algorithm)
    ↓
Task 9 (File Filtering)
    ↓
Task 10 (Quality Extraction)
    ↓
Task 11 (Integration)
    ↓
[Phase 2 Complete - Episode Identification Working]
    ↓
Task 12 (Virtual Path Mapper)
    ↓
Task 13 (WebDAV Integration)
    ↓
Task 14 (End-to-End Testing)
    ↓
[Phase 3 Complete - Infuse Identification Working]
```

---

## Future Enhancement: LLM Fallback

The `FallbackHandler` interface in Task 8 enables future integration with local LLMs for files that regex patterns can't identify.

Example implementation:
```go
type OllamaFallback struct {
    endpoint string
    model    string
}

func (o *OllamaFallback) IdentifyBatch(files []UnidentifiedFile, metadata *TMDBMetadata) (map[string]*IdentifiedFile, error) {
    // Batch files together for efficient LLM calls
    // Send prompt like:
    // "Given TV show 'Breaking Bad' (2008), identify episodes from these filenames: ..."
    // Parse LLM response and return identified files
}
```

This can be implemented as a separate task after the core functionality is working.
