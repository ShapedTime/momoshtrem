import { DEFAULT_FILTERS, type DiscoverFilters } from '@/lib/discover/filters';
import {
  discoverStreams,
  type DiscoverRow,
  isoDay,
  mergeStreams,
  relaxKeys,
  skipReason,
  tmdbDiscoverParams,
  voteFloor,
  yearOf,
} from '@/lib/discover/tmdb-query';

const TODAY = '2026-10-16';
const f = (patch: Partial<DiscoverFilters> = {}): DiscoverFilters => ({ ...DEFAULT_FILTERS, ...patch });
const movie = (patch: Partial<DiscoverFilters> = {}, page = 1) => tmdbDiscoverParams(f(patch), 'movie', page, TODAY);
const tv = (patch: Partial<DiscoverFilters> = {}, page = 1) => tmdbDiscoverParams(f(patch), 'tv', page, TODAY);

describe('tmdbDiscoverParams', () => {
  it('asks for popular, rated titles by default', () => {
    expect(movie()).toEqual({ include_adult: false, page: 1, sort_by: 'popularity.desc', 'vote_count.gte': 50 });
    // TV leaves out News, Reality, Soap and Talk
    expect(tv({}, 3)).toEqual({
      include_adult: false,
      include_null_first_air_dates: false,
      without_genres: '10763|10764|10766|10767',
      page: 3,
      sort_by: 'popularity.desc',
      'vote_count.gte': 50,
    });
  });

  it('ANDs genres with per-type ids, each once', () => {
    expect(movie({ genres: ['action', 'adventure'] }).with_genres).toBe('28,12');
    // Action and Adventure are both "Action & Adventure" on TV
    expect(tv({ genres: ['action', 'adventure'] }).with_genres).toBe('10759');
    expect(tv({ genres: ['fantasy', 'sci-fi', 'comedy'] }).with_genres).toBe('10765,35');
    expect(movie({ genres: ['war'] }).with_genres).toBe('10752');
    expect(tv({ genres: ['war'] }).with_genres).toBe('10768');
  });

  it('maps the year range to each type’s date field', () => {
    expect(movie({ from: 2010, to: 2019 })).toMatchObject({ 'primary_release_date.gte': '2010-01-01', 'primary_release_date.lte': '2019-12-31' });
    expect(tv({ from: 2010 })).toMatchObject({ 'first_air_date.gte': '2010-01-01' });
    expect(tv({ from: 2010 })).not.toHaveProperty('first_air_date.lte');
    expect(tv({ to: 1999 })).toMatchObject({ 'first_air_date.lte': '1999-12-31' });
  });

  it('adds the rating floor', () => {
    expect(movie({ rating: 8 })).toMatchObject({ 'vote_average.gte': 8, 'vote_count.gte': 50 });
  });

  it('maps runtime bands per type', () => {
    expect(tv({ runtime: 'short' })).toMatchObject({ 'with_runtime.lte': 30 });
    expect(tv({ runtime: 'short' })).not.toHaveProperty('with_runtime.gte');
    expect(tv({ runtime: 'standard' })).toMatchObject({ 'with_runtime.gte': 31, 'with_runtime.lte': 50 });
    expect(tv({ runtime: 'long' })).toMatchObject({ 'with_runtime.gte': 51 });
    expect(movie({ runtime: 'short' })).toMatchObject({ 'with_runtime.lte': 99 });
    expect(movie({ runtime: 'standard' })).toMatchObject({ 'with_runtime.gte': 100, 'with_runtime.lte': 140 });
    expect(movie({ runtime: 'long' })).toMatchObject({ 'with_runtime.gte': 141 });
    expect(movie({ runtime: 'long' })).not.toHaveProperty('with_runtime.lte');
  });

  it('maps language and country', () => {
    expect(movie({ lang: 'ko', country: 'KR' })).toMatchObject({ with_original_language: 'ko', with_origin_country: 'KR' });
  });

  it('makes anime Animation plus Japanese', () => {
    expect(tv({ anime: true })).toMatchObject({ with_genres: '16', with_original_language: 'ja' });
    expect(tv({ anime: true, genres: ['comedy'] }).with_genres).toBe('35,16');
    expect(tv({ anime: true, genres: ['animation'] }).with_genres).toBe('16');
  });

  it('maps show status to TMDB’s codes, on TV only', () => {
    expect(tv({ status: 'returning' }).with_status).toBe('0');
    expect(tv({ status: 'ended' }).with_status).toBe('3|4');
    expect(tv({ status: 'upcoming' }).with_status).toBe('1|2');
  });

  it('maps each sort per type', () => {
    expect(movie({ sort: 'rating' })).toMatchObject({ sort_by: 'vote_average.desc', 'vote_count.gte': 200 });
    expect(movie({ sort: 'title' }).sort_by).toBe('title.asc');
    expect(tv({ sort: 'title' }).sort_by).toBe('name.asc');
    expect(movie({ sort: 'newest' }).sort_by).toBe('primary_release_date.desc');
    expect(tv({ sort: 'newest' }).sort_by).toBe('first_air_date.desc');
  });

  it('caps Newest first at today so unreleased titles don’t lead', () => {
    expect(movie({ sort: 'newest' })['primary_release_date.lte']).toBe(TODAY);
    expect(tv({ sort: 'newest', to: 2030 })['first_air_date.lte']).toBe(TODAY);
    expect(tv({ sort: 'newest', to: 2019 })['first_air_date.lte']).toBe('2019-12-31');
    // Upcoming shows are in the future by definition
    expect(tv({ sort: 'newest', status: 'upcoming' })).not.toHaveProperty('first_air_date.lte');
  });
});

