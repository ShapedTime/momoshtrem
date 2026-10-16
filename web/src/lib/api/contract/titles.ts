import type { MediaType, Timestamp } from './common';

/**
 * Detail pages (B1; #4). Fields marked HANDOFF come from design-cinematic/HANDOFF.md §6.
 * The availability rules (stages, the "frontier", default season) are in PLAN §6.
 */

export type AvailabilityStage = 'none' | 'needs_torrent' | 'downloading' | 'ready';
export type EpisodeState = 'watched' | 'ready' | 'downloading' | 'missing' | 'upcoming' | 'needs_review';
export type EpisodeOrdering = 'aired' | 'absolute' | 'episode_group';
export type ClassifierMode = 'auto' | 'rules_only' | 'ai_always';
/** TMDB marks mid-season and season finales; everything else is standard. */
export type EpisodeType = 'standard' | 'mid_season' | 'finale';

export interface WatchState {
  watched_at: Timestamp | null;
  plays: number;
  rating: number | null;
  /** Shows: episodes watched of aired. */
  progress?: { completed: number; aired: number };
}

export interface DownloadProgress {
  pct: number;
  /** Smoothed bytes per second; shows: summed over the torrents fetching their episodes. */
  speed_bps: number;
  /** Null: torrents fetch on demand, so there's no finish time to promise. */
  eta_seconds: number | null;
  seeders: number;
}

/**
 * `not_loaded`: known to the library, not loaded in the torrent client (it loads on first read).
 * `paused`: the user paused it (B6), loaded or not; it wins over the others, and reads don't wake it.
 */
export type TitleTorrentState = 'active' | 'idle' | 'not_loaded' | 'paused';

/** A movie's assigned torrent. */
export interface TitleTorrent {
  info_hash: string;
  release_name: string;
  file_name: string;
  size_bytes: number;
  resolution: string | null;
  source: string | null;
  added_at: Timestamp;
  state: TitleTorrentState;
  /** Null unless the torrent is loaded. */
  seeders: number | null;
}

/** Where an item's file comes from (#63): one active source per item. */
export type FileSource = 'torrent' | 'hdrezka';

/** A movie playing from HDRezka (#63); `torrent` is null while it's set. */
export interface TitleStream {
  provider: 'hdrezka';
  /** HDRezka's original title, else its (Russian) title. */
  title: string;
  /** The title page, to open on HDRezka. */
  url: string;
  /** The voice-over's name, e.g. "Оригинал (+субтитры)". */
  translator: string;
  /** The quality's label, e.g. "1080p". */
  quality: string;
  /** Measured when it was assigned. */
  size_bytes: number;
  added_at: Timestamp;
}

/** An episode playing from HDRezka. */
export interface EpisodeStream {
  translator: string;
  quality: string;
}

/** The first run of aired, unassigned regular episodes after the last assigned one. */
export interface MissingRun {
  season_number: number;
  /** In-season episode numbers. */
  from: number;
  to: number;
  /** Null without absolute numbers; with them, "Find episodes 1122–1123" needs only the title. */
  absolute_from: number | null;
  absolute_to: number | null;
  count: number;
}

export interface TitleLibraryState {
  in_library: boolean;
  library_id: number | null;
  added_at: Timestamp | null;
  /** Drives the primary action panel (HANDOFF §5.3). */
  availability_stage: AvailabilityStage;
  /** Copyable path as Infuse sees it (HANDOFF). Null until something is assigned: the VFS hides empty titles. */
  webdav_path: string | null;
  /** `WEBDAV_PUBLIC_URL` plus the escaped `webdav_path`. */
  webdav_url: string | null;
  /** Shows: combined over the episodes still downloading. */
  download: DownloadProgress | null;
  needs_review: number;
  /** The assignment job to review (B2/#5). */
  review_job_id: number | null;
  /** Shows: aired regular episodes, and how many of them have a file. Null for movies and while not in the library. */
  episodes_total: number | null;
  episodes_ready: number | null;
  /** Shows: what "Find episodes 29–38" offers. */
  missing: MissingRun | null;
  /** Movies only; null while the movie plays from a stream. */
  torrent: TitleTorrent | null;
  /** Movies playing from HDRezka (#63). */
  stream: TitleStream | null;
}

