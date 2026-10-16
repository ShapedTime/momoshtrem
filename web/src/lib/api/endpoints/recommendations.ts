import { queryOptions } from '@tanstack/react-query';
import { apiFetch, apiJson, ApiError, parseErrorBody, statusCode, toQuery } from '../client';
import type {
  AskEvent,
  AskRequest,
  AskThread,
  AskThreadSummary,
  MediaType,
  RecFeedback,
  RecommendationsHome,
  RunAccepted,
  RunDetail,
  RunSummary,
} from '../contract';

/** Recommendations and Ask momoshtrem (PLAN §6 "Recommendations (R1–R3)"; #9). */
export const recKeys = {
  all: ['recommendations'] as const,
  home: ['recommendations', 'home'] as const,
  runs: (limit?: number) => ['recommendations', 'runs', { limit: limit ?? null }] as const,
  run: (runId: number) => ['recommendations', 'runs', runId] as const,
  threads: ['recommendations', 'threads'] as const,
  thread: (threadId: number) => ['recommendations', 'threads', threadId] as const,
};

export const recApi = {
  home: () => apiJson<RecommendationsHome>('/api/recommendations/home'),
  /** 202 {run_id}: a new manual run, or the active one. 409 recs_disabled | no_history, 429 budget_exceeded (resetsAt), 503 without a key. */
  refresh: () => apiJson<RunAccepted>('/api/recommendations/refresh', { method: 'POST' }),
  runs: (limit?: number) => apiJson<{ items: RunSummary[] }>(`/api/recommendations/runs${toQuery({ limit })}`),
  run: (runId: number) => apiJson<RunDetail>(`/api/recommendations/runs/${runId}`),
  threads: () => apiJson<{ items: AskThreadSummary[] }>('/api/recommendations/threads'),
  thread: (threadId: number) => apiJson<AskThread>(`/api/recommendations/threads/${threadId}`),
  /** An upsert: the newest signal wins. */
  feedback: (body: RecFeedback) => apiJson<void>('/api/recommendations/feedback', { method: 'POST', json: body }),
  /** Idempotent. Undo of a first signal, and a second tap on thumbs up. */
  clearFeedback: (mediaType: MediaType, tmdbId: number) =>
    apiJson<void>(`/api/recommendations/feedback/${mediaType}/${tmdbId}`, { method: 'DELETE' }),
};

const ACTIVE_RUN: ReadonlyArray<RunSummary['status']> = ['queued', 'running'];
export const isActiveRun = (run: Pick<RunSummary, 'status'> | null | undefined): boolean => Boolean(run && ACTIVE_RUN.includes(run.status));

/** Polls every 3 s while a run is queued or running (the refreshing banner), never in a hidden tab. */
export const recommendationsHomeQuery = () =>
  queryOptions({
    queryKey: recKeys.home,
    queryFn: recApi.home,
    staleTime: 10 * 60_000,
    refetchInterval: (query) => (query.state.data?.active_run ? 3000 : false),
  });

export const recRunsQuery = (limit?: number) => queryOptions({ queryKey: recKeys.runs(limit), queryFn: () => recApi.runs(limit) });

export const recRunQuery = (runId: number) =>
  queryOptions({
    queryKey: recKeys.run(runId),
    queryFn: () => recApi.run(runId),
    refetchInterval: (query) => (isActiveRun(query.state.data) ? 2000 : false),
  });

export const askThreadsQuery = () => queryOptions({ queryKey: recKeys.threads, queryFn: recApi.threads });
export const askThreadQuery = (threadId: number) =>
  queryOptions({ queryKey: recKeys.thread(threadId), queryFn: () => recApi.thread(threadId) });

/** Parse one server-sent-events frame ("event: …\ndata: …"). Comment-only frames (`: ping`) are null. */
export function parseSseFrame(frame: string): AskEvent | null {
  let event = 'message';
  const data: string[] = [];
  for (const line of frame.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
  }
  if (data.length === 0) return null;
  try {
    return { event, data: JSON.parse(data.join('\n')) } as AskEvent;
  } catch {
    return null;
  }
}

/**
 * POST /recommendations/ask and yield its text/event-stream as typed events
 * (start · status · text · item · error · done). An error before the stream
 * starts (400, 404, 409 ask_busy | thread_full, 429 ask_limit |
 * budget_exceeded, 503) throws an ApiError; the 429s carry `resetsAt`. Abort
 * with the signal (Stop): the backend cancels the turn.
 */
export async function* askStream(body: AskRequest, signal?: AbortSignal): AsyncGenerator<AskEvent> {
  const response = await apiFetch('/api/recommendations/ask', {
    method: 'POST',
    headers: { Accept: 'text/event-stream', 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok || !response.body) {
    const { code, message, field, resetsAt, details } = parseErrorBody(await response.json().catch(() => undefined));
    throw new ApiError(response.status, code ?? statusCode(response.status), message ?? 'Ask momoshtrem is unavailable.', field, resetsAt, details);
  }

  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += value.replace(/\r\n/g, '\n');
      let boundary = buffer.indexOf('\n\n');
      while (boundary !== -1) {
        const parsed = parseSseFrame(buffer.slice(0, boundary));
        buffer = buffer.slice(boundary + 2);
        if (parsed) yield parsed;
        boundary = buffer.indexOf('\n\n');
      }
    }
    const tail = parseSseFrame(buffer.trim());
    if (tail) yield tail;
  } finally {
    // A consumer that stops iterating early still releases the connection
    reader.cancel().catch(() => {});
  }
}
