'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ChevronLeft, History, Plus } from 'lucide-react';
import { CatAvatar } from '@/components/brand';
import { Banner, Button, ButtonLink, EmptyState, IconButton, LoadingRegion, Skeleton } from '@/components/ui';
import { useAskConversation } from '@/hooks/useAskConversation';
import { useDocumentTitle } from '@/hooks/useDocumentTitle';
import { isApiError } from '@/lib/api/client';
import { recommendationsHomeQuery, traktIntegrationQuery } from '@/lib/api/endpoints';
import { askAnnouncement, askProblem, type ComposerBlock, composerPlaceholder, costNote, headerLine } from '@/lib/ask/copy';
import { ASK_NEW_EVENT, askRouteThread } from '@/lib/ask/route';
import { isWorking, type AskTurnView } from '@/lib/ask/turns';
import { ASK_ROUTE } from '@/lib/recommendations/ask-href';
import { useSheet } from '@/lib/sheets/useSheet';
import { AskComposer } from './AskComposer';
import { AskHello, AskNotConfigured } from './AskHello';
import { AskBudget, AskThreadList, askUsageQuery, NewConversation } from './AskThreads';
import { AskTurn } from './AskTurn';
import { useAskLibrary } from './useAskLibrary';

const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
/** Esc stops a streaming answer, unless an overlay is open (Esc is its own). */
const overlayOpen = () => document.querySelector('[role="dialog"], [role="alertdialog"]') !== null;

/**
 * Ask momoshtrem (`/ask`, `/ask/{thread}`; PLAN F5, #156; js/screens/ask.js).
 * Desktop: a 300 px rail (new, recent, the month's AI spend), the
 * conversation and a sticky composer. Phone: its own header (Back, Past
 * conversations, New) in place of the tab bar, and the composer at the
 * bottom. `/ask?q=` asks at once. It lives in the route's layout, so the
 * stream survives the URL moving to the new thread.
 */
