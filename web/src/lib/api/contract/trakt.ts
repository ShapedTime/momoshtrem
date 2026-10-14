import type { MediaType, Timestamp } from './common';

/** Trakt, read-only (B5; #8). */

export type UpNextAvailability = 'ready' | 'downloading' | 'needs_torrent' | 'not_in_library' | 'airs_on';

/** GET /integrations/trakt */
export interface TraktIntegration {
  configured: boolean;
  connected: boolean;
  username: string | null;
  last_sync_at: Timestamp | null;
  sync_status: 'idle' | 'syncing' | 'error';
  counts: { history: number; ratings: number; watchlist: number };
}

/** POST /integrations/trakt/connect (device-code flow) */
export interface TraktConnect {
  user_code: string;
  verification_url: string;
  expires_at: Timestamp;
  interval: number;
}

export interface ShowRef {
  tmdb_id: number;
  title: string;
  poster_path: string | null;
  backdrop_path: string | null;
}

export interface EpisodeRef {
  season: number;
  episode: number;
  absolute_number: number | null;
  name: string;
  still_path: string | null;
}

/** GET /me/up-next (items) */
export interface UpNextItem {
  show: ShowRef;
  next_episode: EpisodeRef;
  availability: UpNextAvailability;
  airs_at: Timestamp | null;
  progress: { completed: number; aired: number };
  last_watched_at: Timestamp | null;
}

/** GET /me/watchlist?type= (items) */
export interface WatchlistItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
  listed_at: Timestamp;
  in_library: boolean;
  availability: UpNextAvailability;
}

/** GET /me/history?page= (a Paginated<HistoryItem>) */
export interface HistoryItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  season: number | null;
  episode: number | null;
  watched_at: Timestamp;
}
