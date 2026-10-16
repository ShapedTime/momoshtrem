import type { JobStatus } from './assignment';
import type { MediaType, Timestamp } from './common';

/**
 * Activity (PLAN §6 "Torrents and activity (B6)"; #98). Torrents stream on
 * demand: pieces are fetched around readers and kept in an LRU cache, and a
 * torrent loads on its first WebDAV read. So one the library knows but the
 * client hasn't loaded is `idle` with `loaded: false` and nulls, there's no
 * ETA, and progress can go down.
 *
 * Until #102 the legacy web route /api/torrents keeps answering the legacy
 * `{torrents}` shape (lib/api/momoshtrem.ts adapts the backend's items), and
 * the legacy routes under app/api/torrents/[hash] shadow summary, pause,
 * resume and the *-all routes on the real stack.
 */

/** The first rule that matches: paused; streaming (loaded, a reader open); downloading (loaded, network on, not complete); idle. */
export type TorrentState = 'streaming' | 'downloading' | 'paused' | 'idle';

/** A library title the torrent is for. */
export interface TorrentTitle {
  media_type: MediaType;
  tmdb_id: number;
  library_id: number;
  title: string;
  year: number;
  poster_path: string | null;
  /** Shows: "S01E29–E38", "Season 1", "Seasons 1–3", "S01E01–E05 +2"; null for movies and for a show only an open job links. */
  scope: string | null;
}

/**
 * A client streaming one of the torrent's files.
 * TODO(#99): the session's shape; the lists are always empty until then.
 */
export type StreamingSession = never;

/** GET /torrents (items) */
export interface TorrentListItem {
  /** 40 lowercase hex characters. Path params also take uppercase or base32. */
  info_hash: string;
  /** The info dict's name, else the newest job's torrent_name, else the magnet's dn, else the hash. */
  name: string;
  state: TorrentState;
  /** The torrent client has it. The fields only the client knows are null while it doesn't. */
  loaded: boolean;
  /** "Complete" only when true. */
  complete: boolean | null;
  /** 0–1 */
  progress: number | null;
  bytes_completed: number | null;
  size_bytes: number | null;
  /** Smoothed; 0 when not loaded. */
  download_bps: number;
  upload_bps: number;
  /** Connected peers; null when not loaded. */
  peers: number | null;
  seeders: number | null;
  /** The earliest active assignment or open job; the load time for a torrent nothing references. */
  added_at: Timestamp;
  /** The last read since the server started (in memory); null when not loaded. */
  last_active_at: Timestamp | null;
  /** Empty for a loaded torrent nothing references (an Inspect dry run, a WebDAV listing). */
  titles: TorrentTitle[];
  /** The newest open, non-dry-run mapping job. */
  open_job: { id: number; status: JobStatus } | null;
  streaming: StreamingSession[];
}

/** GET /torrents/summary */
export interface TorrentsSummary {
  download_bps: number;
  upload_bps: number;
  /** Connected peers over the loaded torrents. */
  peers: number;
  /** The last 60 s of total speeds, oldest first: one sample per interval, fewer right after startup. */
  history: { interval_seconds: number; download_bps: number[]; upload_bps: number[] };
  counts: Record<TorrentState, number>;
  /** The metadata folder's file system and the piece cache on it; null when it can't be read. */
  disk: { total_bytes: number; free_bytes: number; torrents_bytes: number; cache_limit_bytes: number } | null;
  streaming: StreamingSession[];
}

/** POST /torrents/pause-all and /resume-all */
export interface TorrentsPauseCount {
  count: number;
}
