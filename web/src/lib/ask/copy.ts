import type { AskItem, AskStep, AiUsage } from '@/lib/api/contract';
import { stepLabel } from '@/lib/recommendations/steps';
import { timeAgo } from '@/lib/settings/trakt';
import { formatRuntime, formatShortDate, formatYears, plural } from '@/lib/title/format';
import type { AskFailure, AskPauseCode, AskTurnView } from './turns';

/**
 * Ask momoshtrem's words (js/screens/ask.js): the trace line, card facts,
 * the composer, the thread list's dates, and what each failure says with
 * the way forward. Pure.
 */

// ---- When asks are back (`resets_at`) -------------------------------------------------

const WALL_CLOCK = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::\d{2}(?:\.\d+)?)?([+-]\d{2}:?\d{2})$/;

interface WallClock {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
}

/**
 * The server-local date and time of a timestamp. Go writes `time.Local`'s
 * offset ("2026-11-01T00:00:00+03:00"), so its wall clock is the server's,
 * whatever the browser's zone; a UTC ("Z") timestamp is read in the
 * browser's zone, which is the server's when they share one (and the mocks').
 */
function wallClock(timestamp: string): WallClock | null {
  const match = WALL_CLOCK.exec(timestamp);
  if (match) {
    const [year, month, day, hour, minute] = match.slice(1, 6).map(Number);
    return { year, month, day, hour, minute };
  }
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return null;
  return { year: date.getFullYear(), month: date.getMonth() + 1, day: date.getDate(), hour: date.getHours(), minute: date.getMinutes() };
}

const DAY_MONTH = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const MONTH = new Intl.DateTimeFormat('en-US', { month: 'long' });

/**
 * When a pause lifts, for "…pause until Nov 1" and "Asks are paused until
 * midnight": the budget's date (the 1st of next month), the daily limit's
 * time (the next midnight, "02:00" otherwise).
 */
export function pausedUntil(code: AskPauseCode, resetsAt: string | null): string {
  const clock = resetsAt ? wallClock(resetsAt) : null;
  if (code === 'budget_exceeded') return clock ? DAY_MONTH.format(new Date(clock.year, clock.month - 1, clock.day)) : 'next month';
  if (!clock || (clock.hour === 0 && clock.minute === 0)) return 'midnight';
  return `${String(clock.hour).padStart(2, '0')}:${String(clock.minute).padStart(2, '0')}`;
}

/** "2026-10" → "October". */
export function monthName(month: string): string | null {
  const match = /^(\d{4})-(\d{2})$/.exec(month);
  return match ? MONTH.format(new Date(Number(match[1]), Number(match[2]) - 1, 1)) : null;
}

export const formatUsd = (usd: number): string => `$${usd.toFixed(2)}`;

// ---- Problems: a turn that didn't get its answer ----------------------------------------

export type AskProblemIcon = 'budget' | 'limit' | 'full' | 'busy' | 'setup' | 'gone' | 'offline' | 'error';
/** What the card offers: Settings › AI, Discover, New conversation, Try again (resends the message). */
export type AskProblemAction = 'settings' | 'discover' | 'new' | 'retry';

export interface AskProblem {
  icon: AskProblemIcon;
  /** `calm`: nothing broke (a budget or a limit). `error`: something failed. */
  tone: 'calm' | 'error';
  title: string;
  body: string;
  actions: AskProblemAction[];
}

export interface AskProblemContext {
  /** GET /settings/ai/usage, when it's been read: the budget and the month. */
  usage?: Pick<AiUsage, 'budget_usd' | 'month' | 'ask_daily_limit'> | null;
  /** navigator.onLine when it failed. */
  online?: boolean;
}

const OFFLINE_CODES: ReadonlySet<string> = new Set(['backend_unreachable', 'bad_gateway', 'timeout']);
const KEEPS_WORKING = 'Everything else keeps working: your library, torrents and Infuse.';

function budgetProblem(failure: AskFailure, { usage }: AskProblemContext): AskProblem {
  const until = pausedUntil('budget_exceeded', failure.resetsAt);
  const month = usage ? monthName(usage.month) : null;
  const spent =
    usage?.budget_usd != null && month
      ? `You set ${formatUsd(usage.budget_usd)} for ${month} and it’s all spent`
      : 'This month’s budget is all spent';
  return {
    icon: 'budget',
    tone: 'calm',
    title: 'This month’s AI budget is used up',
    body: `${spent}, so asks and new picks pause until ${until}. ${KEEPS_WORKING}`,
    actions: ['settings', 'discover'],
  };
}

