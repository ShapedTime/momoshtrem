import { ApiError } from '@/lib/api/client';
import type { IndexerReport, SearchMissingRun } from '@/lib/api/contract';
import {
  addLabel,
  alsoSearched,
  indexerList,
  indexerProgress,
  missingCopy,
  noResultsBody,
  noResultsSuggestions,
  noResultsTitle,
  pagerText,
  partialProblem,
  resultsHeading,
  searchingDetail,
  searchingTitle,
  searchProblem,
  statusLine,
} from '@/lib/search/copy';

const ok = (name: string, count = 3): IndexerReport => ({ name, status: 'ok', count, ms: 300 });
const FRIEREN_QUERIES = [
  { q: 'Frieren 29', label: null },
  { q: 'Sousou no Frieren 29', label: 'original title' as const },
  { q: 'Frieren S02E01', label: 'scene numbering' as const },
];

describe('status lines', () => {
  it('names up to three indexers that fit, else counts them', () => {
    expect(indexerList(['Nyaa', 'TorrentLeech', 'The Pirate Bay'])).toBe('Nyaa, TorrentLeech, The Pirate Bay');
    expect(indexerList(['Nyaa', 'TorrentLeech', 'The Pirate Bay', 'EZTV'])).toBe('4 indexers');
    expect(indexerList(['A very long indexer name', 'Another long indexer name'])).toBe('2 indexers');
  });

  it('"27 results · Nyaa, TorrentLeech, The Pirate Bay": the indexers that answered; "so far" while searching', () => {
    const indexers = [ok('Nyaa'), ok('TorrentLeech'), ok('The Pirate Bay')];
    expect(statusLine({ total: 27, indexers, complete: true })).toBe('27 results · Nyaa, TorrentLeech, The Pirate Bay');
    expect(
      statusLine({ total: 1, indexers: [ok('Nyaa'), { name: 'The Pirate Bay', status: 'pending', count: 0, ms: 4000 }], complete: false })
    ).toBe('1 result so far · Nyaa');
    expect(statusLine({ total: 0, indexers: [], complete: true })).toBe('0 results');
  });

  it('"Also searched": the queries after the primary, explaining scene numbering and the whole-show fallback', () => {
    expect(alsoSearched(FRIEREN_QUERIES)).toBe('Also searched: Sousou no Frieren 29 · Frieren S02E01 (scene numbering)');
    expect(alsoSearched([{ q: 'Severance S02E08', label: null }, { q: 'Severance', label: 'whole show' }])).toBe(
      'Also searched: Severance (whole show)'
    );
    expect(alsoSearched([{ q: 'Severance S02E08', label: null }])).toBeNull();
  });
});

describe('searching', () => {
  it('counts the indexers once known, and names the first other query', () => {
    expect(searchingTitle([])).toBe('Searching indexers…');
    expect(searchingTitle([ok('Nyaa'), ok('TorrentLeech'), ok('The Pirate Bay')])).toBe('Searching 3 indexers…');
    expect(searchingDetail('Frieren 29', FRIEREN_QUERIES)).toBe('Frieren 29 · also Sousou no Frieren 29');
    expect(searchingDetail('Frieren 29', [])).toBe('Frieren 29');
  });

  it('says how each indexer is doing', () => {
    expect(indexerProgress(ok('Nyaa', 26))).toBe('26 results');
    expect(indexerProgress({ name: 'TPB', status: 'pending', count: 0, ms: 6200 })).toBe('waiting · 6 s');
    expect(indexerProgress({ name: 'TPB', status: 'pending', count: 0, ms: 120 })).toBe('waiting · 1 s');
    expect(indexerProgress({ name: 'TPB', status: 'timeout', count: 0, ms: 8000 })).toBe('timed out after 8 s');
    expect(indexerProgress({ name: 'TPB', status: 'error', count: 0, ms: 50, error: 'Rejected the API key' })).toBe('Rejected the API key');
    expect(indexerProgress({ name: 'TPB', status: 'error', count: 0, ms: 50 })).toBe('failed');
  });
});

describe('partialProblem', () => {
  const timeout = (name: string, ms = 8000): IndexerReport => ({ name, status: 'timeout', count: 0, ms });

  it('names who timed out or failed, and says the results may be incomplete', () => {
    expect(partialProblem([ok('Nyaa'), ok('TorrentLeech'), timeout('The Pirate Bay')])).toEqual({
      title: '2 of 3 indexers answered',
      body: 'The Pirate Bay timed out after 8 s. Results may be incomplete.',
      none: false,
    });
    expect(
      partialProblem([ok('Nyaa'), timeout('EZTV', 7600), timeout('The Pirate Bay'), { name: '1337x', status: 'error', count: 0, ms: 20, error: 'Cloudflare blocked it' }])
    ).toEqual({
      title: '1 of 4 indexers answered',
      body: 'EZTV and The Pirate Bay timed out after 8 s. 1337x failed: Cloudflare blocked it. Results may be incomplete.',
      none: false,
    });
  });

  it('when nobody answered it says so; nothing while anyone is pending or when all answered', () => {
    expect(partialProblem([timeout('Nyaa'), timeout('TorrentLeech')])).toMatchObject({ title: 'None of the 2 indexers answered', none: true });
    expect(partialProblem([timeout('Nyaa')])).toMatchObject({ title: 'Nyaa didn’t answer', body: 'Nyaa timed out after 8 s. Try again in a moment.' });
    expect(partialProblem([ok('Nyaa'), { name: 'TPB', status: 'pending', count: 0, ms: 3000 }, timeout('X')])).toBeNull();
    expect(partialProblem([ok('Nyaa')])).toBeNull();
  });
});

