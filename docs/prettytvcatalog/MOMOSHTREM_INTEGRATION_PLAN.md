# PrettyTVCatalog + momoshtrem Integration Plan

## Overview

Replace distribyted integration with momoshtrem, enabling a library-first media management experience with automatic torrent assignment.

### Key Design Decisions
- **Combined flow**: Adding a torrent automatically adds item to library if not exists
- **No download progress UI**: Keep it simple
- **Library page**: New dedicated page for managing library
- **Auto-assign with results**: Show assignment results after torrent is added
- **Skip torrent management**: No pause/resume/delete for now

---

## User Experience Flow

### Flow 1: Add to Library (Curate without Torrent)
```
User on movie/show detail page
    → Clicks "Add to Library" button
    → Backend: POST /api/movies or /api/shows
    → Show toast: "Added to Library"
    → Button changes to "In Library" (disabled/checked state)
    → User can later search torrents or leave as "want to watch"
```

### Flow 2: Add Movie Torrent (from Movie Detail Page)
```
User on movie detail page → Clicks "Search Torrents"
    → Modal opens, searches Jackett
    → User clicks "Add" on a torrent result
    → Backend: Check if movie in library
        → If not: POST /api/movies (add to library)
    → Backend: POST /api/movies/:id/assign-torrent
    → Show toast: "Added to library" or "Torrent assigned"
    → Update UI to show "In Library" state
```

### Flow 3: Add TV Show Torrent (from TV Detail Page)
```
User on TV show detail page → Clicks "Search Torrents" (for season/show)
    → Modal opens, searches Jackett
    → User clicks "Add" on a torrent result
    → Backend: Check if show in library
        → If not: POST /api/shows (add to library with all episodes)
    → Backend: POST /api/shows/:id/assign-torrent
    → Show assignment results modal:
        "Matched 8/10 episodes"
        - S01E01 ✓ Breaking.Bad.S01E01.mkv
        - S01E02 ✓ Breaking.Bad.S01E02.mkv
        - S01E09 ✗ No matching file
        - S01E10 ✗ No matching file
    → User closes modal
    → Update UI to show library status
```

### Flow 4: Browse Library
```
User clicks "Library" in nav
    → See grid of movies and shows in library
    → Each card shows:
        - Poster
        - Title
        - Status indicator (has torrent / pending)
    → Click card → Navigate to detail page
    → Can remove items from library
```

---

## Implementation Phases

## Phase 1: Foundation (API Client & Types)

### 1.1 Create momoshtrem Types
**File:** `src/types/momoshtrem.ts`

```typescript
// Library Models
export interface LibraryMovie {
  id: number;
  tmdb_id: number;
  title: string;
  year: number;
  created_at: string;
  has_assignment: boolean;
  assignment?: TorrentAssignment;
}

export interface LibraryShow {
  id: number;
  tmdb_id: number;
  title: string;
  year: number;
  created_at: string;
  seasons: LibrarySeason[];
}

export interface LibrarySeason {
  id: number;
  season_number: number;
  episodes: LibraryEpisode[];
}

export interface LibraryEpisode {
  id: number;
  episode_number: number;
  name: string;
  has_assignment: boolean;
  assignment?: TorrentAssignment;
}

export interface TorrentAssignment {
  id: number;
  info_hash: string;
  file_path: string;
  file_size: number;
  resolution?: string;
  source?: string;
}

// API Request/Response Types
export interface AddMovieRequest {
  tmdb_id: number;
}

export interface AddShowRequest {
  tmdb_id: number;
}

export interface AssignTorrentRequest {
  magnet_uri: string;
}

export interface MovieAssignmentResponse {
  success: boolean;
  assignment: TorrentAssignment;
}

export interface ShowAssignmentResponse {
  success: boolean;
  summary: {
    total_files: number;
    matched: number;
    unmatched: number;
    skipped: number;
  };
  matched: EpisodeMatch[];
  unmatched: UnmatchedFile[];
}

export interface EpisodeMatch {
  episode_id: number;
  season: number;
  episode: number;
  file_path: string;
  file_size: number;
  resolution?: string;
  confidence: 'high' | 'medium' | 'low';
}

export interface UnmatchedFile {
  file_path: string;
  reason: string;
  season?: number;
  episode?: number;
}
```

