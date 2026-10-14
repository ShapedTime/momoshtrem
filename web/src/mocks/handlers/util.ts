import { delay, HttpResponse } from 'msw';
import type { ApiErrorBody, JobAccepted, RunAccepted } from '@/lib/api/contract';

/** A little latency in the browser so loading states are visible; none in tests. */
export const latency = () => delay(typeof window === 'undefined' ? 0 : 250);

export const notFound = (message = 'Not found') =>
  HttpResponse.json<ApiErrorBody>({ error: { code: 'not_found', message } }, { status: 404 });

export const accepted = (body: JobAccepted | RunAccepted) => HttpResponse.json(body, { status: 202 });
