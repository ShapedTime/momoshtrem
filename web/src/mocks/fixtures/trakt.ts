import type {
  EpisodeRef,
  HistoryItem,
  MediaType,
  NewEpisodeAvailability,
  NewEpisodeItem,
  Paginated,
  ShowRef,
  SystemStatus,
  TraktConnect,
  TraktConnectError,
  TraktIntegration,
  TraktSettings,
  TraktSettingsPatch,
  TraktState,
  TraktSync,
  TraktSyncKind,
  TraktSyncStep,
  UpNextAvailability,
  UpNextItem,
  WatchlistItem,
} from '@/lib/api/contract';
import { type MockScenario, scenario } from '../scenario';
import { findTitle, iso } from './catalog';
import { buildSeason, buildTitle, episodeFile, libraryKey, type MockLibrary, nextEpisodeOf, webdavUrl } from './titles';
import { type EpisodeKey, MOVIES, seedEpisode, SHOWS, type ShowSeed } from './title-seeds';

/**
 * The mock Trakt account (teymur) and the integration's state machine
 * (PLAN §6 "Trakt (B5)"). The account's plays are the seeds' `watched`
 * episodes and `watch_state`s (title-seeds.ts); Up next names each show's
 * `next_up`, and every card's availability comes from the page's library,
 * so adding a show or applying a mapping job changes it.
 *
 * The state is per page, kept here rather than in handlers/trakt.ts so
 * fixtures/system.ts can read it without msw. The `trakt-*`, `first-time`,
 * `up-next-empty` and `me-error` scenarios pick the starting state.
 */

const DAY = 86_400_000;
const MINUTE = 60_000;
/** The mock's today: the seeds' air dates are relative to it. */
const TODAY = iso(0).slice(0, 10);

export const TRAKT_USERNAME = 'teymur';
export const MAX_UP_NEXT = 40;
export const MAX_NEW_EPISODES = 30;
const IDLE_DAYS = 180;
const MAX_WATCHLIST_STARTS = 3;
const SYNC_EVERY_MS = 30 * MINUTE;
export const SYNC_STEPS: readonly TraktSyncStep[] = ['history', 'ratings', 'watchlist', 'progress', 'details'];

export const DEFAULT_TRAKT_SETTINGS: TraktSettings = { use_ratings: true, show_watchlist: true, include_paused: false };

/** What `POST /integrations/trakt/connect` hands out. */
export const DEVICE_CODE = { user_code: 'A1B2C3D4', verification_url: 'https://trakt.tv/activate', interval: 5, ttl_ms: 10 * MINUTE };

// ---- The account ----------------------------------------------------------------

/** The Trakt watchlist, newest first. */
const WATCHLIST: Array<{ media_type: MediaType; tmdb_id: number; listed_days_ago: number }> = [
  { media_type: 'movie', tmdb_id: 666277, listed_days_ago: 4 }, // Past Lives: not in the library
  { media_type: 'movie', tmdb_id: 1054867, listed_days_ago: 10 }, // One Battle After Another: needs a torrent
  { media_type: 'tv', tmdb_id: 82728, listed_days_ago: 20 }, // Bluey: unstarted, so Up next's watchlist start
  { media_type: 'tv', tmdb_id: 66573, listed_days_ago: 60 }, // The Good Place: not in the library
];

/** Plays the seeds can't hold: episodes the library has no file for. */
const TRAKT_ONLY_PLAYS: Array<{ tmdb_id: number; season: number; episodes: number[] }> = [
  { tmdb_id: 66573, season: 1, episodes: [1, 2, 3, 4, 5] },
];

const showSeeds = () => Object.entries(SHOWS).map(([id, show]) => [Number(id), show] as const);

/** Shows with Trakt progress: Up next's candidates. */
const PROGRESS_SHOWS = showSeeds().filter(([, show]) => show.watch_state?.progress && show.next_up);

