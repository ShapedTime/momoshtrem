# Stage 1: MVP - Implementation Tasks

Each task is designed to be completable in one focused session.

---

## Task 1: Project Setup & Base Configuration

**Goal:** Working Next.js project with Tailwind, TypeScript, and Docker ready.

### Steps
1. Initialize Next.js 14 with App Router and TypeScript
   ```bash
   npx create-next-app@latest PrettyTVCatalog --typescript --tailwind --app --src-dir
   ```
2. Configure Tailwind with dark theme colors from style guide
3. Set up project folder structure (`lib/`, `components/`, `types/`, `hooks/`)
4. Create `.env.example` with all required variables
5. Write Dockerfile for production build
6. Add service to `docker-compose.yml`

### Deliverables
- [ ] Project runs locally with `npm run dev`
- [ ] Dark theme applied globally
- [ ] Docker build succeeds

---

## Task 2: Authentication

**Goal:** Password-protected app with session cookies.

### Steps
1. Create login page (`app/(auth)/login/page.tsx`)
   - Password input field
   - Submit button
   - Error message display
2. Create auth API route (`app/api/auth/route.ts`)
   - POST: Verify password against `APP_PASSWORD` env
   - Set HTTP-only session cookie on success
3. Create auth middleware (`middleware.ts`)
   - Check for valid session cookie
   - Redirect to `/login` if not authenticated
4. Create protected layout (`app/(protected)/layout.tsx`)

### Deliverables
- [ ] Login page styled per style guide
- [ ] Invalid password shows error
- [ ] Valid password redirects to home
- [ ] Direct access to `/` redirects to login if not authenticated

---

## Task 3: Base UI Components

**Goal:** Reusable component library ready for use.

### Steps
1. Create `components/ui/Button.tsx`
   - Primary, secondary, ghost variants
   - Loading state with spinner
   - Responsive sizing
2. Create `components/ui/Input.tsx`
   - Text input with label
   - Error state
3. Create `components/ui/Card.tsx`
   - Base card container
4. Create `components/ui/Modal.tsx`
   - Overlay with backdrop
   - Close button
   - Body content slot
5. Create `components/ui/Skeleton.tsx`
   - Rectangle and circle variants for loading states
6. Create `components/ui/Toast.tsx`
   - Success, error, info variants
   - Auto-dismiss

### Deliverables
- [ ] All components responsive (mobile-first)
- [ ] Keyboard accessible (focus states)
- [ ] Components exported from `components/ui/index.ts`

---

## Task 4: TMDB API Integration

**Goal:** Fetch trending content and search from TMDB.

### Steps
1. Create types (`types/tmdb.ts`)
   - Movie, TVShow, SearchResult interfaces
2. Create TMDB client (`lib/api/tmdb.ts`)
   - `getTrending()` - Fetch trending movies/shows
   - `search(query)` - Multi-search
   - `getMovie(id)` - Movie details with credits
   - `getTVShow(id)` - TV show details
   - `getSeason(showId, seasonNum)` - Season episodes
3. Create API routes
   - `app/api/tmdb/trending/route.ts`
   - `app/api/tmdb/search/route.ts`
   - `app/api/tmdb/movie/[id]/route.ts`
   - `app/api/tmdb/tv/[id]/route.ts`
4. Create React hook (`hooks/useTMDB.ts`)
   - Wrap fetch calls with loading/error states

### Deliverables
- [ ] TMDB API key working
- [ ] Trending endpoint returns data
- [ ] Search returns results
- [ ] Movie/TV details return full info

### Notes
- Consider using cavestri/themoviedb-javascript-library.
- Keep it simple.

---

## Task 5: Home Page with Hero & Carousels

**Goal:** Apple TV, Netflix-style home page with trending content.

### Steps
1. Create `components/media/HeroBanner.tsx`
   - Full-width backdrop image
   - Title, overview, rating
   - Gradient overlay
   - "View Details" button
2. Create `components/media/MediaCard.tsx`
   - Poster image with aspect ratio
   - Title on hover
   - Rating badge
   - Link to details page
3. Create `components/media/MediaCarousel.tsx`
   - Horizontal scroll container
   - Section title
   - Responsive card sizing
4. Create home page (`app/(protected)/page.tsx`)
   - Hero with random trending item
   - "Trending Movies" carousel
   - "Trending TV Shows" carousel
5. Create `components/layout/Header.tsx`
   - Logo
   - Search input (navigates to search page)
   - Basic navigation

### Deliverables
- [ ] Home page displays trending content
- [ ] Hero rotates/selects featured item
- [ ] Carousels scroll horizontally
- [ ] Cards link to detail pages
- [ ] Responsive on mobile/tablet/desktop

---

## Task 6: Search Page

