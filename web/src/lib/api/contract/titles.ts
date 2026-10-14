import type { MediaType, Timestamp } from './common';

/** Detail pages (B1; #4). Fields marked HANDOFF come from design-cinematic/HANDOFF.md §6. */

export type AvailabilityStage = 'none' | 'needs_torrent' | 'downloading' | 'ready';
export type EpisodeState = 'watched' | 'ready' | 'downloading' | 'missing' | 'upcoming' | 'needs_review';
export type EpisodeOrdering = 'aired' | 'absolute' | 'episode_group';
export type ClassifierMode = 'auto' | 'rules_only' | 'ai_always';

export interface WatchState {
  watched_at: Timestamp | null;
  plays: number;
  rating: number | null;
  /** Shows: episodes watched of aired. */
  progress?: { completed: number; aired: number };
}

export interface DownloadProgress {
  pct: number;
  speed_bps: number;
  eta_seconds: number | null;
  seeders: number;
}

export interface TitleLibraryState {
  in_library: boolean;
  library_id: number | null;
  /** Drives the primary action panel (HANDOFF §5.3). */
  availability_stage: AvailabilityStage;
  /** Copyable path as Infuse sees it (HANDOFF). */
  webdav_path: string | null;
  download: DownloadProgress | null;
  needs_review: number;
}

export interface CastMember {
  name: string;
  character: string;
  profile_path: string | null;
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
  /** Hex colour for the ambient hero glow (HANDOFF). */
  backdrop_dominant_color: string | null;
  genres: string[];
  vote_average: number | null;
  cast: CastMember[];
  trailer_youtube_key: string | null;
  library: TitleLibraryState;
  watch_state: WatchState | null;
  recommendation_reason: string | null;
}

export interface MovieTitle extends TitleBase {
  media_type: 'movie';
  runtime: number | null;
}

export interface SeasonSummary {
  season_number: number;
  name: string;
  episodes_total: number;
  ready: number;
  missing: number;
  needs_review: number;
}

export interface EpisodeSummary {
  id: number | null;
  season_number: number;
  episode_number: number;
  absolute_number: number | null;
  name: string;
  air_date: string | null;
  still_path: string | null;
  runtime: number | null;
  state: EpisodeState;
  pct: number | null;
}

export interface ShowTitle extends TitleBase {
  media_type: 'tv';
  status: string | null;
  is_anime: boolean;
  episode_ordering: EpisodeOrdering;
  episode_group_id: string | null;
  include_specials: boolean;
  classifier_mode: ClassifierMode;
  /** Specials (season 0) are listed last. */
  seasons: SeasonSummary[];
  episodes: EpisodeSummary[];
}

/** GET /titles/{movie|tv}/{tmdb_id} */
export type Title = MovieTitle | ShowTitle;

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
