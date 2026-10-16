import { http, HttpResponse } from 'msw';
import type { AskEvent, MediaType, RecFeedbackSource, RecSignal } from '@/lib/api/contract';
import {
  ASK_MESSAGE_MAX,
  clearRecFeedback,
  endAskTurn,
  listRuns,
  recommendationsHome,
  recordAskEvent,
  refreshRecommendations,
  resetMockRecs,
  runDetail,
  setRecFeedback,
  startAsk,
  threadSummaries,
  threadView,
  type RecsResult,
} from '../fixtures/recommendations';
import { mockLibrary } from './titles';
import { apiError, isRecord, jsonBody, latency, noContent } from './util';

/** Tests: back to the starting picks, feedback and threads (fixtures/recommendations.ts keeps them). */
export { resetMockRecs };

const SIGNALS: ReadonlySet<string> = new Set<RecSignal>(['like', 'dislike', 'not_interested', 'seen']);
const SOURCES: ReadonlySet<string> = new Set<RecFeedbackSource>(['shelf', 'hero', 'ask', 'title']);
const DEFAULT_RUNS = 10;
const MAX_RUNS = 50;
const PING_MS = 15_000;

const isMediaType = (value: unknown): value is MediaType => value === 'movie' || value === 'tv';
const isPositiveInt = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value > 0;

function failure(result: Extract<RecsResult<unknown>, { ok: false }>) {
  return apiError(result.status, result.code, result.message, result.resets_at ? { resets_at: result.resets_at } : undefined);
}

/** How long the browser waits before each event, so the steps, the intro and the cards arrive the way the design animates them. */
function pacing(event: AskEvent): number {
  switch (event.event) {
    case 'start':
      return 0;
    case 'status':
      return event.data.state === 'active' ? 250 : 700;
    case 'text':
      return 60;
    case 'item':
      return 450;
    default:
      return 200;
  }
}

const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export const recommendationsHandlers = [
  http.get('*/api/recommendations/home', async () => {
    await latency();
    return HttpResponse.json(recommendationsHome(mockLibrary()));
  }),
  http.post('*/api/recommendations/refresh', async () => {
    await latency();
    const result = refreshRecommendations();
    return result.ok ? HttpResponse.json(result.value, { status: 202 }) : failure(result);
  }),
  http.get('*/api/recommendations/runs', async ({ request }) => {
    await latency();
    const raw = new URL(request.url).searchParams.get('limit');
    const limit = raw === null || raw === '' ? DEFAULT_RUNS : Number(raw);
    if (!Number.isInteger(limit) || limit < 1 || limit > MAX_RUNS) return apiError(400, 'bad_request', 'limit must be a whole number from 1 to 50');
    return HttpResponse.json({ items: listRuns(limit) });
  }),
  http.get('*/api/recommendations/runs/:id', async ({ params }) => {
    await latency();
    const run = runDetail(Number(params.id));
    return run ? HttpResponse.json(run) : apiError(404, 'not_found', 'No such run.');
  }),
  http.post('*/api/recommendations/feedback', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || !isMediaType(body.media_type) || !isPositiveInt(body.tmdb_id) || !SIGNALS.has(String(body.signal)) || !SOURCES.has(String(body.source))) {
      return apiError(400, 'bad_request', 'Send media_type (movie|tv), tmdb_id, signal (like|dislike|not_interested|seen) and source (shelf|hero|ask|title).');
    }
    setRecFeedback(body.media_type, body.tmdb_id, body.signal as RecSignal, body.source as RecFeedbackSource);
    return noContent();
  }),
  http.delete('*/api/recommendations/feedback/:mediaType/:tmdbId', async ({ params }) => {
    await latency();
    const tmdbId = Number(params.tmdbId);
    if (!isMediaType(params.mediaType) || !isPositiveInt(tmdbId)) return apiError(400, 'bad_request', 'Use /recommendations/feedback/{movie|tv}/{tmdb_id}.');
    clearRecFeedback(params.mediaType, tmdbId);
    return noContent();
  }),
  http.get('*/api/recommendations/threads', async () => {
    await latency();
    return HttpResponse.json({ items: threadSummaries() });
  }),
  http.get('*/api/recommendations/threads/:id', async ({ params }) => {
    await latency();
    const thread = threadView(mockLibrary(), Number(params.id));
    return thread ? HttpResponse.json(thread) : apiError(404, 'not_found', 'That conversation doesn’t exist.');
  }),

  // Ask: JSON errors first, then Server-Sent Events: start → status… → text… → item… → done (or error)
  http.post('*/api/recommendations/ask', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    const message = isRecord(body) && typeof body.message === 'string' ? body.message.trim() : '';
    const threadId = isRecord(body) ? body.thread_id : undefined;
    if (!message || message.length > ASK_MESSAGE_MAX || (threadId !== undefined && threadId !== null && !isPositiveInt(threadId))) {
      return apiError(400, 'bad_request', 'Send a message of 1–500 characters, and thread_id to follow up.');
    }
    const result = startAsk(mockLibrary(), { message, thread_id: typeof threadId === 'number' ? threadId : undefined });
    if (!result.ok) return failure(result);

    const { turn, events, hold } = result.value;
    const browser = typeof window !== 'undefined';
    const encoder = new TextEncoder();
    let closed = false;
    let ping: ReturnType<typeof setInterval> | undefined;
    // The client went away (Stop, a closed tab): the turn is cancelled, as the backend does
    const stop = () => {
      closed = true;
      if (ping) clearInterval(ping);
      endAskTurn(turn, 'cancelled');
    };
    request.signal.addEventListener('abort', stop);

    const stream = new ReadableStream<Uint8Array>({
      async start(controller) {
        const send = (chunk: string) => {
          if (!closed) controller.enqueue(encoder.encode(chunk));
        };
        for (const event of events) {
          await pause(browser ? pacing(event) : 0);
          if (closed) return;
          send(`event: ${event.event}\ndata: ${JSON.stringify(event.data)}\n\n`);
          recordAskEvent(turn, event);
        }
        if (hold) {
          // ask-hold: never finishes; the backend's keep-alive comment every 15 s
          if (browser) ping = setInterval(() => send(': ping\n\n'), PING_MS);
          return;
        }
        endAskTurn(turn, events.at(-1)?.event === 'error' ? 'error' : 'done');
        closed = true;
        controller.close();
      },
      cancel: stop,
    });
    return new HttpResponse(stream, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache, no-transform' } });
  }),
];
