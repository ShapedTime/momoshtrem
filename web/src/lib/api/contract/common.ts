/**
 * Shared shapes of the backend API contract (docs/redesign/PLAN.md §6).
 * The browser reaches every endpoint at the same path under the web app's
 * session-checked /api/* (see lib/api/backend-proxy.ts).
 */

export type MediaType = 'movie' | 'tv';

/** Every error: `{error: {code, message}}`. */
export interface ApiErrorBody {
  error: { code: string; message: string };
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
