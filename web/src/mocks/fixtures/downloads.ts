import type {
  Download,
  DownloadItem,
  DownloadList,
  DownloadsPausedReason,
  DownloadState,
  DownloadWindow,
  Keep,
  MediaType,
  Timestamp,
  TorrentListItem,
  TorrentWanted,
  TorrentWantedState,
} from '@/lib/api/contract';
import { type MockScenario, scenario } from '../scenario';
import { iso } from './catalog';
import { MOCK_STORAGE, mockStorageDocument, type StorageDocument, storageCaps, type StorageUsage } from './settings';
import { SHOWS } from './title-seeds';

/**
 * Downloads (PLAN §6 "Downloads (#77)"), kept here (not in handlers/downloads.ts)
 * so the titles, torrents and settings mocks read them without msw. The page's
 * downloads start from the seeds below, and POST, PATCH and DELETE change them.
 * Titles read their `keep` and whether a file is downloading from here; Activity
 * reads `wanted`, `holds` and the disk's kept and smart bytes. Pausing, resuming
 * or removing a torrent, assigning or unassigning a file (a movie's or an
 * episode's), and removing a title reach the downloads through the `on…` hooks.
 *
 * | Id | Download | State |
 * |---|---|---|
 * | 5 | Interstellar, kept until watched (30 days at most) | downloading 42% |
 * | 4 | One Battle After Another, kept: nothing assigned yet | no_file |
 * | 3 | The Studio S01E10, kept | stalled (no peers, retrying) |
 * | 2 | One Piece #1100, kept for 30 days; its torrent is paused | paused |
 * | 1 | Bluey season 3, kept until removed | done |
 * | 6, 7 | Frieren S01E27–E28, smart picks from Up next | done |
 *
 * The rest come from the settings and the hooks: `paused` with background
 * downloads off, `waiting_for_window` outside every download window,
 * `waiting_for_stream` with pause-while-streaming on (Severance streams on the
 * Apple TV, except in `torrents-quiet`), `storage_full` when episodes assigned
 * to a kept show or season don't fit the kept cap, and `unavailable` when a kept
 * movie or episode gets an HDRezka stream. `disk_full` isn't seeded.
 *
 * Releases are soft (PLAN §5 `010`): a released download keeps its row, out of
 * every read. The mock doesn't expire downloads, and its Trakt plays predate
 * every keep, so nothing is released as watched.
 */

/** One torrent's part of a download: its wanted bytes, and what's on disk of them. */
export interface MockDownloadTorrent {
  info_hash: string;
  bytes: number;
  done_bytes: number;
}

/** A download as the mock keeps it: the wire shape with the manager's state for its admitted files, and the torrents it wants. */
export interface MockDownload {
  download: Download;
  torrents: MockDownloadTorrent[];
  /** Episodes ("season:episode") assigned after the Keep that didn't fit the kept cap: not pinned, not fetched. */
  over_cap: string[];
  /** What it was before its torrent was paused, so a resume puts it back. */
  before_pause?: DownloadState;
  /** Set once it's released; released rows stay, out of every read. */
  released_at?: Timestamp;
  release_reason?: 'deleted' | 'expired' | 'watched';
}

/** A download as it reads now, with what its admitted files are doing and the episodes over the cap. */
export interface LiveHold {
  download: Download;
  /** The admitted files' state: the download's, but for `storage_full`. */
  admitted: DownloadState;
  overCap: ReadonlySet<string>;
}

/** The fixtures/torrents.ts hashes the seeds hold, and the one Severance streams from. */
export const DOWNLOAD_HASHES = {
  interstellar: 'c9d4a7b3e5f60718293a4b5c6d7e8f90a1b2c3d4',
  bluey: 'e8d7c6b5a4938271605f4e3d2c1b0a9f8e7d6c5b',
  onePiece: '5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f80',
  severance: 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1',
} as const;

/** What the mock estimates an episode file at, by its runtime. */
export const EPISODE_BYTES_PER_MINUTE = 40_000_000;

