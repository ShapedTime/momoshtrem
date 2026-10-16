/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import { libraryApi, systemApi, titlesApi, traktApi } from '@/lib/api/endpoints';
import type { ShowTitle, TraktSyncStep, UpNextAvailability } from '@/lib/api/contract';
import { type MockScenario, setMockScenario } from '@/mocks/scenario';
import { episodeId } from '@/mocks/fixtures/titles';

/** The Trakt mock (#130): a state machine per page, and Up next's availability from the page's library. */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(() => resetMockState());
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const failure = (promise: Promise<unknown>) =>
  promise.then(
    () => null,
    (error: unknown) => (isApiError(error) ? { status: error.status, code: error.code } : error)
  );

const statusTrakt = async () => (await systemApi.status()).trakt;

describe('the integration', () => {
  it('starts connected and synced, and /status agrees', async () => {
    const trakt = await traktApi.integration();
    expect(trakt).toMatchObject({ configured: true, state: 'connected', connected: true, username: 'teymur', pending: null });
    expect(trakt.sync).toMatchObject({ status: 'idle', kind: 'incremental', step: null });
    expect(trakt.counts.history).toBeGreaterThan(100);
    expect(trakt.counts).toMatchObject({ watchlist: 4, watchlist_in_library: 2 });
    expect(trakt.settings).toEqual({ use_ratings: true, show_watchlist: true, include_paused: false });
    expect(Date.parse(trakt.next_sync_at ?? '') - Date.parse(trakt.last_sync_at ?? '')).toBe(30 * 60_000);
    expect(await statusTrakt()).toEqual({ configured: true, connected: true });
    expect(await failure(traktApi.connect())).toEqual({ status: 409, code: 'conflict' });
  });

  it('connect → pending → connected on the second read, then the initial import one step per read', async () => {
    setMockScenario('trakt-off');
    expect((await traktApi.upNext()).items).toEqual([]);
    expect(await failure(traktApi.sync())).toEqual({ status: 409, code: 'conflict' });

    const code = await traktApi.connect();
    expect(code).toMatchObject({ user_code: 'A1B2C3D4', verification_url: 'https://trakt.tv/activate', interval: 5 });
    expect(await traktApi.connect()).toEqual(code); // the live code again

    expect(await traktApi.integration()).toMatchObject({ state: 'pending', pending: code, connected: false });
    const connected = await traktApi.integration();
    expect(connected).toMatchObject({ state: 'connected', username: 'teymur', pending: null });
    expect(connected.sync).toMatchObject({ status: 'syncing', kind: 'initial', step: 'history', finished_at: null });
    expect((await traktApi.upNext()).items).toEqual([]);

    const steps: Array<TraktSyncStep | null> = [];
    for (let i = 0; i < 5; i++) steps.push((await traktApi.integration()).sync.step);
    expect(steps).toEqual(['ratings', 'watchlist', 'progress', 'details', null]);
    const done = await traktApi.integration();
    expect(done.sync).toMatchObject({ status: 'idle', kind: 'initial' });
    expect(done.sync.progress.plays).toBe(done.counts.history);
    expect(done.last_sync_at).not.toBeNull();
    expect((await traktApi.upNext()).items.length).toBeGreaterThan(0);
    expect(await statusTrakt()).toEqual({ configured: true, connected: true });
  });

  it('a manual sync runs for one read, and a second one joins it', async () => {
    const started = await traktApi.sync();
    expect(started.sync).toMatchObject({ status: 'syncing', kind: 'incremental' });
    expect((await traktApi.sync()).sync.started_at).toBe(started.sync.started_at);
    expect((await traktApi.integration()).sync.status).toBe('idle');
  });

  it('settings are partial and survive a disconnect, which purges /me', async () => {
    expect(await traktApi.updateSettings({ include_paused: true })).toEqual({ use_ratings: true, show_watchlist: true, include_paused: true });
    expect(await traktApi.updateSettings({ show_watchlist: false })).toMatchObject({ show_watchlist: false, include_paused: true });
    expect((await traktApi.upNext()).items.some((item) => item.source === 'watchlist')).toBe(false);

    await traktApi.disconnect();
    const off = await traktApi.integration();
    expect(off).toMatchObject({ state: 'off', connected: false, username: null, last_sync_at: null, next_sync_at: null });
    expect(off.counts).toEqual({ history: 0, ratings: 0, watchlist: 0, watchlist_in_library: 0 });
    expect(off.settings).toEqual({ use_ratings: true, show_watchlist: false, include_paused: true });
    expect((await traktApi.upNext()).items).toEqual([]);
    expect((await traktApi.watchlist()).items).toEqual([]);
    expect((await traktApi.history()).total).toBe(0);
    expect(await statusTrakt()).toEqual({ configured: true, connected: false });
  });

  it('a body that isn’t booleans is a 400', async () => {
    const response = await fetch('/api/integrations/trakt/settings', { method: 'PUT', body: JSON.stringify({ use_ratings: 'yes' }) });
    expect(response.status).toBe(400);
  });

  it('cancelling a pending code goes back to where it was', async () => {
    setMockScenario('trakt-reauth');
    await traktApi.connect();
    expect((await traktApi.integration()).state).toBe('pending');
    await traktApi.disconnect();
    const back = await traktApi.integration();
    expect(back).toMatchObject({ state: 'reauth', username: 'teymur' });
    expect(back.reauth_reason).toMatch(/Connect again/);
    expect((await traktApi.upNext()).items.length).toBeGreaterThan(0); // the imported data stays
  });
});

