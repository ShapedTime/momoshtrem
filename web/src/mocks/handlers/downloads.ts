import { http, HttpResponse } from 'msw';
import type {
  Download,
  DownloadEstimate,
  DownloadItem,
  DownloadScope,
  DownloadState,
  EpisodeSummary,
  MovieTitle,
  ShowTitle,
  TorrentListItem,
} from '@/lib/api/contract';
import {
  addMockDownload,
  downloadsUsage,
  findMockDownload,
  liveDownload,
  mockActiveDownloadRecords,
  type MockDownload,
  type MockDownloadTorrent,
  mockDownloadList,
  mockDownloadRecords,
  movieItem,
  EPISODE_BYTES_PER_MINUTE,
  releaseMockDownload,
  showItem,
} from '../fixtures/downloads';
import { mockStorageDocument, storageCaps } from '../fixtures/settings';
import { buildSeason, buildTitle, findByLibraryId, findEpisode, libraryKey } from '../fixtures/titles';
import { scenario } from '../scenario';
import { mockLibrary } from './titles';
import { mockTorrents } from './torrents';
import { apiError, isRecord, jsonBody, latency, noContent, notFound } from './util';

/**
 * Downloads (PLAN §6 "Downloads (#77)"): GET lists the page's downloads, POST
 * keeps a library item (GET /downloads/estimate says what it would need first),
 * PATCH changes a kept one's duration, DELETE releases it (the row stays,
 * `released_at` set, out of every read).
 * They change fixtures/downloads.ts, so titles (`keep`, what's downloading),
 * Activity (`wanted`, `holds`, the disk) and Settings › Storage follow.
 *
 * A Keep wants what the library has for its scope: a movie's torrent, or the
 * torrents with the show's (or season's) episodes in Activity. Without one of
 * those, an episode is estimated at 40 MB a minute. It's `done` when those
 * torrents are complete, `paused` when one is paused, `no_file` with nothing
 * assigned, and `queued` otherwise. Scenarios: `downloads-full` leaves 1.2 GB
 * of the kept cap, so a movie, a season or a show answers 409 `storage_full`;
 * `downloads-empty` starts a fresh install with none.
 */

const SCOPES: readonly DownloadScope[] = ['movie', 'show', 'season', 'episode'];
const DAY = 86_400_000;

const isId = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value > 0;
const gb = (bytes: number) => `${(bytes / 1e9).toFixed(1)} GB`;

/** null or a time in the future; the message when it isn't. */
function checkExpiry(value: unknown): string | null {
  if (value === null) return null;
  if (typeof value !== 'string' || Number.isNaN(Date.parse(value))) return 'expires_at must be a time (RFC 3339) or null';
  return Date.parse(value) > Date.now() ? null : 'expires_at must be in the future';
}

/** The duration fields of a POST or PATCH, as the backend checks them. */
function checkDuration(body: Record<string, unknown>): { field: string; message: string } | null {
  if ('until_watched' in body && typeof body.until_watched !== 'boolean') return { field: 'until_watched', message: 'until_watched must be true or false' };
  const expiry = 'expires_at' in body ? checkExpiry(body.expires_at) : null;
  return expiry ? { field: 'expires_at', message: expiry } : null;
}

/** POST /downloads' body checks, in the contract's order. */
function checkCreate(body: Record<string, unknown>): { field: string; message: string } | null {
  if (!SCOPES.some((scope) => scope === body.scope)) return { field: 'scope', message: 'scope must be movie, show, season or episode' };
  if (!isId(body.id)) return { field: 'id', message: 'id must be a library id' };
  if (body.scope === 'season' && !(typeof body.season_number === 'number' && Number.isInteger(body.season_number) && body.season_number >= 0)) {
    return { field: 'season_number', message: 'season_number must be a season of the show' };
  }
  return checkDuration(body);
}

/** What a Keep would want: everything but its id, state and duration. */
interface Plan {
  scope: DownloadScope;
  item: DownloadItem;
  /** A new Keep has nothing over the cap: only episodes assigned later can be. */
  files: Omit<Download['files'], 'over_cap'>;
  wanted_bytes: number;
  torrents: MockDownloadTorrent[];
  state: DownloadState;
}

/** A torrent scope ("Season 3", "Seasons 1–3", "Specials", "S02E01–E04") covers the season; `whole`: only whole seasons. */
function coversSeason(scope: string, season: number, whole: boolean): boolean {
  const seasons = /^Seasons? (\d+)(?:–(\d+))?/.exec(scope);
  if (seasons) return season >= Number(seasons[1]) && season <= Number(seasons[2] ?? seasons[1]);
  if (scope.startsWith('Specials')) return season === 0;
  const run = /^S(\d+)E/.exec(scope);
  return !whole && run !== null && Number(run[1]) === season;
}