const DAY = 86_400_000;
const later = (from: Timestamp, days: number): Timestamp => new Date(Date.parse(from) + days * DAY).toISOString();

// ---- The clock (tests pin it) ---------------------------------------------------

let pinned: Date | null = null;

/** Tests: the time the download windows are read against; null is the real clock. */
export function setMockDownloadsClock(at: Date | null): void {
  pinned = at;
}

const clock = () => (pinned ? new Date(pinned) : new Date());

/** Seconds left at a speed; null without one (PLAN §6: no ETA without a speed). */
export const etaSeconds = (bytesLeft: number, bps: number): number | null => (bps > 0 ? Math.round(Math.max(0, bytesLeft) / bps) : null);

export const movieItem = (tmdb_id: number, library_id: number, title: string): DownloadItem => ({
  media_type: 'movie',
  tmdb_id,
  library_id,
  title,
  poster_path: null,
  season_number: null,
  episode_number: null,
  episode_name: null,
});

export const showItem = (
  tmdb_id: number,
  library_id: number,
  title: string,
  season_number: number | null = null,
  episode_number: number | null = null,
  episode_name: string | null = null
): DownloadItem => ({ media_type: 'tv', tmdb_id, library_id, title, poster_path: null, season_number, episode_number, episode_name });

type SeedFields = Pick<Download, 'id' | 'reason' | 'scope' | 'item' | 'state' | 'wanted_bytes' | 'done_bytes' | 'created_at'> & Partial<Download>;

function seed(fields: SeedFields, torrents: MockDownloadTorrent[] = []): MockDownload {
  const download: Download = {
    smart_source: null,
    download_bps: 0,
    eta_seconds: null,
    resumes_at: null,
    files: { total: 1, done: fields.state === 'done' ? 1 : 0, unavailable: 0, over_cap: 0 },
    until_watched: true,
    expires_at: null,
    ...fields,
  };
  return { download, torrents, over_cap: [] };
}

const INTERSTELLAR_BYTES = 63_100_000_000;
const INTERSTELLAR_DONE = 26_500_000_000;
const INTERSTELLAR_BPS = 2_300_000;
/** Frieren's 24-minute episodes. */
const FRIEREN_EPISODE_BYTES = 24 * EPISODE_BYTES_PER_MINUTE;

const SEED_DOWNLOADS: readonly MockDownload[] = [
  seed(
    {
      id: 1,
      reason: 'kept',
      scope: 'season',
      item: showItem(82728, 5, 'Bluey', 3),
      state: 'done',
      wanted_bytes: 21_700_000_000,
      done_bytes: 21_700_000_000,
      files: { total: 49, done: 49, unavailable: 0, over_cap: 0 },
      until_watched: false,
      created_at: iso(20),
    },
    [{ info_hash: DOWNLOAD_HASHES.bluey, bytes: 21_700_000_000, done_bytes: 21_700_000_000 }]
  ),
  seed(
    {
      id: 2,
      reason: 'kept',
      scope: 'episode',
      item: showItem(37854, 2, 'One Piece', 22, 15, 'Episode 1100'),
      state: 'paused',
      wanted_bytes: 400_000_000,
      done_bytes: 124_000_000,
      until_watched: false,
      expires_at: later(iso(6), 30),
      created_at: iso(6),
    },
    [{ info_hash: DOWNLOAD_HASHES.onePiece, bytes: 400_000_000, done_bytes: 124_000_000 }]
  ),
  seed({
    id: 3,
    reason: 'kept',
    scope: 'episode',
    item: showItem(247767, 6, 'The Studio', 1, 10, 'The Pitch'),
    state: 'stalled',
    wanted_bytes: 1_200_000_000,
    done_bytes: 444_000_000,
    resumes_at: new Date(Date.parse(iso(0)) + 4 * 60_000).toISOString(),
    expires_at: later(iso(3), 30),
    created_at: iso(3),
  }),
  seed({
    id: 4,
    reason: 'kept',
    scope: 'movie',
    item: movieItem(1054867, 3, 'One Battle After Another'),
    state: 'no_file',
    wanted_bytes: 0,
    done_bytes: 0,
    files: { total: 0, done: 0, unavailable: 0, over_cap: 0 },
    expires_at: later(iso(2), 30),
    created_at: iso(2),
  }),
  seed(
    {
      id: 5,
      reason: 'kept',
      scope: 'movie',
      item: movieItem(157336, 2, 'Interstellar'),
      state: 'downloading',
      wanted_bytes: INTERSTELLAR_BYTES,
      done_bytes: INTERSTELLAR_DONE,
      download_bps: INTERSTELLAR_BPS,
      eta_seconds: etaSeconds(INTERSTELLAR_BYTES - INTERSTELLAR_DONE, INTERSTELLAR_BPS),
      expires_at: later(iso(1), 30),
      created_at: iso(1),
    },
    [{ info_hash: DOWNLOAD_HASHES.interstellar, bytes: INTERSTELLAR_BYTES, done_bytes: INTERSTELLAR_DONE }]
  ),
  ...([
    [6, 27, 'An Ordinary Mage'],
    [7, 28, 'It Would Be Embarrassing When We Met Again'],
  ] as const).map(([id, episode, name]) =>
    // A smart pick: until watched, 14 days at most
    seed({
      id,
      reason: 'smart',
      smart_source: 'up_next',
      scope: 'episode',
      item: showItem(209867, 1, 'Frieren: Beyond Journey’s End', 1, episode, name),
      state: 'done',
      wanted_bytes: FRIEREN_EPISODE_BYTES,
      done_bytes: FRIEREN_EPISODE_BYTES,
      expires_at: later(iso(0), 14),
      created_at: iso(0),
    })
  ),
];