const RATINGS = [...Object.values(SHOWS), ...Object.values(MOVIES)].filter((seed) => seed.watch_state?.rating != null).length;

/** Every play, newest first: each show's last play at its `watched_at`, the earlier ones a day apart. */
function buildHistory(): HistoryItem[] {
  const plays: Array<Omit<HistoryItem, 'id'>> = [];
  for (const [tmdbId, show] of showSeeds()) {
    const entry = findTitle('tv', tmdbId);
    const last = show.watch_state?.watched_at;
    if (!entry || !last) continue;
    const watched = show.seasons.flatMap((season) =>
      season.episodes.filter((e) => e.state === 'watched').map((e) => ({ season: season.n, episode: e.n, name: e.name }))
    );
    for (const extra of TRAKT_ONLY_PLAYS.filter((p) => p.tmdb_id === tmdbId)) {
      for (const n of extra.episodes) watched.push({ season: extra.season, episode: n, name: seedEpisode(show, [extra.season, n])?.name ?? `Episode ${n}` });
    }
    watched.forEach((e, i) =>
      plays.push({
        media_type: 'tv',
        tmdb_id: tmdbId,
        title: entry.title,
        year: entry.year,
        season: e.season,
        episode: e.episode,
        episode_name: e.name,
        watched_at: new Date(Date.parse(last) - (watched.length - 1 - i) * DAY).toISOString(),
      })
    );
  }
  for (const [id, movie] of Object.entries(MOVIES)) {
    const entry = findTitle('movie', Number(id));
    const state = movie.watch_state;
    if (!entry || !state?.watched_at) continue;
    for (let i = 0; i < state.plays; i++) {
      const at = new Date(Date.parse(state.watched_at) - i * 400 * DAY).toISOString();
      plays.push({ media_type: 'movie', tmdb_id: entry.tmdb_id, title: entry.title, year: entry.year, season: null, episode: null, episode_name: null, watched_at: at });
    }
  }
  plays.sort((a, b) => b.watched_at.localeCompare(a.watched_at));
  return plays.map((play, i) => ({ id: 9_000_000 - i, ...play }));
}

const HISTORY = buildHistory();

// ---- Lists ------------------------------------------------------------------------

const aired = (airDate: string | null) => airDate !== null && airDate <= TODAY;
const midnight = (date: string | null) => (date === null ? null : `${date}T00:00:00.000Z`);

interface Card {
  show: ShowRef;
  episode: EpisodeRef;
  availability: UpNextAvailability;
  webdav_path: string | null;
  webdav_url: string | null;
  download: UpNextItem['download'];
  review_job_id: number | null;
}

/**
 * A show's episode as a card, against the page's library (PLAN §6 "Trakt (B5)",
 * first match): a file (ready or downloading), a mapping job that wants it,
 * not aired yet, no file, not in the library. Null when the seeds don't list it.
 */
function episodeCard(library: MockLibrary, tmdbId: number, key: EpisodeKey): Card | null {
  const show = SHOWS[tmdbId];
  const entry = findTitle('tv', tmdbId);
  const next = show && nextEpisodeOf(show, key);
  if (!entry || !next) return null;
  const row = library.get(libraryKey('tv', tmdbId));
  const card: Card = {
    show: { tmdb_id: tmdbId, title: entry.title, poster_path: null, backdrop_path: null, library_id: row?.library_id ?? null },
    episode: { ...next, still_path: null, tmdb_id: null },
    availability: 'needs_torrent',
    webdav_path: null,
    webdav_url: null,
    download: null,
    review_job_id: null,
  };
  const episode = row && buildSeason(tmdbId, key[0], library)?.episodes.find((e) => e.episode_number === key[1]);
  if (episode?.webdav_path) {
    const downloading = episode.state === 'downloading';
    return {
      ...card,
      availability: downloading ? 'downloading' : 'ready',
      webdav_path: episode.webdav_path,
      webdav_url: webdavUrl(episode.webdav_path),
      download: downloading ? { pct: episode.pct ?? 0, info_hash: episodeFile(library, episode.id)?.info_hash ?? '' } : null,
    };
  }
  if (episode?.state === 'needs_review') return { ...card, availability: 'needs_review', review_job_id: row?.review.job_id ?? null };
  if (!aired(next.air_date)) return { ...card, availability: 'airs_on' };
  return { ...card, availability: row ? 'needs_torrent' : 'not_in_library' };
}