function failureProblem(failure: AskFailure, context: AskProblemContext): AskProblem {
  switch (failure.code) {
    case 'budget_exceeded':
      return budgetProblem(failure, context);
    case 'ask_limit': {
      const limit = context.usage?.ask_daily_limit;
      const asked = limit ? `You’ve asked ${limit} times today, the daily limit` : 'You’ve reached today’s limit';
      return {
        icon: 'limit',
        tone: 'calm',
        title: 'That’s all the asks for today',
        body: `${asked}, so asks pause until ${pausedUntil('ask_limit', failure.resetsAt)}. Your picks on For You, your library and Infuse keep working.`,
        actions: ['discover'],
      };
    }
    case 'thread_full':
      return {
        icon: 'full',
        tone: 'calm',
        title: 'This conversation is full',
        body: 'A conversation holds up to 20 asks. Start a new one to keep going; this one stays under Recent.',
        actions: ['new'],
      };
    case 'ask_busy':
      return {
        icon: 'busy',
        tone: 'calm',
        title: 'Noir is answering another ask',
        body: 'One ask runs at a time, maybe in another tab. Try again once it’s done.',
        actions: ['retry'],
      };
    case 'unavailable':
      return {
        icon: 'setup',
        tone: 'calm',
        title: 'Ask momoshtrem isn’t set up',
        body: 'It needs an Anthropic API key (ANTHROPIC_API_KEY) on the server. Your library, torrents and Infuse work without it.',
        actions: ['settings'],
      };
    case 'not_found':
      return { icon: 'gone', tone: 'calm', title: 'This conversation is gone', body: 'It may have been removed. Start a new one.', actions: ['new'] };
  }
  if (context.online === false) {
    return { icon: 'offline', tone: 'error', title: 'You’re offline', body: 'Noir couldn’t start. Try again once you’re back online.', actions: ['retry'] };
  }
  if (failure.status === 0 || OFFLINE_CODES.has(failure.code)) {
    return {
      icon: 'offline',
      tone: 'error',
      title: 'Couldn’t reach momoshtrem',
      body: 'Noir couldn’t start because the server didn’t answer. Check that it’s running, then try again.',
      actions: ['retry'],
    };
  }
  return { icon: 'error', tone: 'error', title: 'Noir couldn’t start', body: failure.message || 'Something went wrong on the server.', actions: ['retry'] };
}

function streamProblem(error: NonNullable<AskTurnView['error']>): AskProblem {
  switch (error.code) {
    case 'refused':
      return { icon: 'error', tone: 'calm', title: 'Noir couldn’t answer that', body: error.message || 'Claude declined this one. Try asking another way.', actions: [] };
    case 'limit_reached':
      return {
        icon: 'limit',
        tone: 'calm',
        title: 'That ask grew too big to finish',
        body: error.message || 'Noir reached its limit for one answer. Try a narrower ask.',
        actions: [],
      };
    case 'upstream':
      return {
        icon: 'error',
        tone: 'error',
        title: 'Claude didn’t answer',
        body: error.message || 'The Anthropic API failed after a few tries. Try again in a minute.',
        actions: ['retry'],
      };
    case 'connection':
      return {
        icon: 'offline',
        tone: 'error',
        title: 'The answer was cut off',
        body: 'The connection dropped before Noir finished. Your library and Infuse aren’t affected.',
        actions: ['retry'],
      };
    default:
      return { icon: 'error', tone: 'error', title: 'Something went wrong', body: error.message || 'The server couldn’t finish this answer.', actions: ['retry'] };
  }
}

/** What an ended turn without its answer says, or null when it has one (done, or stopped). */
export function askProblem(turn: Pick<AskTurnView, 'failure' | 'error'>, context: AskProblemContext = {}): AskProblem | null {
  if (turn.failure) return failureProblem(turn.failure, context);
  if (turn.error) return streamProblem(turn.error);
  return null;
}

// ---- The turn -----------------------------------------------------------------------------

const CHECKED: Partial<Record<AskStep['step'], string>> = { history: 'history', web: 'web', tmdb: 'TMDB' };

/** "History, web and TMDB checked · 9 s"; a refinement's "Re-ranked your picks · 4 s". */
export function traceLabel(steps: readonly AskStep[], elapsedMs: number | null): string {
  const seconds = elapsedMs === null ? '' : ` · ${Math.max(1, Math.round(elapsedMs / 1000))} s`;
  if (steps.some((step) => step.step === 'rerank')) return `Re-ranked your picks${seconds}`;
  const names = steps.flatMap((step) => (CHECKED[step.step] ? [CHECKED[step.step]!] : []));
  if (names.length === 0) return `Picks written${seconds}`;
  const list = names.length === 1 ? names[0] : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
  return `${list.charAt(0).toUpperCase()}${list.slice(1)} checked${seconds}`;
}