**Goal:** Search TMDB and display results.

### Steps
1. Create search page (`app/(protected)/search/page.tsx`)
   - Search input (pre-filled from URL query)
   - Results grid
   - Empty state
   - Loading skeleton
2. Create `components/search/SearchBar.tsx`
   - Debounced input (300ms)
   - Clear button
   - Search icon
3. Create `components/search/SearchResults.tsx`
   - Grid of MediaCards
   - Shows movie/TV type badge

### Deliverables
- [ ] Search from header navigates to search page
- [ ] Results update as user types (debounced)
- [ ] Clicking result goes to details
- [ ] Empty query shows message

---

## Task 7: Movie Details Page

**Goal:** Display movie info with torrent search trigger.

### Steps
1. Create movie page (`app/(protected)/movie/[id]/page.tsx`)
   - Backdrop hero with movie info
   - Overview, release date, runtime, rating
   - Cast carousel (top 10)
   - "Search Torrents" button (opens torrent section)
2. Create `components/media/MediaDetails.tsx`
   - Reusable details layout
   - Metadata display
3. Create `components/media/CastCarousel.tsx`
   - Cast member cards with photo and name

### Deliverables
- [ ] Movie details load from TMDB
- [ ] Page is responsive
- [ ] "Search Torrents" button visible (functionality in Task 9)

---

## Task 8: TV Show Details Page

**Goal:** Browse TV shows with seasons and episodes.

### Steps
1. Create TV show page (`app/(protected)/tv/[id]/page.tsx`)
   - Show info (similar to movie)
   - Season selector dropdown
   - Episode list for selected season
2. Create season page (`app/(protected)/tv/[id]/season/[season]/page.tsx`)
   - Episode cards with thumbnails
   - Episode title, number, air date, overview
   - "Search Torrents" button per episode
3. Create `components/media/SeasonPicker.tsx`
   - Dropdown to select season
4. Create `components/media/EpisodeList.tsx`
   - List of episode cards

### Deliverables
- [ ] TV show details display
- [ ] Seasons load and switch
- [ ] Episodes display with info
- [ ] Torrent search button on episodes

---

## Task 9: Jackett Integration & Torrent Search Modal

**Goal:** Search torrents for selected media via a modal triggered from detail pages.

### Steps
1. Create types (`types/jackett.ts`)
   - TorrentResult interface
2. Create Jackett client (`lib/api/jackett.ts`)
   - `search(query, category)` - Search torrents
   - Parse Torznab XML response to JSON
   - Extract: title, size, seeders, leechers, magnet
3. Create API route (`app/api/jackett/search/route.ts`)
4. Create `components/torrent/TorrentSearchModal.tsx`
   - Modal overlay using the Modal component from Task 3
   - Search input (pre-filled with movie/show title + year)
   - Loading state while fetching results
   - Close button (X) and click-outside-to-close
5. Create `components/torrent/TorrentResults.tsx`
   - List of torrent cards inside modal
   - Sort by seeders (default)
   - Filter by quality (4K, 1080p, 720p)
6. Create `components/torrent/TorrentCard.tsx`
   - Title, size, seeders/leechers
   - Quality badge (4K, 1080p, 720p parsed from title)
   - "Add" button
7. Wire up "Search Torrents" buttons on detail pages
   - Movie page: Opens modal with movie title + year
   - TV show page: Opens modal with show title
   - Episode: Opens modal with "Show S01E01" format

### Deliverables
- [ ] Jackett search returns results
- [ ] Results display in modal with quality info
- [ ] Modal opens from "Search Torrents" button on movie/TV pages
- [ ] Sort by seeders works
- [ ] "Add" button ready (functionality in Task 10)

---

## Task 10: Distribyted Integration

**Goal:** Add torrents to distribyted for streaming.

### Steps
1. Create types (`types/distribyted.ts`)
   - AddTorrentRequest, StatusResponse interfaces
2. Create distribyted client (`lib/api/distribyted.ts`)
   - `addTorrent(magnet)` - POST to add magnet
   - `getStatus()` - GET status
   - `getRoutes()` - GET routes and torrents
3. Create API routes
   - `app/api/distribyted/add/route.ts`
   - `app/api/distribyted/status/route.ts`
4. Wire up "Add" button in TorrentCard
   - Call add API
   - Show success/error toast
   - Disable button after adding (prevent duplicates)
5. Add status indicator somewhere visible
   - Show if distribyted is connected

### Deliverables
- [ ] Adding torrent sends magnet to distribyted
- [ ] Success toast confirms addition
- [ ] Error toast on failure
- [ ] Distribyted status visible in UI

---

## Task 11: Docker & Proxmox Deployment

**Goal:** Production-ready Docker containers that work in the full stack on Proxmox.

