'use client';

import { useCallback, useEffect, useReducer, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { isApiError } from '@/lib/api/client';
import { askStream, askThreadQuery, recKeys, settingsKeys } from '@/lib/api/endpoints';
import { askReducer, initialSession, isWorking, visibleTurns, type AskFailure, type AskTurnView } from '@/lib/ask/turns';
import { askThreadHref } from '@/lib/recommendations/ask-href';

/** A refinement sent right after Stop can still meet `ask_busy` while the cancel crosses the proxy: one retry after this pause. */
const BUSY_RETRY_MS = 600;
/** A turn streaming in another tab: read the thread again until it ends. */
const REMOTE_POLL_MS = 3000;

let turnCount = 0;

function failureOf(error: unknown): AskFailure {
  if (isApiError(error)) return { status: error.status, code: error.code, message: error.message, resetsAt: error.resetsAt ?? null };
  // fetch rejected: no connection, or the server went away before answering
  return { status: 0, code: 'network', message: '', resetsAt: null };
}

function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        resolve();
      },
      { once: true }
    );
  });
}

interface SendOptions {
  /** A refinement chip: it re-ranks instead of searching again. */
  refine?: boolean;
  /** Try again: the failed turn sent from this page gives its place to the new one. */
  replaces?: string;
}

/**
 * Ask momoshtrem's conversation for the thread in the URL (`/ask/{id}`, or
 * null on /ask): its saved turns, plus the turns sent from this page as
 * they stream. `send` POSTs /recommendations/ask and folds its events into
 * the turn (lib/ask/turns.ts); `stop` aborts it, which cancels the turn on
 * the server. A new thread's `start` moves the URL to `/ask/{id}` without
 * leaving the page (the screen lives in the route's layout).
 */
export function useAskConversation(routeThread: number | null) {
  const queryClient = useQueryClient();
  const [session, dispatch] = useReducer(askReducer, routeThread, initialSession);
  // Another thread in the URL is another conversation; the page's own replace after `start` isn't
  if (session.route !== routeThread) dispatch({ type: 'route', thread: routeThread });

  /** The turn streaming now, and its thread (null until a new thread's `start`). */
  const running = useRef<{ key: string; abort: AbortController; thread: number | null } | null>(null);
  const stoppedLast = useRef(false);
  const mounted = useRef(false);

  const working = session.live.find(isWorking);
  const liveIds = new Set(session.live.flatMap((turn) => (turn.id === null ? [] : [turn.id])));
  const thread = useQuery({
    ...askThreadQuery(session.threadId ?? 0),
    enabled: session.threadId !== null,
    refetchInterval: (query) =>
      !working && query.state.data?.turns.some((turn) => turn.status === 'streaming' && !liveIds.has(turn.id)) ? REMOTE_POLL_MS : false,
  });
  const turns: AskTurnView[] = visibleTurns(thread.data, session.live);

  // Another thread in the URL, or /ask from a thread, stops what this page was streaming. Compared with
  // the stream's own thread, not the session, so StrictMode's second run of effects never stops a fresh ask.
  useEffect(() => {
    const current = running.current;
    if (current && current.thread !== routeThread) {
      stoppedLast.current = true;
      current.abort.abort();
      running.current = null;
    }
  }, [routeThread]);

  // Leaving the page closes the stream, which cancels the turn. StrictMode remounts at once, so only a real unmount aborts.
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      setTimeout(() => {
        if (mounted.current) return;
        running.current?.abort.abort();
        running.current = null;
      }, 0);
    };
  }, []);

  // A new thread's `start`: /ask → /ask/{id}, so a reload or Back finds it. The History API (which
  // Next's router follows, usePathname included) moves the URL at once, with no server round trip.
  useEffect(() => {
    if (session.threadId !== null && routeThread === null && session.route === null) {
      window.history.replaceState(null, '', askThreadHref(session.threadId));
    }
  }, [routeThread, session.route, session.threadId]);

  const threadId = session.threadId;
  const send = useCallback(
    async (raw: string, { refine = false, replaces }: SendOptions = {}) => {
      const message = raw.trim();
      if (!message || running.current) return;
      const key = `ask:${++turnCount}`;
      const abort = new AbortController();
      running.current = { key, abort, thread: threadId };
      dispatch({ type: 'send', key, message, refine, replaces });
      const afterStop = stoppedLast.current;
      stoppedLast.current = false;

      let target = threadId;
      let streamed = false;
      let ended = false;
      try {
        for (let attempt = 0; ; attempt++) {
          try {
            for await (const event of askStream(target === null ? { message } : { thread_id: target, message }, abort.signal)) {
              streamed = true;
              dispatch({ type: 'event', key, event });
              if (event.event === 'start') {
                target = event.data.thread_id;
                if (running.current?.key === key) running.current.thread = target;
                void queryClient.invalidateQueries({ queryKey: recKeys.threads, exact: true });
              }
              if (event.event === 'done' || event.event === 'error') ended = true;
            }
            if (!ended && !abort.signal.aborted) dispatch({ type: 'cut', key });
          } catch (error) {
            if (abort.signal.aborted) break;
            if (!streamed && afterStop && attempt === 0 && isApiError(error) && error.code === 'ask_busy') {
              await pause(BUSY_RETRY_MS, abort.signal);
              if (abort.signal.aborted) break;
              continue;
            }
            dispatch(streamed ? { type: 'cut', key } : { type: 'failed', key, failure: failureOf(error) });
          }
          break;
        }
      } finally {
        if (running.current?.key === key) running.current = null;
        // The thread's saved view, its place in Recent and the month's spend all moved
        if (target !== null) void queryClient.invalidateQueries({ queryKey: recKeys.thread(target) });
        void queryClient.invalidateQueries({ queryKey: recKeys.threads, exact: true });
        void queryClient.invalidateQueries({ queryKey: settingsKeys.aiUsage });
      }
    },
    [queryClient, threadId]
  );

  /** Stop (the button, or Esc): abort the request; the server cancels the turn and keeps what it spent. */
  const stop = useCallback(() => {
    const current = running.current;
    if (!current) return;
    running.current = null;
    stoppedLast.current = true;
    current.abort.abort();
    dispatch({ type: 'stopped', key: current.key });
  }, []);

  /** New conversation while already on /ask (a link to /ask from a thread resets through the route). */
  const reset = useCallback(() => {
    if (running.current) {
      // Like Stop: the next ask may still meet the cancelled turn's `ask_busy`
      stoppedLast.current = true;
      running.current.abort.abort();
      running.current = null;
    }
    dispatch({ type: 'reset' });
  }, []);

  return { session, turns, thread, working, send, stop, reset };
}
