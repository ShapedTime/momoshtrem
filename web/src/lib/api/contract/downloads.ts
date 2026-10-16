import type { MediaType, Timestamp } from './common';

/**
 * Downloads (PLAN §6 "Downloads (#77)"): a Keep (`kept`) or a smart pick
 * (`smart`) fetches a movie's, show's, season's or episode's torrent files in
 * the background and pins them on disk while it lasts. A show or season also
 * covers episodes assigned later. Releasing one (DELETE, expiry, watched)
 * unpins the data: it stays as ordinary cache and plays until it's evicted.
 * Sizes are bytes; what's on disk comes from the piece cache.
 */

export type DownloadReason = 'kept' | 'smart';
/** Why a smart pick was made: Trakt's Up next, or the episodes after the one streaming now. */
export type SmartSource = 'up_next' | 'streaming';
export type DownloadScope = 'movie' | 'show' | 'season' | 'episode';

/**
 * The first that matches: `unavailable` (nothing it covers can be downloaded: HDRezka only, or pure v2);
 * `no_file` (nothing assigned yet); `done` (all on disk); `paused` (the user paused its torrent, or
 * background downloads are off); `disk_full`; `waiting_for_window` (outside every download window);
 * `waiting_for_stream` (something else streams); `stalled` (no progress for 10 min, retried with backoff);
 * `downloading` (in one of the 2 slots); `queued`.
 */
export type DownloadState =
  | 'queued'
  | 'downloading'
  | 'waiting_for_stream'
  | 'waiting_for_window'
  | 'paused'
  | 'stalled'
  | 'disk_full'
  | 'done'
  | 'no_file'
  | 'unavailable';

/** What a download is for: the movie, or the show with its season and episode for those scopes. */
export interface DownloadItem {
  media_type: MediaType;
  tmdb_id: number;
  /** The movie's or the show's library id. */
  library_id: number;
  /** The movie's or the show's. */
  title: string;
  poster_path: string | null;
  /** Season and episode scopes. */
  season_number: number | null;
  /** Episode scope. */
  episode_number: number | null;
  episode_name: string | null;
}

/** GET /downloads (items), and every POST and PATCH answer. */
export interface Download {
  id: number;
  reason: DownloadReason;
  /** Null for kept downloads. */
  smart_source: SmartSource | null;
  scope: DownloadScope;
  item: DownloadItem;
  state: DownloadState;
  /** Over its torrent files (a file several episodes share counts once). */
  wanted_bytes: number;
  /** On disk, from the piece cache. */
  done_bytes: number;
  /** Smoothed, over its torrents; 0 unless downloading. */
  download_bps: number;
  /** Only while downloading with a speed. */
  eta_seconds: number | null;
  /** `waiting_for_window`: the next window's start; `stalled`: the next retry. */
  resumes_at: Timestamp | null;
  /** The assigned files it covers, those on disk, and those that can't be downloaded (HDRezka, pure v2). */
  files: { total: number; done: number; unavailable: number };
  /** Released once what it covers is watched (a Trakt play or a local session at 90%, after `created_at`). */
  until_watched: boolean;
  /** Null: no maximum. */
  expires_at: Timestamp | null;
  created_at: Timestamp;
}

/** Why background downloads wait now: `pause_while_streaming`, outside every window, or `background.enabled` off. */
export type DownloadsPausedReason = 'streaming' | 'window' | 'disabled';

/** GET /downloads */
export interface DownloadList {
  /** Kept first (newest first), then smart picks by show and episode order. */
  items: Download[];
  usage: {
    /** On disk now. */
    kept_bytes: number;
    smart_bytes: number;
    /** The wanted size of every kept download, on disk or not, as /settings/storage's. */
    committed_kept_bytes: number;
    /** The effective caps (/settings/storage's `kept.effective_max_bytes` and `smart.effective_max_bytes`). */
    kept_max_bytes: number;
    smart_max_bytes: number;
  };
  paused_reason: DownloadsPausedReason | null;
  /** `window`: the next window's start; `streaming`: a minute after the last stream ended (null while one streams); `disabled`: null. */
  resumes_at: Timestamp | null;
}

/** How long a Keep lasts. Left out, both come from /settings/storage's `kept` defaults ("until watched, 30 days at most"). */
export interface DownloadDuration {
  until_watched?: boolean;
  /** Null: no maximum. A time must be in the future. */
  expires_at?: Timestamp | null;
}

/** What a Keep is on. `id` is a library id: the movie's, the show's (show and season scopes) or the episode's. */
export type DownloadTarget =
  | { scope: 'movie'; id: number }
  | { scope: 'show'; id: number }
  | { scope: 'season'; id: number; season_number: number }
  | { scope: 'episode'; id: number };

/** POST /downloads: a Keep. */
export type DownloadCreate = DownloadTarget & DownloadDuration;

/** GET /downloads/estimate (the target as query params): what a Keep would need, before it's made. Writes nothing. */
export interface DownloadEstimate {
  /** What it would want: its torrent files. */
  wanted_bytes: number;
  /** What it would add to `committed_kept_bytes`: 0 when it's already kept, or another kept download has its files. */
  needed_bytes: number;
  /** `kept.effective_max_bytes − committed_kept_bytes`. More needed than this is POST's 409 `storage_full`. */
  available_bytes: number;
  /** False where POST answers 422 `not_downloadable` (it plays from HDRezka). */
  downloadable: boolean;
}

/** PATCH /downloads/{id}: kept downloads only (a smart pick is 422: Keep it first). */
export type DownloadPatch = DownloadDuration;

/** The extra fields of POST /downloads' `409 storage_full`. */
export interface StorageFullDetails {
  /** What the Keep adds to `committed_kept_bytes` (files another kept download has count nothing). */
  needed_bytes: number;
  /** `kept.effective_max_bytes − committed_kept_bytes`. */
  available_bytes: number;
}

/**
 * The download that holds a title, a season or an episode (PLAN §6 "Titles and library"). On an
 * episode it's the narrowest kept one (its own, its season's, its show's), else a smart pick, and
 * `state` and `pct` describe the episode's file: `done` and 100 once it's on disk.
 */
export interface Keep {
  download_id: number;
  reason: DownloadReason;
  scope: DownloadScope;
  state: DownloadState;
  /** 0–100: what's on disk of what it wants. */
  pct: number;
  until_watched: boolean;
  expires_at: Timestamp | null;
}
