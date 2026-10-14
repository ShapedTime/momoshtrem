import { queryOptions } from '@tanstack/react-query';
import { apiFetch, apiJson, ApiError, parseErrorBody } from '../client';
import type { AskEvent, AskRequest, AskThread, AskThreadSummary, RecFeedback, RecRun, RecommendationsHome, RunAccepted } from '../contract';

/** Recommendations and Ask momoshtrem (PLAN §6 "Recommendations"; #8/#9). */
export const recKeys = {
  home: ['recommendations', 'home'] as const,
  run: (runId: number) => ['recommendations', 'runs', runId] as const,
  threads: ['recommendations', 'threads'] as const,
  thread: (threadId: number) => ['recommendations', 'threads', threadId] as const,
};

export const recApi = {
  home: () => apiJson<RecommendationsHome>('/api/recommendations/home'),
  refresh: () => apiJson<RunAccepted>('/api/recommendations/refresh', { method: 'POST' }),
  run: (runId: number) => apiJson<RecRun>(`/api/recommendations/runs/${runId}`),
  threads: () => apiJson<{ items: AskThreadSummary[] }>('/api/recommendations/threads'),
  thread: (threadId: number) => apiJson<AskThread>(`/api/recommendations/threads/${threadId}`),
  feedback: (body: RecFeedback) => apiJson<void>('/api/recommendations/feedback', { method: 'POST', json: body }),
};

export const recommendationsHomeQuery = () =>
  queryOptions({ queryKey: recKeys.home, queryFn: recApi.home, staleTime: 10 * 60_000 });

export const recRunQuery = (runId: number) =>
  queryOptions({
    queryKey: recKeys.run(runId),
    queryFn: () => recApi.run(runId),
    refetchInterval: (query) => (query.state.data && ['queued', 'running'].includes(query.state.data.status) ? 2000 : false),
  });

export const askThreadsQuery = () => queryOptions({ queryKey: recKeys.threads, queryFn: recApi.threads });
export const askThreadQuery = (threadId: number) =>
  queryOptions({ queryKey: recKeys.thread(threadId), queryFn: () => recApi.thread(threadId) });

/** Parse one server-sent-events frame ("event: …\ndata: …"). */
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
 * (status · item · text · error · done). Abort with the signal.
 */
export async function* askStream(body: AskRequest, signal?: AbortSignal): AsyncGenerator<AskEvent> {
  const response = await apiFetch('/api/recommendations/ask', {
    method: 'POST',
    headers: { Accept: 'text/event-stream', 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok || !response.body) {
    const { code, message } = parseErrorBody(await response.json().catch(() => undefined));
    throw new ApiError(response.status, code ?? 'ask_failed', message ?? 'Ask momoshtrem is unavailable.');
  }

  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
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
}
