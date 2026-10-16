/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import {
  assignmentApi,
  downloadsApi,
  hdrezkaApi,
  isLimitBelowCommittedError,
  isNotDownloadableError,
  isStorageFullError,
  settingsApi,
  titlesApi,
  torrentsApi,
} from '@/lib/api/endpoints';
import type { Download, DownloadList, EpisodeSummary } from '@/lib/api/contract';
import { setMockScenario } from '@/mocks/scenario';
import { DOWNLOAD_HASHES, mockDownloadRows, setMockDownloadsClock } from '@/mocks/fixtures/downloads';
import { episodeId } from '@/mocks/fixtures/titles';
import { FULL_KEPT_MAX_BYTES } from '@/mocks/fixtures/settings';
import { TORRENTS } from '@/mocks/fixtures/torrents';

/** The downloads and storage mocks (#77): stateful, and titles, Activity and Settings › Storage follow them. */

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

const DUNE_TORRENT = '4f1a2b3c4d5e6f708192a3b4c5d6e7f8091abe90';
const BATTLE = 'https://rezka.ag/films/action/78655-bitva-za-bitvoy-2025.html';

const failure = (promise: Promise<unknown>) => promise.then(() => undefined, (error: unknown) => error);
const byTitle = (list: DownloadList, title: string) => list.items.filter((d) => d.item.title === title);
const only = (list: DownloadList, title: string): Download => {
  const found = byTitle(list, title);
  if (found.length !== 1) throw new Error(`${found.length} downloads of ${title}`);
  return found[0];
};
async function episode(tmdbId: number, season: number, n: number): Promise<EpisodeSummary> {
  const found = (await titlesApi.season(tmdbId, season)).episodes.find((e) => e.episode_number === n);
  if (!found) throw new Error(`No S${season}E${n}`);
  return found;
}
const torrent = async (hash: string) => (await torrentsApi.list()).items.find((t) => t.info_hash === hash);

