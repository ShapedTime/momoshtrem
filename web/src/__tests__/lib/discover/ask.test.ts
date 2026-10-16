import { discoverAskQuery } from '@/lib/discover/ask';
import { DEFAULT_FILTERS, type DiscoverFilters } from '@/lib/discover/filters';

/** Discover's empty state asks Noir for the filters in words (#156; discover.js). */

const filters = (overrides: Partial<DiscoverFilters>): DiscoverFilters => ({ ...DEFAULT_FILTERS, ...overrides });

describe('discoverAskQuery', () => {
  it('reads the design’s filters as an ask', () => {
    expect(discoverAskQuery(filters({ type: 'tv', genres: ['comedy'], anime: true, status: 'ended' }))).toBe('an anime comedy show that has finished airing');
    expect(discoverAskQuery(filters({ type: 'tv', genres: ['comedy'], anime: true, status: 'ended', from: 2020, to: 2025, rating: 8 }))).toBe(
      'an anime comedy show from 2020 to 2025, rated 8+, that has finished airing'
    );
  });

  it('names genres, the type, years, origin, rating and runtime', () => {
    expect(discoverAskQuery(filters({ genres: ['sci-fi', 'thriller'] }))).toBe('a sci-fi thriller');
    expect(discoverAskQuery(filters({ type: 'movie', runtime: 'short' }))).toBe('a movie on the short side');
    expect(discoverAskQuery(filters({ from: 2010, to: 2019 }))).toBe('something from the 2010s');
    expect(discoverAskQuery(filters({ from: null, to: 1999 }))).toBe('something from before 2000');
    expect(discoverAskQuery(filters({ from: 2015, to: null }))).toBe('something from 2015 on');
    expect(discoverAskQuery(filters({ from: null, to: 1985 }))).toBe('something from 1985 or earlier');
    expect(discoverAskQuery(filters({ lang: 'ko', country: 'KR', rating: 7 }))).toBe('something in Korean, made in South Korea, rated 7+');
    expect(discoverAskQuery(filters({ lang: 'ko', genres: ['drama'] }))).toBe('a Korean drama');
    expect(discoverAskQuery(filters({ type: 'tv', status: 'returning' }))).toBe('a show that’s still airing');
  });

  it('doesn’t say Japanese twice for anime, nor a show status for movies', () => {
    expect(discoverAskQuery(filters({ anime: true, lang: 'ja' }))).toBe('an anime');
    expect(discoverAskQuery(filters({ type: 'movie', status: 'ended' }))).toBe('a movie');
  });

  it('is empty without filters, so the button opens a blank ask', () => {
    expect(discoverAskQuery(DEFAULT_FILTERS)).toBe('');
    expect(discoverAskQuery(filters({ type: 'tv' }))).toBe('a show');
  });
});
