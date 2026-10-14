/**
 * Browser-side client for the web app's own `/api/*` routes (which proxy
 * the backend; see backend-proxy.ts). Every client fetch goes through here so
 * that an expired session always lands on /login instead of a broken page.
 */

import { LOGIN_ROUTE } from '@/config/auth';
import type { ApiErrorBody } from './contract/common';

/** Error from an /api call: HTTP status plus the contract's {code, message}. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

/** Read both error shapes: `{error: {code, message}}` (PLAN §6) and legacy `{error: "message"}`. */
export function parseErrorBody(body: unknown): { code?: string; message?: string } {
  if (!body || typeof body !== 'object' || !('error' in body)) return {};
  const error = (body as { error: unknown }).error;
  if (typeof error === 'string') return { message: error };
  if (error && typeof error === 'object') {
    const { code, message } = error as Partial<ApiErrorBody['error']>;
    return { code: typeof code === 'string' ? code : undefined, message: typeof message === 'string' ? message : undefined };
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
 * Drop-in `fetch` for `/api/*`. The proxy's "session ended" answer
 * (401 `unauthorized`) sends the browser to /login and throws, so callers
 * never render a half-signed-in page. Any other 401 (an upstream service
 * rejecting a key) is returned like every other status: redirecting on it
 * would loop through /login.
 */
export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(input, { credentials: 'same-origin', ...init });
  if (response.status === 401) {
    const body: unknown = await response.clone().json().catch(() => undefined);
    if (parseErrorBody(body).code === 'unauthorized') {
      redirectToLogin();
      throw new ApiError(401, 'unauthorized', 'Your session has ended. Sign in again.');
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
    const { code, message } = parseErrorBody(body);
    throw new ApiError(response.status, code ?? statusCode(response.status), message ?? `Request failed (${response.status})`);
  }
  return body as T;
}

/** Same derivation as the backend's internal/api/errors.go. */
function statusCode(status: number): string {
  switch (status) {
    case 400:
      return 'bad_request';
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
      return status >= 500 ? 'internal_error' : 'error';
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