describe('GET /downloads', () => {
  it('lists kept downloads newest first, then smart picks, with the usage the caps are read against', async () => {
    const list = await downloadsApi.list();
    expect(list.items.map((d) => [d.id, d.reason, d.scope, d.state])).toEqual([
      [5, 'kept', 'movie', 'downloading'],
      [4, 'kept', 'movie', 'no_file'],
      [3, 'kept', 'episode', 'stalled'],
      [2, 'kept', 'episode', 'paused'],
      [1, 'kept', 'season', 'done'],
      [6, 'smart', 'episode', 'done'],
      [7, 'smart', 'episode', 'done'],
    ]);
    expect(only(list, 'Interstellar')).toMatchObject({ wanted_bytes: 63_100_000_000, done_bytes: 26_500_000_000, download_bps: 2_300_000, eta_seconds: 15_913 });
    expect(list.items.find((d) => d.reason === 'smart')).toMatchObject({ smart_source: 'up_next', item: { title: 'Frieren: Beyond Journey’s End', season_number: 1, episode_number: 27 } });
    expect(list.usage).toEqual({
      kept_bytes: 48_768_000_000,
      smart_bytes: 1_920_000_000,
      committed_kept_bytes: 86_400_000_000,
      kept_max_bytes: 375_000_000_000,
      smart_max_bytes: 112_500_000_000,
    });
    expect(list).toMatchObject({ paused_reason: null, resumes_at: null });
  });

  it('puts the keeps on titles, seasons and episodes, and only wanted files download', async () => {
    const interstellar = (await titlesApi.get('movie', 157336)).library;
    expect(interstellar).toMatchObject({ availability_stage: 'downloading', keep: { download_id: 5, reason: 'kept', scope: 'movie', state: 'downloading', pct: 42, until_watched: true } });
    expect((await titlesApi.get('movie', 693134)).library).toMatchObject({ availability_stage: 'ready', keep: null });

    const bluey = await titlesApi.season(82728, 3);
    expect(bluey.keep).toMatchObject({ download_id: 1, scope: 'season', state: 'done', pct: 100, until_watched: false, expires_at: null });
    expect(bluey.episodes[0]).toMatchObject({ state: 'ready', keep: { download_id: 1, state: 'done', pct: 100 } });
    expect((await titlesApi.season(82728, 1)).keep).toBeNull();
    expect((await titlesApi.get('tv', 82728)).library.keep).toBeNull(); // the season is kept, not the show

    expect(await episode(247767, 1, 10)).toMatchObject({ state: 'downloading', pct: 37, keep: { download_id: 3, scope: 'episode', state: 'stalled', pct: 37 } });
    // A paused torrent never downloads; a watched episode keeps its keep
    expect(await episode(37854, 22, 15)).toMatchObject({ state: 'watched', keep: { download_id: 2, state: 'paused', pct: 31 } });
    expect(await episode(209867, 1, 27)).toMatchObject({ state: 'ready', keep: { download_id: 6, reason: 'smart', state: 'done' } });
    expect((await episode(209867, 1, 29)).keep).toBeNull();
  });

  it('joins the downloads into Activity: wanted bytes, holds and the disk', async () => {
    expect(await torrent(DOWNLOAD_HASHES.interstellar)).toMatchObject({
      state: 'downloading',
      wanted: { bytes: 63_100_000_000, done_bytes: 26_500_000_000, eta_seconds: 15_913, state: 'downloading', resumes_at: null },
      holds: [{ download_id: 5, reason: 'kept', scope: 'movie', title: 'Interstellar' }],
    });
    expect(await torrent(DOWNLOAD_HASHES.bluey)).toMatchObject({ state: 'idle', wanted: { bytes: 21_700_000_000, done_bytes: 21_700_000_000, state: 'done' } });
    expect((await torrent(DOWNLOAD_HASHES.onePiece))?.wanted).toMatchObject({ bytes: 400_000_000, state: 'paused', eta_seconds: null });
    expect(await torrent(DUNE_TORRENT)).toMatchObject({ wanted: null, holds: [] });
    // The static fixtures carry the same, for tests that read them directly
    expect(TORRENTS.find((t) => t.info_hash === DOWNLOAD_HASHES.interstellar)?.holds).toEqual([{ download_id: 5, reason: 'kept', scope: 'movie', title: 'Interstellar' }]);
    expect((await torrentsApi.detail(DOWNLOAD_HASHES.bluey)).holds).toHaveLength(1);

    expect((await torrentsApi.summary()).disk).toEqual({
      total_bytes: 1_000_000_000_000,
      free_bytes: 812_000_000_000,
      torrents_bytes: 62_400_000_000,
      cache_limit_bytes: 750_000_000_000,
      kept_bytes: 48_768_000_000,
      smart_bytes: 1_920_000_000,
    });
  });
});