describe('voteFloor', () => {
  it('is higher for Highest rated and off for upcoming shows', () => {
    expect(voteFloor(f())).toBe(50);
    expect(voteFloor(f({ sort: 'rating' }))).toBe(200);
    expect(voteFloor(f({ status: 'upcoming' }))).toBe(0);
    expect(voteFloor(f({ status: 'upcoming', rating: 7 }))).toBe(50);
    expect(tv({ status: 'upcoming' })).not.toHaveProperty('vote_count.gte');
  });
});

describe('discoverStreams', () => {
  const types = (filters: DiscoverFilters) => discoverStreams(filters, 1, TODAY).map((stream) => stream.mediaType);

  it('queries both types for All and one for Movies or Shows', () => {
    expect(types(f())).toEqual(['movie', 'tv']);
    expect(types(f({ type: 'movie' }))).toEqual(['movie']);
    expect(types(f({ type: 'tv' }))).toEqual(['tv']);
  });

  it('skips TV when a genre has no TV id, and finds nothing for Shows', () => {
    expect(skipReason(f({ genres: ['thriller'] }), 'tv')).toBe('genre');
    expect(types(f({ genres: ['sci-fi', 'thriller'] }))).toEqual(['movie']);
    expect(types(f({ type: 'tv', genres: ['romance'] }))).toEqual([]);
  });

  it('skips movies when a show status is set', () => {
    expect(types(f({ status: 'ended' }))).toEqual(['tv']);
  });

  it('finds nothing for anime in a language other than Japanese', () => {
    expect(types(f({ anime: true, lang: 'en' }))).toEqual([]);
    expect(types(f({ anime: true, lang: 'ja' }))).toEqual(['movie', 'tv']);
  });

  it('passes the page through', () => {
    expect(discoverStreams(f(), 4, TODAY).map((stream) => stream.params.page)).toEqual([4, 4]);
  });
});

describe('relaxKeys', () => {
  it('lists every active filter, not the type', () => {
    expect(relaxKeys(f({ type: 'tv', anime: true, genres: ['comedy'], from: 2020, rating: 8, status: 'ended' }))).toEqual([
      'genre:comedy',
      'anime',
      'year',
      'rating',
      'status',
    ]);
    expect(relaxKeys(f({ type: 'movie' }))).toEqual([]);
  });
});

describe('mergeStreams', () => {
  const row = (media_type: 'movie' | 'tv', tmdb_id: number, keys: Partial<DiscoverRow>): DiscoverRow => ({
    media_type,
    tmdb_id,
    title: `T${tmdb_id}`,
    year: 2020,
    poster_path: null,
    popularity: 0,
    vote_average: 0,
    date: '',
    ...keys,
  });

  it('interleaves by popularity, ties to movies', () => {
    const movies = [row('movie', 1, { popularity: 90 }), row('movie', 2, { popularity: 50 }), row('movie', 3, { popularity: 10 })];
    const shows = [row('tv', 4, { popularity: 70 }), row('tv', 5, { popularity: 50 })];
    expect(mergeStreams('popular', [movies, shows]).map((r) => r.tmdb_id)).toEqual([1, 4, 2, 5, 3]);
  });

  it('interleaves by date with undated titles last', () => {
    const movies = [row('movie', 1, { date: '2026-01-02' }), row('movie', 2, { date: '' })];
    const shows = [row('tv', 3, { date: '2026-03-01' }), row('tv', 4, { date: '2020-01-01' })];
    expect(mergeStreams('newest', [movies, shows]).map((r) => r.tmdb_id)).toEqual([3, 1, 4, 2]);
  });

  it('interleaves by title and by rating', () => {
    const movies = [row('movie', 1, { title: 'Arrival', vote_average: 7.6 }), row('movie', 2, { title: 'Dune', vote_average: 7.8 })];
    const shows = [row('tv', 3, { title: 'Bluey', vote_average: 8.7 })];
    expect(mergeStreams('title', [movies, shows]).map((r) => r.title)).toEqual(['Arrival', 'Bluey', 'Dune']);
    expect(mergeStreams('rating', [shows, movies]).map((r) => r.tmdb_id)).toEqual([3, 1, 2]);
  });

  it('passes one stream through', () => {
    const shows = [row('tv', 3, {}), row('tv', 1, {})];
    expect(mergeStreams('popular', [shows])).toEqual(shows);
  });
});

describe('helpers', () => {
  it('reads years and days', () => {
    expect(yearOf('2024-03-01')).toBe(2024);
    expect(yearOf('')).toBeNull();
    expect(yearOf(undefined)).toBeNull();
    expect(isoDay(new Date('2026-10-16T23:00:00Z'))).toBe('2026-10-16');
  });
});
