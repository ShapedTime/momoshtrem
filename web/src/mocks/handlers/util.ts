import { delay, HttpResponse } from 'msw';

/** A little latency in the browser so loading states are visible; none in tests. */
export const latency = () => delay(typeof window === 'undefined' ? 0 : 250);

export const notFound = (message = 'Not found') =>
  HttpResponse.json({ error: { code: 'not_found', message } }, { status: 404 });

export const accepted = (body: object) => HttpResponse.json(body, { status: 202 });