describe('POST /downloads', () => {
  it('keeps a movie: 201 queued, with the storage defaults, and the title downloads; again it is 200 unchanged', async () => {
    const before = Date.now();
    const kept = await downloadsApi.create({ scope: 'movie', id: 1 });
    expect(kept).toMatchObject({
      id: 8,
      reason: 'kept',
      scope: 'movie',
      item: { media_type: 'movie', tmdb_id: 693134, library_id: 1, title: 'Dune: Part Two' },
      state: 'queued',
      wanted_bytes: 18_400_000_000,
      done_bytes: 0,
      files: { total: 1, done: 0, unavailable: 0 },
      until_watched: true,
    });
    // Until watched, 30 days at most
    expect(Date.parse(kept.expires_at ?? '') - before).toBeGreaterThanOrEqual(30 * 86_400_000 - 1000);

    expect((await titlesApi.get('movie', 693134)).library).toMatchObject({
      availability_stage: 'downloading',
      download: { pct: 0, eta_seconds: null },
      keep: { download_id: 8, state: 'queued', pct: 0 },
    });
    expect((await torrent(DUNE_TORRENT))?.holds).toEqual([{ download_id: 8, reason: 'kept', scope: 'movie', title: 'Dune: Part Two' }]);
    expect((await downloadsApi.list()).usage.committed_kept_bytes).toBe(86_400_000_000 + 18_400_000_000);

    const again = await downloadsApi.create({ scope: 'movie', id: 1, until_watched: false, expires_at: null });
    expect(again).toMatchObject({ id: 8, until_watched: true });
    expect(byTitle(await downloadsApi.list(), 'Dune: Part Two')).toHaveLength(1);
  });

  it('takes the duration sent, and a whole season on disk is done at once', async () => {
    const season = await downloadsApi.create({ scope: 'season', id: 5, season_number: 3, until_watched: false, expires_at: null });
    expect(season).toMatchObject({ until_watched: false, expires_at: null, state: 'done' }); // already kept as season 3: unchanged
    const show = await downloadsApi.create({ scope: 'show', id: 5, until_watched: false, expires_at: null });
    expect(show).toMatchObject({
      scope: 'show',
      state: 'done',
      wanted_bytes: 21_700_000_000,
      files: { total: 153, unavailable: 0 },
      until_watched: false,
      expires_at: null,
    });
    expect((await titlesApi.get('tv', 82728)).library.keep).toMatchObject({ scope: 'show', state: 'done' });
    // A file another kept download has commits nothing more
    expect((await downloadsApi.list()).usage.committed_kept_bytes).toBe(86_400_000_000);
  });

  it('turns a smart pick kept: 200, the same row, created anew with the duration asked for', async () => {
    const smart = (await downloadsApi.list()).items.find((d) => d.id === 6);
    expect(smart).toMatchObject({ reason: 'smart', until_watched: true });
    expect(Date.parse(smart?.expires_at ?? '') - Date.parse(smart?.created_at ?? '')).toBe(14 * 86_400_000); // a smart pick: 14 days at most

    const before = Date.now();
    const turned = await downloadsApi.create({ scope: 'episode', id: episodeId(209867, 1, 27), until_watched: false, expires_at: null });
    expect(turned).toMatchObject({ id: 6, reason: 'kept', smart_source: null, until_watched: false, expires_at: null, state: 'done' });
    expect(Date.parse(turned.created_at)).toBeGreaterThanOrEqual(before - 1000);
    expect((await episode(209867, 1, 27)).keep).toMatchObject({ download_id: 6, reason: 'kept' });
  });

  it('keeps an episode or a show with nothing assigned yet as no_file', async () => {
    expect(await downloadsApi.create({ scope: 'episode', id: episodeId(209867, 1, 30) })).toMatchObject({ state: 'no_file', wanted_bytes: 0, files: { total: 0 } });
    expect((await episode(209867, 1, 30)).keep).toMatchObject({ state: 'no_file' });
  });

  it('answers 400, 422 naming the field, and 404 for what the library hasn’t got', async () => {
    const res = await fetch('/api/downloads', { method: 'POST', body: '[1]', headers: { 'Content-Type': 'application/json' } });
    expect(res.status).toBe(400);
    const cases: Array<[Record<string, unknown>, string]> = [
      [{ scope: 'trailer', id: 1 }, 'scope'],
      [{ scope: 'movie', id: 0 }, 'id'],
      [{ scope: 'season', id: 5 }, 'season_number'],
      [{ scope: 'movie', id: 1, until_watched: 'yes' }, 'until_watched'],
      [{ scope: 'movie', id: 1, expires_at: '2020-01-01T00:00:00Z' }, 'expires_at'],
      [{ scope: 'movie', id: 1, expires_at: 'soon' }, 'expires_at'],
    ];
    for (const [body, field] of cases) {
      const error = await failure(downloadsApi.create(body as never));
      expect(isApiError(error) && [error.status, error.code, error.field]).toEqual([422, 'validation_error', field]);
    }
    await expect(downloadsApi.create({ scope: 'movie', id: 99 })).rejects.toMatchObject({ status: 404 });
    await expect(downloadsApi.create({ scope: 'season', id: 5, season_number: 9 })).rejects.toMatchObject({ status: 404 });
    await expect(downloadsApi.create({ scope: 'episode', id: 123 })).rejects.toMatchObject({ status: 404 });
  });

  it('answers 422 not_downloadable for what plays from HDRezka, and the movie’s keep becomes unavailable', async () => {
    await hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 111, quality: '1080p' });
    expect(only(await downloadsApi.list(), 'One Battle After Another')).toMatchObject({ state: 'unavailable', files: { total: 1, unavailable: 1 } });
    const error = await failure(downloadsApi.create({ scope: 'movie', id: 3 }));
    expect(isNotDownloadableError(error)).toBe(true);
    expect(isStorageFullError(error)).toBe(false);
  });

  it('downloads-full: a Keep past the kept cap is 409 storage_full with what it needs; one inside a kept season fits', async () => {
    setMockScenario('downloads-full');
    const error = await failure(downloadsApi.create({ scope: 'movie', id: 1 }));
    expect(isStorageFullError(error)).toBe(true);
    if (!isStorageFullError(error)) return;
    expect(error.details).toMatchObject({ needed_bytes: 18_400_000_000, available_bytes: 1_200_000_000 });
    expect((await settingsApi.storage()).kept).toMatchObject({ max_bytes: FULL_KEPT_MAX_BYTES, effective_max_bytes: FULL_KEPT_MAX_BYTES });

    expect(await downloadsApi.create({ scope: 'episode', id: episodeId(82728, 3, 1) })).toMatchObject({ state: 'done' });
  });
});

