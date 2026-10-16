import type { MediaType, Timestamp } from './common';
import type { SimilarTitle } from './titles';

/**
 * Recommendations and Ask momoshtrem (PLAN §6 "Recommendations (R1–R3)"; #9).
 * Picks need ANTHROPIC_API_KEY and `rec_enabled`; Ask needs the key only.
 */

/**
 * The error codes these routes add (`ApiError.code`). The 429s carry
 * `resets_at` (`ApiError.resetsAt`): the 1st of next month for the budget,
 * the next server-local midnight for Ask's daily limit.
 */
export type RecErrorCode =
  | 'recs_disabled' // 409: refresh while rec_enabled is off
  | 'no_history' // 409: refresh without Trakt connected or with fewer than 5 loved or completed titles
  | 'budget_exceeded' // 429: refresh or ask once this month's budget is spent
  | 'ask_busy' // 409: another Ask turn is streaming
  | 'thread_full' // 409: the thread has 20 turns
  | 'ask_limit'; // 429: 30 asks today

/** Feedback on a title. `dislike`, `not_interested` and `seen` hide it from every AI surface. */
export type RecSignal = 'like' | 'dislike' | 'not_interested' | 'seen';
/** Where the feedback was given. */
export type RecFeedbackSource = 'shelf' | 'hero' | 'ask' | 'title';

/** A recommended title. The facts are a snapshot from when the run wrote it; `library`, `on_watchlist` and `feedback` are read live. */
export interface RecItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  /** Shows: the last year; null while it's running, and for movies. */
  year_end: number | null;
  poster_path: string | null;
  /** Shows: the number of seasons (Specials left out); null for movies. */
  seasons: number | null;
  /** A movie's runtime, a show's episode runtime; null when unknown. */
  runtime_minutes: number | null;
  /** TMDB's; null without votes. */
  vote_average: number | null;
  /** YYYY-MM-DD: a movie's release, a show's first air date; null when unknown. */
  release_date: string | null;
  /** A short label on the poster ("Jan 16", "2026"), mostly on `coming_soon` shelves; else null. */
  badge: string | null;
  /** One line, at most 140 characters, grounded in the taste profile. */
  reason: string;
  /** As `SimilarTitle.library`: null when not in the library. */
  library: SimilarTitle['library'];
  /** On the Trakt watchlist (what the sync imported). */
  on_watchlist: boolean;
  /** Only `like` shows here: a title with negative feedback is left out instead. */
  feedback: 'like' | null;
}

/** A title Ask found: a rec card with a match tag ("Mystery + workplace · lighter", at most 32 characters). */
export interface AskItem extends RecItem {
  match_tag: string;
}

export type ShelfKind = 'because' | 'new' | 'anime' | 'gems' | 'coming_soon' | 'other';

export interface Shelf {
  slug: string;
  kind: ShelfKind;
  title: string;
  subtitle: string | null;
  /** 6–15 when written; feedback can thin it at read time (a shelf left empty is dropped). */
  items: RecItem[];
}

/** "Tonight's pick" (HANDOFF): three a server-local day. The hero loads the title for its artwork and colour. */
export interface HeroPick {
  media_type: MediaType;
  tmdb_id: number;
  kicker: string;
  reason: string;
  feedback: 'like' | null;
}

export type RecRunKind = 'scheduled' | 'manual' | 'first_sync';
export type RecRunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'skipped';
export type RecSkipReason = 'budget_exceeded' | 'no_history';
/** A run's steps; they appear in the order reached (the web lists the rest as pending: history, web, tmdb, write). */
export type RecRunStep = 'history' | 'web' | 'tmdb' | 'write';
/** Ask's steps: a refinement re-ranks (`rerank`) instead of reading history, the web and TMDB. */
export type AskStepId = 'history' | 'rerank' | 'web' | 'tmdb' | 'write';
export type RecStepState = 'active' | 'done';

export interface RunSummary {
  id: number;
  kind: RecRunKind;
  status: RecRunStatus;
  model: string;
  /** Null while queued, and for a skipped run. */
  started_at: Timestamp | null;
  finished_at: Timestamp | null;
  /** So far, while it runs. */
  cost_usd: number;
  /** Only while skipped. */
  skip_reason: RecSkipReason | null;
  /** For the user, while failed. */
  error: string | null;
}

