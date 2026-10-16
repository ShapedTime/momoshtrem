import type { JobStatus } from './assignment';
import type { MediaType, Timestamp } from './common';
import type { DownloadReason, DownloadScope, DownloadState } from './downloads';

/**
 * Activity (PLAN §6 "Torrents and activity (B6)"; #98, #99, #77). Torrents
 * still stream on demand: pieces are fetched around readers and kept in an LRU
 * cache, and a torrent loads on its first WebDAV read, a mapping job, or when
 * the download manager takes it. Downloads (#77) fetch the files they want in
 * the background and pin them, so only those have an ETA (`wanted`); a
 * torrent nothing keeps can still lose progress. One the library knows but
 * the client hasn't loaded is `idle` with `loaded: false` and nulls.
 */

/**
 * The first rule that matches: paused; streaming (loaded, a streaming session); downloading (loaded,
 * network on, and `wanted.state` is `downloading` or `download_bps > 0`: a queued or waiting torrent
 * isn't); idle.
 */
export type TorrentState = 'streaming' | 'downloading' | 'paused' | 'idle';

/** The download manager's state for one torrent: `Download.state` without the ones about a download's files. */
export type TorrentWantedState = Exclude<DownloadState, 'no_file' | 'unavailable' | 'storage_full'>;

/** What downloads want of a torrent (#77). */
export interface TorrentWanted {
  /** Its wanted files' total. */
  bytes: number;
  /** What's on disk of them, from the piece cache (not anacrolix's completion: the LRU can contradict it). */
  done_bytes: number;
  /** While it downloads with a speed. */
  eta_seconds: number | null;
  state: TorrentWantedState;
  /** As `Download.resumes_at`: the next window's start, or a stalled torrent's next retry. */
  resumes_at: Timestamp | null;
}

/** A download that wants a torrent's files (#77). */
export interface TorrentHold {
  download_id: number;
  reason: DownloadReason;
  scope: DownloadScope;
  /** The movie's or the show's title. */
  title: string;
}

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

/** From the User-Agent; `Unknown` when it names none of these. */
export type StreamingClient = 'Infuse' | 'VLC' | 'Safari' | 'Kodi' | 'Unknown';

/** The library item a session plays: a movie, or a show's episode. */
export interface StreamingSessionItem {
  media_type: MediaType;
  tmdb_id: number;
  /** The movie's or the show's library id. */
  library_id: number;
  title: string;
  season_number: number | null;
  episode_number: number | null;
  episode_name: string | null;
}

/**
 * A client streaming a file over WebDAV (#99). Reads with the same remote address,
 * User-Agent and file are one session (Infuse opens several Range GETs per playback);
 * it ends 30 s after its last read. Sniffs (under 64 KB, closed within 2 s) and
 * PROPFIND/HEAD opens never are sessions.
 */
export interface StreamingSession {
  id: string;
  source: 'torrent' | 'hdrezka';
  /** Null for HDRezka. */
  info_hash: string | null;
  /** Null when the library doesn't know the file. */
  item: StreamingSessionItem | null;
  /** The name WebDAV shows ("Shōgun - S01E05 - Broken to the Fist.mkv"). */
  file_name: string;
  /** The file's path inside the torrent, as TorrentDetail.files[].path; null for HDRezka. */
  file_path: string | null;
  /** Null without a User-Agent. */
  client: StreamingClient | null;
  /** "Apple TV", "iPhone", "iPad", "Mac", "Android", "Windows", "Linux" when the User-Agent says; Infuse's own usually doesn't. */
  device: string | null;
  /** The end of the last read: where the client reads now (ahead of what plays by its buffer). */
  offset_bytes: number;
  size_bytes: number;
  /** offset ÷ size × runtime; null without a runtime. */
  position_seconds: number | null;
  duration_seconds: number | null;
  /** Smoothed (EWMA, 5 s). */
  rate_bps: number;
  /** Complete bytes contiguous from the offset; null for HDRezka. The seconds need a runtime too. */
  buffered_ahead_bytes: number | null;
  buffered_ahead_seconds: number | null;
  started_at: Timestamp;
  last_read_at: Timestamp;
}

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
  /** The last read, resume, load or download-manager wake since the server started (in memory); null when not loaded. */
  last_active_at: Timestamp | null;
  /** Empty for a loaded torrent nothing references (an Inspect dry run, a WebDAV listing). */
  titles: TorrentTitle[];
  /** The newest open, non-dry-run mapping job. */
  open_job: { id: number; status: JobStatus } | null;
  /** Its files' sessions, the most recently read first. */
  streaming: StreamingSession[];
  /** What downloads want of it (#77); null when nothing does. Progress and ETA for Downloading come from here. */
  wanted: TorrentWanted | null;
  /** The downloads that want it, kept first, then by id: the Kept and Smart pills. */
  holds: TorrentHold[];
}