### 1.2 Create momoshtrem API Client
**File:** `src/lib/api/momoshtrem.ts`

```typescript
class MomoshtremClient {
  private baseUrl: string;
  private timeout: number;

  constructor() {
    this.baseUrl = process.env.MOMOSHTREM_URL || 'http://localhost:4444';
    this.timeout = 30000; // Longer timeout for torrent metadata
  }

  // Movies
  async addMovie(tmdbId: number): Promise<LibraryMovie>;
  async getMovies(): Promise<LibraryMovie[]>;
  async getMovie(id: number): Promise<LibraryMovie>;
  async deleteMovie(id: number): Promise<void>;
  async assignMovieTorrent(id: number, magnetUri: string): Promise<MovieAssignmentResponse>;
  async unassignMovie(id: number): Promise<void>;

  // Shows
  async addShow(tmdbId: number): Promise<LibraryShow>;
  async getShows(): Promise<LibraryShow[]>;
  async getShow(id: number): Promise<LibraryShow>;
  async deleteShow(id: number): Promise<void>;
  async assignShowTorrent(id: number, magnetUri: string): Promise<ShowAssignmentResponse>;

  // Library (combined)
  async getLibrary(): Promise<{ movies: LibraryMovie[]; shows: LibraryShow[] }>;

  // Utility
  async findMovieByTmdbId(tmdbId: number): Promise<LibraryMovie | null>;
  async findShowByTmdbId(tmdbId: number): Promise<LibraryShow | null>;
}

export const momoshtremClient = new MomoshtremClient();
```

### 1.3 Update Environment Config
**File:** `src/config/momoshtrem.ts`

```typescript
export const MOMOSHTREM_CONFIG = {
  baseUrl: process.env.MOMOSHTREM_URL || 'http://localhost:4444',
  timeout: 30000,
} as const;
```

**Update:** `src/lib/env.ts` - Add MOMOSHTREM_URL validation

---

## Phase 2: API Routes

### 2.1 Library API Routes
**File:** `src/app/api/library/movies/route.ts`
- GET: List all movies in library
- POST: Add movie to library

**File:** `src/app/api/library/movies/[id]/route.ts`
- GET: Get single movie
- DELETE: Remove movie from library

**File:** `src/app/api/library/movies/[id]/assign/route.ts`
- POST: Assign torrent to movie

**File:** `src/app/api/library/shows/route.ts`
- GET: List all shows in library
- POST: Add show to library

**File:** `src/app/api/library/shows/[id]/route.ts`
- GET: Get single show with episodes
- DELETE: Remove show from library

**File:** `src/app/api/library/shows/[id]/assign/route.ts`
- POST: Assign torrent to show (returns match results)

### 2.2 Combined Add & Assign Route
**File:** `src/app/api/library/add-torrent/route.ts`

This is the key endpoint that implements the combined flow:
```typescript
// POST /api/library/add-torrent
// Body: { magnetUri, mediaType: 'movie' | 'tv', tmdbId, title?, year? }
//
// 1. Check if item exists in library (by tmdbId)
// 2. If not, add to library
// 3. Assign torrent
// 4. Return assignment result
```

---

## Phase 3: React Hooks

### 3.1 Library Hooks
**File:** `src/hooks/useLibrary.ts`

```typescript
// Fetch all library items
export function useLibrary() {
  // Returns { movies, shows, isLoading, error, refresh }
}

// Check if specific item is in library
export function useLibraryStatus(mediaType: 'movie' | 'tv', tmdbId: number) {
  // Returns { inLibrary, libraryId, hasAssignment, isLoading }
}
```

### 3.2 Add to Library Hook
**File:** `src/hooks/useAddToLibrary.ts`

```typescript
// Combined add + assign flow
export function useAddToLibrary() {
  // Returns {
  //   addMovieTorrent: (tmdbId, magnetUri) => Promise<MovieAssignmentResponse>,
  //   addShowTorrent: (tmdbId, magnetUri) => Promise<ShowAssignmentResponse>,
  //   isAdding,
  //   error
  // }
}
```