// ---- The page's downloads -------------------------------------------------------

/** Copies to change (jsdom, where component tests build titles, has no structuredClone). */
const cloneDownload = (d: Download): Download => ({ ...d, item: { ...d.item }, files: { ...d.files } });
const cloneRecord = (d: MockDownload): MockDownload => ({
  ...d,
  download: cloneDownload(d.download),
  torrents: d.torrents.map((t) => ({ ...t })),
  over_cap: [...d.over_cap],
});

/** Scenarios that start with no downloads. */
const EMPTY: ReadonlySet<MockScenario | null> = new Set(['downloads-empty']);

/** Rebuilt when the scenario changes: `downloads-empty` starts with none. */
let state: { from: MockScenario | null; items: MockDownload[]; nextId: number } | undefined;

function current() {
  const from = scenario();
  if (!state || state.from !== from) {
    const items = EMPTY.has(from) ? [] : SEED_DOWNLOADS.map(cloneRecord);
    state = { from, items, nextId: SEED_DOWNLOADS.length + 1 };
  }
  return state;
}

/** Tests: back to the seeded downloads and the real clock. */
export function resetMockDownloads(): void {
  state = undefined;
  pinned = null;
}

/** Active rows (not released), and smart picks only while smart downloads are on. */
const visible = (items: readonly MockDownload[], doc: StorageDocument) =>
  items.filter((d) => d.released_at === undefined && (d.download.reason === 'kept' || doc.smart.enabled));

/** Active rows, smart picks too whatever the setting: what the hooks change. */
const active = () => current().items.filter((d) => d.released_at === undefined);

/** The page's downloads as stored (active, smart ones while smart downloads are on), for the handlers to change. */
export function mockDownloadRecords(): MockDownload[] {
  return visible(current().items, mockStorageDocument());
}

export function findMockDownload(id: number): MockDownload | undefined {
  return mockDownloadRecords().find((d) => d.download.id === id);
}

/** POST /downloads, once checked: a new kept download. */
export function addMockDownload(download: Omit<Download, 'id'>, torrents: MockDownloadTorrent[]): MockDownload {
  const page = current();
  const record: MockDownload = { download: { ...download, id: page.nextId++ }, torrents, over_cap: [] };
  page.items.push(record);
  return record;
}

