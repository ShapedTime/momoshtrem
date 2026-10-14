import type { MediaType, Timestamp } from './common';

/** Activity (B6; #7). The legacy web route /api/torrents keeps its own shape until #7. */

export type TorrentState = 'downloading' | 'streaming' | 'seeding' | 'paused' | 'idle';

export interface TorrentTitleLink {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  /** e.g. "S01E29–E38" */
  scope: string | null;
}

/** GET /torrents (items) */
export interface TorrentListItem {
  info_hash: string;
  name: string;
  state: TorrentState;
  progress: number;
  size_bytes: number;
  download_speed: number;
  upload_speed: number;
  peers: number;
  seeders: number;
  titles: TorrentTitleLink[];
  added_at: Timestamp;
}

/** GET /torrents/summary (HANDOFF) */
export interface TorrentsSummary {
  down_bps: number;
  up_bps: number;
  active: number;
  idle: number;
  paused: number;
  disk: { used: number; free: number };
}