describe('scenarios', () => {
  it.each<[MockScenario, Record<string, unknown>]>([
    ['trakt-off', { configured: true, state: 'off', connected: false }],
    ['trakt-unconfigured', { configured: false, state: 'off' }],
    ['trakt-reauth', { state: 'reauth', connected: false, username: 'teymur' }],
    ['first-time', { state: 'off' }],
  ])('%s', async (name, want) => {
    setMockScenario(name);
    expect(await traktApi.integration()).toMatchObject(want);
  });

  it('trakt-unconfigured: connect answers 503', async () => {
    setMockScenario('trakt-unconfigured');
    expect(await failure(traktApi.connect())).toEqual({ status: 503, code: 'unavailable' });
    expect(await statusTrakt()).toEqual({ configured: false, connected: false });
  });

  it('trakt-denied: the code is denied on the second read', async () => {
    setMockScenario('trakt-denied');
    await traktApi.connect();
    await traktApi.integration();
    expect(await traktApi.integration()).toMatchObject({ state: 'off', connect_error: 'denied', pending: null });
    await traktApi.connect();
    expect((await traktApi.integration()).connect_error).toBeNull();
  });

  it('trakt-pending: the code is never approved, until Cancel', async () => {
    setMockScenario('trakt-pending');
    expect(await traktApi.integration()).toMatchObject({ state: 'off', username: null });
    const code = await traktApi.connect();
    for (let i = 0; i < 4; i++) expect(await traktApi.integration()).toMatchObject({ state: 'pending', pending: code, connect_error: null });
    await traktApi.disconnect();
    expect(await traktApi.integration()).toMatchObject({ state: 'off', pending: null });
  });

  it('trakt-syncing holds the first import at details, with Up next in', async () => {
    setMockScenario('trakt-syncing');
    for (let i = 0; i < 3; i++) expect((await traktApi.integration()).sync).toMatchObject({ status: 'syncing', kind: 'initial', step: 'details' });
    expect((await traktApi.upNext()).items.length).toBeGreaterThan(0);
  });

  it('trakt-sync-error: history and ratings are in, and Sync now picks up at the watchlist', async () => {
    setMockScenario('trakt-sync-error');
    const failed = await traktApi.integration();
    expect(failed.sync).toMatchObject({ status: 'error', kind: 'initial', step: 'watchlist' });
    expect(failed.sync.error).toMatch(/watchlist/);
    expect(failed.counts).toMatchObject({ watchlist: 0, ratings: failed.sync.progress.ratings });
    expect((await traktApi.upNext()).items).toEqual([]);
    expect((await traktApi.sync()).sync).toMatchObject({ status: 'syncing', kind: 'initial', step: 'watchlist' });
    for (let i = 0; i < 3; i++) await traktApi.integration();
    expect((await traktApi.integration()).sync.status).toBe('idle');
    expect((await traktApi.watchlist()).items).toHaveLength(4);
  });

  it('first-time: an empty library too', async () => {
    setMockScenario('first-time');
    expect((await libraryApi.list()).summary.total).toBe(0);
  });

  it('up-next-empty and me-error', async () => {
    setMockScenario('up-next-empty');
    expect((await traktApi.upNext()).items).toEqual([]);
    expect((await traktApi.watchlist()).items).toHaveLength(4);
    setMockScenario('me-error');
    for (const call of [traktApi.upNext, () => traktApi.newEpisodes(7), traktApi.watchlist, traktApi.history]) {
      expect(await failure(call())).toEqual({ status: 500, code: 'internal_error' });
    }
  });
});