/** DELETE /downloads/{id}: a soft release (`deleted`); false when there's no such active download. */
export function releaseMockDownload(id: number): boolean {
  const record = findMockDownload(id);
  if (!record) return false;
  record.released_at = clock().toISOString();
  record.release_reason = 'deleted';
  return true;
}

/** Tests: every row, released ones too. */
export const mockDownloadRows = (): readonly MockDownload[] => current().items;

// ---- Rules ----------------------------------------------------------------------

/** States the manager moves on its own; the background-download settings can hold them. */
const MOVING: ReadonlySet<DownloadState> = new Set(['queued', 'downloading', 'stalled']);
/** Admitted files in these still want bytes, so they're `downloading` on title pages (PLAN §6 availability rules). */
const WANTING: ReadonlySet<DownloadState> = new Set(['queued', 'downloading', 'waiting_for_stream', 'waiting_for_window', 'stalled', 'disk_full']);
/** The states that outrank `storage_full`. */
const BEFORE_FULL: ReadonlySet<DownloadState> = new Set(['unavailable', 'no_file', 'paused', 'disk_full']);

export const wantsBytes = (state: DownloadState | undefined): boolean => state !== undefined && WANTING.has(state);

const minutesOf = (hhmm: string) => {
  const [h, m] = hhmm.split(':').map(Number);
  return h * 60 + m;
};

/** A window may cross midnight. */
function inside(window: DownloadWindow, minute: number): boolean {
  const start = minutesOf(window.start);
  const end = minutesOf(window.end);
  return start < end ? minute >= start && minute < end : minute >= start || minute < end;
}

/** The soonest window start after `now`, in local time (the server's TZ, for the mock the browser's). */
function nextWindowStart(windows: DownloadWindow[], now: Date): Timestamp {
  const starts = windows.map(({ start }) => {
    const at = new Date(now);
    at.setHours(Math.floor(minutesOf(start) / 60), minutesOf(start) % 60, 0, 0);
    if (at <= now) at.setDate(at.getDate() + 1);
    return at.getTime();
  });
  return new Date(Math.min(...starts)).toISOString();
}

/** Severance streams on the Apple TV in every scenario but `torrents-quiet` (fixtures/torrents.ts). */
const somethingStreams = () => scenario() !== 'torrents-quiet';

/** Why background downloads wait now (GET /downloads' `paused_reason`), and when they resume. */
export function pausedReason(doc: StorageDocument, now = clock()): { reason: DownloadsPausedReason | null; resumes_at: Timestamp | null } {
  const { enabled, windows, pause_while_streaming } = doc.background;
  if (!enabled) return { reason: 'disabled', resumes_at: null };
  const minute = now.getHours() * 60 + now.getMinutes();
  if (windows.length > 0 && !windows.some((w) => inside(w, minute))) return { reason: 'window', resumes_at: nextWindowStart(windows, now) };
  // While the stream lasts, there's no resume time yet
  if (pause_while_streaming && somethingStreams()) return { reason: 'streaming', resumes_at: null };
  return { reason: null, resumes_at: null };
}

/**
 * A download as it reads now (PLAN §6 state order): the background-download
 * settings hold the moving ones, and episodes over the kept cap make it
 * `storage_full` unless something outranks that.
 */
function live(record: MockDownload, doc: StorageDocument, now: Date): LiveHold {
  const download = cloneDownload(record.download);
  let admitted = download.state;
  if (MOVING.has(admitted)) {
    const { reason, resumes_at } = pausedReason(doc, now);
    // Prefetch inside the streaming torrent carries on
    const onTheStream = record.torrents.some((t) => t.info_hash === DOWNLOAD_HASHES.severance);
    const held: DownloadState | null =
      reason === 'disabled' ? 'paused' : reason === 'window' ? 'waiting_for_window' : reason === 'streaming' && !onTheStream ? 'waiting_for_stream' : null;
    if (held) {
      admitted = held;
      Object.assign(download, { download_bps: 0, eta_seconds: null, resumes_at: held === 'waiting_for_window' ? resumes_at : null });
    }
  }
  download.state = record.over_cap.length > 0 && !BEFORE_FULL.has(admitted) ? 'storage_full' : admitted;
  download.files.over_cap = record.over_cap.length;
  return { download, admitted, overCap: new Set(record.over_cap) };
}

