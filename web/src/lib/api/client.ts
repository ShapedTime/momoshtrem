/**
 * Browser-side client for the web app's own `/api/*` routes (which proxy
 * the backend; see backend-proxy.ts). Every client fetch goes through here so
 * that an expired session always lands on /login instead of a broken page.
 */

import { LOGIN_ROUTE } from '@/config/auth';
import { clearMemberCookie } from '@/lib/members/cookie';
import type { ApiErrorBody } from './contract/common';
import { UNKNOWN_MEMBER_CODE } from './contract/members';

/** Error from an /api call: HTTP status plus the contract's {code, message, field?, resets_at?, …}. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  /** The field a 422 is about, when the endpoint names one. */
  readonly field?: string;
  /** `resets_at`: when a 429's limit lifts (the AI budget, Ask's daily limit). */
  readonly resetsAt?: string;
  /**
   * Every field of the error object but `code` and `message`, as sent: the extras some errors
   * carry (`needed_bytes` on `storage_full`). Read them through a type guard, e.g. `isStorageFullError`.
   */
  readonly details: Readonly<Record<string, unknown>>;

  constructor(status: number, code: string, message: string, field?: string, resetsAt?: string, details: Readonly<Record<string, unknown>> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.field = field;
    this.resetsAt = resetsAt;
    this.details = details;
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

/** What an error body says; `details` holds the error object's other fields. */
export interface ParsedErrorBody {
  code?: string;
  message?: string;
  field?: string;
  resetsAt?: string;
  details?: Record<string, unknown>;
}

/** Read both error shapes: `{error: {code, message, …}}` (PLAN §6) and legacy `{error: "message"}`. */
export function parseErrorBody(body: unknown): ParsedErrorBody {
  if (!body || typeof body !== 'object' || !('error' in body)) return {};
  const error = (body as { error: unknown }).error;
  if (typeof error === 'string') return { message: error };
  if (error && typeof error === 'object' && !Array.isArray(error)) {
    const { code, message, ...details } = error as Record<string, unknown>;
    const { field, resets_at } = details as Partial<ApiErrorBody['error']>;
    return {
      code: typeof code === 'string' ? code : undefined,
      message: typeof message === 'string' ? message : undefined,
      field: typeof field === 'string' ? field : undefined,
      resetsAt: typeof resets_at === 'string' ? resets_at : undefined,
      details,
    };
  }
  return {};
}

let redirecting = false;

/** Send the browser to sign in, coming back to the current page afterwards. */
function redirectToLogin(): void {
  if (typeof window === 'undefined' || redirecting) return;
  const { pathname, search, hash } = window.location;
  if (pathname === LOGIN_ROUTE) return; // never loop on the login page itself
  redirecting = true;
  window.location.assign(`${LOGIN_ROUTE}?redirect=${encodeURIComponent(`${pathname}${search}${hash}`)}`);
}

/**
 * The cookie names a household member nobody has any more (removed on
 * another device while this tab stayed open): forget the choice and load
 * this page again, which opens "Who's watching?" (or picks the only member).
 * The cookie is cleared before the reload, so it can't loop.
 */
function forgetMember(): void {
  if (typeof window === 'undefined' || redirecting) return;
  redirecting = true;
  clearMemberCookie();
  const { pathname, search, hash } = window.location;
  window.location.assign(`${pathname}${search}${hash}`);
}

/**
 * Drop-in `fetch` for `/api/*`. The proxy's "session ended" answer
 * (401 `unauthorized`) sends the browser to /login and throws, so callers
 * never render a half-signed-in page. Any other 401 (an upstream service
 * rejecting a key) is returned like every other status: redirecting on it
 * would loop through /login. A 400 `unknown_member` reloads the page
 * without the member cookie (`forgetMember`) and throws.
 */
export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(input, { credentials: 'same-origin', ...init });
  if (response.status === 401 || response.status === 400) {
    const body: unknown = await response.clone().json().catch(() => undefined);
    const { code, message } = parseErrorBody(body);
    if (response.status === 401 && code === 'unauthorized') {
      redirectToLogin();
      throw new ApiError(401, 'unauthorized', 'Your session has ended. Sign in again.');
    }
    if (response.status === 400 && code === UNKNOWN_MEMBER_CODE) {
      forgetMember();
      throw new ApiError(400, UNKNOWN_MEMBER_CODE, message ?? 'That person isn’t in the household any more.');
    }
  }
  return response;
}

/** Fetch JSON and throw an ApiError for non-2xx answers. 204 resolves to undefined. */
export async function apiJson<T>(input: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, headers, ...rest } = init ?? {};
  const response = await apiFetch(input, {
    ...rest,
    headers: {
      Accept: 'application/json',
      ...(json !== undefined ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });

  if (response.status === 204) return undefined as T;
  const body: unknown = await response.json().catch(() => undefined);
  if (!response.ok) {
    const { code, message, field, resetsAt, details } = parseErrorBody(body);
    throw new ApiError(response.status, code ?? statusCode(response.status), message ?? `Request failed (${response.status})`, field, resetsAt, details);
  }
  return body as T;
}

/**
 * Same derivation as the backend's internal/api/errors.go, plus 502
 * `bad_gateway`, the web proxy's own code.
 */
export function statusCode(status: number): string {
  switch (status) {
    case 400:
      return 'bad_request';
    case 401:
      return 'unauthorized';
    case 403:
      return 'forbidden';
    case 404:
      return 'not_found';
    case 409:
      return 'conflict';
    case 422:
      return 'validation_error';
    case 429:
      return 'rate_limited';
    case 502:
      return 'bad_gateway';
    case 503:
      return 'unavailable';
    case 504:
      return 'timeout';
    default:
      return status >= 500 ? 'internal_error' : 'bad_request';
  }
}

/**
 * Fire a request while the page unloads (`pagehide`), e.g. an Undo window's
 * removal that hasn't been sent yet. The one place a raw fetch is allowed:
 * the page is going away, so there's nobody to read an answer or redirect on
 * a 401, and only `keepalive` lets the request outlive the page.
 */
export function apiKeepalive(input: string, method: 'DELETE' | 'POST'): void {
  try {
    void fetch(input, { method, keepalive: true, credentials: 'same-origin' }).catch(() => undefined);
  } catch {
    // The browser refused (e.g. over the keepalive quota): nothing more to do while unloading
  }
}

/** Build a query string from defined params only. */
export function toQuery(params: object): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue;
    if (Array.isArray(value)) value.forEach((item) => search.append(key, String(item)));
    else search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `?${query}` : '';
}

/** Tests only. */
export function resetRedirectGuard(): void {
  redirecting = false;
}