---

## Phase 4: UI Components

### 4.1 Add to Library Button
**File:** `src/components/library/AddToLibraryButton.tsx`

Button for adding items to library without a torrent:
```
┌─────────────────────────────────────────────────────┐
│  States:                                            │
│                                                     │
│  Default:     [+ Add to Library]     (primary)      │
│  Loading:     [○ Adding...]          (disabled)     │
│  In Library:  [✓ In Library]         (success)      │
│  Has Torrent: [✓ Ready to Stream]    (success+icon) │
└─────────────────────────────────────────────────────┘
```

### 4.2 Library Status Badge
**File:** `src/components/library/LibraryStatusBadge.tsx`

Small badge showing library status on media cards/pages:
- Not in library: (no badge)
- In library, no torrent: "In Library" (neutral)
- In library, has torrent: "Ready to Stream" (green checkmark)

### 4.3 Assignment Results Modal
**File:** `src/components/library/AssignmentResultsModal.tsx`

Modal shown after assigning torrent to a show:
```
┌─────────────────────────────────────────┐
│  Assignment Results              [X]    │
├─────────────────────────────────────────┤
│                                         │
│  ✓ Matched 8 of 10 episodes             │
│                                         │
│  Season 1                               │
│  ├─ ✓ E01 - Pilot                       │
│  ├─ ✓ E02 - Cat's in the Bag           │
│  ├─ ✓ E03 - ...And the Bag's in the... │
│  │   ...                                │
│  ├─ ✗ E09 - No matching file            │
│  └─ ✗ E10 - No matching file            │
│                                         │
│               [Done]                    │
└─────────────────────────────────────────┘
```

### 4.4 Library Page Components
**File:** `src/components/library/LibraryGrid.tsx`
- Responsive grid of library items
- Filter tabs: All | Movies | TV Shows
- Empty state when library is empty

**File:** `src/components/library/LibraryCard.tsx`
- Poster with overlay
- Title, year
- Status indicator
- Click → navigate to detail page
- Remove button (with confirmation)

**File:** `src/components/library/LibraryCard.skeleton.tsx`
- Loading skeleton for library cards

**File:** `src/components/library/EmptyLibrary.tsx`
- Friendly empty state with CTA to browse

---

## Phase 5: Pages

### 5.1 Library Page
**File:** `src/app/(protected)/library/page.tsx`

```
┌─────────────────────────────────────────────────────┐
│  [Logo]  Home  Browse  Library  Search      [User]  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  My Library                                         │
│                                                     │
│  [All] [Movies] [TV Shows]                          │
│                                                     │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐       │
│  │ Poster │ │ Poster │ │ Poster │ │ Poster │       │
│  │   ✓    │ │   ○    │ │   ✓    │ │   ✓    │       │
│  │ Title  │ │ Title  │ │ Title  │ │ Title  │       │
│  │ 2024   │ │ 2023   │ │ 2024   │ │ 2022   │       │
│  └────────┘ └────────┘ └────────┘ └────────┘       │
│                                                     │
└─────────────────────────────────────────────────────┘

✓ = Has torrent assigned (green indicator)
○ = In library but no torrent (neutral)
```

### 5.2 Update Movie Detail Page
**File:** `src/app/(protected)/movie/[id]/page.tsx`

Changes:
- Add library status indicator
- Update TorrentSearchModal integration to use new flow
- Show "In Library" badge if applicable
- After adding torrent, show success state

### 5.3 Update TV Show Detail Page
**File:** `src/app/(protected)/tv/[id]/page.tsx`

Changes:
- Add library status indicator
- Update TorrentSearchModal integration
- After adding torrent, show AssignmentResultsModal
- Per-episode status indicators (optional, phase 2)

---

## Phase 6: Update Torrent Components

### 6.1 Update TorrentSearchModal
**File:** `src/components/torrent/TorrentSearchModal.tsx`