export interface CastMember {
  name: string;
  character: string;
  profile_path: string | null;
}

/** "More like this": TMDB recommendations, else similar titles; at most 12. */
export interface SimilarTitle {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
  /** Null when not in the library. */
  library: {
    library_id: number;
    availability_stage: AvailabilityStage;
    episodes_total: number | null;
    episodes_ready: number | null;
  } | null;
}

interface TitleBase {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  original_title: string | null;
  year: number | null;
  overview: string;
  poster_path: string | null;
  backdrop_path: string | null;
  /** Hex colour for the ambient hero glow (HANDOFF). Null while it's still being computed. */
  backdrop_dominant_color: string | null;
  genres: string[];
  vote_average: number | null;
  vote_count: number;
  /** ISO 639-1, e.g. "ja". */
  original_language: string | null;
  cast: CastMember[];
  trailer_youtube_key: string | null;
  similar: SimilarTitle[];
  library: TitleLibraryState;
  watch_state: WatchState | null;
  recommendation_reason: string | null;
}

export interface MovieTitle extends TitleBase {
  media_type: 'movie';
  runtime: number | null;
  directors: string[];
}

/** For a show that isn't in the library only `episodes_total` is known; the other counts are 0 and the absolute range null. */
export interface SeasonSummary {
  season_number: number;
  name: string;
  /** Every episode TMDB lists, upcoming ones included. */
  episodes_total: number;
  /** Episodes with a file (ready, watched or downloading). */
  ready: number;
  missing: number;
  needs_review: number;
  upcoming: number;
  absolute_from: number | null;
  absolute_to: number | null;
}

export interface EpisodeSummary {
  /** Library episode id: unassign and subtitles use it. */
  id: number;
  season_number: number;
  episode_number: number;
  absolute_number: number | null;
  name: string;
  overview: string;
  air_date: string | null;
  still_path: string | null;
  runtime: number | null;
  episode_type: EpisodeType | null;
  state: EpisodeState;
  pct: number | null;
  /** Only while the episode has a file. */
  webdav_path: string | null;
  /** The file's source (#63); null without one. */
  source: FileSource | null;
  /** Set while it plays from HDRezka. */
  stream: EpisodeStream | null;
}

/** A jump target in a long season: a TMDB story arc, or a block of up to 10 episodes. */
export interface EpisodeRange {
  label: string;
  kind: 'arc' | 'block';
  /** In-season episode numbers. */
  from: number;
  to: number;
  absolute_from: number | null;
  absolute_to: number | null;
  missing: number;
}

/** GET /titles/tv/{tmdb_id}/seasons/{n}. Ranges only for seasons over 20 episodes. */
export interface SeasonDetail {
  season_number: number;
  name: string;
  episodes: EpisodeSummary[];
  ranges: EpisodeRange[];
}

export interface ShowTitle extends TitleBase {
  media_type: 'tv';
  status: string | null;
  is_anime: boolean;
  /** Set to `absolute` when the show is added as anime, else `aired`. */
  episode_ordering: EpisodeOrdering;
  episode_group_id: string | null;
  include_specials: boolean;
  classifier_mode: ClassifierMode;
  creators: string[];
  networks: string[];
  end_year: number | null;
  /** Typical minutes per episode. */
  episode_run_time: number | null;
  /** Specials (season 0) are listed last, and left out while `include_specials` is off. Episodes load per season. */
  seasons: SeasonSummary[];
  /** The season to open: the frontier's, else the latest with a file, else the first regular one. */
  default_season: number | null;
}

/** GET /titles/{movie|tv}/{tmdb_id} */
export type Title = MovieTitle | ShowTitle;

/** POST /movies and POST /shows (201, or 200 when already in the library) */
export interface AddTitleResult {
  id: number;
  tmdb_id: number;
  title: string;
  year: number;
}

/** PATCH /shows/{id} */
export interface ShowSettingsPatch {
  episode_ordering?: EpisodeOrdering;
  episode_group_id?: string | null;
  include_specials?: boolean;
  classifier_mode?: ClassifierMode;
}

/** GET /shows/{id}/episode-groups */
export interface EpisodeGroup {
  id: string;
  name: string;
  type: number;
  group_count: number;
  episode_count: number;
}
