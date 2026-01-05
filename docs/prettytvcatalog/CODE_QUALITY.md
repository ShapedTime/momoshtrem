# PrettyTVCatalog - Code Quality Guide

## Core Principles

1. **Readable over clever** - Code is read 10x more than written
2. **SOLID principles** - Single responsibility, open for extension
3. **DRY but not WET** - Don't repeat, but don't over-abstract early
4. **Clear domain boundaries** - Backend API logic separate from frontend UI

---

## Project Architecture

```
src/
├── app/                  # Next.js routes (thin layer)
│   ├── api/              # API routes - orchestration only
│   └── (pages)/          # React pages - composition only
│
├── lib/                  # Business logic (domain layer)
│   ├── api/              # External API clients
│   │   ├── tmdb.ts       # TMDB API
│   │   ├── jackett.ts    # Jackett API
│   │   ├── momoshtrem.ts # momoshtrem API (library, torrents)
│   │   └── subtitle.ts   # OpenSubtitles API
│   ├── services/         # Business logic services
│   ├── auth/             # Authentication logic
│   └── utils/            # Pure utility functions
│
├── components/           # React components (UI layer)
│   ├── ui/               # Generic, reusable (Button, Card)
│   ├── media/            # Media-specific (MediaCard, Hero)
│   └── layout/           # Layout components (Header, Nav)
│
├── hooks/                # Custom React hooks
├── types/                # TypeScript interfaces
└── config/               # App configuration
```

---

## Separation of Concerns

### API Routes (`app/api/`)
- **Only** handle HTTP request/response
- **No** business logic - delegate to services
- **No** direct external API calls - use lib/api clients

```typescript
// Good: API route as thin orchestration layer
export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const query = searchParams.get('q');

  // Validate input
  if (!query) {
    return Response.json({ error: 'Query required' }, { status: 400 });
  }

  // Delegate to service
  const results = await tmdbService.search(query);
  return Response.json(results);
}
```

### API Clients (`lib/api/`)
- Handle external API communication
- Transform external responses to our types
- Handle errors consistently

```typescript
// lib/api/tmdb.ts
export class TMDBClient {
  private baseUrl = 'https://api.themoviedb.org/3';

  async search(query: string): Promise<SearchResult[]> {
    const response = await fetch(
      `${this.baseUrl}/search/multi?query=${encodeURIComponent(query)}`,
      { headers: this.getHeaders() }
    );

    if (!response.ok) {
      throw new APIError('TMDB search failed', response.status);
    }

    const data = await response.json();
    return this.transformSearchResults(data.results);
  }

  // Transform external format to our internal type
  private transformSearchResults(raw: TMDBRawResult[]): SearchResult[] {
    return raw.map(item => ({
      id: item.id,
      title: item.title || item.name,
      mediaType: item.media_type,
      posterPath: item.poster_path,
      year: this.extractYear(item.release_date || item.first_air_date),
    }));
  }
}
```

### Components (`components/`)
- **Only** handle UI rendering
- Receive data via props - no direct API calls
- Use hooks for data fetching (in pages/parent components)

```typescript
// Good: Component receives data, renders UI
interface MediaCardProps {
  title: string;
  posterUrl: string;
  rating: number;
  onClick: () => void;
}

export function MediaCard({ title, posterUrl, rating, onClick }: MediaCardProps) {
  return (
    <button onClick={onClick} className="...">
      <Image src={posterUrl} alt={title} />
      <RatingBadge value={rating} />
    </button>
  );
}
```

---

## TypeScript Guidelines

### Define Types in `types/`
```typescript
// types/media.ts
export interface Movie {
  id: number;
  title: string;
  overview: string;
  posterPath: string | null;
  backdropPath: string | null;
  releaseDate: string;
  voteAverage: number;
}

export interface TVShow {
  id: number;
  name: string;
  overview: string;
  posterPath: string | null;
  firstAirDate: string;
  voteAverage: number;
  numberOfSeasons: number;
}

// Union type for shared handling
export type MediaItem = Movie | TVShow;
```

### Use Type Guards
```typescript
export function isMovie(item: MediaItem): item is Movie {
  return 'title' in item && 'releaseDate' in item;
}

export function isTVShow(item: MediaItem): item is TVShow {
  return 'name' in item && 'firstAirDate' in item;
}
```

### Avoid `any`
```typescript
// Bad
function processData(data: any) { ... }

// Good
function processData(data: unknown) {
  if (!isValidResponse(data)) {
    throw new Error('Invalid response format');
  }
  // data is now typed
}
```

---

## Error Handling