/** GET /downloads' order: kept first, newest first; then smart picks by show and episode. */
function byListOrder(a: LiveHold, b: LiveHold): number {
  const [x, y] = [a.download, b.download];
  if (x.reason !== y.reason) return x.reason === 'kept' ? -1 : 1;
  if (x.reason === 'kept') return y.created_at.localeCompare(x.created_at) || y.id - x.id;
  return x.item.tmdb_id - y.item.tmdb_id || (x.item.season_number ?? 0) - (y.item.season_number ?? 0) || (x.item.episode_number ?? 0) - (y.item.episode_number ?? 0);
}

/** The page's downloads as they read now, in GET /downloads' order. */
export function mockHolds(now = clock()): LiveHold[] {
  const doc = mockStorageDocument();
  return mockDownloadRecords()
    .map((d) => live(d, doc, now))
    .sort(byListOrder);
}

/** The page's downloads as GET /downloads lists them. */
export const mockDownloads = (now = clock()): Download[] => mockHolds(now).map((h) => h.download);

export const liveDownload = (record: MockDownload, now = clock()): Download => live(record, mockStorageDocument(), now).download;

/** Per torrent, the largest share any of `records` wants: a file two downloads want counts once. */
function sharedBytes(records: readonly MockDownload[], pick: (t: MockDownloadTorrent) => number, alone: (d: Download) => number): number {
  const perHash = new Map<string, number>();
  let rest = 0;
  for (const record of records) {
    if (record.torrents.length === 0) rest += alone(record.download);
    for (const t of record.torrents) perHash.set(t.info_hash, Math.max(perHash.get(t.info_hash) ?? 0, pick(t)));
  }
  return rest + [...perHash.values()].reduce((sum, bytes) => sum + bytes, 0);
}

/** On disk and committed (admitted files only), over `records` (the page's, unless given). */
export function downloadsUsage(records: readonly MockDownload[] = mockDownloadRecords()): StorageUsage {
  const kept = records.filter((d) => d.download.reason === 'kept');
  const smart = records.filter((d) => d.download.reason === 'smart');
  return {
    kept_bytes: sharedBytes(kept, (t) => t.done_bytes, (d) => d.done_bytes),
    smart_bytes: sharedBytes(smart, (t) => t.done_bytes, (d) => d.done_bytes),
    committed_kept_bytes: sharedBytes(kept, (t) => t.bytes, (d) => d.wanted_bytes),
  };
}

/** The seeds' usage, for the static torrent fixtures. */
export const SEED_USAGE: StorageUsage = downloadsUsage(visible(SEED_DOWNLOADS, MOCK_STORAGE));

/** GET /downloads. */
export function mockDownloadList(now = clock()): DownloadList {
  const doc = mockStorageDocument();
  const caps = storageCaps(doc);
  const { reason, resumes_at } = pausedReason(doc, now);
  return {
    items: mockDownloads(now),
    usage: { ...downloadsUsage(), kept_max_bytes: caps.kept, smart_max_bytes: caps.smart },
    paused_reason: reason,
    resumes_at,
  };
}

// ---- Titles (PLAN §6 "Titles and library": keep, and what's downloading) --------

const pctOf = (d: Download) => (d.state === 'done' ? 100 : d.wanted_bytes > 0 ? Math.round((d.done_bytes / d.wanted_bytes) * 100) : 0);

export const keepOf = (d: Download): Keep => ({
  download_id: d.id,
  reason: d.reason,
  scope: d.scope,
  state: d.state,
  pct: pctOf(d),
  until_watched: d.until_watched,
  expires_at: d.expires_at,
});

/** A movie's download, or a show's or season's own (not an episode's), as it reads now. */
export function scopeHold(mediaType: MediaType, tmdbId: number, seasonNumber?: number): LiveHold | undefined {
  const scope = mediaType === 'movie' ? 'movie' : seasonNumber === undefined ? 'show' : 'season';
  return mockHolds().find(
    ({ download: d }) =>
      d.scope === scope && d.item.media_type === mediaType && d.item.tmdb_id === tmdbId && (scope !== 'season' || d.item.season_number === seasonNumber)
  );
}

