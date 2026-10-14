import { delay, HttpResponse } from 'msw';
import type { ApiErrorBody, JobAccepted, RunAccepted } from '@/lib/api/contract';

/** A little latency in the browser so loading states are visible; none in tests. */
export const latency = () => delay(typeof window === 'undefined' ? 0 : 250);

export const apiError = (status: number, code: string, message: string) =>
  HttpResponse.json<ApiErrorBody>({ error: { code, message } }, { status });

export const notFound = (message = 'Not found') => apiError(404, 'not_found', message);

export const noContent = () => new HttpResponse(null, { status: 204 });

export const accepted = (body: JobAccepted | RunAccepted) => HttpResponse.json(body, { status: 202 });