### Create Custom Error Classes
```typescript
// lib/errors.ts
export class AppError extends Error {
  constructor(
    message: string,
    public code: string,
    public statusCode: number = 500
  ) {
    super(message);
    this.name = 'AppError';
  }
}

export class APIError extends AppError {
  constructor(message: string, statusCode: number) {
    super(message, 'API_ERROR', statusCode);
  }
}

export class NotFoundError extends AppError {
  constructor(resource: string) {
    super(`${resource} not found`, 'NOT_FOUND', 404);
  }
}
```

### Handle Errors at Boundaries
```typescript
// API route - catch and format errors
export async function GET(request: Request) {
  try {
    const data = await service.getData();
    return Response.json(data);
  } catch (error) {
    if (error instanceof AppError) {
      return Response.json(
        { error: error.message, code: error.code },
        { status: error.statusCode }
      );
    }
    // Log unexpected errors, return generic message
    console.error('Unexpected error:', error);
    return Response.json(
      { error: 'Internal server error' },
      { status: 500 }
    );
  }
}
```

---

## Commenting Guidelines

### When to Comment

**DO comment:**
- Complex business logic
- Non-obvious workarounds
- External API quirks
- Performance optimizations

**DON'T comment:**
- Obvious code
- Every function
- Temporary debug code

### Comment Examples

```typescript
// Good: Explains WHY, not WHAT
// Jackett returns size as string with units (e.g., "1.5 GB")
// We normalize to bytes for consistent sorting
function parseSize(sizeStr: string): number {
  const match = sizeStr.match(/^([\d.]+)\s*(GB|MB|KB)?$/i);
  if (!match) return 0;
  // ... conversion logic
}

// Good: Documents API quirk
// TMDB returns release_date for movies, first_air_date for TV
// Both can be empty string or undefined
function extractYear(date: string | undefined): number | null {
  if (!date) return null;
  return parseInt(date.substring(0, 4), 10) || null;
}

// Good: Explains workaround
// Next.js Image requires explicit width/height OR fill
// Using fill with aspect-ratio container for responsive posters
// See: https://nextjs.org/docs/api-reference/next/image#fill

// Bad: States the obvious
// This function gets the user
function getUser() { ... }
```

---

## File Organization

### One Concern Per File
```
components/media/
├── MediaCard.tsx        # Card component
├── MediaCard.skeleton.tsx  # Loading skeleton
├── MediaCarousel.tsx    # Carousel container
└── index.ts             # Public exports
```

### Export Barrels
```typescript
// components/media/index.ts
export { MediaCard } from './MediaCard';
export { MediaCardSkeleton } from './MediaCard.skeleton';
export { MediaCarousel } from './MediaCarousel';
```

### Naming Conventions
```
Components:  PascalCase    (MediaCard.tsx)
Hooks:       camelCase     (useTMDB.ts)
Utilities:   camelCase     (formatters.ts)
Types:       PascalCase    (types/Media.ts)
Constants:   SCREAMING_SNAKE (config/constants.ts)
```

---

## React Best Practices

### Component Structure
```typescript
// Standard component structure
import { useState, useEffect } from 'react';
import type { ComponentProps } from './types';

interface Props extends ComponentProps {
  title: string;
  onAction: () => void;
}

export function MyComponent({ title, onAction }: Props) {
  // 1. Hooks first
  const [state, setState] = useState(false);

  // 2. Derived values
  const isDisabled = !title;

  // 3. Effects
  useEffect(() => {
    // Effect logic
  }, [dependency]);

  // 4. Event handlers
  const handleClick = () => {
    onAction();
  };

  // 5. Render
  return (
    <div>
      {/* JSX */}
    </div>
  );
}
```

### Avoid Prop Drilling
```typescript
// Use composition instead of passing props through layers
// Bad
<Parent user={user}>
  <Child user={user}>
    <GrandChild user={user} />
  </Child>
</Parent>

// Good - use context for shared state, or composition
<UserProvider value={user}>
  <Parent>
    <Child>
      <GrandChild />
    </Child>
  </Parent>
</UserProvider>
```

---

## Performance Guidelines

### Memoization
```typescript
// Memoize expensive computations
const sortedResults = useMemo(
  () => results.sort((a, b) => b.seeders - a.seeders),
  [results]
);

// Memoize callbacks passed to children
const handleSelect = useCallback(
  (id: number) => onSelect(id),
  [onSelect]
);
```

### Code Splitting
```typescript
// Lazy load heavy components
const TorrentModal = dynamic(
  () => import('@/components/torrent/TorrentModal'),
  { loading: () => <ModalSkeleton /> }
);
```

---

## Git Commit Messages

```
feat: Add torrent search to movie details page
fix: Handle empty search results gracefully
refactor: Extract TMDB client to separate module
style: Fix button alignment on mobile
docs: Add API documentation
chore: Update dependencies
```