describe('/me', () => {
  it('Up next covers every availability, in order: by last watched, then watchlist starts, then what airs next', async () => {
    const { items } = await traktApi.upNext();
    expect(items.map((item) => [item.show.title, `S${item.next_episode.season}E${item.next_episode.episode}`, item.availability, item.source])).toEqual([
      ['The Studio', 'S1E10', 'downloading', 'progress'],
      ['Frieren: Beyond Journey’s End', 'S1E27', 'ready', 'progress'],
      ['One Piece', 'S21E1', 'needs_review', 'progress'],
      ['The Good Place', 'S1E6', 'not_in_library', 'progress'],
      ['Doctor Who', 'S3E4', 'needs_torrent', 'progress'],
      ['Bluey', 'S1E1', 'ready', 'watchlist'],
      ['Severance', 'S2E10', 'airs_on', 'progress'],
    ]);
    const by = (availability: UpNextAvailability) => items.find((item) => item.availability === availability)!;
    expect(by('downloading').download).toEqual({ pct: 37, info_hash: 'seed-5' });
    expect(by('ready').webdav_path).toMatch(/Season 01\/Frieren.* - S01E27 - An Ordinary Mage\.mkv$/);
    expect(by('ready').webdav_url).toMatch(/^https:\/\/momoshtrem\.local:36911\/TV%20Shows\//);
    expect(by('needs_review')).toMatchObject({ review_job_id: 102, next_episode: { absolute_number: 892 }, show: { library_id: 2 } });
    expect(by('not_in_library')).toMatchObject({ show: { library_id: null }, webdav_path: null });
    expect(by('airs_on').airs_at).toMatch(/^2026-10-21T00:00:00/);
    expect(items.filter((item) => item.availability !== 'airs_on').every((item) => item.airs_at === null)).toBe(true);
    expect(items.find((item) => item.source === 'watchlist')).toMatchObject({ last_watched_at: null, progress: { completed: 0, aired: 153 } });
  });

  it('adding a show flips its card from Add to Find', async () => {
    await titlesApi.addShow(66573);
    const card = (await traktApi.upNext()).items.find((item) => item.show.tmdb_id === 66573);
    expect(card).toMatchObject({ availability: 'needs_torrent', show: { library_id: expect.any(Number) } });
  });

  it('unassigning the next episode makes it need a torrent', async () => {
    await titlesApi.unassignEpisode(episodeId(209867, 1, 27));
    const card = (await traktApi.upNext()).items.find((item) => item.show.tmdb_id === 209867);
    expect(card).toMatchObject({ availability: 'needs_torrent', webdav_path: null });
  });

  it('new episodes: aired in the window and unwatched; without Trakt the library’s still count', async () => {
    const week = await traktApi.newEpisodes(7);
    expect(week.items.map((item) => [item.show.title, item.episode.absolute_number, item.availability, item.aired_at.slice(0, 10)])).toEqual([
      ['One Piece', 1121, 'ready', '2026-10-11'],
    ]);
    expect((await traktApi.newEpisodes(30)).items.map((item) => item.episode.absolute_number)).toEqual([1121, 1120, 1119, 1118]);
    const days = await fetch('/api/me/new-episodes?days=31');
    expect(days.status).toBe(400);

    setMockScenario('trakt-off');
    expect((await traktApi.newEpisodes(7)).items.map((item) => [item.show.title, item.episode.episode])).toEqual([
      ['One Piece', 36],
      ['Severance', 9],
    ]);
  });

  it('the watchlist, by type, with library state', async () => {
    const { items } = await traktApi.watchlist();
    expect(items.map((item) => [item.title, item.library?.availability_stage ?? null])).toEqual([
      ['Past Lives', null],
      ['One Battle After Another', 'needs_torrent'],
      ['Bluey', 'ready'],
      ['The Good Place', null],
    ]);
    expect((await traktApi.watchlist('movie')).items).toHaveLength(2);
    expect((await fetch('/api/me/watchlist?type=show')).status).toBe(400);
  });

  it('history pages, newest first', async () => {
    const first = await traktApi.history();
    expect(first).toMatchObject({ page: 1, page_size: 50 });
    expect(first.items[0]).toMatchObject({ media_type: 'tv', title: 'The Studio', season: 1, episode: 9, episode_name: 'CinemaCon' });
    const second = await traktApi.history(2, 20);
    expect(second.items).toHaveLength(20);
    expect(second.items[0].watched_at <= first.items[19].watched_at).toBe(true);
    const all = [...(await traktApi.history(1, 200)).items, ...(await traktApi.history(2, 200)).items];
    expect(all).toHaveLength(first.total);
    expect(all.every((item, i) => i === 0 || item.watched_at <= all[i - 1].watched_at)).toBe(true);
    expect(new Set(all.map((item) => item.id)).size).toBe(all.length);
    expect(all.filter((item) => item.media_type === 'movie').map((item) => [item.title, item.season])).toEqual([
      ['Dune: Part Two', null],
      ['Interstellar', null],
      ['Interstellar', null],
      ['Interstellar', null],
    ]);
    expect((await fetch('/api/me/history?page_size=500')).status).toBe(400);
  });

  it('shows carry watch_state.next_episode, Frieren 1–26 watched', async () => {
    const frieren = (await titlesApi.get('tv', 209867)) as ShowTitle;
    expect(frieren.watch_state?.next_episode).toEqual({ season: 1, episode: 27, absolute_number: 27, name: 'An Ordinary Mage', air_date: '2024-03-29' });
    const season = await titlesApi.season(209867, 1);
    expect(season.episodes.filter((e) => e.state === 'watched').map((e) => e.episode_number)).toEqual(Array.from({ length: 26 }, (_, i) => i + 1));
    expect((await titlesApi.get('tv', 82728)).watch_state).toBeNull(); // Bluey: not started
  });
});
