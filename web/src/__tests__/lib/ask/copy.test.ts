import {
  askAnnouncement,
  askItemFacts,
  askProblem,
  askRating,
  composerPlaceholder,
  costNote,
  headerLine,
  monthName,
  pausedUntil,
  threadWhen,
  traceLabel,
} from '@/lib/ask/copy';
import type { AskStep } from '@/lib/api/contract';

/** Ask's words (#156; js/screens/ask.js): when asks are back, each failure's card, the trace, card facts and the composer. */

const USAGE = { budget_usd: 5, month: '2026-10', ask_daily_limit: 30 };

describe('pausedUntil', () => {
  it('says the budget’s date and the limit’s time on the server’s clock', () => {
    expect(pausedUntil('budget_exceeded', '2026-11-01T00:00:00+03:00')).toBe('Nov 1');
    expect(pausedUntil('budget_exceeded', '2026-11-01T00:00:00-05:00')).toBe('Nov 1');
    // A UTC timestamp is read in the browser's zone (the mocks', and a server sharing it)
    expect(pausedUntil('budget_exceeded', new Date(2026, 10, 1).toISOString())).toBe('Nov 1');
    expect(pausedUntil('ask_limit', '2026-10-17T00:00:00+02:00')).toBe('midnight');
    expect(pausedUntil('ask_limit', '2026-10-17T02:30:00+02:00')).toBe('02:30');
    expect(pausedUntil('ask_limit', new Date(2026, 9, 17).toISOString())).toBe('midnight');
  });

  it('falls back without a date', () => {
    expect(pausedUntil('budget_exceeded', null)).toBe('next month');
    expect(pausedUntil('ask_limit', null)).toBe('midnight');
    expect(pausedUntil('budget_exceeded', 'soon')).toBe('next month');
  });

  it('names the month', () => {
    expect(monthName('2026-10')).toBe('October');
    expect(monthName('nope')).toBeNull();
  });
});

describe('askProblem', () => {
  const failure = (code: string, status: number, resetsAt: string | null = null) => ({ failure: { status, code, message: 'From the server', resetsAt }, error: null });

  it('says the design’s budget copy, with the budget and month when usage is known', () => {
    expect(askProblem(failure('budget_exceeded', 429, '2026-11-01T00:00:00+03:00'), { usage: USAGE })).toEqual({
      icon: 'budget',
      tone: 'calm',
      title: 'This month’s AI budget is used up',
      body: 'You set $5.00 for October and it’s all spent, so asks and new picks pause until Nov 1. Everything else keeps working: your library, torrents and Infuse.',
      actions: ['settings', 'discover'],
    });
    expect(askProblem(failure('budget_exceeded', 429, '2026-11-01T00:00:00+03:00'))?.body).toBe(
      'This month’s budget is all spent, so asks and new picks pause until Nov 1. Everything else keeps working: your library, torrents and Infuse.'
    );
  });

  it('explains the daily limit, a full thread, a busy Noir, no key and a missing thread', () => {
    expect(askProblem(failure('ask_limit', 429, '2026-10-17T00:00:00+03:00'), { usage: USAGE })).toMatchObject({
      title: 'That’s all the asks for today',
      body: 'You’ve asked 30 times today, the daily limit, so asks pause until midnight. Your picks on For You, your library and Infuse keep working.',
      actions: ['discover'],
    });
    expect(askProblem(failure('thread_full', 409))).toMatchObject({ title: 'This conversation is full', actions: ['new'] });
    expect(askProblem(failure('ask_busy', 409))).toMatchObject({ title: 'Noir is answering another ask', actions: ['retry'] });
    expect(askProblem(failure('unavailable', 503))).toMatchObject({ title: 'Ask momoshtrem isn’t set up', actions: ['settings'] });
    expect(askProblem(failure('not_found', 404))).toMatchObject({ title: 'This conversation is gone', actions: ['new'] });
  });

  it('tells offline from a server that didn’t answer, and offers Try again', () => {
    expect(askProblem(failure('network', 0), { online: false })).toMatchObject({ title: 'You’re offline', tone: 'error', actions: ['retry'] });
    expect(askProblem(failure('network', 0))).toMatchObject({ title: 'Couldn’t reach momoshtrem', actions: ['retry'] });
    expect(askProblem(failure('backend_unreachable', 502))).toMatchObject({ title: 'Couldn’t reach momoshtrem' });
    expect(askProblem(failure('internal_error', 500))).toMatchObject({ title: 'Noir couldn’t start', body: 'From the server', actions: ['retry'] });
  });

  it('explains a stream’s error by its code', () => {
    const streamed = (code: 'refused' | 'limit_reached' | 'upstream' | 'internal' | 'connection', message = '') => ({ failure: null, error: { code, message } });
    expect(askProblem(streamed('upstream', 'The Anthropic API didn’t answer. Try again in a minute.'))).toEqual({
      icon: 'error',
      tone: 'error',
      title: 'Claude didn’t answer',
      body: 'The Anthropic API didn’t answer. Try again in a minute.',
      actions: ['retry'],
    });
    // Asking the same again won't help a refusal or an ask that was too big
    expect(askProblem(streamed('refused'))).toMatchObject({ title: 'Noir couldn’t answer that', actions: [] });
    expect(askProblem(streamed('limit_reached'))).toMatchObject({ title: 'That ask grew too big to finish', actions: [] });
    expect(askProblem(streamed('connection'))).toMatchObject({ title: 'The answer was cut off', actions: ['retry'] });
    expect(askProblem(streamed('internal'))).toMatchObject({ title: 'Something went wrong', actions: ['retry'] });
    expect(askProblem({ failure: null, error: null })).toBeNull();
  });
});