describe('GET /downloads/estimate', () => {
  it('says what a Keep would need against the space left, and writes nothing', async () => {
    expect(await downloadsApi.estimate({ scope: 'movie', id: 1 })).toEqual({
      wanted_bytes: 18_400_000_000,
      needed_bytes: 18_400_000_000,
      available_bytes: 375_000_000_000 - 86_400_000_000,
      downloadable: true,
    });
    expect(byTitle(await downloadsApi.list(), 'Dune: Part Two')).toEqual([]);
    // Already kept, or inside a kept season: nothing more
    expect(await downloadsApi.estimate({ scope: 'season', id: 5, season_number: 3 })).toMatchObject({ needed_bytes: 0 });
    expect(await downloadsApi.estimate({ scope: 'episode', id: episodeId(82728, 3, 2) })).toMatchObject({ needed_bytes: 0, wanted_bytes: 280_000_000 });

    setMockScenario('downloads-full');
    expect(await downloadsApi.estimate({ scope: 'movie', id: 1 })).toMatchObject({ needed_bytes: 18_400_000_000, available_bytes: 1_200_000_000 });
  });

  it('says an HDRezka item can’t be kept, and answers 400 and 404', async () => {
    await hdrezkaApi.assignMovie(3, { url: BATTLE, translator_id: 111, quality: '1080p' });
    expect((await downloadsApi.estimate({ scope: 'movie', id: 3 })).downloadable).toBe(false);
    const res = await fetch('/api/downloads/estimate?scope=trailer&id=1');
    expect(res.status).toBe(400);
    await expect(downloadsApi.estimate({ scope: 'show', id: 99 })).rejects.toMatchObject({ status: 404 });
  });
});