describe('missingCopy', () => {
  const run: SearchMissingRun = { season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10, best_covers: true };

  it('suggests a pack, and says when the best match is one', () => {
    expect(missingCopy(run, false)).toEqual({ title: '10 episodes are missing', body: 'A pack gets 29–38 in one go. The best match below already does.' });
    expect(missingCopy({ ...run, best_covers: false }, false).body).toBe('A pack gets 29–38 in one go.');
  });

  it('uses absolute numbers for a show ordered that way', () => {
    const onePiece = { ...run, season_number: 22, from: 15, to: 16, absolute_from: 1100, absolute_to: 1101, count: 2, best_covers: false };
    expect(missingCopy(onePiece, true).body).toBe('A pack gets 1100–1101 in one go.');
    expect(missingCopy(onePiece, false).body).toBe('A pack gets 15–16 in one go.');
  });
});

describe('no results', () => {
  const frieren = { title: 'Frieren: Beyond Journey’s End', original_language: 'ja' };

  it('says what was searched and where', () => {
    expect(noResultsTitle('Frieren 29')).toBe('No releases for “Frieren 29”');
    expect(noResultsBody(3)).toBe('3 indexers searched. Release names are often shorter than titles.');
  });

  it('suggests the short title, a wider scope and the original title', () => {
    expect(noResultsSuggestions({ q: 'Frieren 29', title: frieren, scope: 'episode', episode: 29, queries: FRIEREN_QUERIES })).toEqual({
      shorter: 'Frieren',
      wider: { scope: 'any', label: 'Search any pack, not just episode 29' },
      original: { q: 'Sousou no Frieren 29', label: 'Use the Japanese title' },
    });
    expect(noResultsSuggestions({ q: 'Frieren S01', title: frieren, scope: 'season', episode: null, queries: [] })).toEqual({
      shorter: 'Frieren',
      wider: { scope: 'any', label: 'Search any release, not just season packs' },
      original: null,
    });
  });

  it('a query that is already the short title gets shorter by a word; a movie has no wider scope', () => {
    const movie = { title: 'One Battle After Another', original_language: 'en' };
    expect(noResultsSuggestions({ q: 'One Battle After Another', title: movie, scope: null, episode: null, queries: [] })).toEqual({
      shorter: 'One Battle After',
      wider: null,
      original: null,
    });
    expect(
      noResultsSuggestions({ q: 'x', title: movie, scope: null, episode: null, queries: [{ q: 'x', label: null }, { q: 'Una batalla', label: 'original title' }] })
        .original
    ).toEqual({ q: 'Una batalla', label: 'Use the original title' });
  });
});

describe('searchProblem', () => {
  it('explains each contract error, with Retry only when it can help', () => {
    expect(searchProblem(new ApiError(503, 'unavailable', ''))).toMatchObject({ title: 'Torrent search is off', retry: false });
    expect(searchProblem(new ApiError(502, 'jackett_unavailable', 'Jackett rejected the API key.'))).toEqual({
      title: 'Jackett isn’t answering',
      body: 'Jackett rejected the API key.',
      retry: true,
    });
    expect(searchProblem(new ApiError(502, 'tmdb_unavailable', ''))).toMatchObject({ title: 'TMDB isn’t answering', retry: true });
    expect(searchProblem(new ApiError(404, 'not_found', ''))).toMatchObject({ retry: false });
    expect(searchProblem(new ApiError(400, 'bad_request', 'Unknown scope.'))).toMatchObject({ body: 'Unknown scope.', retry: false });
    expect(searchProblem(new TypeError('Failed to fetch'))).toEqual({ title: 'Couldn’t search for torrents', body: 'Failed to fetch', retry: true });
  });
});

describe('labels', () => {
  const library = (in_library: boolean) => ({ in_library }) as Parameters<typeof addLabel>[0]['library'];

  it('Add says what it does', () => {
    expect(addLabel({ media_type: 'tv', library: library(false) })).toBe('Add to library & assign');
    expect(addLabel({ media_type: 'movie', library: library(false) })).toBe('Add to library & assign');
    expect(addLabel({ media_type: 'tv', library: library(true) })).toBe('Add & review mapping');
    expect(addLabel({ media_type: 'movie', library: library(true) })).toBe('Add & assign to movie');
  });

  it('headings and the pager', () => {
    expect(resultsHeading(true, 'best')).toBe('Other results · sorted by best');
    expect(resultsHeading(false, 'seeders')).toBe('Results · sorted by seeders');
    expect(pagerText(25, 1043)).toBe('Showing 25 of 1,043');
  });
});