/** Regular episodes that have aired. */
const airedRegular = (show: ShowSeed) =>
  show.seasons.filter((s) => s.n > 0).reduce((sum, s) => sum + s.episodes.filter((e) => aired(e.air_date)).length, 0);

const byLastWatched = (a: UpNextItem, b: UpNextItem) => (b.last_watched_at ?? '').localeCompare(a.last_watched_at ?? '');
const byAirsAt = (a: UpNextItem, b: UpNextItem) => (a.airs_at ?? '9999').localeCompare(b.airs_at ?? '9999');

/** GET /me/up-next: actionable shows by last watched, then watchlist starts, then what airs next. */
export function upNextItems(library: MockLibrary, settings: TraktSettings): UpNextItem[] {
  const item = (card: Card, rest: Pick<UpNextItem, 'progress' | 'last_watched_at' | 'source'>): UpNextItem => {
    const { episode, ...fields } = card;
    return { ...fields, next_episode: episode, airs_at: card.availability === 'airs_on' ? midnight(episode.air_date) : null, ...rest };
  };
  const fromProgress = PROGRESS_SHOWS.flatMap(([tmdbId, show]) => {
    const state = show.watch_state;
    const card = show.next_up && episodeCard(library, tmdbId, show.next_up);
    if (!state?.progress || !card) return [];
    const idle = state.watched_at !== null && Date.parse(TODAY) - Date.parse(state.watched_at) > IDLE_DAYS * DAY;
    if (idle && !settings.include_paused) return [];
    return [item(card, { progress: state.progress, last_watched_at: state.watched_at, source: 'progress' })];
  });
  const starts = !settings.show_watchlist
    ? []
    : WATCHLIST.filter((w) => w.media_type === 'tv' && library.has(libraryKey('tv', w.tmdb_id)) && !SHOWS[w.tmdb_id]?.watch_state)
        .flatMap((w) => {
          const card = episodeCard(library, w.tmdb_id, [1, 1]);
          return card && card.availability !== 'airs_on'
            ? [item(card, { progress: { completed: 0, aired: airedRegular(SHOWS[w.tmdb_id]) }, last_watched_at: null, source: 'watchlist' })]
            : [];
        })
        .slice(0, MAX_WATCHLIST_STARTS);
  const actionable = fromProgress.filter((i) => i.availability !== 'airs_on').sort(byLastWatched);
  const upcoming = fromProgress.filter((i) => i.availability === 'airs_on').sort(byAirsAt);
  return [...actionable, ...starts, ...upcoming].slice(0, MAX_UP_NEXT);
}

/**
 * GET /me/new-episodes?days=: regular episodes that aired in the last `days`
 * days, of library shows and (with Trakt) the shows Trakt follows, that
 * Trakt hasn't seen watched. Newest first.
 */