/** A run of pieces in piece order. `priority`: not complete, priority ≥ High (the streaming windows), not started. */
export interface PieceRun {
  state: 'have' | 'partial' | 'priority' | 'missing';
  count: number;
}

/** A library item a torrent file plays as. */
export interface TorrentFilePlaysAs {
  media_type: MediaType;
  /** The movie's or the episode's library id. */
  item_id: number;
  season_number: number | null;
  episode_number: number | null;
  /** Null for movies and episodes without a name. */
  episode_name: string | null;
  /** "S01E29" or "Movie". */
  label: string;
}

/** One of a torrent's files, in its order. */
export interface TorrentDetailFile {
  /** The torrent's own path: "{name}/…" in a multi-file torrent. */
  path: string;
  size_bytes: number;
  /** 0–1; null when not loaded. */
  progress: number | null;
  /** From its active assignments: a movie, or its episodes in order. */
  plays_as: TorrentFilePlaysAs[];
  /**
   * Why the newest job left it out; null when it plays as something or no job ignored it.
   * `reason`: the extra's type (opening, ending, sample, …), `subtitle`, or `user`.
   */
  ignored: { reason: string; label: string } | null;
}

/** anacrolix keeps its announce results to itself: `unknown`, with the rest null. */
export interface TorrentTracker {
  url: string;
  status: 'working' | 'error' | 'unknown';
  error: string | null;
  seeders: number | null;
  peers: number | null;
  last_announce_at: Timestamp | null;
}

/**
 * GET /torrents/{hash} (#99): the list item plus the detail page's facts. It never loads
 * the torrent: from the client when loaded, else its stored metainfo, else the newest
 * job's file list. 400 for a malformed hash, 404 for one nothing knows.
 */
export interface TorrentDetail extends TorrentListItem {
  magnet_uri: string | null;
  piece_length: number | null;
  pieces_total: number | null;
  /** Null when not loaded. */
  pieces_have: number | null;
  /** Run-length in piece order; null when not loaded. */
  pieces: PieceRun[] | null;
  files: TorrentDetailFile[];
  trackers: TorrentTracker[];
  /** Since it loaded; these, ratio and swarm are null when it isn't. */
  downloaded_bytes: number | null;
  uploaded_bytes: number | null;
  /** uploaded ÷ downloaded; null before anything arrived. */
  ratio: number | null;
  /** total_seeders needs a tracker scrape: null for now. */
  swarm: { connected_peers: number; connected_seeders: number; total_seeders: number | null } | null;
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
  /**
   * The metadata folder's file system and the piece cache on it; null when it can't be read.
   * `torrents_bytes` is the cache's fill (kept, smart and streaming cache together) and `cache_limit_bytes`
   * the cap it applies now (/settings/storage's `effective_bytes`); `kept_bytes` and `smart_bytes` are
   * what's pinned on disk for downloads (#77).
   */
  disk: {
    total_bytes: number;
    free_bytes: number;
    torrents_bytes: number;
    cache_limit_bytes: number;
    kept_bytes: number;
    smart_bytes: number;
  } | null;
  /** Every session (HDRezka too), the most recently read first: "Streaming now" reads streaming[0]. */
  streaming: StreamingSession[];
}

/** POST /torrents/pause-all and /resume-all */
export interface TorrentsPauseCount {
  count: number;
}
