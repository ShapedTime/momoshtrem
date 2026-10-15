import type { MediaType, Timestamp } from './common';

/**
 * Assignment jobs and mapping review (PLAN §6 "Assignment (B2/B3)"; #5).
 *
 * Assigning a torrent creates a job. The worker fetches the file list,
 * classifies every file (rules, then the AI resolver for the unsure ones) and
 * stops for review, unless everything is confident and it applies on its own.
 */

/**
 * `queued → fetching_metadata → classifying → [resolving] → needs_review ⇄ ready → applied`;
 * any open state can end in `failed` or `cancelled`.
 * Working: `queued…resolving`. Open: working plus `needs_review`, `ready`. Closed: the last three.
 */
export type JobStatus =
  | 'queued'
  | 'fetching_metadata'
  | 'classifying'
  | 'resolving'
  | 'needs_review'
  | 'ready'
  | 'applied'
  | 'failed'
  | 'cancelled';

/** What a file is. Subtitles that belong to a video aren't listed (see `subtitle_count`). */
export type ProposalKind = 'episode' | 'movie' | 'extra' | 'subtitle';

/**
 * Set when the job is classified and never changed, so rows don't jump.
 * Starting (default) decision: matched and specials `accepted`, needs_review and unmatched `pending`, extras `ignored`.
 */
export type ProposalGroup = 'matched' | 'needs_review' | 'specials' | 'extras' | 'unmatched';

/** Why an extra stays out of Infuse. */
export type ExtraType =
  | 'opening'
  | 'ending'
  | 'sample'
  | 'trailer'
  | 'featurette'
  | 'font'
  | 'info'
  | 'image'
  | 'superseded'
  | 'other';

export type MatchConfidence = 'high' | 'medium' | 'low';
export type MatchSource = 'rules' | 'ai' | 'manual';
export type ProposalDecision = 'pending' | 'accepted' | 'ignored';

/** One episode a file maps to. A multi-episode file has several, in order. */
export interface ProposalTarget {
  episode_id: number;
  season: number;
  episode: number;
  /** Null for specials and shows without absolute numbers. */
  absolute_number: number | null;
  name: string;
}

/** The file the first target already has, which Apply replaces. Computed when read; never blocks. */
export interface ProposalReplaces {
  release_name: string;
  file_path: string;
}

export interface Proposal {
  id: number;
  /** The torrent's path for the file: a multi-file torrent's start with `{torrent.name}/` (show them without it). */
  file_path: string;
  file_size: number;
  kind: ProposalKind;
  group: ProposalGroup;
  /** Only for `kind: extra`. */
  extra_type: ExtraType | null;
  /** Empty with no episode, and always for movie candidates (they map to the job's movie). */
  targets: ProposalTarget[];
  /** What the classifier or AI last suggested: Undo goes back to it. */
  suggested_targets: ProposalTarget[];
  /** Null with no target and for a manual pick. */
  confidence: MatchConfidence | null;
  source: MatchSource;
  /** ≤ 120 characters. Never starts with "AI:": the UI labels AI reasons from `source`. */
  reason: string;
  decision: ProposalDecision;
  /** The user changed it; AI runs leave it alone. False again once it's back on its suggestion and default decision. */
  user_decided: boolean;
  replaces: ProposalReplaces | null;
  /** The latest re-check changed it (and had a hint). Both reset when the next re-check starts. */
  updated: boolean;
  hinted: boolean;
  /** Subtitle files that belong to this video; they follow its mapping. */
  subtitle_count: number;
  /** The proposal that replaces this one in the same torrent (its v2); this one is an `extras` row of type `superseded`. */
  superseded_by: number | null;
}

/** The title the job maps to. */
export interface AssignmentJobItem {
  /** Library id of the movie or show; null for a dry run on a title that isn't in the library (#6). */
  id: number | null;
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
}

export interface AssignmentJobTorrent {
  info_hash: string;
  name: string;
  /** Null until the file list is in. */
  size_bytes: number | null;
  file_count: number | null;
  /** Video files that aren't extras (Frieren's batch: 16 files, 13 videos). */
  video_count: number | null;
  release_group: string | null;
  /** Where it was found, when the assign request said so. */
  indexer: string | null;
}

/** Details for the named progress steps; each is null until its step has run. */
export interface AssignmentJobProgress {
  /** Fetching metadata: the show's seasons (Specials included) and episodes. Null for movies. */
  seasons: number | null;
  episodes: number | null;
  /** Matching files; `videos` counts video files that aren't extras, like `torrent.video_count`. */
  files: number | null;
  videos: number | null;
  /** Asking AI: files sent in the current or latest AI run; null when AI hasn't run. */
  ai_files: number | null;
}

