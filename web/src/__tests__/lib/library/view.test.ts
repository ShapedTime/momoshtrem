import { DEFAULT_VIEW, libraryParams, libraryViewQuery, parseLibraryView, withoutPending } from '@/lib/library/view';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

describe('the view state in the URL', () => {
  it('reads it, falling back to the defaults', () => {
    expect(parseLibraryView(new URLSearchParams())).toEqual(DEFAULT_VIEW);
    expect(parseLibraryView(new URLSearchParams('filter=review&sort=size&view=list&q=fri'))).toEqual({ filter: 'review', sort: 'size', view: 'list', q: 'fri' });
    expect(parseLibraryView(new URLSearchParams('filter=bogus&sort=rating&view=table'))).toEqual(DEFAULT_VIEW);
  });

  it('writes it without the defaults, keeping other params', () => {
    expect(libraryViewQuery(DEFAULT_VIEW)).toBe('');
    expect(libraryViewQuery({ ...DEFAULT_VIEW, filter: 'shows', view: 'list' }, new URLSearchParams('sheet=infuse-setup&q=old'))).toBe(
      'sheet=infuse-setup&filter=shows&view=list'
    );
    expect(libraryViewQuery({ ...DEFAULT_VIEW, q: '   ' })).toBe('');
    expect(libraryViewQuery({ ...DEFAULT_VIEW, sort: 'title', q: 'dune' })).toBe('sort=title&q=dune');
  });

  it('maps it to GET /library', () => {
    expect(libraryParams(DEFAULT_VIEW)).toEqual({ sort: 'added' });
    expect(libraryParams({ filter: 'movies', sort: 'title', view: 'grid', q: ' dune ' })).toEqual({ type: 'movie', sort: 'title', q: 'dune' });
    expect(libraryParams({ ...DEFAULT_VIEW, filter: 'review' })).toEqual({ status: 'needs_review', sort: 'added' });
    expect(libraryParams({ ...DEFAULT_VIEW, filter: 'watching' })).toEqual({ status: 'watching', sort: 'added' });
  });
});

describe('withoutPending', () => {
  const page = {
    total: 9,
    counts: { all: 9, movie: 3, tv: 6, watching: 0, ready: 5, incomplete: 4, needs_review: 3 },
    summary: { total: 9, size_bytes: 700_000_000_000, ready: 5 },
  };

  it('takes a title waiting out its Undo window off the counts, the total and the summary', () => {
    const removed = libraryItem({ availability_stage: 'downloading', size_bytes: 63_000_000_000 });
    expect(withoutPending(page, [removed], DEFAULT_VIEW)).toEqual({
      total: 8,
      counts: { all: 8, movie: 2, tv: 6, watching: 0, ready: 5, incomplete: 3, needs_review: 3 },
      summary: { total: 8, size_bytes: 637_000_000_000, ready: 5 },
    });
  });

  it('leaves the total alone when the title isn’t under the filter, and the counts when it doesn’t match the search', () => {
    const show = libraryShow({ needs_review: 2 });
    expect(withoutPending(page, [show], { ...DEFAULT_VIEW, filter: 'movies' })).toMatchObject({ total: 9, counts: { tv: 5, needs_review: 2 } });
    expect(withoutPending(page, [show], { ...DEFAULT_VIEW, q: 'dune' })).toMatchObject({ total: 9, counts: { all: 9 }, summary: { total: 8 } });
  });

  it('never goes below zero', () => {
    const empty = { total: 0, counts: { ...page.counts, all: 0, movie: 0 }, summary: { total: 0, size_bytes: 0, ready: 0 } };
    expect(withoutPending(empty, [libraryItem()], DEFAULT_VIEW)).toMatchObject({ total: 0, counts: { all: 0, movie: 0 }, summary: { total: 0, ready: 0 } });
  });
});