const NARROWEST = { episode: 0, season: 1, show: 2, movie: 3 } as const;

/** What holds an episode, of `holds` (the page's, from `mockHolds`): kept before smart, then its own, its season's, its show's. */
export function episodeHold(holds: readonly LiveHold[], tmdbId: number, season: number, episode: number): LiveHold | undefined {
  return holds
    .filter(
      ({ download: d }) =>
        d.item.media_type === 'tv' &&
        d.item.tmdb_id === tmdbId &&
        (d.scope === 'show' || (d.item.season_number === season && (d.scope === 'season' || d.item.episode_number === episode)))
    )
    .sort((a, b) =>
      a.download.reason === b.download.reason ? NARROWEST[a.download.scope] - NARROWEST[b.download.scope] : a.download.reason === 'kept' ? -1 : 1
    )[0];
}

/** An episode's file as the titles mock sees it. */
export interface EpisodeFileFacts {
  season: number;
  episode: number;
  has_file: boolean;
  hdrezka: boolean;
  /** What's on disk of it, when the seeds say. */
  pct: number | null;
}

const episodeKey = (season: number, episode: number) => `${season}:${episode}`;

/** The state of an episode's file under `hold`: no_file, unavailable (HDRezka), storage_full (over the cap), else its admitted files'. */
function fileState(hold: LiveHold, file: EpisodeFileFacts): DownloadState {
  if (!file.has_file) return 'no_file';
  if (file.hdrezka) return 'unavailable';
  if (hold.overCap.has(episodeKey(file.season, file.episode))) return 'storage_full';
  return hold.admitted;
}

/** The episode's file is downloading: a download wants it and it isn't on disk (PLAN §6 availability rules). */
export const episodeWanted = (hold: LiveHold | undefined, file: EpisodeFileFacts): boolean => hold !== undefined && wantsBytes(fileState(hold, file));

/** `EpisodeSummary.keep`: the holding download, with `state` and `pct` for the episode's own file. */
export function episodeKeep(hold: LiveHold | undefined, file: EpisodeFileFacts): Keep | null {
  if (!hold) return null;
  const keep = keepOf(hold.download);
  const fileNow = fileState(hold, file);
  if (fileNow === 'done') return { ...keep, state: 'done', pct: 100 };
  if (!wantsBytes(fileNow) && fileNow !== 'paused') return { ...keep, state: fileNow, pct: 0 };
  return { ...keep, state: fileNow, pct: hold.download.scope === 'episode' ? pctOf(hold.download) : (file.pct ?? 0) };
}

// ---- Torrents (PLAN §6 B6: wanted and holds) --------------------------------------

/** A torrent that isn't done takes the first of these its downloads have (finished ones don't count). */
const TORRENT_ORDER: readonly Exclude<TorrentWantedState, 'done'>[] = ['paused', 'disk_full', 'waiting_for_window', 'waiting_for_stream', 'stalled', 'downloading', 'queued'];
const isTorrentState = (s: DownloadState): s is TorrentWantedState => s !== 'no_file' && s !== 'unavailable' && s !== 'storage_full';