export function newEpisodeItems(library: MockLibrary, days: number, withTrakt: boolean): NewEpisodeItem[] {
  const from = new Date(Date.parse(TODAY) - days * DAY).toISOString().slice(0, 10);
  const shows = new Set(showSeeds().map(([id]) => id).filter((id) => library.has(libraryKey('tv', id)) || (withTrakt && SHOWS[id].watch_state)));
  const items: NewEpisodeItem[] = [];
  for (const tmdbId of shows) {
    for (const season of SHOWS[tmdbId].seasons.filter((s) => s.n > 0)) {
      for (const seed of season.episodes) {
        if (!seed.air_date || seed.air_date < from || seed.air_date > TODAY || (withTrakt && seed.state === 'watched')) continue;
        const card = episodeCard(library, tmdbId, [season.n, seed.n]);
        if (!card || card.availability === 'airs_on') continue;
        items.push({ ...card, availability: card.availability as NewEpisodeAvailability, aired_at: midnight(seed.air_date) ?? '' });
      }
    }
  }
  return items
    .sort((a, b) => b.aired_at.localeCompare(a.aired_at) || a.show.title.localeCompare(b.show.title) || a.episode.episode - b.episode.episode)
    .slice(0, MAX_NEW_EPISODES);
}

/** GET /me/watchlist?type=, newest first, with each title's library state. */
export function watchlistItems(library: MockLibrary, type?: MediaType): WatchlistItem[] {
  return WATCHLIST.filter((w) => !type || w.media_type === type).flatMap((w) => {
    const title = buildTitle(w.media_type, w.tmdb_id, library);
    if (!title) return [];
    const state = title.library;
    return [
      {
        media_type: w.media_type,
        tmdb_id: w.tmdb_id,
        title: title.title,
        year: title.year,
        poster_path: null,
        listed_at: iso(w.listed_days_ago),
        library:
          state.library_id === null
            ? null
            : { library_id: state.library_id, availability_stage: state.availability_stage, episodes_total: state.episodes_total, episodes_ready: state.episodes_ready },
      },
    ];
  });
}

export const DEFAULT_HISTORY_PAGE_SIZE = 50;

/** GET /me/history: every play, newest first. */
export function historyPage(page: number, pageSize = DEFAULT_HISTORY_PAGE_SIZE, withTrakt = true): Paginated<HistoryItem> {
  const all = withTrakt ? HISTORY : [];
  return { items: all.slice((page - 1) * pageSize, page * pageSize), total: all.length, page, page_size: pageSize };
}

// ---- The integration's state machine ----------------------------------------------

interface MockTraktAccount {
  /** The scenario it started from: another one starts over. */
  scenario: MockScenario | null;
  configured: boolean;
  state: TraktState;
  /** An account is linked (connected, reauth, or a code pending from reauth): its data is kept. */
  linked: boolean;
  pending: TraktConnect | null;
  /** GETs since the code was handed out: the second one connects. */
  reads: number;
  connectError: TraktConnectError | null;
  reauthReason: string | null;
  /** The import's steps that are done: /me shows what they brought. */
  imported: Set<TraktSyncStep>;
  sync: Omit<TraktSync, 'progress'>;
  /** `trakt-syncing`: the initial import stays at this step. */
  hold: TraktSyncStep | null;
  lastSyncAt: string | null;
  settings: TraktSettings;
}

const at = (ms: number) => new Date(ms).toISOString();
const neverSynced = (): MockTraktAccount['sync'] => ({ status: 'idle', kind: null, step: null, error: null, started_at: null, finished_at: null });
const running = (kind: TraktSyncKind, step: TraktSyncStep, now: number): MockTraktAccount['sync'] => ({
  status: 'syncing',
  kind,
  step,
  error: null,
  started_at: at(now),
  finished_at: null,
});