describe('PATCH and DELETE /downloads/{id}', () => {
  it('changes a kept download’s duration, and refuses a smart pick or a bad value', async () => {
    expect(await downloadsApi.update(5, { until_watched: false, expires_at: null })).toMatchObject({ id: 5, until_watched: false, expires_at: null });
    expect((await titlesApi.get('movie', 157336)).library.keep).toMatchObject({ until_watched: false, expires_at: null });

    await expect(downloadsApi.update(6, { until_watched: false })).rejects.toMatchObject({ status: 422, code: 'validation_error' });
    await expect(downloadsApi.update(5, { expires_at: 'tomorrow' })).rejects.toMatchObject({ status: 422, field: 'expires_at' });
    await expect(downloadsApi.update(99, { until_watched: true })).rejects.toMatchObject({ status: 404 });
    const res = await fetch('/api/downloads/5', { method: 'PATCH', body: '"later"', headers: { 'Content-Type': 'application/json' } });
    expect(res.status).toBe(400);
  });

  it('stops keeping: a soft release; the file streams on demand again, and nothing holds the torrent', async () => {
    await downloadsApi.remove(5);
    expect(byTitle(await downloadsApi.list(), 'Interstellar')).toEqual([]);
    expect(mockDownloadRows().find((d) => d.download.id === 5)).toMatchObject({ release_reason: 'deleted', released_at: expect.any(String) });
    expect((await titlesApi.get('movie', 157336)).library).toMatchObject({ availability_stage: 'ready', download: null, keep: null });
    expect(await torrent(DOWNLOAD_HASHES.interstellar)).toMatchObject({ wanted: null, holds: [] });
    expect((await downloadsApi.list()).usage.committed_kept_bytes).toBe(86_400_000_000 - 63_100_000_000);
    await expect(downloadsApi.remove(5)).rejects.toMatchObject({ status: 404 });
    await expect(downloadsApi.update(5, { until_watched: true })).rejects.toMatchObject({ status: 404 });
    // Only active rows are unique per target: keeping it again is a new download
    expect(await downloadsApi.create({ scope: 'movie', id: 2 })).toMatchObject({ id: 8, state: 'queued' });
  });
});