/** The page's torrents with the show's episodes (by Activity's `scope`), for a season when given. */
function showTorrents(tmdbId: number, season?: number, whole = false): readonly TorrentListItem[] {
  return mockTorrents().filter((t) =>
    t.titles.some((x) => x.media_type === 'tv' && x.tmdb_id === tmdbId && x.scope !== null && (season === undefined || coversSeason(x.scope, season, whole)))
  );
}

const share = (t: TorrentListItem, bytes: number): MockDownloadTorrent => ({ info_hash: t.info_hash, bytes, done_bytes: t.complete ? bytes : 0 });

/** done when the torrents are complete, paused when one is, else queued; no_file without files. */
function stateOf(files: number, torrents: readonly TorrentListItem[]): DownloadState {
  if (files === 0) return 'no_file';
  if (torrents.length > 0 && torrents.every((t) => t.complete)) return 'done';
  return torrents.some((t) => t.state === 'paused') ? 'paused' : 'queued';
}

const episodeBytes = (e: Pick<EpisodeSummary, 'runtime'>) => (e.runtime ?? 45) * EPISODE_BYTES_PER_MINUTE;

/**
 * A show's or season's Keep over the episodes the library has files for. A file several episodes share (a grouped
 * "S01E01-E02", named by `fileOf`) counts once.
 */
function episodesPlan(
  scope: 'show' | 'season',
  item: DownloadItem,
  episodes: EpisodeSummary[],
  torrents: readonly TorrentListItem[],
  fileOf: (e: EpisodeSummary) => string
): Plan {
  const once = (list: EpisodeSummary[]) => [...new Map(list.map((e) => [fileOf(e), e])).values()];
  const withFile = once(episodes.filter((e) => e.webdav_path !== null));
  const fromTorrent = withFile.filter((e) => e.source !== 'hdrezka');
  const state = stateOf(withFile.length, torrents);
  const parts = torrents.map((t) => share(t, t.size_bytes ?? 0));
  return {
    scope,
    item,
    files: { total: withFile.length, done: state === 'done' ? fromTorrent.length : 0, unavailable: withFile.length - fromTorrent.length },
    wanted_bytes: parts.length > 0 ? parts.reduce((sum, p) => sum + p.bytes, 0) : fromTorrent.reduce((sum, e) => sum + episodeBytes(e), 0),
    torrents: parts,
    state: fromTorrent.length === 0 && withFile.length > 0 ? 'unavailable' : state,
  };
}

/** What POST /downloads would keep, or the 404 when the library hasn't got it. */
function plan(body: Record<string, unknown>): Plan | Response {
  const library = mockLibrary();
  const id = body.id as number;
  if (body.scope === 'movie') {
    const found = findByLibraryId(library, 'movie', id);
    const title = found && (buildTitle('movie', found.entry.tmdb_id, library) as MovieTitle | undefined);
    if (!found || !title) return notFound('No such movie in the library');
    const item = movieItem(found.entry.tmdb_id, id, found.entry.title);
    const { torrent, stream } = title.library;
    if (stream) return { scope: 'movie', item, files: { total: 1, done: 0, unavailable: 1 }, wanted_bytes: 0, torrents: [], state: 'unavailable' };
    if (!torrent) return { scope: 'movie', item, files: { total: 0, done: 0, unavailable: 0 }, wanted_bytes: 0, torrents: [], state: 'no_file' };
    const live = mockTorrents().find((t) => t.info_hash === torrent.info_hash);
    const state = stateOf(1, live ? [live] : []);
    return {
      scope: 'movie',
      item,
      files: { total: 1, done: state === 'done' ? 1 : 0, unavailable: 0 },
      wanted_bytes: torrent.size_bytes,
      torrents: [{ info_hash: torrent.info_hash, bytes: torrent.size_bytes, done_bytes: state === 'done' ? torrent.size_bytes : 0 }],
      state,
    };
  }

  if (body.scope === 'episode') {
    const episode = findEpisode(id);
    const row = episode && library.get(libraryKey('tv', episode.tmdbId));
    const built = episode && row ? buildSeason(episode.tmdbId, episode.season, library)?.episodes.find((e) => e.id === id) : undefined;
    const found = row && findByLibraryId(library, 'tv', row.library_id);
    if (!episode || !built || !found) return notFound('No such episode in the library');
    const item = showItem(episode.tmdbId, found.row.library_id, found.entry.title, episode.season, episode.episode, built.name);
    if (built.webdav_path === null) return { scope: 'episode', item, files: { total: 0, done: 0, unavailable: 0 }, wanted_bytes: 0, torrents: [], state: 'no_file' };
    if (built.source === 'hdrezka') return { scope: 'episode', item, files: { total: 1, done: 0, unavailable: 1 }, wanted_bytes: 0, torrents: [], state: 'unavailable' };
    // A whole-season torrent holds the episode; a run of episodes can't be told apart here
    const torrents = showTorrents(episode.tmdbId, episode.season, true);
    const state = stateOf(1, torrents);
    const bytes = episodeBytes(built);
    return { scope: 'episode', item, files: { total: 1, done: state === 'done' ? 1 : 0, unavailable: 0 }, wanted_bytes: bytes, torrents: torrents.map((t) => share(t, bytes)), state };
  }

  const found = findByLibraryId(library, 'tv', id);
  const show = found && (buildTitle('tv', found.entry.tmdb_id, library) as ShowTitle | undefined);
  if (!found || !show) return notFound('No such show in the library');
  const tmdbId = found.entry.tmdb_id;
  // A job's file is its torrent's "hash/path" (shared by a grouped file's episodes); a seeded one is the episode's own
  const fileOf = (e: EpisodeSummary) => {
    const filed = found.row.filed.get(e.id);
    return filed ? `${filed.info_hash}/${filed.file_path}` : (e.webdav_path ?? String(e.id));
  };
  if (body.scope === 'season') {
    const seasonNumber = body.season_number as number;
    const season = buildSeason(tmdbId, seasonNumber, library);
    if (!season) return notFound('No such season');
    return episodesPlan('season', showItem(tmdbId, id, found.entry.title, seasonNumber), season.episodes, showTorrents(tmdbId, seasonNumber), fileOf);
  }
  const episodes = show.seasons.flatMap((s) => buildSeason(tmdbId, s.season_number, library)?.episodes ?? []);
  return episodesPlan('show', showItem(tmdbId, id, found.entry.title), episodes, showTorrents(tmdbId), fileOf);
}

