/** @jest-environment node */
import { server } from '@/mocks/node';
import {
  assignmentApi,
  askStream,
  libraryApi,
  parseSseFrame,
  recApi,
  searchApi,
  settingsApi,
  systemApi,
  titlesApi,
  torrentsApi,
  traktApi,
} from '@/lib/api/endpoints';
import type { AskEvent } from '@/lib/api/contract';

// The endpoint modules call relative /api/* URLs, as the browser does; resolve
// them against a base. Wrap the fetch MSW installs on listen(), not the original.
let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

describe('mock mode answers every PLAN §6 endpoint', () => {
  it.each([
    ['GET /titles/tv/{id}', () => titlesApi.get('tv', 209867)],
    ['GET /titles/movie/{id}', () => titlesApi.get('movie', 666277)],
    ['PATCH /shows/{id}', () => titlesApi.patchShow(1, { include_specials: false })],
    ['GET /shows/{id}/episode-groups', () => titlesApi.episodeGroups(1)],
    ['GET /library', () => libraryApi.list({ type: 'tv' })],
    ['POST /shows/{id}/assign-torrent', () => assignmentApi.assignShow(1, { magnet_uri: 'magnet:?xt=urn:btih:x' })],
    ['POST /movies/{id}/assign-torrent', () => assignmentApi.assignMovie(1, { magnet_uri: 'magnet:?xt=urn:btih:x' })],
    ['GET /assignment-jobs/{id}', () => assignmentApi.job(101)],
    ['PATCH /assignment-jobs/{id}/proposals/{pid}', () => assignmentApi.patchProposal(101, 20, { kind: 'ignore' })],
    ['POST /assignment-jobs/{id}/reclassify', () => assignmentApi.reclassify(101, { scope: 'unresolved', hint: 'absolute numbers' })],
    ['POST /assignment-jobs/{id}/apply', () => assignmentApi.apply(101)],
    ['PUT /episodes/{id}/assignment', () => assignmentApi.setEpisodeAssignment(1029, { info_hash: 'x', file_path: 'a.mkv' })],
    ['PUT /movies/{id}/assignment', () => assignmentApi.setMovieAssignment(1, { info_hash: 'x', file_path: 'a.mkv' })],
    ['GET /torrents/{hash}/files', () => assignmentApi.torrentFiles('abc')],
    ['GET /search/torrents', () => searchApi.torrents({ media_type: 'tv', tmdb_id: 209867, season: 1, episode: 29 })],
    ['POST /search/torrents/inspect', () => searchApi.inspect({ release_id: 'rel-0', media_type: 'tv', tmdb_id: 209867 })],
    ['GET /settings/quality-profile', () => searchApi.qualityProfile()],
    ['GET /integrations/trakt', () => traktApi.integration()],
    ['POST /integrations/trakt/connect', () => traktApi.connect()],
    ['POST /integrations/trakt/sync', () => traktApi.sync()],
    ['DELETE /integrations/trakt', () => traktApi.disconnect()],
    ['GET /me/up-next', () => traktApi.upNext()],
    ['GET /me/watchlist', () => traktApi.watchlist('movie')],
    ['GET /me/history', () => traktApi.history(2)],
    ['GET /recommendations/home', () => recApi.home()],
    ['POST /recommendations/refresh', () => recApi.refresh()],
    ['GET /recommendations/runs/{id}', () => recApi.run(42)],
    ['GET /recommendations/threads', () => recApi.threads()],
    ['GET /recommendations/threads/{id}', () => recApi.thread(7)],
    ['POST /recommendations/feedback', () => recApi.feedback({ media_type: 'tv', tmdb_id: 95396, signal: 'like' })],
    ['GET /settings/ai', () => settingsApi.ai()],
    ['GET /settings/ai/usage', () => settingsApi.aiUsage()],
    ['GET /status', () => systemApi.status()],
    ['GET /torrents/summary', () => torrentsApi.summary()],
  ])('%s', async (_name, call) => {
    await call(); // throws an ApiError on any non-2xx answer
  });

  it('streams Ask as typed events, ending with done', async () => {
    const events: AskEvent[] = [];
    for await (const event of askStream({ message: 'something like Severance but lighter' })) events.push(event);
    expect(events.map((e) => e.event)).toEqual(['status', 'status', 'item', 'text', 'text', 'done']);
    const done = events.at(-1);
    expect(done?.event === 'done' && done.data.refinements).toContain('lighter');
  });
});

describe('parseSseFrame', () => {
  it('reads event and JSON data, ignoring comments', () => {
    expect(parseSseFrame(': keep-alive\nevent: text\ndata: {"delta":"hi"}')).toEqual({ event: 'text', data: { delta: 'hi' } });
  });

  it('skips frames without data or with broken JSON', () => {
    expect(parseSseFrame('event: ping')).toBeNull();
    expect(parseSseFrame('data: {nope')).toBeNull();
  });
});