describe('what other mocks change', () => {
  it('pausing a torrent pauses its downloads, and resuming brings them back', async () => {
    await torrentsApi.pause(DOWNLOAD_HASHES.interstellar);
    expect(only(await downloadsApi.list(), 'Interstellar').state).toBe('paused');
    expect((await titlesApi.get('movie', 157336)).library).toMatchObject({ availability_stage: 'ready', keep: { state: 'paused' } });
    await torrentsApi.resume(DOWNLOAD_HASHES.interstellar);
    expect(only(await downloadsApi.list(), 'Interstellar').state).toBe('downloading');

    await torrentsApi.resume(DOWNLOAD_HASHES.onePiece); // paused from the start
    expect(only(await downloadsApi.list(), 'One Piece').state).toBe('downloading');
  });

  it('removing a torrent leaves its downloads with no file; removing a title takes its downloads', async () => {
    await torrentsApi.remove(DOWNLOAD_HASHES.bluey);
    expect(only(await downloadsApi.list(), 'Bluey')).toMatchObject({ state: 'no_file', wanted_bytes: 0 });
    await titlesApi.removeMovie(2);
    expect(byTitle(await downloadsApi.list(), 'Interstellar')).toEqual([]);
  });

  it('unassigning a kept movie or episode leaves it with no file', async () => {
    await titlesApi.unassignMovie(2);
    expect(only(await downloadsApi.list(), 'Interstellar')).toMatchObject({ state: 'no_file', wanted_bytes: 0, done_bytes: 0 });
    await titlesApi.unassignEpisode(episodeId(247767, 1, 10));
    expect(only(await downloadsApi.list(), 'The Studio')).toMatchObject({ state: 'no_file', wanted_bytes: 0, files: { total: 0 } });
    expect(await episode(247767, 1, 10)).toMatchObject({ pct: null, keep: { state: 'no_file' } });
  });

  it('episodes a mapping job assigns to a kept season past the kept cap make it storage_full', async () => {
    const season = await downloadsApi.create({ scope: 'season', id: 1, season_number: 1 });
    expect(season).toMatchObject({ state: 'queued', wanted_bytes: 28 * 24 * 40_000_000, files: { total: 28, over_cap: 0 } });
    // Leave the kept cap no room past what's committed now (not a lowering below it: no 409)
    const { committed_kept_bytes } = await settingsApi.storage();
    await settingsApi.putStorage({ kept: { max_bytes: committed_kept_bytes } });

    // Job 101 maps Frieren's #29–#38
    const { proposals } = await assignmentApi.job(101);
    const pending = proposals.filter((p) => p.decision === 'pending' && p.targets.length > 0);
    await assignmentApi.patchProposals(101, { items: pending.map((p) => ({ id: p.id, decision: 'accepted' as const })) });
    await assignmentApi.apply(101);

    const full = (await downloadsApi.list()).items.find((d) => d.id === season.id);
    expect(full).toMatchObject({ state: 'storage_full', wanted_bytes: season.wanted_bytes });
    expect(full?.files.over_cap).toBeGreaterThan(0);
    expect((await downloadsApi.list()).usage.committed_kept_bytes).toBe(committed_kept_bytes); // over the cap commits nothing
    // Over the cap: not pinned, not fetched
    expect(await episode(209867, 1, 29)).toMatchObject({ state: 'ready', pct: null, keep: { download_id: season.id, state: 'storage_full', pct: 0 } });
    expect(await episode(209867, 1, 1)).toMatchObject({ keep: { state: 'queued' } });
  });

  it('downloads-empty: a fresh install, with nothing kept and nothing downloading', async () => {
    setMockScenario('downloads-empty');
    expect(await downloadsApi.list()).toMatchObject({ items: [], usage: { kept_bytes: 0, smart_bytes: 0, committed_kept_bytes: 0 } });
    expect((await titlesApi.get('movie', 157336)).library).toMatchObject({ availability_stage: 'ready', download: null, keep: null });
    expect(await episode(247767, 1, 10)).toMatchObject({ state: 'ready', pct: null, keep: null });
    expect((await torrentsApi.list()).items.every((t) => t.wanted === null && t.holds.length === 0)).toBe(true);
    expect((await settingsApi.storage()).smart.enabled).toBe(false);
    // The defaults pause background downloads while something streams, and Severance does
    expect(await downloadsApi.create({ scope: 'movie', id: 2 })).toMatchObject({ id: 8, state: 'waiting_for_stream' });
    expect((await downloadsApi.list()).paused_reason).toBe('streaming');
  });
});