/** The card's facts (ask.js `card`): "2016–2020 · 4 seasons · 22 min", "2016 · Movie · 1h 56m". The rating shows apart, with its star. */
export function askItemFacts(item: Pick<AskItem, 'media_type' | 'year' | 'year_end' | 'seasons' | 'runtime_minutes'>): string[] {
  if (item.media_type === 'movie') {
    return [item.year === null ? null : String(item.year), 'Movie', formatRuntime(item.runtime_minutes)].filter((fact): fact is string => fact !== null);
  }
  return [
    formatYears(item.year, item.year_end, item.year_end === null),
    item.seasons ? plural(item.seasons, 'season') : 'Show',
    item.runtime_minutes ? `${item.runtime_minutes} min` : null,
  ].filter((fact): fact is string => fact !== null);
}

/** TMDB's vote: "8.2", "8" (never "8.0"); null without votes. */
export const askRating = (vote: number | null): string | null => (vote === null || vote <= 0 ? null : String(Number(vote.toFixed(1))));

/** "just now", "4 min ago", "yesterday"; a date after a week. */
export function turnWhen(createdAt: string, now: number): string {
  const days = (now - Date.parse(createdAt)) / 86_400_000;
  return days < 7 ? timeAgo(createdAt, now) : formatShortDate(createdAt, new Date(now));
}

/** The thread list's date (ask.js `askThreads`): "Now", "Today", "Yesterday", "Oct 2". */
export function threadWhen(updatedAt: string, now: number): string {
  const then = new Date(updatedAt);
  if (Number.isNaN(then.getTime())) return '';
  if (now - then.getTime() < 2 * 60_000) return 'Now';
  const today = new Date(now);
  const startOfToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
  if (then.getTime() >= startOfToday) return 'Today';
  if (then.getTime() >= startOfToday - 86_400_000) return 'Yesterday';
  return formatShortDate(updatedAt, today);
}

// ---- The composer ------------------------------------------------------------------------

export type ComposerBlock = { kind: 'paused'; code: AskPauseCode; resetsAt: string | null } | { kind: 'setup' } | null;

/** The field's placeholder: why it's off, or what to type. */
export function composerPlaceholder(block: ComposerBlock, empty: boolean): string {
  if (block?.kind === 'setup') return 'Ask needs an Anthropic API key';
  if (block?.kind === 'paused') return `Asks are paused until ${pausedUntil(block.code, block.resetsAt)}`;
  return empty ? 'Ask for anything to watch…' : 'Ask a follow-up, or refine…';
}

/** The phone header's second line. */
export function headerLine(block: ComposerBlock, traktConnected: boolean): string {
  if (block?.kind === 'paused') return `Paused until ${pausedUntil(block.code, block.resetsAt)}`;
  return traktConnected ? 'Noir · knows your Trakt' : 'Noir · a reason for every pick';
}

/** The desktop composer's note: what Noir reads and what an ask costs (the mean of the last 20, when known). */
export function costNote(askUsd: number | null | undefined): string {
  const reads = 'Noir uses your Trakt history and TMDB.';
  return askUsd != null && askUsd > 0 ? `${reads} An ask costs about ${formatUsd(askUsd)}.` : reads;
}

// ---- Screen readers ------------------------------------------------------------------------

/** What the page's live region says about the turn it's streaming (or just ended): each step as it starts, then the outcome. */
export function askAnnouncement(turn: Pick<AskTurnView, 'phase' | 'steps' | 'items' | 'intro'> | undefined, problem: AskProblem | null): string {
  if (!turn) return '';
  switch (turn.phase) {
    case 'sending':
      return 'Noir is thinking…';
    case 'streaming': {
      if (turn.intro) return 'Noir is writing…';
      // Between one step's `done` and the next one's `active`, the last step's words stay, so nothing is said twice
      const step = turn.steps.find((s) => s.state === 'active') ?? turn.steps.at(-1);
      return step ? stepLabel(step.step, 'active') : 'Noir is thinking…';
    }
    case 'done':
      return turn.items.length > 0 ? `Noir found ${plural(turn.items.length, 'pick')}.` : 'Noir answered.';
    case 'cancelled':
      return 'Stopped.';
    case 'error':
      return problem?.title ?? 'Noir couldn’t finish.';
  }
}

/** The six example asks of the empty state (ask.js `EXAMPLES`); the first three are the highlighted ones. */
export const ASK_EXAMPLES = [
  'something like Severance but lighter',
  'a 90-minute thriller for tonight',
  'anime with a Frieren vibe',
  'a cosy show to fall asleep to',
  'films to watch with my parents',
  'what’s new this month for me?',
] as const;
