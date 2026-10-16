import {
  activeFilters,
  clearFilters,
  countLabel,
  DECADES,
  DEFAULT_FILTERS,
  type DiscoverFilters,
  filterBadgeCount,
  normalizeFilters,
  toggleDecade,
  withoutFilter,
  yearLabel,
} from '@/lib/discover/filters';
import { GENRE_SLUGS, GENRES, genreIds, isGenreSlug } from '@/lib/discover/genres';

const NOW = new Date('2026-10-16T12:00:00Z');
const f = (patch: Partial<DiscoverFilters> = {}): DiscoverFilters => ({ ...DEFAULT_FILTERS, ...patch });

describe('genre table', () => {
  it('has the design’s 14 genres in order, with slugs', () => {
    expect(GENRES.map((genre) => genre.label)).toEqual([
      'Action',
      'Adventure',
      'Animation',
      'Comedy',
      'Documentary',
      'Drama',
      'Family',
      'Fantasy',
      'History',
      'Mystery',
      'Romance',
      'Sci-Fi',
      'Thriller',
      'War',
    ]);
    expect(GENRES.map((genre) => genre.slug)).toEqual([...GENRE_SLUGS]);
    expect(isGenreSlug('sci-fi')).toBe(true);
    expect(isGenreSlug('horror')).toBe(false);
  });

  it('holds TMDB’s ids per type (checked against /genre/movie/list and /genre/tv/list)', () => {
    const ids = Object.fromEntries(GENRES.map((genre) => [genre.slug, [genre.ids.movie, genre.ids.tv]]));
    expect(ids).toEqual({
      action: [28, 10759],
      adventure: [12, 10759],
      animation: [16, 16],
      comedy: [35, 35],
      documentary: [99, 99],
      drama: [18, 18],
      family: [10751, 10751],
      fantasy: [14, 10765],
      history: [36, null],
      mystery: [9648, 9648],
      romance: [10749, null],
      'sci-fi': [878, 10765],
      thriller: [53, null],
      war: [10752, 10768],
    });
  });

  it('gives null when one genre has no id for the type', () => {
    expect(genreIds(['drama', 'history'], 'tv')).toBeNull();
    expect(genreIds(['drama', 'history'], 'movie')).toEqual([18, 36]);
    expect(genreIds([], 'tv')).toEqual([]);
  });
});

describe('normalizeFilters', () => {
  it('orders genres, swaps years and drops status for movies', () => {
    expect(normalizeFilters(f({ genres: ['war', 'action', 'war'], from: 2020, to: 2010 }))).toMatchObject({ genres: ['action', 'war'], from: 2010, to: 2020 });
    expect(normalizeFilters(f({ type: 'movie', status: 'ended' })).status).toBeNull();
    expect(normalizeFilters(f({ type: 'all', status: 'ended' })).status).toBe('ended');
  });
});

describe('years', () => {
  it('names decades and ranges', () => {
    expect(yearLabel(f(), NOW)).toBeNull();
    expect(yearLabel(f({ from: 2020 }), NOW)).toBe('2020s');
    expect(yearLabel(f({ to: 1999 }), NOW)).toBe('Before 2000');
    expect(yearLabel(f({ from: 2010, to: 2019 }), NOW)).toBe('2010s');
    expect(yearLabel(f({ from: 2015 }), NOW)).toBe('2015–2026');
    expect(yearLabel(f({ to: 1985 }), NOW)).toBe('Until 1985');
    expect(yearLabel(f({ from: 2001, to: 2010 }), NOW)).toBe('2001–2010');
  });

  it('toggles a decade on and off', () => {
    const tens = DECADES[1];
    const on = toggleDecade(f(), tens);
    expect(on).toMatchObject({ from: 2010, to: 2019 });
    expect(toggleDecade(on, tens)).toMatchObject({ from: null, to: null });
    expect(toggleDecade(on, DECADES[0])).toMatchObject({ from: 2020, to: null });
  });
});

describe('active filters', () => {
  const busy = f({ type: 'tv', genres: ['comedy'], anime: true, from: 2020, rating: 8, runtime: 'short', lang: 'ja', country: 'JP', status: 'ended', sort: 'rating' });

  it('lists chips in the design’s order, without the type', () => {
    expect(activeFilters(busy, NOW)).toEqual([
      { key: 'genre:comedy', label: 'Comedy' },
      { key: 'anime', label: 'Anime' },
      { key: 'year', label: '2020s' },
      { key: 'rating', label: 'Rated 8+' },
      { key: 'runtime', label: 'Short runtime' },
      { key: 'lang', label: 'Japanese' },
      { key: 'country', label: 'Japan' },
      { key: 'status', label: 'Ended' },
    ]);
    expect(filterBadgeCount(busy)).toBe(9);
    expect(filterBadgeCount(f())).toBe(0);
  });

  it('removes one filter at a time', () => {
    expect(withoutFilter(busy, 'genre:comedy').genres).toEqual([]);
    expect(withoutFilter(busy, 'year')).toMatchObject({ from: null, to: null });
    expect(withoutFilter(busy, 'anime').anime).toBe(false);
    expect(withoutFilter(busy, 'status').status).toBeNull();
  });

  it('clears all but type and sort', () => {
    expect(clearFilters(busy)).toEqual({ ...DEFAULT_FILTERS, type: 'tv', sort: 'rating' });
  });

  it('counts with the type’s noun', () => {
    expect(countLabel('all', 10296)).toBe('10,296 titles');
    expect(countLabel('tv', 1)).toBe('1 show');
    expect(countLabel('movie', 0)).toBe('0 movies');
    // TMDB's totals stop at about 20,000
    expect(countLabel('movie', 20001)).toBe('20,000+ movies');
    expect(countLabel('all', 27016)).toBe('20,000+ titles');
    expect(countLabel('tv', 19999)).toBe('19,999 shows');
  });
});