/** A torrent's `wanted` and `holds` from the downloads that want its files. */
function torrentShare(
  t: Pick<TorrentListItem, 'info_hash' | 'download_bps'>,
  records: readonly MockDownload[],
  holds: readonly LiveHold[]
): Pick<TorrentListItem, 'wanted' | 'holds'> {
  const holding = records
    .map((record) => ({ part: record.torrents.find((p) => p.info_hash === t.info_hash), hold: holds.find((h) => h.download.id === record.download.id) }))
    .filter((h): h is { part: MockDownloadTorrent; hold: LiveHold } => h.part !== undefined && h.hold !== undefined)
    .sort((a, b) =>
      a.hold.download.reason === b.hold.download.reason ? a.hold.download.id - b.hold.download.id : a.hold.download.reason === 'kept' ? -1 : 1
    );
  if (holding.length === 0) return { wanted: null, holds: [] };
  const bytes = Math.max(...holding.map((h) => h.part.bytes));
  const done = Math.max(...holding.map((h) => h.part.done_bytes));
  // The torrent's state is its admitted files': all done is done; one downloading is downloading; else the first of the order
  const states = holding.map((h) => h.hold.admitted).filter(isTorrentState);
  const fetching = states.filter((s) => s !== 'done');
  const torrentState: TorrentWantedState =
    fetching.length === 0 ? 'done' : fetching.includes('downloading') ? 'downloading' : (TORRENT_ORDER.find((s) => fetching.includes(s)) ?? 'queued');
  const wanted: TorrentWanted = {
    bytes,
    done_bytes: done,
    eta_seconds: torrentState === 'downloading' ? etaSeconds(bytes - done, t.download_bps) : null,
    state: torrentState,
    resumes_at: holding.find((h) => h.hold.admitted === torrentState)?.hold.download.resumes_at ?? null,
  };
  return {
    wanted,
    holds: holding.map(({ hold: { download } }) => ({ download_id: download.id, reason: download.reason, scope: download.scope, title: download.item.title })),
  };
}

/** `wanted` and `holds` for one of the page's torrents. */
export function mockTorrentShare(t: Pick<TorrentListItem, 'info_hash' | 'download_bps'>, now = clock()): Pick<TorrentListItem, 'wanted' | 'holds'> {
  return torrentShare(t, mockDownloadRecords(), mockHolds(now));
}

/** `wanted` and `holds` from the seeds and the mock install's settings: the static torrent fixtures. */
export function seededTorrentShare(t: Pick<TorrentListItem, 'info_hash' | 'download_bps'>): Pick<TorrentListItem, 'wanted' | 'holds'> {
  const records = visible(SEED_DOWNLOADS, MOCK_STORAGE);
  const now = new Date();
  return torrentShare(t, records, records.map((d) => live(d, MOCK_STORAGE, now)));
}

// ---- Hooks: what other mocks change ---------------------------------------------

const withTorrent = (hash: string) => active().filter((d) => d.torrents.some((t) => t.info_hash === hash));

/** A torrent was paused (B6): its downloads that still want bytes are `paused`. */
export function onTorrentPaused(hash: string): void {
  for (const record of withTorrent(hash)) {
    const { download } = record;
    if (download.state === 'paused' || !WANTING.has(download.state)) continue;
    record.before_pause = download.state;
    Object.assign(download, { state: 'paused', download_bps: 0, eta_seconds: null, resumes_at: null });
  }
}

/** A torrent was resumed: its paused downloads go back to what they were (one paused from the start downloads). */
export function onTorrentResumed(hash: string): void {
  for (const record of withTorrent(hash)) {
    if (record.download.state !== 'paused') continue;
    record.download.state = record.before_pause ?? 'downloading';
    record.before_pause = undefined;
  }
}

/** Nothing assigned any more: wants nothing. */
function noFile(record: MockDownload): void {
  record.torrents = [];
  record.over_cap = [];
  Object.assign(record.download, { state: 'no_file', wanted_bytes: 0, done_bytes: 0, download_bps: 0, eta_seconds: null, resumes_at: null });
  record.download.files = { total: 0, done: 0, unavailable: 0, over_cap: 0 };
}

/** A torrent was removed (B6 DELETE): downloads keep their rows; with nothing else assigned they're `no_file`. */
export function onTorrentRemoved(hash: string): void {
  for (const record of withTorrent(hash)) {
    record.torrents = record.torrents.filter((t) => t.info_hash !== hash);
    if (record.torrents.length === 0) {
      noFile(record);
      continue;
    }
    record.download.wanted_bytes = record.torrents.reduce((sum, t) => sum + t.bytes, 0);
    record.download.done_bytes = record.torrents.reduce((sum, t) => sum + t.done_bytes, 0);
  }
}