### Context
The Dockerfile and docker-compose.yml scaffolding exists but needs finalization. This task ensures the app builds and runs correctly alongside Jackett and distribyted in the production environment described in `proxmox-installation.md`.

### Steps

**1. Finalize docker-compose.yml**
- Uncomment the `prettytvcatalog` service
- Verify all environment variables are passed correctly
- Add container health checks for all services:
  ```yaml
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:3000/api/health"]
    interval: 30s
    timeout: 10s
    retries: 3
  ```
- Ensure `depends_on` with health conditions for service startup order

**2. Create health check endpoint**
- Add `app/api/health/route.ts` that verifies:
  - App is running
  - Can reach TMDB (optional, graceful degradation)
  - Returns JSON `{ status: "healthy", timestamp: ... }`

**3. Environment validation**
- Create `lib/env.ts` with runtime validation of required env vars
- Fail fast on startup if critical vars missing (APP_PASSWORD, TMDB_API_KEY)
- Log warnings for optional vars (JACKETT_*, DISTRIBYTED_*)

**4. Optimize Docker build**
- Verify `.dockerignore` excludes: `node_modules`, `.next`, `.git`, `*.md`, `.env*`
- Confirm `next.config.js` has `output: 'standalone'` for minimal image
- Test build completes without errors: `docker build -t prettytvcatalog ./PrettyTVCatalog`

**5. Test full stack locally**
- Run `docker-compose up -d --build`
- Verify all 3 containers start and stay healthy
- Test inter-service communication:
  - PrettyTVCatalog → Jackett (torrent search)
  - PrettyTVCatalog → distribyted (add torrent)
- Check container logs for errors: `docker-compose logs prettytvcatalog`

**6. Update documentation**
- Ensure `.env.example` has all required variables with descriptions
- Update `proxmox-installation.md` if any new env vars or config changes
- Add troubleshooting section for common Docker issues

### Deliverables
- [ ] `docker-compose up -d --build` succeeds with all services
- [ ] Health check endpoint returns 200
- [ ] PrettyTVCatalog can communicate with Jackett (search works)
- [ ] PrettyTVCatalog can communicate with distribyted (add torrent works)
- [ ] Container stays running for 5+ minutes without crashes
- [ ] No unhandled errors in container logs
- [ ] `.env.example` is complete and documented

---

## Task 12: UI Polish & Error Handling

**Goal:** Production-quality user experience with proper loading states and error handling.

### Context
The app is functional but needs polish for a good user experience. Users should never see raw errors, broken layouts, or wonder if something is loading.

### Steps

**1. Add loading skeletons**
- Home page: Hero skeleton + carousel skeletons
- Search page: Grid of card skeletons
- Movie/TV detail pages: Backdrop skeleton + info skeleton
- Torrent modal: List item skeletons
- Use the `Skeleton` component from Task 3, create if missing

**2. Implement error boundaries**
- Create `components/ErrorBoundary.tsx` (class component required for error boundaries)
- Create `app/error.tsx` for app-wide error catching
- Create `app/(protected)/error.tsx` for protected route errors
- Show user-friendly message with retry button
- **Always log errors to console** for debugging

**3. Handle API failure states**
- TMDB unreachable: Show cached content or "Unable to load content" message
- Jackett unreachable: Show "Torrent search unavailable" in modal
- distribyted unreachable: Show "Streaming service offline" toast
- Network errors: Retry button where appropriate
- **Log all API errors to console** with request details

**4. Empty states**
- Search with no results: "No results found for '{query}'"
- Torrent search with no results: "No torrents found. Try different keywords."
- Use illustrations or icons to make empty states feel intentional

**5. Test full user flow**
- Login → Home → Browse carousels → Search → View details → Search torrents → Add torrent
- Test on mobile viewport (375px) and desktop (1440px)
- Fix any responsive breakage found
- Document any edge cases discovered

**6. Production build verification**
- Run `npm run build` and check for warnings
- Run `npm run start` and test the production build
- Open browser devtools, verify no uncaught exceptions
- Check Network tab for failed requests

### Deliverables
- [ ] All pages show skeletons while loading (no blank screens)
- [ ] Errors display user-friendly messages (no raw stack traces shown to user)
- [ ] All errors logged to console for debugging
- [ ] Empty states have helpful messaging
- [ ] Full flow works without errors on mobile and desktop
- [ ] Production build runs without uncaught exceptions

### Testing Checklist
```
[ ] Home page loads with skeletons → content
[ ] Search shows skeleton → results → empty state
[ ] Movie detail handles missing backdrop gracefully
[ ] Torrent modal shows loading → results → error states
[ ] Add torrent shows success/error toast
[ ] Resize to mobile - no horizontal scroll or overflow
[ ] Slow network (Chrome devtools) - skeletons visible
[ ] Offline mode - appropriate error messages
```

