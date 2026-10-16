import type { AskErrorCode, AskEvent, AskItem, AskStep, AskThread, AskTurn } from '@/lib/api/contract';
import { applyStatus } from '@/lib/recommendations/steps';

/**
 * Ask momoshtrem's conversation as the page shows it (PLAN §6 "Ask
 * momoshtrem"; #156): the turns read from the thread, then the ones sent
 * from this page, each folded from its stream's events. Pure: the page
 * (`useAskConversation`) runs the requests and dispatches what happened.
 */

/** sending: the POST is out, no event yet. streaming: `start` arrived. The rest are ended. */
export type AskTurnPhase = 'sending' | 'streaming' | 'done' | 'error' | 'cancelled';

/** Why a turn never streamed: the answer before the stream (an ApiError, e.g. 429 budget_exceeded), or no answer at all (status 0). */
export interface AskFailure {
  status: number;
  code: string;
  message: string;
  /** The 429s: when asks are back. */
  resetsAt: string | null;
}

/** `connection`: the stream stopped without `done` or `error` (the connection dropped). */
export type AskStreamErrorCode = AskErrorCode | 'connection';

export interface AskTurnView {
  /** React's key: `turn:{id}` for a saved turn, the page's own key for one sent from it. */
  key: string;
  /** The turn's id once `start` named it. */
  id: number | null;
  message: string;
  phase: AskTurnPhase;
  /** A refinement chip: it re-ranks (`rerank`, `write`) instead of reading history, the web and TMDB. */
  refine: boolean;
  /** Sent from this page, so this page can stop it. A saved turn still streaming is another tab's. */
  live: boolean;
  steps: AskStep[];
  intro: string;
  items: AskItem[];
  refinements: string[];
  /** The stream's `error` event. */
  error: { code: AskStreamErrorCode; message: string } | null;
  /** The answer before the stream. */
  failure: AskFailure | null;
  elapsedMs: number | null;
  /** A saved turn's; null for one sent from this page ("just now"). */
  createdAt: string | null;
}

/** The 429s that turn the composer off until `resetsAt`. */
export type AskPauseCode = 'budget_exceeded' | 'ask_limit';

export interface AskSession {
  /** The thread in the URL this session last saw: null on /ask. */
  route: number | null;
  /** The conversation's thread: the route's, or the one `start` just opened. */
  threadId: number | null;
  /** Turns sent from this page, oldest first. */
  live: AskTurnView[];
  /** The last 429 (budget or daily limit): asks pause until it lifts. Kept across conversations. */
  paused: { code: AskPauseCode; resetsAt: string | null } | null;
}

export type AskAction =
  /** The URL's thread changed (a link, Back, or the page's own replace after `start`). */
  | { type: 'route'; thread: number | null }
  /** New conversation, on the same URL. */
  | { type: 'reset' }
  /** `replaces`: Try again on a turn sent from this page takes its place. */
  | { type: 'send'; key: string; message: string; refine: boolean; replaces?: string }
  | { type: 'event'; key: string; event: AskEvent }
  /** Stop: the request was aborted. */
  | { type: 'stopped'; key: string }
  /** The request was refused or never answered. */
  | { type: 'failed'; key: string; failure: AskFailure }
  /** The stream ended (or dropped) without `done` or `error`. */
  | { type: 'cut'; key: string };

export function initialSession(route: number | null): AskSession {
  return { route, threadId: route, live: [], paused: null };
}

const PAUSES: ReadonlySet<string> = new Set<AskPauseCode>(['budget_exceeded', 'ask_limit']);

export const isWorking = (turn: Pick<AskTurnView, 'phase'>): boolean => turn.phase === 'sending' || turn.phase === 'streaming';

/** An ended turn never shows a spinner: the step still active when it failed or stopped is dropped (PLAN §6). */
const settled = (steps: AskStep[]): AskStep[] => steps.filter((step) => step.state !== 'active');

const sameTitle = (a: AskItem, b: AskItem) => a.media_type === b.media_type && a.tmdb_id === b.tmdb_id;

