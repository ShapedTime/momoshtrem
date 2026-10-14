import type { MediaType, Timestamp } from './common';

/** Recommendations and Ask momoshtrem (R1–R3; #8/#9). */

export interface RecItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
  reason: string;
  in_library: boolean;
  on_watchlist: boolean;
}

export interface Shelf {
  slug: string;
  title: string;
  subtitle: string | null;
  items: RecItem[];
}

/** "Tonight's pick" (HANDOFF): stable per day. */
export interface HeroPick {
  tmdb_id: number;
  media_type: MediaType;
  kicker: string;
  reason: string;
}

/** GET /recommendations/home (falls back to trending without a run or Trakt) */
export interface RecommendationsHome {
  run: { id: number; finished_at: Timestamp } | null;
  hero_picks: HeroPick[];
  shelves: Shelf[];
}

/** GET /recommendations/runs/{id} */
export interface RecRun {
  id: number;
  kind: 'nightly' | 'manual';
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'skipped';
  model: string;
  progress: { step: string; detail: string | null }[];
  usage: { input_tokens: number; output_tokens: number; web_searches: number };
  cost_usd: number;
  error: string | null;
  started_at: Timestamp;
  finished_at: Timestamp | null;
}

/** POST /recommendations/ask body; the answer is a text/event-stream of AskEvent. */
export interface AskRequest {
  thread_id?: number;
  message: string;
}

export type AskEvent =
  | { event: 'status'; data: { step: string; detail: string | null } }
  | { event: 'item'; data: RecItem & { match_tag: string | null } }
  | { event: 'text'; data: { delta: string } }
  | { event: 'error'; data: { code: string; message: string } }
  | { event: 'done'; data: { thread_id: number; usage: { input_tokens: number; output_tokens: number }; refinements: string[] } };

export interface AskThreadSummary {
  id: number;
  title: string;
  created_at: Timestamp;
  updated_at: Timestamp;
}

/** GET /recommendations/threads/{id} */
export interface AskThread extends AskThreadSummary {
  messages: { role: 'user' | 'assistant'; text: string; items: RecItem[]; created_at: Timestamp }[];
}

/** POST /recommendations/feedback */
export interface RecFeedback {
  media_type: MediaType;
  tmdb_id: number;
  signal: 'like' | 'dislike' | 'not_interested' | 'seen';
}