Changes:
- Replace distribyted `addTorrent` with momoshtrem `addToLibrary`
- For TV shows: After add, open AssignmentResultsModal with results
- Update success handling

### 6.2 Update TorrentCard
**File:** `src/components/torrent/TorrentCard.tsx`

Changes:
- Update `onAdd` callback to work with new flow
- No structural changes needed

---

## Phase 7: Navigation & Polish

### 7.1 Update Header Navigation
**File:** `src/components/layout/Header.tsx`

Add "Library" link between Browse and Search

### 7.2 Cleanup

- Delete `src/lib/api/distribyted.ts`
- Delete `src/app/api/distribyted/` routes
- Delete `src/types/distribyted.ts`
- Delete `src/config/distribyted.ts`
- Update `src/hooks/useDistribyted.ts` → remove or replace
- Remove DISTRIBYTED_URL from env validation

---

## File Checklist

### New Files
- [ ] `src/types/momoshtrem.ts`
- [ ] `src/lib/api/momoshtrem.ts`
- [ ] `src/config/momoshtrem.ts`
- [ ] `src/app/api/library/movies/route.ts`
- [ ] `src/app/api/library/movies/[id]/route.ts`
- [ ] `src/app/api/library/movies/[id]/assign/route.ts`
- [ ] `src/app/api/library/shows/route.ts`
- [ ] `src/app/api/library/shows/[id]/route.ts`
- [ ] `src/app/api/library/shows/[id]/assign/route.ts`
- [ ] `src/app/api/library/add-torrent/route.ts`
- [ ] `src/hooks/useLibrary.ts`
- [ ] `src/hooks/useAddToLibrary.ts`
- [ ] `src/components/library/AddToLibraryButton.tsx`
- [ ] `src/components/library/LibraryStatusBadge.tsx`
- [ ] `src/components/library/AssignmentResultsModal.tsx`
- [ ] `src/components/library/LibraryGrid.tsx`
- [ ] `src/components/library/LibraryCard.tsx`
- [ ] `src/components/library/LibraryCard.skeleton.tsx`
- [ ] `src/components/library/EmptyLibrary.tsx`
- [ ] `src/components/library/index.ts`
- [ ] `src/app/(protected)/library/page.tsx`

### Modified Files
- [ ] `src/lib/env.ts` - Add MOMOSHTREM_URL
- [ ] `src/components/layout/Header.tsx` - Add Library nav
- [ ] `src/components/torrent/TorrentSearchModal.tsx` - Use new API
- [ ] `src/app/(protected)/movie/[id]/page.tsx` - Library status
- [ ] `src/app/(protected)/tv/[id]/page.tsx` - Library status + results modal

### Deleted Files
- [ ] `src/lib/api/distribyted.ts`
- [ ] `src/config/distribyted.ts`
- [ ] `src/types/distribyted.ts`
- [ ] `src/hooks/useDistribyted.ts`
- [ ] `src/app/api/distribyted/add/route.ts`

---

## Implementation Order

1. **Phase 1**: Types & API Client (foundation)
2. **Phase 2**: API Routes (backend)
3. **Phase 3**: React Hooks (state management)
4. **Phase 4**: UI Components (building blocks)
5. **Phase 5**: Pages (assembly)
6. **Phase 6**: Torrent Components (integration)
7. **Phase 7**: Navigation & Cleanup (polish)

Each phase builds on the previous, allowing for incremental testing.

---

## Testing Checklist

### Happy Paths
- [ ] Add movie torrent from detail page → appears in library
- [ ] Add show torrent → see assignment results with matched episodes
- [ ] View library page with mixed movies/shows
- [ ] Filter library by type
- [ ] Remove item from library
- [ ] Add torrent to item already in library (just assigns)

### Edge Cases
- [ ] Add torrent when momoshtrem is down → graceful error
- [ ] Show with 0 matched episodes → show warning
- [ ] Empty library state
- [ ] Very long show title / many episodes in results modal

### Responsive
- [ ] Library page on mobile (2-column grid)
- [ ] Assignment results modal on mobile (scrollable)
- [ ] Library cards touch targets (48px minimum)