function startingAccount(from: MockScenario | null, now: number): MockTraktAccount {
  const synced = now - 4 * MINUTE;
  const account: MockTraktAccount = {
    scenario: from,
    configured: true,
    state: 'connected',
    linked: true,
    pending: null,
    reads: 0,
    connectError: null,
    reauthReason: null,
    imported: new Set(SYNC_STEPS),
    sync: { status: 'idle', kind: 'incremental', step: null, error: null, started_at: at(synced - 3000), finished_at: at(synced) },
    hold: null,
    lastSyncAt: at(synced),
    settings: { ...DEFAULT_TRAKT_SETTINGS },
  };
  const off: Partial<MockTraktAccount> = { state: 'off', linked: false, imported: new Set(), sync: neverSynced(), lastSyncAt: null };
  switch (from) {
    case 'trakt-off':
    case 'trakt-denied':
    case 'first-time':
      return { ...account, ...off };
    case 'trakt-unconfigured':
      return { ...account, ...off, configured: false };
    case 'trakt-syncing':
      return {
        ...account,
        imported: new Set(['history', 'ratings', 'watchlist', 'progress']),
        sync: running('initial', 'details', now - 40_000),
        hold: 'details',
        lastSyncAt: null,
      };
    case 'trakt-sync-error':
      return {
        ...account,
        imported: new Set(['history', 'ratings']),
        sync: {
          status: 'error',
          kind: 'initial',
          step: 'watchlist',
          error: 'Trakt didn’t answer while importing your watchlist (502).',
          started_at: at(now - 2 * MINUTE),
          finished_at: at(now - MINUTE),
        },
        lastSyncAt: null,
      };
    case 'trakt-reauth': {
      const lastSync = now - 2 * DAY;
      return {
        ...account,
        state: 'reauth',
        reauthReason: 'Trakt stopped accepting momoshtrem’s sign-in. Connect again to keep syncing.',
        sync: { status: 'idle', kind: 'incremental', step: null, error: null, started_at: at(lastSync - 3000), finished_at: at(lastSync) },
        lastSyncAt: at(lastSync),
      };
    }
    default:
      return account;
  }
}

let account: MockTraktAccount | undefined;

function current(now = Date.now()): MockTraktAccount {
  const from = scenario();
  if (!account || account.scenario !== from) account = startingAccount(from, now);
  return account;
}

/** Tests: back to the starting account (it's rebuilt for the scenario on the next request). */
export function resetMockTrakt(): void {
  account = undefined;
}

/** The imported data for a step's section is there (/me reads it). */
export const traktImported = (step: TraktSyncStep): boolean => current().imported.has(step);

export const traktSettings = (): TraktSettings => current().settings;

/** For GET /status. */
export function traktStatus(): SystemStatus['trakt'] {
  const a = current();
  return { configured: a.configured, connected: a.state === 'connected' };
}

function view(a: MockTraktAccount, library: MockLibrary, now: number): TraktIntegration {
  const has = (step: TraktSyncStep) => a.imported.has(step);
  const watchlist = has('watchlist') ? watchlistItems(library) : [];
  return {
    configured: a.configured,
    state: a.state,
    connected: a.state === 'connected',
    username: a.linked ? TRAKT_USERNAME : null,
    pending: a.state === 'pending' ? a.pending : null,
    connect_error: a.connectError,
    reauth_reason: a.state === 'reauth' ? a.reauthReason : null,
    last_sync_at: a.lastSyncAt,
    next_sync_at: a.state === 'connected' ? at(Date.parse(a.lastSyncAt ?? at(now)) + SYNC_EVERY_MS) : null,
    sync: {
      ...a.sync,
      progress: {
        plays: has('history') ? HISTORY.length : 0,
        plays_total: a.sync.kind === null ? 0 : HISTORY.length,
        ratings: has('ratings') ? RATINGS : 0,
        watchlist: watchlist.length,
        shows: has('progress') ? PROGRESS_SHOWS.length : 0,
      },
    },
    counts: {
      history: has('history') ? HISTORY.length : 0,
      ratings: has('ratings') ? RATINGS : 0,
      watchlist: watchlist.length,
      watchlist_in_library: watchlist.filter((w) => w.library !== null).length,
    },
    settings: a.settings,
  };
}

