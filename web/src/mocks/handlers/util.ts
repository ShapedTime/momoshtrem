import { delay, HttpResponse } from 'msw';
import type { ApiErrorBody, JobAccepted, RunAccepted } from '@/lib/api/contract';

/** A little latency in the browser so loading states are visible; none in tests. */
export const latency = () => delay(typeof window === 'undefined' ? 0 : 250);

/** `extra`: a 422's `field`, a 429's `resets_at`. */
export const apiError = (status: number, code: string, message: string, extra?: Omit<ApiErrorBody['error'], 'code' | 'message'>) =>
  HttpResponse.json<ApiErrorBody>({ error: { code, message, ...extra } }, { status });

export const notFound = (message = 'Not found') => apiError(404, 'not_found', message);

export const noContent = () => new HttpResponse(null, { status: 204 });

export const accepted = (body: JobAccepted | RunAccepted) => HttpResponse.json(body, { status: 202 });

/** For request-body guards: a JSON object whose fields can be checked one by one. */
export const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

/** The request's JSON body, or undefined when it isn't JSON. */
export const jsonBody = (request: Request): Promise<unknown> => request.json().catch(() => undefined);
