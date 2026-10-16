/**
 * Shared shapes of the backend API contract (docs/redesign/PLAN.md §6).
 * The browser reaches every endpoint at the same path under the web app's
 * session-checked /api/* (see lib/api/backend-proxy.ts).
 */

export type MediaType = 'movie' | 'tv';

/** Every error: `{error: {code, message}}`. */
export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    /** A 422 that's about one field names its JSON key, where the endpoint says so (the quality profile). */
    field?: string;
    /** When a limit lifts, on the 429s that have one (the AI budget, Ask's daily limit). */
    resets_at?: Timestamp;
    /** POST /downloads' 409 `storage_full` (#77): what the Keep needs, and what the kept cap has left. */
    needed_bytes?: number;
    available_bytes?: number;
    /** PUT /settings/storage's 409 `limit_below_committed` (#77): what kept downloads need. */
    committed_kept_bytes?: number;
  };
}

/** Every list: `{items, total, page, page_size}`. */
export interface Paginated<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

/** Long operations answer 202 with a job or run id and are polled. */
export interface JobAccepted {
  job_id: number;
}

export interface RunAccepted {
  run_id: number;
}

/** ISO 8601 / RFC 3339 timestamp. */
export type Timestamp = string;