/** A proposal "maps" when it has targets or is a movie candidate. */
export interface AssignmentJobCounts {
  /** Listed proposals. */
  files: number;
  /** Not ignored and maps: what Apply writes ("Apply N mappings"). */
  to_apply: number;
  /** Maps and `pending`: they block Apply. */
  pending: number;
  /** Non-ignored proposals whose targets share an episode with another (subtitles never do): they block Apply. */
  conflicts: number;
  /** Pending and maps, or in a conflict: the files the title's `needs_review` counts. */
  needs_review: number;
  /** Accepted regular episodes (or the accepted movie file). */
  matched: number;
  /** Accepted, Season 0. */
  specials: number;
  ignored: number;
  /** Not ignored and doesn't map. Never blocks. */
  unmatched: number;
}

/** Assign-time hint; a re-check's `hint` replaces `text`. */
export interface AssignHint {
  season?: number;
  episode?: number;
  text?: string;
}

export interface AssignmentJob {
  id: number;
  status: JobStatus;
  /** Inspect (#6): never applied. */
  dry_run: boolean;
  item: AssignmentJobItem;
  torrent: AssignmentJobTorrent;
  progress: AssignmentJobProgress;
  hint: AssignHint | null;
  /** AI is configured, enabled and allowed for this show (`classifier_mode` isn't `rules_only`). */
  ai_available: boolean;
  /** The latest AI run failed, so it changed nothing. Cleared when a re-check starts. */
  ai_error: string | null;
  /** Up to 5 hint chips for "Re-check with AI", e.g. "these are absolute numbers". */
  suggested_hints: string[];
  counts: AssignmentJobCounts;
  /** Why it failed. */
  error: string | null;
  created_at: Timestamp;
  updated_at: Timestamp;
  applied_at: Timestamp | null;
}

/**
 * GET /assignment-jobs/{id}; also what PATCH, apply and discard answer.
 * Proposals are sorted by group (needs_review, unmatched, specials, matched,
 * extras), then first target (regular seasons before Specials), then confidence, then path.
 */
export interface AssignmentJobDetail {
  job: AssignmentJob;
  /** Empty until classification is done (`queued…classifying`). */
  proposals: Proposal[];
}

/** GET /assignment-jobs?status=: `open` (the default) or a list of statuses. Answers `{items: AssignmentJob[]}`, newest first, no dry runs. */
export type JobListFilter = 'open' | JobStatus[];

/** POST /shows/{id}/assign-torrent: 202 → JobAccepted, or 200 with the open job for the same show and torrent. */
export interface AssignShowTorrentRequest {
  magnet_uri: string;
  hint?: AssignHint;
  /** The search result's indexer, shown on the review page. */
  indexer?: string;
}

/** POST /movies/{id}/assign-torrent: as for shows. `file_path` picks the movie file up front. */
export interface AssignMovieTorrentRequest {
  magnet_uri: string;
  file_path?: string;
  indexer?: string;
}

/**
 * One proposal's change: at least one of `decision` and `episode_ids` (else 400).
 * `episode_ids` replaces the targets and is never empty (to leave a file out,
 * ignore it); the suggestion's ids restore the suggestion, any others make a manual pick.
 */
export interface ProposalChange {
  id: number;
  decision?: ProposalDecision;
  episode_ids?: number[];
}

/** PATCH /assignment-jobs/{id}/proposals: all or nothing; answers the detail. 400 for a malformed body, 422 for a change that can't apply. */
export interface ProposalsPatch {
  items: ProposalChange[];
}

/** POST /assignment-jobs/{id}/reclassify: 202 → JobAccepted; clears `ai_error`. 400 for a malformed body, 503 `unavailable` without AI. */
export interface ReclassifyRequest {
  /** `unresolved`: pending proposals the user hasn't decided. `all`: every proposal the user hasn't decided. */
  scope: 'unresolved' | 'all';
  hint?: string;
}

/** `{error: {code}}` of a 409 from apply. */
export type ApplyConflictCode = 'dry_run' | 'job_closed' | 'job_working' | 'conflicting_proposals' | 'pending_proposals';

/** GET /shows/{id}/episodes: every season, Specials whenever season 0 rows exist. */
export interface ShowEpisode {
  id: number;
  episode_number: number;
  absolute_number: number | null;
  name: string;
  air_date: string | null;
  /** It has an active assignment ("In library" in the picker). */
  has_file: boolean;
}

export interface ShowEpisodesSeason {
  season_number: number;
  name: string;
  episodes: ShowEpisode[];
}

/** PUT /episodes/{id}/assignment and PUT /movies/{id}/assignment (204; `match_source` manual). */
export interface ManualAssignment {
  info_hash: string;
  file_path: string;
}

/** GET /torrents/{hash}/files */
export interface TorrentFileEntry {
  path: string;
  size: number;
  mapped_to: { episodes: ProposalTarget[] } | { movie_id: number } | null;
}
