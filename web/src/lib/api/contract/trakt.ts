import type { MediaType, Timestamp } from './common';
import type { LibraryDownload } from './library';
import type { NextEpisode, SimilarTitle } from './titles';

/** Trakt, read-only (B5; #8). PLAN §6 "Trakt (B5)" has the rules. */

/**
 * `off`: no account linked. `pending`: a device code waits for approval on trakt.tv.
 * `connected`: syncing works. `reauth`: Trakt stopped accepting the tokens; the imported data stays.
 */
export type TraktState = 'off' | 'pending' | 'connected' | 'reauth';
/**
 * Why the last device code didn't connect. `account_in_use`: that Trakt account is already connected to
 * another household member, decided after the code is approved.
 */
export type TraktConnectError = 'expired' | 'denied' | 'failed' | 'account_in_use';
export type TraktSyncStatus = 'idle' | 'syncing' | 'error';
/** `initial` is the first import after connecting (a full reconcile, with progress). */
export type TraktSyncKind = 'initial' | 'incremental' | 'reconcile';
/** The import's steps, in order. */
export type TraktSyncStep = 'history' | 'ratings' | 'watchlist' | 'progress' | 'details';

/** POST /integrations/trakt/connect (device-code flow), and `pending` while it waits. */
export interface TraktConnect {
  user_code: string;
  verification_url: string;
  expires_at: Timestamp;
  /** Seconds between the backend's checks. */
  interval: number;
}

/** The running sync, else the last one. */
export interface TraktSync {
  status: TraktSyncStatus;
  /** Null before the first sync. */
  kind: TraktSyncKind | null;
  /** The running step, or the one that failed; null while idle. */
  step: TraktSyncStep | null;
  /** What this run imported so far. `plays_total` is Trakt's count, 0 until it's known. */
  progress: { plays: number; plays_total: number; ratings: number; watchlist: number; shows: number };
  /** For the user, while `status` is error. */
  error: string | null;
  started_at: Timestamp | null;
  /** Null while syncing. */
  finished_at: Timestamp | null;
}

/** `settings` of GET /integrations/trakt, and what PUT /integrations/trakt/settings answers. Kept across a disconnect. */
export interface TraktSettings {
  /** Recommendations (#9) weigh Trakt ratings. */
  use_ratings: boolean;
  /** The watchlist row on For You, and Up next's watchlist starts. */
  show_watchlist: boolean;
  /** Up next keeps shows idle for over 180 days. */
  include_paused: boolean;
}

/** PUT /integrations/trakt/settings body: fields left out keep their value. */
export type TraktSettingsPatch = Partial<TraktSettings>;

/** GET /integrations/trakt */
export interface TraktIntegration {
  /** Trakt's client id and secret and a valid SECRET_KEY are set. */
  configured: boolean;
  state: TraktState;
  /** `state` is connected. */
  connected: boolean;
  /** While an account is linked: connected, reauth, or a code pending from reauth. */
  username: string | null;
  /** The live device code while `state` is pending. */
  pending: TraktConnect | null;
  /** Why the last code didn't connect; cleared by the next connect. */
  connect_error: TraktConnectError | null;
  /** Set while `state` is reauth, for the user. */
  reauth_reason: string | null;
  /** The last sync that finished without an error. */
  last_sync_at: Timestamp | null;
  /** The next scheduled check; null unless connected. */
  next_sync_at: Timestamp | null;
  sync: TraktSync;
  /** What's imported: history plays, ratings, watchlist titles and how many of those are in the library. */
  counts: { history: number; ratings: number; watchlist: number; watchlist_in_library: number };
  settings: TraktSettings;
}

/** A show in Up next and new episodes. */
export interface ShowRef {
  tmdb_id: number;
  title: string;
  poster_path: string | null;
  backdrop_path: string | null;
  /** Null when the show isn't in the library. */
  library_id: number | null;
}

/** An episode in Up next and new episodes: the landscape card's still, name and `#1124`. */
export interface EpisodeRef extends NextEpisode {
  still_path: string | null;
  /** TMDB's episode id; null when unknown. */
  tmdb_id: number | null;
}

/**
 * What an episode card offers (PLAN §6 "Trakt (B5)", first match): `ready`/`downloading`
 * (it has a file), `needs_review` (a mapping job wants it), `airs_on` (not aired yet),
 * `needs_torrent` (in the library, no file), `not_in_library`.
 */
export type UpNextAvailability = 'ready' | 'downloading' | 'needs_review' | 'needs_torrent' | 'not_in_library' | 'airs_on';
/** New episodes have aired: never `airs_on`. */
export type NewEpisodeAvailability = Exclude<UpNextAvailability, 'airs_on'>;

/** What the card's action needs; null fields don't apply to the availability. */
interface EpisodeCardState<A extends UpNextAvailability> {
  availability: A;
  /** The episode's file as Infuse sees it: set while ready or downloading. */
  webdav_path: string | null;
  webdav_url: string | null;
  /** While downloading ("Details" opens /activity/{info_hash}). */
  download: LibraryDownload | null;
  /** While needs_review ("Review" opens /mapping/{id}). */
  review_job_id: number | null;
}

/** GET /me/up-next (items, at most 40) */
export interface UpNextItem extends EpisodeCardState<UpNextAvailability> {
  show: ShowRef;
  next_episode: EpisodeRef;
  /** When it airs: set only while `airs_on` (null when the date is unknown). */
  airs_at: Timestamp | null;
  /** Regular episodes watched of aired, from Trakt; 0 of aired for a watchlist start. */
  progress: { completed: number; aired: number };
  /** Null for a watchlist start. */
  last_watched_at: Timestamp | null;
  /** `watchlist`: S01E01 of a watchlisted show you haven't started. */
  source: 'progress' | 'watchlist';
}

/** GET /me/new-episodes?days= (items, at most 30, newest first) */
export interface NewEpisodeItem extends EpisodeCardState<NewEpisodeAvailability> {
  show: ShowRef;
  episode: EpisodeRef;
  aired_at: Timestamp;
}

/** GET /me/watchlist?type= (items, newest first) */
export interface WatchlistItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
  listed_at: Timestamp;
  /** As `SimilarTitle.library`: null when not in the library. */
  library: SimilarTitle['library'];
}

/** GET /me/history?page=&page_size= (a Paginated<HistoryItem>, newest first) */
export interface HistoryItem {
  /** Trakt's history event id. */
  id: number;
  media_type: MediaType;
  /** The movie's or the show's. */
  tmdb_id: number;
  title: string;
  year: number | null;
  /** Episodes only; null for movies. */
  season: number | null;
  episode: number | null;
  episode_name: string | null;
  watched_at: Timestamp;
}
