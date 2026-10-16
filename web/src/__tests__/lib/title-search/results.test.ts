import type { LibraryItem, TitleSearchItem } from '@/lib/api/contract';
import { inTab, searchSections } from '@/lib/title-search/results';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

function tmdb(title: string, overrides: Partial<TitleSearchItem> = {}): TitleSearchItem {
  return { media_type: 'movie', tmdb_id: 1, title, original_title: null, year: 2021, poster_path: null, match: 'exact', match_reason: null, ...overrides };
}

const none = () => undefined;
const lookupOf = (items: LibraryItem[]) => (mediaType: string, tmdbId: number) => items.find((item) => item.media_type === mediaType && item.tmdb_id === tmdbId);

const dunePartTwo = libraryItem();
const frieren = libraryShow();

describe('searchSections', () => {
  it('puts your library first and leaves TMDB the titles you don’t have', () => {
    const items = [tmdb('Dune', { tmdb_id: 438631 }), tmdb('Dune: Part Two', { tmdb_id: 693134, match: 'prefix' }), tmdb('Dune: Prophecy', { media_type: 'tv', tmdb_id: 90228, match: 'prefix' })];
    const sections = searchSections('dune', items, [dunePartTwo], none);
    expect(sections.library.map((hit) => hit.item.title)).toEqual(['Dune: Part Two']);
    expect(sections.titles.map((title) => title.title)).toEqual(['Dune', 'Dune: Prophecy']);
    expect(sections.counts).toEqual({ all: 3, tv: 1, movie: 2 });
  });

  it('finds library titles TMDB matched by another name, with the reason', () => {
    const items = [tmdb('Frieren: Beyond Journey’s End', { media_type: 'tv', tmdb_id: 209867, match: 'original', match_reason: 'Original title match' })];
    const sections = searchSections('sousou no frieren', items, [], lookupOf([frieren]));
    expect(sections.library).toEqual([{ item: frieren, reason: 'Original title match' }]);
    expect(sections.titles).toEqual([]);
    expect(sections.counts.all).toBe(1);
  });

  it('keeps the title match when both agree, and ranks library hits by their best match', () => {
    const studio = libraryShow({ tmdb_id: 247767, title: 'The Studio', missing: null });
    const items = [tmdb('Dune: Part Two', { tmdb_id: 693134, match: 'person', match_reason: 'With Zendaya' })];
    const sections = searchSections('dune part', items, [studio, dunePartTwo], none);
    expect(sections.library.map((hit) => [hit.item.title, hit.reason])).toEqual([
      ['Dune: Part Two', null],
      ['The Studio', null],
    ]);
  });

  it('counts library titles TMDB didn’t list', () => {
    const sections = searchSections('frieren', [], [frieren], none);
    expect(sections.counts).toEqual({ all: 1, tv: 1, movie: 0 });
  });
});

describe('inTab', () => {
  const sections = searchSections('dune', [tmdb('Dune', { tmdb_id: 438631 }), tmdb('Dune: Prophecy', { media_type: 'tv', tmdb_id: 90228 })], [dunePartTwo], none);

  it('shows everything on All', () => {
    expect(inTab(sections, 'all')).toBe(sections);
  });

  it('keeps one type on Shows and Movies', () => {
    expect(inTab(sections, 'tv').library).toEqual([]);
    expect(inTab(sections, 'tv').titles.map((title) => title.title)).toEqual(['Dune: Prophecy']);
    expect(inTab(sections, 'movie').library.map((hit) => hit.item.title)).toEqual(['Dune: Part Two']);
    expect(inTab(sections, 'movie').titles.map((title) => title.title)).toEqual(['Dune']);
  });
});