export function AskScreen() {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const routeThread = askRouteThread(pathname);
  const q = searchParams.get('q');
  const sheets = useSheet();
  const { session, turns, thread, working, send, stop, reset } = useAskConversation(routeThread);
  // The answered turns' titles are looked up in the library; a streaming turn's cards keep what Noir saw
  const library = useAskLibrary(turns.filter((turn) => !isWorking(turn)).flatMap((turn) => turn.items));
  const home = useQuery({ ...recommendationsHomeQuery(), refetchInterval: false });
  const usage = useQuery(askUsageQuery());
  const trakt = useQuery({ ...traktIntegrationQuery(), refetchInterval: false, retry: false });
  const [now] = useState(() => Date.now());
  const log = useRef<HTMLDivElement | null>(null);

  const notConfigured = home.data?.configured === false;
  const paused = session.paused && (!session.paused.resetsAt || Date.parse(session.paused.resetsAt) > now) ? session.paused : null;
  const block: ComposerBlock = notConfigured
    ? { kind: 'setup' }
    : paused
      ? { kind: 'paused', ...paused }
      : usage.data?.budget_reached
        ? { kind: 'paused', code: 'budget_exceeded', resetsAt: usage.data.resets_at }
        : null;
  const threadMissing = routeThread !== null && isApiError(thread.error) && thread.error.status === 404;
  const threadLoading = routeThread !== null && thread.isPending && session.live.length === 0;
  const empty = routeThread === null && turns.length === 0;
  const state = empty ? 'empty' : working ? 'working' : 'conversation';
  const problemContext = { usage: usage.data, online: typeof navigator === 'undefined' ? true : navigator.onLine };
  const newest = session.live.at(-1);

  useDocumentTitle(thread.data?.title ?? 'Ask momoshtrem');

  // /ask?q=… asks at once (the entry points' askHref); the same q again later is a new ask
  const askedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!q) {
      askedFor.current = null;
      return;
    }
    if (routeThread !== null || askedFor.current === q || notConfigured) return;
    askedFor.current = q;
    if (session.live.length > 0) reset();
    void send(q);
  }, [notConfigured, q, reset, routeThread, send, session.live.length]);

  // The history sheet's New conversation, and Esc to stop
  useEffect(() => {
    const onNew = () => reset();
    window.addEventListener(ASK_NEW_EVENT, onNew);
    return () => window.removeEventListener(ASK_NEW_EVENT, onNew);
  }, [reset]);
  useEffect(() => {
    if (!working) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented && !overlayOpen()) stop();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [stop, working]);

  // A new ask scrolls its message to the top, under the header
  const liveCount = session.live.length;
  const seenLive = useRef(liveCount);
  useEffect(() => {
    const message = newest ? log.current?.querySelector(`[data-turn="${newest.key}"]`) : null;
    // Only when it would move a fair way down (a follow-up under the fold), never to nudge the first ask
    if (liveCount > seenLive.current && message && message.getBoundingClientRect().top - parseFloat(getComputedStyle(message).scrollMarginTop) > 40) {
      message.scrollIntoView({ block: 'start', behavior: reducedMotion() ? 'auto' : 'smooth' });
    }
    seenLive.current = liveCount;
  }, [liveCount, newest]);

  const onNew = () => {
    if (routeThread === null) reset();
  };
  const onSend = (message: string, refine = false) => void send(message, { refine });
  // A turn refused before its stream was never saved, so the new one takes its place; one that failed mid-stream stays in the thread
  const onRetry = (turn: AskTurnView) => void send(turn.message, { refine: turn.refine, replaces: turn.live && turn.id === null ? turn.key : undefined });
  const lastIndex = turns.length - 1;

  let body;
  if (threadMissing) {
    body = (
      <EmptyState cat="peek" title="This conversation isn’t here" headingLevel="h1" actions={<NewConversation onNew={onNew} />}>
        It may have been removed, or the link is wrong. Start a new one.
      </EmptyState>
    );
  } else if (threadLoading) {
    body = (
      <LoadingRegion label="Loading the conversation" className="flex flex-col gap-6">
        <Skeleton height={52} width="60%" className="self-end" />
        <Skeleton height={36} width={220} />
        <Skeleton height={180} />
        <Skeleton height={180} />
      </LoadingRegion>
    );
  } else if (empty) {
    body = notConfigured ? <AskNotConfigured /> : <AskHello />;
  } else {
    body = (
      <>
        <h1 className="sr-only">{thread.data?.title ?? 'Ask momoshtrem'}</h1>
        {thread.isError && !threadMissing && (
          <Banner
            tone="error"
            title="Couldn’t load this conversation"
            action={
              <Button size="sm" onClick={() => void thread.refetch()} loading={thread.isFetching}>
                Try again
              </Button>
            }
          >
            Your library and Infuse aren’t affected.
          </Banner>
        )}
        {turns.map((turn, index) => (
          <AskTurn
            key={turn.key}
            turn={turn}
            last={index === lastIndex}
            idle={!working && !block}
            library={library}
            problemContext={problemContext}
            now={now}
            onRefine={(refinement) => onSend(refinement, true)}
            onRetry={onRetry}
            onNew={() => (routeThread === null ? reset() : router.push(ASK_ROUTE))}
          />
        ))}
      </>
    );
  }

  return (
    <div className="ask-screen" data-state={state}>
      <header className="ask-bar">
        <ButtonLink href="/" variant="ghost" iconOnly aria-label="Back to For You">
          <ChevronLeft size={24} aria-hidden="true" />
        </ButtonLink>
        <div className="ask-bar-title">
          <CatAvatar size={28} />
          <span>
            <span className="font-semibold">Ask momoshtrem</span>
            <span className="t-small text-fg-3">{headerLine(block, trakt.data?.state === 'connected')}</span>
          </span>
        </div>
        <IconButton aria-label="Past conversations" onClick={() => sheets.open('ask-history')}>
          <History size={22} aria-hidden="true" />
        </IconButton>
        <ButtonLink href={ASK_ROUTE} variant="ghost" iconOnly className="ask-bar-new" aria-label="New conversation" onClick={onNew}>
          <Plus size={22} aria-hidden="true" />
        </ButtonLink>
      </header>

      <div className="ask-layout">
        <aside className="ask-rail" aria-label="Conversations">
          <div className="ask-rail-in">
            <NewConversation onNew={onNew} className="ask-new" />
            <h2 className="ask-rail-h t-micro text-fg-3">Recent</h2>
            <AskThreadList current={session.threadId} />
            <AskBudget />
          </div>
        </aside>

        <section className="ask-main" aria-label="Conversation">
          <div className="ask-log" ref={log}>
            {body}
          </div>
          <p className="sr-only" role="status" aria-live="polite">
            {askAnnouncement(newest, newest ? askProblem(newest, problemContext) : null)}
          </p>
          <AskComposer
            focusOnDesktop={empty}
            placeholder={composerPlaceholder(block, empty)}
            disabled={block !== null || threadMissing}
            working={Boolean(working)}
            note={costNote(usage.data?.estimates.ask_usd)}
            onSend={(message) => onSend(message)}
            onStop={stop}
          />
        </section>
      </div>
    </div>
  );
}
