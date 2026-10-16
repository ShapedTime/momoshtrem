import type { SourceContext } from '@/lib/source/context';
import { absoluteNumber, defaultQuery, defaultScope, normaliseTitle, samePlan, scopeOptions, searchParams, shortTitle } from '@/lib/search/query';
import { buildTitle, seedLibrary } from '@/mocks/fixtures/titles';
import type { MovieTitle, ShowTitle } from '@/lib/api/contract';

const show = (tmdbId: number, season: number | null, episode: number | null): SourceContext => ({ kind: 'show', tmdbId, season, episode, key: '' });
const movie = (tmdbId: number): SourceContext => ({ kind: 'movie', tmdbId, key: '' });

const library = seedLibrary();
const frieren = buildTitle('tv', 209867, library) as ShowTitle;
const severance = buildTitle('tv', 95396, library) as ShowTitle;
const battle = buildTitle('movie', 1054867, library) as MovieTitle;
const goodPlace = buildTitle('tv', 66573, library) as ShowTitle;

describe('normaliseTitle and shortTitle', () => {
  it('drops the punctuation release names leave out', () => {
    expect(normaliseTitle('Frieren: Beyond Journey’s End')).toBe('Frieren Beyond Journeys End');
    expect(normaliseTitle('Dune: Part Two')).toBe('Dune Part Two');
    expect(normaliseTitle('Mr. Robot')).toBe('Mr Robot');
    expect(normaliseTitle('Spider-Man - Into the Spider-Verse!')).toBe('Spider-Man Into the Spider-Verse');
  });

  it('keeps the part before a colon or a dash', () => {
    expect(shortTitle('Frieren: Beyond Journey’s End')).toBe('Frieren');
    expect(shortTitle('Star Wars - A New Hope')).toBe('Star Wars');
    expect(shortTitle('Severance')).toBe('Severance');
  });
});

describe('scope', () => {
  it('starts on the context: episode, season packs, anything for a whole show, none for a movie', () => {
    expect(defaultScope(show(1, 1, 29))).toBe('episode');
    expect(defaultScope(show(1, 1, null))).toBe('season');
    expect(defaultScope(show(1, null, null))).toBe('any');
    expect(defaultScope(movie(1))).toBeNull();
  });

  it('offers Episode · Season pack · Any for an episode, Season pack · Any for a season, nothing otherwise', () => {
    expect(scopeOptions(show(1, 1, 29))).toEqual(['episode', 'season', 'any']);
    expect(scopeOptions(show(1, 2, null))).toEqual(['season', 'any']);
    expect(scopeOptions(show(1, null, null))).toEqual([]);
    expect(scopeOptions(movie(1))).toEqual([]);
  });
});

describe('defaultQuery', () => {
  it('a movie: "Title Year"', () => {
    expect(defaultQuery(movie(1054867), battle)).toBe('One Battle After Another 2025');
  });

  it('an episode: "Title SxxEyy"; anime with absolute numbers: the short title and the number', () => {
    expect(defaultQuery(show(95396, 2, 8), severance)).toBe('Severance S02E08');
    expect(absoluteNumber(frieren, 1, 29)).toBe(29);
    expect(defaultQuery(show(209867, 1, 29), frieren)).toBe('Frieren 29');
  });

  it('anime without absolute numbers (not in the library) falls back to SxxEyy', () => {
    const outside = { ...frieren, seasons: frieren.seasons.map((s) => ({ ...s, absolute_from: null, absolute_to: null })) };
    expect(defaultQuery(show(209867, 1, 29), outside)).toBe('Frieren Beyond Journeys End S01E29');
  });

  it('a season: "Title S01"; a whole show: the title', () => {
    expect(defaultQuery(show(66573, 1, null), goodPlace)).toBe('The Good Place S01');
    expect(defaultQuery(show(66573, null, null), goodPlace)).toBe('The Good Place');
  });
});

describe('searchParams', () => {
  const base = { scope: null, sort: 'best' as const, filters: {}, q: '' };

  it('sends the context, the scope for shows, and q only when typed', () => {
    expect(searchParams(show(209867, 1, 29), { ...base, scope: 'episode' })).toEqual({
      media_type: 'tv',
      tmdb_id: 209867,
      season: 1,
      episode: 29,
      scope: 'episode',
      sort: 'best',
      page_size: 25,
    });
    expect(searchParams(movie(1054867), { ...base, q: '  battle  ' })).toEqual({
      media_type: 'movie',
      tmdb_id: 1054867,
      q: 'battle',
      sort: 'best',
      page_size: 25,
    });
  });

  it('carries the filters and the sort', () => {
    expect(searchParams(show(1, 1, null), { ...base, scope: 'season', sort: 'seeders', filters: { resolution: ['2160p'], min_seeders: 5 } })).toMatchObject({
      season: 1,
      scope: 'season',
      resolution: ['2160p'],
      min_seeders: 5,
      sort: 'seeders',
    });
  });

  it('samePlan: filters, sort and paging don’t change the plan; scope and q do', () => {
    const a = searchParams(show(1, 1, 29), { ...base, scope: 'episode' });
    expect(samePlan(a, { ...a, resolution: ['1080p'], sort: 'size', page: 2 })).toBe(true);
    expect(samePlan(a, { ...a, scope: 'season' })).toBe(false);
    expect(samePlan(a, { ...a, q: 'Frieren' })).toBe(false);
  });
});