export interface RunStep {
  step: RecRunStep;
  state: RecStepState;
  /** The step's tally: "2,184 plays · 312 ratings", "2 of 5 shelves"; null while it starts. */
  detail: string | null;
}

export interface RunProgress extends RunSummary {
  /** The steps reached so far, in order; at most one `active` (the newest). */
  steps: RunStep[];
  shelves_done: number;
  /** The shelves asked for; 0 until the run starts. */
  shelves_target: number;
}

/** A run's API usage: its `ai_usage` rows, summed. */
export interface RecUsage {
  api_calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  web_searches: number;
  cost_usd: number;
}

/** GET /recommendations/runs/{id} */
export interface RunDetail extends RunProgress {
  usage: RecUsage;
}

/**
 * GET /recommendations/home: always 200. Without a key or with `rec_enabled`
 * off, `enabled` is false and everything else empty. Before the first
 * succeeded run `run` is null and the web keeps its trending fallback.
 */
export interface RecommendationsHome {
  /** ANTHROPIC_API_KEY is set. False: hide the Ask entries too; true with `enabled` false: picks are switched off, Ask still works. */
  configured: boolean;
  /** `configured` and `rec_enabled`. */
  enabled: boolean;
  /** The newest succeeded run: `hero_picks` and `shelves` are its. */
  run: RunSummary | null;
  /** The queued or running run: poll while it's set. */
  active_run: RunProgress | null;
  /** At most three: the day's (stable per server-local day). */
  hero_picks: HeroPick[];
  shelves: Shelf[];
}

/** POST /recommendations/feedback (an upsert); DELETE /recommendations/feedback/{media_type}/{tmdb_id} clears it. */
export interface RecFeedback {
  media_type: MediaType;
  tmdb_id: number;
  signal: RecSignal;
  source: RecFeedbackSource;
}

// ---- Ask momoshtrem ---------------------------------------------------------------

/** POST /recommendations/ask body (message: 1–500 characters). The answer is a text/event-stream of AskEvent. */
export interface AskRequest {
  thread_id?: number;
  message: string;
}

export interface AskStep {
  step: AskStepId;
  state: RecStepState;
  /** Set when the step is done: "2,184 plays · you loved Severance, The Bear, Ted Lasso". */
  detail: string | null;
}

export type AskErrorCode = 'refused' | 'limit_reached' | 'upstream' | 'internal';

export interface AskTurnUsage {
  input_tokens: number;
  output_tokens: number;
  web_searches: number;
  cost_usd: number;
}

export interface AskStartEvent {
  thread_id: number;
  turn_id: number;
  /** The thread's title (a new thread's comes from the message). */
  title: string;
}

export interface AskDoneEvent {
  thread_id: number;
  turn_id: number;
  elapsed_ms: number;
  /** At most five chips ("Lighter", "Only anime"), sent back as follow-up messages. */
  refinements: string[];
  usage: AskTurnUsage;
}

/** One server-sent event of POST /recommendations/ask, in order: start, status…, text…, item…, then done or error. `: ping` comments are skipped. */
export type AskEvent =
  | { event: 'start'; data: AskStartEvent }
  | { event: 'status'; data: AskStep }
  | { event: 'text'; data: { delta: string } }
  | { event: 'item'; data: AskItem }
  | { event: 'error'; data: { code: AskErrorCode; message: string } }
  | { event: 'done'; data: AskDoneEvent };

export type AskTurnStatus = 'streaming' | 'done' | 'error' | 'cancelled';

export interface AskTurn {
  id: number;
  message: string;
  status: AskTurnStatus;
  /** What the turn showed, saved when it ended: a streaming turn has none of it yet. An ended turn has no `active` step (the one it failed or stopped in is dropped). */
  steps: AskStep[];
  /** The turn's streamed text. */
  intro: string;
  items: AskItem[];
  refinements: string[];
  /** While error. */
  error: { code: AskErrorCode; message: string } | null;
  /** Null while streaming. */
  elapsed_ms: number | null;
  created_at: Timestamp;
}

/** GET /recommendations/threads → {items}, the newest updated first. */
export interface AskThreadSummary {
  id: number;
  title: string;
  created_at: Timestamp;
  updated_at: Timestamp;
  turn_count: number;
}

/** GET /recommendations/threads/{id}. Library, watchlist and feedback are read live; items with negative feedback are left out. */
export interface AskThread extends AskThreadSummary {
  /** The model the thread started on; later turns keep it. */
  model: string;
  turns: AskTurn[];
}