/** A movie's file changed: a torrent assigned (queued), an HDRezka stream (unavailable), or unassigned (no_file). */
export function onMovieFile(tmdbId: number, file: { info_hash: string; size_bytes: number } | 'stream' | null): void {
  for (const record of active()) {
    const { download } = record;
    if (download.scope !== 'movie' || download.item.tmdb_id !== tmdbId) continue;
    noFile(record);
    if (file === 'stream') Object.assign(download, { state: 'unavailable', files: { total: 1, done: 0, unavailable: 1, over_cap: 0 } });
    else if (file) {
      record.torrents = [{ info_hash: file.info_hash, bytes: file.size_bytes, done_bytes: 0 }];
      Object.assign(download, { state: 'queued', wanted_bytes: file.size_bytes, files: { total: 1, done: 0, unavailable: 0, over_cap: 0 } });
    }
  }
}

/** A library episode id's show and numbers, and its runtime (title-seeds: key × 100,000 + season × 1,000 + episode). */
function episodeOf(id: number): { tmdbId: number; season: number; episode: number; bytes: number } | undefined {
  const key = Math.floor(id / 100_000);
  const season = Math.floor((id % 100_000) / 1000);
  const episode = id % 1000;
  const found = Object.entries(SHOWS).find(([, show]) => show.key === key);
  const seed = found?.[1].seasons.find((s) => s.n === season)?.episodes.find((e) => e.n === episode);
  return found && seed ? { tmdbId: Number(found[0]), season, episode, bytes: seed.runtime * EPISODE_BYTES_PER_MINUTE } : undefined;
}

/** What's left of the kept cap now. */
const keptRoom = () => storageCaps(mockStorageDocument()).kept - downloadsUsage().committed_kept_bytes;

/**
 * An episode's file changed: a torrent file assigned (`bytes`; an applied mapping
 * job), an HDRezka stream, or unassigned (null). An episode download follows it;
 * a kept show or season takes a new file while the kept cap has room, in episode
 * order, and otherwise holds it over the cap (`storage_full`).
 */
export function onEpisodeFile(episodeId: number, file: { bytes: number } | 'stream' | null): void {
  const found = episodeOf(episodeId);
  if (!found) return;
  const key = episodeKey(found.season, found.episode);
  for (const record of active()) {
    const { download: d } = record;
    if (d.item.media_type !== 'tv' || d.item.tmdb_id !== found.tmdbId) continue;
    if (d.scope === 'episode') {
      if (d.item.season_number !== found.season || d.item.episode_number !== found.episode) continue;
      noFile(record);
      if (file === 'stream') Object.assign(d, { state: 'unavailable', files: { total: 1, done: 0, unavailable: 1, over_cap: 0 } });
      else if (file) Object.assign(d, { state: 'queued', wanted_bytes: file.bytes, files: { total: 1, done: 0, unavailable: 0, over_cap: 0 } });
      continue;
    }
    if (d.scope === 'season' && d.item.season_number !== found.season) continue;
    const wasOver = record.over_cap.includes(key);
    record.over_cap = record.over_cap.filter((k) => k !== key);
    if (file === null) {
      d.files.total = Math.max(0, d.files.total - 1);
      if (!wasOver) d.wanted_bytes = Math.max(0, d.wanted_bytes - found.bytes);
      if (d.files.total === 0) noFile(record);
      continue;
    }
    d.files.total += 1;
    if (file === 'stream') {
      d.files.unavailable += 1;
      continue;
    }
    // Admitted in episode order while the kept cap has room; the rest wait over it
    if (d.reason === 'kept' && keptRoom() < file.bytes) {
      record.over_cap.push(key);
      continue;
    }
    d.wanted_bytes += file.bytes;
    if (d.state === 'done' || d.state === 'no_file') d.state = 'queued';
  }
}

/** A title left the library: its downloads go with it (the FK cascades). */
export function onTitleRemoved(mediaType: MediaType, tmdbId: number): void {
  const page = current();
  page.items = page.items.filter((d) => d.download.item.media_type !== mediaType || d.download.item.tmdb_id !== tmdbId);
}