describe('the turn', () => {
  const done = (step: AskStep['step'], detail: string): AskStep => ({ step, state: 'done', detail });

  it('traces what was checked and how long it took', () => {
    expect(traceLabel([done('history', 'a'), done('web', 'b'), done('tmdb', 'c'), done('write', 'd')], 9100)).toBe('History, web and TMDB checked · 9 s');
    expect(traceLabel([done('rerank', '23 checked'), done('write', '3 picks')], 4200)).toBe('Re-ranked your picks · 4 s');
    expect(traceLabel([done('history', 'a')], null)).toBe('History checked');
    expect(traceLabel([done('web', 'a'), done('tmdb', 'b')], 300)).toBe('Web and TMDB checked · 1 s');
    expect(traceLabel([done('write', 'd')], 2000)).toBe('Picks written · 2 s');
  });

  it('lists a card’s facts: years, seasons or Movie, runtime', () => {
    expect(askItemFacts({ media_type: 'tv', year: 2016, year_end: 2020, seasons: 4, runtime_minutes: 22 })).toEqual(['2016–2020', '4 seasons', '22 min']);
    expect(askItemFacts({ media_type: 'tv', year: 2023, year_end: null, seasons: 1, runtime_minutes: null })).toEqual(['2023–', '1 season']);
    expect(askItemFacts({ media_type: 'tv', year: 2026, year_end: null, seasons: null, runtime_minutes: 55 })).toEqual(['2026–', 'Show', '55 min']);
    expect(askItemFacts({ media_type: 'movie', year: 2016, year_end: null, seasons: null, runtime_minutes: 116 })).toEqual(['2016', 'Movie', '1h 56m']);
    expect(askRating(8)).toBe('8');
    expect(askRating(8.24)).toBe('8.2');
    expect(askRating(null)).toBeNull();
    expect(askRating(0)).toBeNull();
  });

  it('dates threads as the rail does', () => {
    const now = new Date(2026, 9, 16, 15, 0).getTime();
    expect(threadWhen(new Date(now - 30_000).toISOString(), now)).toBe('Now');
    expect(threadWhen(new Date(2026, 9, 16, 9, 0).toISOString(), now)).toBe('Today');
    expect(threadWhen(new Date(2026, 9, 15, 21, 0).toISOString(), now)).toBe('Yesterday');
    expect(threadWhen(new Date(2026, 9, 2, 12, 0).toISOString(), now)).toBe('Oct 2');
  });

  it('announces each step as it starts, then how it ended', () => {
    const turn = (phase: 'sending' | 'streaming' | 'done' | 'cancelled' | 'error', steps: AskStep[] = [], items = 0, intro = '') => ({
      phase,
      steps,
      intro,
      items: Array.from({ length: items }, () => ({}) as never),
    });
    expect(askAnnouncement(undefined, null)).toBe('');
    expect(askAnnouncement(turn('sending'), null)).toBe('Noir is thinking…');
    expect(askAnnouncement(turn('streaming', [done('history', 'a'), { step: 'web', state: 'active', detail: null }]), null)).toBe('Searching the web for new releases…');
    // Between a step's done and the next one's start, the words don't change (nothing is announced twice)
    expect(askAnnouncement(turn('streaming', [done('history', 'a')]), null)).toBe('Reading your history…');
    expect(askAnnouncement(turn('streaming'), null)).toBe('Noir is thinking…');
    expect(askAnnouncement(turn('streaming', [done('write', 'a')], 0, 'Five'), null)).toBe('Noir is writing…');
    expect(askAnnouncement(turn('done', [], 5), null)).toBe('Noir found 5 picks.');
    expect(askAnnouncement(turn('cancelled'), null)).toBe('Stopped.');
    expect(askAnnouncement(turn('error'), { icon: 'error', tone: 'error', title: 'Claude didn’t answer', body: '', actions: [] })).toBe('Claude didn’t answer');
  });
});

describe('the composer', () => {
  it('says why it’s off, or what to type', () => {
    expect(composerPlaceholder(null, true)).toBe('Ask for anything to watch…');
    expect(composerPlaceholder(null, false)).toBe('Ask a follow-up, or refine…');
    expect(composerPlaceholder({ kind: 'paused', code: 'budget_exceeded', resetsAt: '2026-11-01T00:00:00+03:00' }, false)).toBe('Asks are paused until Nov 1');
    expect(composerPlaceholder({ kind: 'paused', code: 'ask_limit', resetsAt: null }, true)).toBe('Asks are paused until midnight');
    expect(composerPlaceholder({ kind: 'setup' }, true)).toBe('Ask needs an Anthropic API key');
  });

  it('heads the phone screen and notes the cost', () => {
    expect(headerLine(null, true)).toBe('Noir · knows your Trakt');
    expect(headerLine(null, false)).toBe('Noir · a reason for every pick');
    expect(headerLine({ kind: 'paused', code: 'budget_exceeded', resetsAt: '2026-11-01T00:00:00Z' }, true)).toMatch(/^Paused until (Oct 31|Nov 1)$/);
    expect(costNote(0.02)).toBe('Noir uses your Trakt history and TMDB. An ask costs about $0.02.');
    expect(costNote(null)).toBe('Noir uses your Trakt history and TMDB.');
  });
});