/** One step of the running import: an initial import walks the steps, any other sync is one step. */
function advance(a: MockTraktAccount, now: number): void {
  const step = a.sync.step ?? 'history';
  if (a.sync.kind === 'initial') {
    a.imported.add(step);
    const next = SYNC_STEPS[SYNC_STEPS.indexOf(step) + 1];
    if (next) {
      a.sync = { ...a.sync, step: next };
      return;
    }
  } else {
    a.imported = new Set(SYNC_STEPS);
  }
  a.sync = { ...a.sync, status: 'idle', step: null, finished_at: at(now) };
  a.lastSyncAt = at(now);
}

/**
 * GET /integrations/trakt. Reading moves the machine on: the second read after
 * a connect approves the code (`trakt-denied`: denies it) and starts the
 * initial import; every later read imports one step.
 */
export function readTraktIntegration(library: MockLibrary, now = Date.now()): TraktIntegration {
  const a = current(now);
  if (a.state === 'pending' && a.pending) {
    const expired = Date.parse(a.pending.expires_at) <= now;
    if (expired || ++a.reads >= 2) {
      a.pending = null;
      if (expired || a.scenario === 'trakt-denied') {
        a.state = a.linked ? 'reauth' : 'off';
        a.connectError = expired ? 'expired' : 'denied';
      } else {
        a.state = 'connected';
        a.linked = true;
        a.reauthReason = null;
        a.sync = running('initial', 'history', now);
      }
    }
  } else if (a.sync.status === 'syncing' && a.sync.step !== a.hold) {
    advance(a, now);
  }
  return view(a, library, now);
}

export type TraktResult<T> = { ok: true; value: T } | { ok: false; status: number; code: string; message: string };

/** POST /integrations/trakt/connect: a device code; the live one again while it lasts. */
export function connectTrakt(now = Date.now()): TraktResult<TraktConnect> {
  const a = current(now);
  if (!a.configured) {
    return { ok: false, status: 503, code: 'unavailable', message: 'Trakt isn’t set up: set TRAKT_CLIENT_ID, TRAKT_CLIENT_SECRET and SECRET_KEY.' };
  }
  if (a.state === 'connected') return { ok: false, status: 409, code: 'conflict', message: 'Trakt is already connected.' };
  if (a.state === 'pending' && a.pending && Date.parse(a.pending.expires_at) > now) return { ok: true, value: a.pending };
  const { ttl_ms, ...code } = DEVICE_CODE;
  const pending: TraktConnect = { ...code, expires_at: at(now + ttl_ms) };
  a.state = 'pending';
  a.pending = pending;
  a.reads = 0;
  a.connectError = null;
  return { ok: true, value: pending };
}

/** POST /integrations/trakt/sync: starts one (a failed initial import picks up where it failed) or joins the running one. */
export function startTraktSync(library: MockLibrary, now = Date.now()): TraktResult<TraktIntegration> {
  const a = current(now);
  if (a.state !== 'connected') return { ok: false, status: 409, code: 'conflict', message: 'Connect Trakt first.' };
  if (a.sync.status !== 'syncing') {
    const retry = a.sync.status === 'error' && a.sync.kind === 'initial';
    a.sync = retry ? running('initial', a.sync.step ?? 'history', now) : running('incremental', 'history', now);
  }
  return { ok: true, value: view(a, library, now) };
}

/**
 * DELETE /integrations/trakt: a pending code is cancelled (back to off, or to
 * reauth while an account is linked); otherwise the sync stops and the
 * imported data goes. The settings stay.
 */
export function disconnectTrakt(): void {
  const a = current();
  if (a.state === 'pending') {
    a.state = a.linked ? 'reauth' : 'off';
    a.pending = null;
    a.reads = 0;
    return;
  }
  account = { ...startingAccount('trakt-off', Date.now()), scenario: a.scenario, configured: a.configured, settings: a.settings };
}

/** PUT /integrations/trakt/settings: the fields it sets; kept across a disconnect. */
export function updateTraktSettings(patch: TraktSettingsPatch): TraktSettings {
  const a = current();
  a.settings = { ...a.settings, ...patch };
  return a.settings;
}