/** Fold one event into a turn. Events after the turn ended are ignored. */
export function applyAskEvent(turn: AskTurnView, event: AskEvent): AskTurnView {
  if (!isWorking(turn)) return turn;
  switch (event.event) {
    case 'start':
      return { ...turn, id: event.data.turn_id, phase: 'streaming' };
    case 'status':
      return { ...turn, phase: 'streaming', steps: applyStatus(turn.steps, event.data) };
    case 'text':
      return { ...turn, phase: 'streaming', intro: turn.intro + event.data.delta };
    case 'item': {
      // A title named twice keeps its first place
      const index = turn.items.findIndex((item) => sameTitle(item, event.data));
      const items = index === -1 ? [...turn.items, event.data] : turn.items.map((item, i) => (i === index ? event.data : item));
      return { ...turn, phase: 'streaming', items };
    }
    case 'error':
      return { ...turn, phase: 'error', steps: settled(turn.steps), error: event.data };
    case 'done':
      return {
        ...turn,
        phase: 'done',
        // When it ends, every step is done (the server sends each `done`; a dropped one mustn't spin forever)
        steps: turn.steps.map((step) => (step.state === 'active' ? { ...step, state: 'done' } : step)),
        refinements: event.data.refinements,
        elapsedMs: event.data.elapsed_ms,
      };
    default:
      return turn;
  }
}

function updateTurn(session: AskSession, key: string, update: (turn: AskTurnView) => AskTurnView): AskSession {
  const index = session.live.findIndex((turn) => turn.key === key);
  // A turn from a conversation the page has left: its late events change nothing
  if (index === -1) return session;
  const live = [...session.live];
  live[index] = update(live[index]);
  return { ...session, live };
}

export function askReducer(session: AskSession, action: AskAction): AskSession {
  switch (action.type) {
    case 'route':
      if (action.thread === session.route) return session;
      // The page's own replace after `start` (/ask → /ask/{thread}): the conversation goes on
      if (action.thread !== null && action.thread === session.threadId) return { ...session, route: action.thread };
      return { ...initialSession(action.thread), paused: session.paused };
    case 'reset':
      return { ...initialSession(null), paused: session.paused };
    case 'send': {
      const turn: AskTurnView = {
        key: action.key,
        id: null,
        message: action.message,
        phase: 'sending',
        refine: action.refine,
        live: true,
        steps: [],
        intro: '',
        items: [],
        refinements: [],
        error: null,
        failure: null,
        elapsedMs: null,
        createdAt: null,
      };
      const kept = action.replaces ? session.live.filter((t) => t.key !== action.replaces || isWorking(t)) : session.live;
      return { ...session, live: [...kept, turn] };
    }
    case 'event': {
      if (!session.live.some((turn) => turn.key === action.key)) return session;
      const next = updateTurn(session, action.key, (turn) => applyAskEvent(turn, action.event));
      if (action.event.event === 'start') return { ...next, threadId: action.event.data.thread_id };
      // A turn that streams means the budget and the daily limit let it through
      return action.event.event === 'done' ? { ...next, paused: null } : next;
    }
    case 'stopped':
      return updateTurn(session, action.key, (turn) => (isWorking(turn) ? { ...turn, phase: 'cancelled', steps: settled(turn.steps) } : turn));
    case 'failed': {
      const next = updateTurn(session, action.key, (turn) => (isWorking(turn) ? { ...turn, phase: 'error', steps: settled(turn.steps), failure: action.failure } : turn));
      if (next === session || !PAUSES.has(action.failure.code)) return next;
      return { ...next, paused: { code: action.failure.code as AskPauseCode, resetsAt: action.failure.resetsAt } };
    }
    case 'cut':
      return updateTurn(session, action.key, (turn) =>
        isWorking(turn) ? { ...turn, phase: 'error', steps: settled(turn.steps), error: { code: 'connection', message: '' } } : turn
      );
    default:
      return session;
  }
}

/** A turn read from GET /recommendations/threads/{id}. */
export function turnFromSaved(turn: AskTurn): AskTurnView {
  return {
    key: `turn:${turn.id}`,
    id: turn.id,
    message: turn.message,
    phase: turn.status,
    refine: turn.steps.some((step) => step.step === 'rerank'),
    live: false,
    steps: turn.status === 'streaming' ? turn.steps : settled(turn.steps),
    intro: turn.intro,
    items: turn.items,
    refinements: turn.refinements,
    error: turn.error,
    failure: null,
    elapsedMs: turn.elapsed_ms,
    createdAt: turn.created_at,
  };
}

/**
 * The turns to show: the thread's saved turns, then the ones sent from this
 * page (their streamed view wins over the saved copy of the same turn, which
 * is empty while it streams). A turn refused before its stream has no id and
 * is never saved, so it stays where it was sent.
 */
export function visibleTurns(thread: Pick<AskThread, 'turns'> | undefined, live: readonly AskTurnView[]): AskTurnView[] {
  const liveIds = new Set(live.flatMap((turn) => (turn.id === null ? [] : [turn.id])));
  const saved = (thread?.turns ?? []).filter((turn) => !liveIds.has(turn.id)).map(turnFromSaved);
  return [...saved, ...live];
}