describe('GET/PUT /settings/storage', () => {
  it('reads the auto limit, the caps and what downloads hold', async () => {
    expect(await settingsApi.storage()).toMatchObject({
      limit_bytes: null,
      source: 'auto',
      auto_bytes: 750_000_000_000,
      config_bytes: null,
      limit_effective_bytes: 750_000_000_000,
      effective_bytes: 750_000_000_000,
      floored: false,
      floor_free_bytes: 5 * 1024 ** 3,
      min_bytes: 4 * 1024 ** 3,
      max_bytes: 900_000_000_000,
      reserve_bytes: 20 * 1024 ** 3,
      usage: { kept_bytes: 48_768_000_000, smart_bytes: 1_920_000_000, cache_bytes: 62_400_000_000 - 48_768_000_000 - 1_920_000_000 },
      committed_kept_bytes: 86_400_000_000,
      kept: { max_bytes: null, effective_max_bytes: 375_000_000_000, default_until_watched: true, default_days: 30 },
      smart: { enabled: true, episodes_ahead: 2, max_bytes: null, effective_max_bytes: 112_500_000_000 },
      background: { enabled: true, pause_while_streaming: false, windows: [] },
    });
  });

  it('saves partially: nested objects merge, and a smaller limit shrinks smart first', async () => {
    const saved = await settingsApi.putStorage({ kept: { default_days: 7 }, smart: { episodes_ahead: 3 } });
    expect(saved).toMatchObject({ kept: { default_days: 7, default_until_watched: true }, smart: { episodes_ahead: 3, enabled: true } });
    expect(await settingsApi.storage()).toMatchObject({ kept: { default_days: 7 }, smart: { episodes_ahead: 3 } });

    const custom = await settingsApi.putStorage({ limit_bytes: 200_000_000_000, kept: { max_bytes: 150_000_000_000 } });
    expect(custom).toMatchObject({
      source: 'setting',
      limit_effective_bytes: 200_000_000_000,
      kept: { effective_max_bytes: 150_000_000_000 },
      // 200 GB − the 20 GiB reserve − 150 GB kept
      smart: { effective_max_bytes: 200_000_000_000 - 20 * 1024 ** 3 - 150_000_000_000 },
    });

    expect((await settingsApi.putStorage({ smart: { enabled: false } })).usage.smart_bytes).toBe(0);
    expect((await downloadsApi.list()).items.some((d) => d.reason === 'smart')).toBe(false);
  });

  it('answers 422 naming a field sent: types first, then the ranges that depend on the disk', async () => {
    await settingsApi.putStorage({ kept: { max_bytes: 300_000_000_000 } });
    const cases: Array<[Record<string, unknown>, string]> = [
      // The first pass: types and allowed values, whatever the disk says
      [{ limit_bytes: -1 }, 'limit_bytes'],
      [{ kept: 'half' }, 'kept'],
      [{ kept: { default_until_watched: 1 } }, 'kept.default_until_watched'],
      [{ kept: { default_days: 14 } }, 'kept.default_days'],
      [{ smart: { episodes_ahead: 6 } }, 'smart.episodes_ahead'],
      [{ smart: { max_bytes: -1 } }, 'smart.max_bytes'],
      [{ background: { pause_while_streaming: 'on' } }, 'background.pause_while_streaming'],
      [{ background: { windows: null } }, 'background.windows'],
      [{ background: { windows: [{ start: '25:00', end: '06:00' }] } }, 'background.windows'],
      [{ background: { windows: [{ start: '01:00', end: '01:00' }] } }, 'background.windows'],
      [{ background: { windows: Array.from({ length: 5 }, () => ({ start: '01:00', end: '02:00' })) } }, 'background.windows'],
      // A range is checked only once every type passed
      [{ limit_bytes: 1, kept: { default_days: 14 } }, 'kept.default_days'],
      // The second pass, on the stored settings with the patch applied
      [{ limit_bytes: 1_000 }, 'limit_bytes'],
      [{ limit_bytes: 2_000_000_000_000 }, 'limit_bytes'],
      [{ kept: { max_bytes: 740_000_000_000 } }, 'kept.max_bytes'],
      // An explicit smart cap is at most the limit less the reserve and the kept cap: 422, never shrunk
      [{ smart: { max_bytes: 500_000_000_000 } }, 'smart.max_bytes'],
      // The stored kept cap (300 GB) no longer fits a smaller limit: the limit sent squeezed it
      [{ limit_bytes: 200_000_000_000 }, 'limit_bytes'],
    ];
    for (const [body, field] of cases) {
      const error = await failure(settingsApi.putStorage(body as never));
      expect(isApiError(error) && [body, error.status, error.code, error.field]).toEqual([body, 422, 'validation_error', field]);
    }
    expect(await settingsApi.storage()).toMatchObject({ limit_bytes: null, kept: { default_days: 30, max_bytes: 300_000_000_000 } }); // nothing written
  });

  it('answers 409 limit_below_committed only for a change that lowers the kept cap below what’s kept', async () => {
    const error = await failure(settingsApi.putStorage({ kept: { max_bytes: 50_000_000_000 } }));
    expect(isLimitBelowCommittedError(error)).toBe(true);
    expect(isLimitBelowCommittedError(error) && error.details.committed_kept_bytes).toBe(86_400_000_000);
    await expect(settingsApi.putStorage({ limit_bytes: 100_000_000_000 })).rejects.toMatchObject({ status: 409, code: 'limit_below_committed' });
    expect((await settingsApi.storage()).limit_bytes).toBeNull();
  });

  it('storage-config: config.yaml’s limit until a PUT sends one; unrelated PUTs and raises still short save', async () => {
    setMockScenario('storage-config');
    const config = 53_760 * 1024 ** 2;
    const before = await settingsApi.storage();
    expect(before).toMatchObject({ limit_bytes: null, source: 'config', config_bytes: config, limit_effective_bytes: config });
    // Half of 52.5 GB is less than the seeded downloads commit
    expect(before.kept.effective_max_bytes).toBeLessThan(before.committed_kept_bytes);

    expect(await settingsApi.putStorage({ smart: { episodes_ahead: 3 } })).toMatchObject({ source: 'config', smart: { episodes_ahead: 3 } });
    expect((await settingsApi.putStorage({ kept: { max_bytes: 30_000_000_000 } })).kept.effective_max_bytes).toBe(30_000_000_000); // a raise, still short
    await expect(settingsApi.putStorage({ kept: { max_bytes: 20_000_000_000 } })).rejects.toMatchObject({ status: 409 }); // a lowering
    // "Use auto": limit_bytes null, sent
    expect(await settingsApi.putStorage({ limit_bytes: null })).toMatchObject({ limit_bytes: null, source: 'auto', limit_effective_bytes: 750_000_000_000 });
  });

  it('holds background downloads: off, outside every window, or while something streams', async () => {
    await settingsApi.putStorage({ background: { enabled: false } });
    let list = await downloadsApi.list();
    expect(list).toMatchObject({ paused_reason: 'disabled', resumes_at: null });
    expect(only(list, 'Interstellar')).toMatchObject({ state: 'paused', download_bps: 0, eta_seconds: null });
    expect(only(list, 'Bluey').state).toBe('done'); // done stays done
    expect((await torrent(DOWNLOAD_HASHES.interstellar))?.wanted?.state).toBe('paused');

    // 12:30 local, on the fixtures' day
    setMockDownloadsClock(new Date(2026, 9, 14, 12, 30));
    await settingsApi.putStorage({ background: { enabled: true, windows: [{ start: '14:00', end: '15:00' }, { start: '23:00', end: '06:00' }] } });
    list = await downloadsApi.list();
    expect(list).toMatchObject({ paused_reason: 'window', resumes_at: new Date(2026, 9, 14, 14, 0).toISOString() });
    expect(only(list, 'Interstellar')).toMatchObject({ state: 'waiting_for_window', resumes_at: list.resumes_at, download_bps: 0 });
    expect(only(list, 'The Studio').state).toBe('waiting_for_window'); // window before stalled
    expect((await titlesApi.get('movie', 157336)).library).toMatchObject({ availability_stage: 'downloading', keep: { state: 'waiting_for_window' } });
    // A waiting torrent isn't downloading at 0 B/s
    expect(await torrent(DOWNLOAD_HASHES.interstellar)).toMatchObject({ state: 'idle', download_bps: 0, wanted: { state: 'waiting_for_window' } });
    expect((await torrentsApi.summary()).counts.downloading).toBe(1);

    // Inside a window that crosses midnight
    setMockDownloadsClock(new Date(2026, 9, 14, 2, 0));
    expect((await downloadsApi.list()).paused_reason).toBeNull();
    expect(await torrent(DOWNLOAD_HASHES.interstellar)).toMatchObject({ state: 'downloading', download_bps: 2_300_000 });

    await settingsApi.putStorage({ background: { pause_while_streaming: true } });
    list = await downloadsApi.list();
    expect(list).toMatchObject({ paused_reason: 'streaming', resumes_at: null });
    expect(only(list, 'Interstellar')).toMatchObject({ state: 'waiting_for_stream', resumes_at: null });

    setMockScenario('torrents-quiet'); // nothing streams (the scenario starts its settings over)
    await settingsApi.putStorage({ background: { pause_while_streaming: true } });
    expect((await downloadsApi.list()).paused_reason).toBeNull();
  });
});