---

## Task 13: Navigation Menu

**Goal:** Persistent navigation for quick access to content sections.

### Steps
1. Update `components/layout/Header.tsx`
   - Add navigation links: Home, Movies, TV Shows
   - Active state styling for current route
   - Mobile: Collapse to hamburger menu
2. Create Movies browse page (`app/(protected)/movies/page.tsx`)
   - Grid of trending/popular movies
   - Link from nav
3. Create TV Shows browse page (`app/(protected)/tv-shows/page.tsx`)
   - Grid of trending/popular TV shows
   - Link from nav
4. Add breadcrumb component (`components/ui/Breadcrumb.tsx`)
   - Show path: Home > Movies > Movie Title
   - Clickable links back to parent sections

### Deliverables
- [ ] Navigation visible on all pages
- [ ] Active route highlighted
- [ ] Mobile hamburger menu works
- [ ] Breadcrumbs on detail pages

---

## Task 14: Genre Filtering & Browsing

**Goal:** Browse content by genre for better discovery.

### Steps
1. Extend TMDB client (`lib/api/tmdb.ts`)
   - `getGenres(type)` - Fetch movie/TV genre lists
   - `discoverByGenre(type, genreId)` - Fetch content by genre
2. Create API routes
   - `app/api/tmdb/genres/route.ts`
   - `app/api/tmdb/discover/route.ts`
3. Create `components/browse/GenreFilter.tsx`
   - Horizontal scrollable genre pills/chips
   - Multi-select support
   - Clear filters button
4. Update Movies/TV Shows browse pages
   - Add GenreFilter component
   - Filter results by selected genres
5. Create genre page (`app/(protected)/genre/[id]/page.tsx`)
   - Show all content for a specific genre
   - Paginated results

### Deliverables
- [ ] Genre list loads from TMDB
- [ ] Clicking genre filters results
- [ ] Genre chips show selected state
- [ ] Dedicated genre pages work

---

## Task 15: Watchlist & Favorites

**Goal:** Save content for later viewing and track favorites.

### Steps
1. Create watchlist storage
   - Option A: localStorage (simple, per-browser)
   - Option B: JSON file on server (shared across devices)
2. Create types (`types/watchlist.ts`)
   - WatchlistItem interface (id, type, title, poster, addedAt)
3. Create watchlist API routes (if using server storage)
   - `app/api/watchlist/route.ts` - GET all, POST add
   - `app/api/watchlist/[id]/route.ts` - DELETE remove
4. Create `components/watchlist/WatchlistButton.tsx`
   - Toggle button (bookmark icon)
   - Shows filled when in watchlist
   - Optimistic UI update
5. Add WatchlistButton to:
   - MediaCard component
   - Movie details page
   - TV show details page
6. Create Watchlist page (`app/(protected)/watchlist/page.tsx`)
   - Grid of saved items
   - Remove button on each item
   - Empty state message
7. Add "My Watchlist" to navigation menu

### Deliverables
- [ ] Can add/remove items from watchlist
- [ ] Watchlist persists across sessions
- [ ] Watchlist page shows all saved items
- [ ] Button state reflects watchlist status

---

## Environment Variables Required

```env
# Auth
APP_PASSWORD=your-secure-password

# TMDB (get from themoviedb.org)
TMDB_API_KEY=your-api-key

# Jackett (from Jackett dashboard)
JACKETT_URL=http://jackett:9117
JACKETT_API_KEY=your-api-key

# Distribyted
DISTRIBYTED_URL=http://distribyted:4444
DISTRIBYTED_ROUTE=media
```

---

## Task Dependency Order

```
Task 1 (Setup)
    ↓
Task 2 (Auth)
    ↓
Task 3 (UI Components)
    ↓
Task 4 (TMDB API)
    ↓
Task 5 (Home Page)
    ↓
Task 6 (Search) ←──┐
    ↓              │
Task 7 (Movie) ────┤
    ↓              │
Task 8 (TV Show) ──┘
    ↓
Task 9 (Jackett + Modal)
    ↓
Task 10 (Distribyted)
    ↓
Task 11 (Docker Deployment)
    ↓
Task 12 (UI Polish)
    ↓
┌───────────────────────────────────┐
│  Post-MVP Enhancements            │
│  (can be done in any order)       │
├───────────────────────────────────┤
│ Task 13 (Navigation Menu)         │
│ Task 14 (Genre Filtering)         │
│ Task 15 (Watchlist/Favorites)     │
└───────────────────────────────────┘
```

Tasks 6, 7, 8 can be done in parallel after Task 5.
Tasks 13, 14, 15 are post-MVP enhancements and can be done in any order after Task 12.
