/** @jest-environment node */
// nuqs's loader looks for a global Request, which jsdom lacks
import { DEFAULT_FILTERS, type DiscoverFilters } from '@/lib/discover/filters';
import { applyMood, MOODS, moodMatches, selectedMood } from '@/lib/discover/moods';
import { discoverApiQuery, discoverHref, parseDiscoverSearch } from '@/lib/discover/url';

const f = (patch: Partial<DiscoverFilters> = {}): DiscoverFilters => ({ ...DEFAULT_FILTERS, ...patch });

describe('moods', () => {
  it('are six distinct presets', () => {
    expect(MOODS.map((mood) => mood.label)).toEqual(['Cosy', 'Mind-bending', 'Slow cinema', 'Laugh-out-loud', 'Short & sharp', 'Epic nights']);
    const presets = MOODS.map((mood) => applyMood(f(), mood.id));
    for (const [index, preset] of presets.entries()) {
      expect(selectedMood(preset)?.id).toBe(MOODS[index].id);
    }
  });

  it('replace the filters and keep the type and sort', () => {
    const before = f({ type: 'tv', sort: 'newest', genres: ['war'], country: 'GB' });
    expect(applyMood(before, 'epic')).toEqual({ ...DEFAULT_FILTERS, type: 'tv', sort: 'newest', genres: ['adventure'], runtime: 'long', rating: 7, mood: 'epic' });
    expect(applyMood(before, 'cosy')).toMatchObject({ genres: ['comedy', 'family'], rating: 7, country: null, mood: 'cosy' });
  });

  it('are selected only while the filters equal the preset', () => {
    const cosy = applyMood(f(), 'cosy');
    expect(moodMatches('cosy', cosy)).toBe(true);
    // Type and sort don't matter
    expect(moodMatches('cosy', { ...cosy, type: 'movie', sort: 'title' })).toBe(true);
    // Genre order doesn't either
    expect(moodMatches('cosy', { ...cosy, genres: ['family', 'comedy'] })).toBe(true);
    // An edit does
    expect(moodMatches('cosy', { ...cosy, rating: 8 })).toBe(false);
    expect(moodMatches('cosy', { ...cosy, genres: ['comedy'] })).toBe(false);
    expect(selectedMood({ ...cosy, anime: true })).toBeUndefined();
  });

  it('don’t trust `mood` in the URL', () => {
    expect(selectedMood(f({ mood: 'cosy' }))).toBeUndefined();
    expect(selectedMood(f())).toBeUndefined();
  });
});

describe('URL', () => {
  it('reads every key', () => {
    expect(parseDiscoverSearch('type=tv&genres=comedy,sci-fi&from=2015&to=2020&rating=8&runtime=short&lang=ja&country=JP&anime=1&status=ended&mood=cosy&sort=rating')).toEqual({
      type: 'tv',
      genres: ['comedy', 'sci-fi'],
      from: 2015,
      to: 2020,
      rating: 8,
      runtime: 'short',
      lang: 'ja',
      country: 'JP',
      anime: true,
      status: 'ended',
      mood: 'cosy',
      sort: 'rating',
    });
  });

  it('falls back to the defaults and drops what it doesn’t know', () => {
    expect(parseDiscoverSearch('')).toEqual(DEFAULT_FILTERS);
    expect(parseDiscoverSearch('type=anime&genres=horror,drama&from=99&rating=9&runtime=epic&lang=de&anime=yes&status=paused&sort=random')).toEqual({
      ...DEFAULT_FILTERS,
      genres: ['drama'],
    });
  });

  it('normalises: years in order, no status for movies', () => {
    expect(parseDiscoverSearch('type=movie&status=ended&from=2020&to=2010')).toMatchObject({ type: 'movie', status: null, from: 2010, to: 2020 });
  });

  it('writes only what differs from the defaults', () => {
    expect(discoverHref(DEFAULT_FILTERS)).toBe('/discover');
    expect(discoverHref(f({ type: 'tv', genres: ['sci-fi', 'comedy'], from: 2015, rating: 8, anime: true, mood: 'cosy' }))).toBe(
      '/discover?type=tv&genres=comedy,sci-fi&from=2015&rating=8&anime=1&mood=cosy'
    );
    expect(discoverHref(f({ sort: 'title' }))).toBe('/discover?sort=title');
  });

  it('round-trips', () => {
    const filters = f({ type: 'movie', genres: ['war'], to: 1999, runtime: 'long', lang: 'fr', country: 'FR', sort: 'newest' });
    expect(parseDiscoverSearch(discoverHref(filters).split('?')[1])).toEqual(filters);
  });

  it('gives the API the filters without `mood`', () => {
    expect(discoverApiQuery(f({ type: 'tv', genres: ['comedy'], mood: 'laugh' }))).toBe('type=tv&genres=comedy');
    expect(discoverApiQuery(DEFAULT_FILTERS)).toBe('');
  });
});