const sameTarget = (a: DownloadItem, scope: DownloadScope, b: Download) =>
  b.scope === scope &&
  b.item.media_type === a.media_type &&
  b.item.tmdb_id === a.tmdb_id &&
  b.item.season_number === a.season_number &&
  b.item.episode_number === a.episode_number;

/** The duration a POST asks for, else the storage settings' defaults. */
function duration(body: Record<string, unknown>, now: number): Pick<Download, 'until_watched' | 'expires_at'> {
  const { default_until_watched, default_days } = mockStorageDocument().kept;
  return {
    until_watched: typeof body.until_watched === 'boolean' ? body.until_watched : default_until_watched,
    expires_at:
      'expires_at' in body ? (body.expires_at as string | null) : default_days === null ? null : new Date(now + default_days * DAY).toISOString(),
  };
}

/**
 * The kept download a Keep would make, before it has an id. A smart pick turned kept
 * keeps its state and bytes, but starts over: `created_at` now, the duration asked for.
 */
function keptFrom(planned: Plan, existing: MockDownload | undefined, body: Record<string, unknown>, now: number): Omit<Download, 'id'> {
  return {
    reason: 'kept',
    smart_source: null,
    scope: planned.scope,
    item: planned.item,
    state: existing?.download.state ?? planned.state,
    wanted_bytes: planned.wanted_bytes,
    done_bytes: existing?.download.done_bytes ?? planned.torrents.reduce((sum, t) => sum + t.done_bytes, 0),
    download_bps: 0,
    eta_seconds: null,
    resumes_at: null,
    files: { ...planned.files, over_cap: 0 },
    ...duration(body, now),
    created_at: new Date(now).toISOString(),
  };
}

/** The 409's maths: what a Keep adds to what kept downloads commit, and what the kept cap has left. */
function room(records: readonly MockDownload[], existing: MockDownload | undefined, candidate: MockDownload) {
  const committed = downloadsUsage(records).committed_kept_bytes;
  const needed = downloadsUsage([...records.filter((d) => d !== existing), candidate]).committed_kept_bytes - committed;
  return { needed, available: Math.max(0, storageCaps(mockStorageDocument()).kept - committed) };
}

