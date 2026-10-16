import { isWatching, parseLibraryIds, parseLibrarySearch, queryLibrary } from '@/lib/library/query';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

const items = [
  libraryItem({ id: 1, tmdb_id: 693134, title: 'Dune: Part Two', added_at: '2026-10-01T00:00:00Z', size_bytes: 18_400_000_000, last_watched_at: '2026-10-12T20:00:00Z' }),
  libraryItem({ id: 2, tmdb_id: 157336, title: 'Interstellar', availability_stage: 'downloading', added_at: '2026-10-10T00:00:00Z', size_bytes: 63_100_000_000 }),
  libraryItem({ id: 3, tmdb_id: 1054867, title: 'One Battle After Another', availability_stage: 'needs_torrent', added_at: '2026-10-09T00:00:00Z', size_bytes: 0, webdav_path: null }),
  // 26 of 38 watched: watching
  libraryShow({ id: 1, tmdb_id: 209867, title: 'Frieren', needs_review: 2, added_at: '2026-06-01T00:00:00Z', last_watched_at: '2026-10-13T20:00:00Z' }),
  // Caught up: not watching
  libraryShow({
    id: 2,
    tmdb_id: 95396,
    title: 'Severance',
    availability_stage: 'ready',
    added_at: '2025-01-01T00:00:00Z',
    size_bytes: 25_000_000_000,
    episodes_total: 18,
    episodes_ready: 18,
    episodes_watched: 18,
    last_watched_at: '2026-10-10T20:00:00Z',
  }),
];

const ids = (page: ReturnType<typeof queryLibrary>) => page.items.map((item) => item.title);

describe('queryLibrary', () => {
  it('lists newest first, with counts and the summary', () => {
    const page = queryLibrary(items);
    expect(ids(page)).toEqual(['Interstellar', 'One Battle After Another', 'Dune: Part Two', 'Frieren', 'Severance']);
    expect(page).toMatchObject({ total: 5, page: 1, page_size: 50 });
    expect(page.counts).toEqual({ all: 5, movie: 3, tv: 2, watching: 1, ready: 2, incomplete: 3, needs_review: 1 });
    expect(page.summary).toEqual({ total: 5, size_bytes: 145_700_000_000, ready: 2 });
  });

  it('filters by type and status; the counts ignore both, the summary everything', () => {
    const page = queryLibrary(items, { type: 'tv', status: 'incomplete' });
    expect(ids(page)).toEqual(['Frieren']);
    expect(page.counts.all).toBe(5);
    expect(queryLibrary(items, { status: 'needs_review' }).items.map((item) => item.title)).toEqual(['Frieren']);
    expect(queryLibrary(items, { status: 'ready' }).total).toBe(2);
    expect(queryLibrary(items, { status: 'watching' })).toMatchObject({ items: [{ title: 'Frieren' }], total: 1, counts: { all: 5, watching: 1 } });
  });

  it('watching: shows partway through, never movies, unstarted or caught-up shows', () => {
    expect(isWatching(libraryShow())).toBe(true);
    expect(isWatching(libraryShow({ episodes_watched: 0 }))).toBe(false);
    expect(isWatching(libraryShow({ episodes_watched: 38 }))).toBe(false);
    expect(isWatching(libraryShow({ episodes_watched: null }))).toBe(false);
    expect(isWatching(libraryItem({ last_watched_at: '2026-10-12T20:00:00Z' }))).toBe(false);
  });

  it('searches titles, and the counts follow the search', () => {
    const page = queryLibrary(items, { q: '  ER  ' });
    expect(ids(page)).toEqual(['Interstellar', 'One Battle After Another', 'Frieren', 'Severance']);
    expect(page.counts).toMatchObject({ all: 4, movie: 2, tv: 2 });
    expect(page.summary.total).toBe(5);
  });

  it('sorts by title, size and last watched (newest first, never-watched last as added)', () => {
    expect(ids(queryLibrary(items, { sort: 'title' }))).toEqual(['Dune: Part Two', 'Frieren', 'Interstellar', 'One Battle After Another', 'Severance']);
    expect(ids(queryLibrary(items, { sort: 'size' }))[0]).toBe('Interstellar');
    expect(ids(queryLibrary(items, { sort: 'last_watched' }))).toEqual(['Frieren', 'Dune: Part Two', 'Severance', 'Interstellar', 'One Battle After Another']);
  });

  it('pages, capping page_size at 200', () => {
    const page = queryLibrary(items, { page: 2, page_size: 2 });
    expect(ids(page)).toEqual(['Dune: Part Two', 'Frieren']);
    expect(page).toMatchObject({ total: 5, page: 2, page_size: 2 });
    expect(queryLibrary(items, { page_size: 500 }).page_size).toBe(200);
    expect(queryLibrary(items, { page: 9 }).items).toEqual([]);
  });

  it('looks titles up by ids, ANDed with the filters, on one page of them all', () => {
    const page = queryLibrary(items, { ids: 'movie:693134,tv:209867,tv:1', page: 3, page_size: 1 });
    expect(ids(page)).toEqual(['Dune: Part Two', 'Frieren']);
    expect(page).toMatchObject({ total: 2, page: 1, page_size: 3, counts: { all: 2, movie: 1, tv: 1 } });
    expect(page.summary.total).toBe(5);
    // movie:209867 is no movie in this library
    expect(ids(queryLibrary(items, { ids: 'movie:209867,movie:693134', type: 'movie' }))).toEqual(['Dune: Part Two']);
  });
});

describe('parseLibraryIds', () => {
  it('reads media_type:tmdb_id pairs, each once', () => {
    expect(parseLibraryIds('movie:603, tv:1399,movie:603')).toEqual(new Set(['movie:603', 'tv:1399']));
  });

  it.each(['', 'movie', 'movie:', 'show:1', 'movie:0', 'movie:-1', 'tv:1x', 'movie:1,'])('rejects %p', (raw) => {
    expect(parseLibraryIds(raw)).toBeNull();
  });

  it('takes at most 200', () => {
    const many = (n: number) => Array.from({ length: n }, (_, i) => `movie:${i + 1}`).join(',');
    expect(parseLibraryIds(many(200))?.size).toBe(200);
    expect(parseLibraryIds(many(201))).toBeNull();
  });
});

describe('parseLibrarySearch', () => {
  const parse = (query: string) => parseLibrarySearch(new URLSearchParams(query));

  it('reads every parameter', () => {
    expect(parse('type=tv&status=ready&sort=title&q=fri&page=2&page_size=10&ids=tv:1')).toEqual({
      ok: true,
      params: { type: 'tv', status: 'ready', sort: 'title', q: 'fri', page: 2, page_size: 10, ids: 'tv:1' },
    });
  });

  it.each(['type=music', 'status=done', 'sort=rating', 'page=0', 'page=x', 'page_size=-1', 'ids=603'])('answers 400 for %p', (query) => {
    expect(parse(query)).toMatchObject({ ok: false });
  });
});