/** A route's download id, as the backend parses ids: a positive whole number, else null (400). */
function downloadId(param: string | readonly string[] | undefined): number | null {
  const id = typeof param === 'string' && /^\d+$/.test(param) ? Number(param) : NaN;
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

/** POST's target from GET /downloads/estimate's query. */
function targetFrom(search: URLSearchParams): Record<string, unknown> {
  const target: Record<string, unknown> = { scope: search.get('scope'), id: Number(search.get('id')) };
  if (search.has('season_number')) target.season_number = Number(search.get('season_number'));
  return target;
}

export const downloadsHandlers = [
  // `downloads-absent`: a backend before #77's P4 has no /downloads routes at all; `downloads-error`: the list
  // fails (500). Anything else falls through to the handlers below.
  http.all('*/api/downloads*', ({ request }) => {
    if (scenario() === 'downloads-absent') return notFound('404 page not found');
    if (scenario() === 'downloads-error' && request.method === 'GET' && new URL(request.url).pathname.endsWith('/api/downloads')) {
      return apiError(500, 'internal_error', 'The downloads couldn’t be read.');
    }
    return undefined;
  }),
  http.get('*/api/downloads', async () => {
    await latency();
    return HttpResponse.json(mockDownloadList());
  }),
  http.get('*/api/downloads/estimate', async ({ request }) => {
    await latency();
    const target = targetFrom(new URL(request.url).searchParams);
    const invalid = checkCreate(target);
    if (invalid) return apiError(400, 'bad_request', invalid.message);
    const planned = plan(target);
    if (planned instanceof Response) return planned;
    const records = mockDownloadRecords();
    const existing = mockActiveDownloadRecords().find((d) => sameTarget(planned.item, planned.scope, d.download));
    const fresh = keptFrom(planned, existing, target, Date.now());
    const { needed, available } = room(records, existing, { download: { ...fresh, id: 0 }, torrents: existing?.torrents ?? planned.torrents, over_cap: [] });
    return HttpResponse.json<DownloadEstimate>({
      wanted_bytes: planned.wanted_bytes,
      needed_bytes: existing?.download.reason === 'kept' ? 0 : needed,
      available_bytes: available,
      downloadable: planned.state !== 'unavailable',
    });
  }),
  http.post('*/api/downloads', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return apiError(400, 'bad_request', 'Send the Keep as a JSON object.');
    const invalid = checkCreate(body);
    if (invalid) return apiError(422, 'validation_error', invalid.message, { field: invalid.field });
    const planned = plan(body);
    if (planned instanceof Response) return planned;
    if (planned.state === 'unavailable') return apiError(422, 'not_downloadable', 'This plays from HDRezka, so it can’t be kept on disk.');

    const records = mockDownloadRecords();
    // A smart pick hidden while smart downloads are off is still the target's row: a Keep turns it kept
    const existing = mockActiveDownloadRecords().find((d) => sameTarget(planned.item, planned.scope, d.download));
    if (existing?.download.reason === 'kept') return HttpResponse.json(liveDownload(existing), { status: 200 });

    const fresh = keptFrom(planned, existing, body, Date.now());
    const torrents = existing?.torrents ?? planned.torrents;
    const { needed, available } = room(records, existing, { download: { ...fresh, id: existing?.download.id ?? 0 }, torrents, over_cap: [] });
    if (needed > available) {
      return apiError(409, 'storage_full', `Not enough space for kept downloads: this needs ${gb(needed)} and ${gb(available)} is left.`, {
        needed_bytes: needed,
        available_bytes: available,
      });
    }

    if (existing) {
      // A smart pick turned kept: same row, created now, with the duration asked for
      const { until_watched, expires_at, created_at } = fresh;
      Object.assign(existing.download, { reason: 'kept', smart_source: null, until_watched, expires_at, created_at });
      return HttpResponse.json(liveDownload(existing), { status: 200 });
    }
    return HttpResponse.json(liveDownload(addMockDownload(fresh, torrents)), { status: 201 });
  }),
  http.patch('*/api/downloads/:id', async ({ params, request }) => {
    await latency();
    const id = downloadId(params.id);
    if (id === null) return apiError(400, 'bad_request', 'Invalid ID format');
    const record = findMockDownload(id);
    if (!record) return notFound('No such download');
    const body = await jsonBody(request);
    if (!isRecord(body)) return apiError(400, 'bad_request', 'Send the duration as a JSON object.');
    if (record.download.reason === 'smart') return apiError(422, 'validation_error', 'A smart pick has no duration: keep it first.');
    const invalid = checkDuration(body);
    if (invalid) return apiError(422, 'validation_error', invalid.message, { field: invalid.field });
    if (typeof body.until_watched === 'boolean') record.download.until_watched = body.until_watched;
    if ('expires_at' in body) record.download.expires_at = body.expires_at as string | null;
    return HttpResponse.json(liveDownload(record));
  }),
  http.delete('*/api/downloads/:id', async ({ params }) => {
    await latency();
    const id = downloadId(params.id);
    if (id === null) return apiError(400, 'bad_request', 'Invalid ID format');
    return releaseMockDownload(id) ? noContent() : notFound('No such download');
  }),
];
