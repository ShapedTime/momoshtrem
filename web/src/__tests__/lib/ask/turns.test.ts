import type { AskEvent, AskItem, AskTurn } from '@/lib/api/contract';
import { askReducer, initialSession, isWorking, turnFromSaved, visibleTurns, workingTurn, type AskAction, type AskSession } from '@/lib/ask/turns';

/** Ask's conversation reducer (#156): the events of POST /recommendations/ask folded into the turn the page shows. */

const item = (tmdbId: number, title: string, overrides: Partial<AskItem> = {}): AskItem => ({
  media_type: 'tv',
  tmdb_id: tmdbId,
  title,
  year: 2016,
  year_end: 2020,
  poster_path: null,
  seasons: 4,
  runtime_minutes: 22,
  vote_average: 8.1,
  release_date: '2016-09-19',
  badge: null,
  reason: 'Kind, fast and very funny.',
  library: null,
  on_watchlist: false,
  feedback: null,
  match_tag: 'Mystery + workplace · lighter',
  ...overrides,
});

const GOOD_PLACE = item(66573, 'The Good Place');
const MYTHIC_QUEST = item(94605, 'Mythic Quest');

const START: AskEvent = { event: 'start', data: { thread_id: 13, turn_id: 46, title: 'Something like Severance but lighter' } };
const DONE: AskEvent = {
  event: 'done',
  data: { thread_id: 13, turn_id: 46, elapsed_ms: 9100, refinements: ['Lighter', 'Shorter'], usage: { input_tokens: 1, output_tokens: 1, web_searches: 2, cost_usd: 0.02 } },
};
const status = (step: 'history' | 'web' | 'tmdb' | 'write' | 'rerank', state: 'active' | 'done', detail: string | null = null): AskEvent => ({
  event: 'status',
  data: { step, state, detail },
});

function run(actions: AskAction[], from: AskSession = initialSession(null)): AskSession {
  return actions.reduce(askReducer, from);
}

const send = (key = 'a', message = 'something like Severance but lighter', refine = false): AskAction => ({ type: 'send', key, message, refine });
const events = (key: string, list: AskEvent[]): AskAction[] => list.map((event) => ({ type: 'event', key, event }));

describe('askReducer: a turn streams', () => {
  it('sends, starts the thread, runs its steps, streams the intro and the cards, then ends done', () => {
    let session = run([send()]);
    expect(session.live).toHaveLength(1);
    expect(session.live[0]).toMatchObject({ key: 'a', id: null, phase: 'sending', live: true, steps: [], createdAt: null });
    expect(workingTurn(session)?.key).toBe('a');

    session = run(events('a', [START, status('history', 'active'), status('history', 'done', '2,184 plays'), status('web', 'active')]), session);
    expect(session.threadId).toBe(13);
    expect(session.route).toBeNull();
    expect(session.live[0]).toMatchObject({ id: 46, phase: 'streaming' });
    expect(session.live[0].steps).toEqual([
      { step: 'history', state: 'done', detail: '2,184 plays' },
      { step: 'web', state: 'active', detail: null },
    ]);

    session = run(
      events('a', [
        status('web', 'done', '14 new since Oct 1'),
        { event: 'text', data: { delta: 'Five picks with' } },
        { event: 'text', data: { delta: ' the same itch.' } },
        { event: 'item', data: GOOD_PLACE },
        { event: 'item', data: MYTHIC_QUEST },
        // A title named twice keeps its first place
        { event: 'item', data: { ...GOOD_PLACE, match_tag: 'Again' } },
        status('write', 'active'),
        DONE,
      ]),
      session
    );
    const [turn] = session.live;
    expect(turn.phase).toBe('done');
    expect(isWorking(turn)).toBe(false);
    expect(turn.intro).toBe('Five picks with the same itch.');
    expect(turn.items.map((i) => [i.title, i.match_tag])).toEqual([
      ['The Good Place', 'Again'],
      ['Mythic Quest', 'Mystery + workplace · lighter'],
    ]);
    // The step still active when it ended is closed: nothing spins after the end
    expect(turn.steps.map((s) => s.state)).toEqual(['done', 'done', 'done']);
    expect(turn.refinements).toEqual(['Lighter', 'Shorter']);
    expect(turn.elapsedMs).toBe(9100);
    expect(workingTurn(session)).toBeUndefined();
  });

  it('ignores events after the turn ended, and events for a turn the page left', () => {
    const done = run([send(), ...events('a', [START, DONE]), ...events('a', [{ event: 'text', data: { delta: 'late' } }])]);
    expect(done.live[0].intro).toBe('');
    expect(run(events('gone', [START]), done)).toBe(done);
  });

  it('a mid-stream error ends the turn with what it got and drops the step it was on', () => {
    const session = run([
      send(),
      ...events('a', [START, status('history', 'active'), status('history', 'done', '2,184 plays'), status('web', 'active')]),
      ...events('a', [{ event: 'error', data: { code: 'upstream', message: 'The Anthropic API didn’t answer.' } }]),
    ]);
    expect(session.live[0]).toMatchObject({ phase: 'error', error: { code: 'upstream', message: 'The Anthropic API didn’t answer.' }, failure: null });
    expect(session.live[0].steps).toEqual([{ step: 'history', state: 'done', detail: '2,184 plays' }]);
  });

  it('a stream that ends without done or error is cut off', () => {
    const session = run([send(), ...events('a', [START, status('history', 'active')]), { type: 'cut', key: 'a' }]);
    expect(session.live[0]).toMatchObject({ phase: 'error', error: { code: 'connection' } });
    expect(session.live[0].steps).toEqual([]);
  });
});

describe('askReducer: Stop and refusals', () => {
  it('Stop cancels the turn, keeps what arrived and drops the active step; later events change nothing', () => {
    let session = run([send(), ...events('a', [START, status('history', 'done', '2,184 plays'), status('web', 'active')]), { type: 'stopped', key: 'a' }]);
    expect(session.live[0]).toMatchObject({ phase: 'cancelled' });
    expect(session.live[0].steps.map((s) => s.step)).toEqual(['history']);
    session = run(events('a', [DONE]), session);
    expect(session.live[0].phase).toBe('cancelled');
  });

  it('a 429 before the stream pauses asks until it lifts (kept across conversations); a done turn lifts it', () => {
    const resetsAt = '2026-11-01T00:00:00+03:00';
    let session = run([send(), { type: 'failed', key: 'a', failure: { status: 429, code: 'budget_exceeded', message: 'Used up', resetsAt } }]);
    expect(session.live[0]).toMatchObject({ phase: 'error', failure: { code: 'budget_exceeded', resetsAt } });
    expect(session.paused).toEqual({ code: 'budget_exceeded', resetsAt });

    session = run([{ type: 'route', thread: 11 }], session);
    expect(session.live).toEqual([]);
    expect(session.paused).toEqual({ code: 'budget_exceeded', resetsAt });
    expect(run([{ type: 'reset' }], session).paused).toEqual({ code: 'budget_exceeded', resetsAt });

    session = run([send('b'), ...events('b', [START, DONE])], session);
    expect(session.paused).toBeNull();
  });

  it('other refusals fail the turn without pausing', () => {
    const session = run([send(), { type: 'failed', key: 'a', failure: { status: 409, code: 'thread_full', message: 'Full', resetsAt: null } }]);
    expect(session.live[0].failure?.code).toBe('thread_full');
    expect(session.paused).toBeNull();
  });

  it('Try again takes the failed turn’s place', () => {
    const failed = run([send('a', 'a thriller'), { type: 'failed', key: 'a', failure: { status: 0, code: 'network', message: '', resetsAt: null } }]);
    const retried = run([{ type: 'send', key: 'b', message: 'a thriller', refine: false, replaces: 'a' }], failed);
    expect(retried.live.map((turn) => [turn.key, turn.phase])).toEqual([['b', 'sending']]);
  });
});

describe('askReducer: the URL', () => {
  it('keeps the conversation when the URL moves to the thread its start opened', () => {
    const started = run([send(), ...events('a', [START])]);
    const moved = run([{ type: 'route', thread: 13 }], started);
    expect(moved.route).toBe(13);
    expect(moved.live).toBe(started.live);
  });

  it('another thread, or /ask from a thread, is another conversation', () => {
    const onThread = run([{ type: 'route', thread: 13 }], run([send(), ...events('a', [START])]));
    expect(run([{ type: 'route', thread: 12 }], onThread)).toMatchObject({ route: 12, threadId: 12, live: [] });
    expect(run([{ type: 'route', thread: null }], onThread)).toMatchObject({ route: null, threadId: null, live: [] });
    expect(run([{ type: 'route', thread: 13 }], onThread)).toBe(onThread);
  });

  it('starts on the URL’s thread', () => {
    expect(initialSession(12)).toEqual({ route: 12, threadId: 12, live: [], paused: null });
  });
});

describe('saved turns', () => {
  const saved = (id: number, overrides: Partial<AskTurn> = {}): AskTurn => ({
    id,
    message: 'something like Severance but lighter',
    status: 'done',
    steps: [{ step: 'history', state: 'done', detail: '2,184 plays' }],
    intro: 'Five picks.',
    items: [GOOD_PLACE],
    refinements: ['Lighter'],
    error: null,
    elapsed_ms: 9100,
    created_at: '2026-10-16T09:00:00Z',
    ...overrides,
  });

  it('reads a thread’s turn as the page shows it', () => {
    expect(turnFromSaved(saved(44))).toMatchObject({ key: 'turn:44', id: 44, phase: 'done', live: false, refine: false, elapsedMs: 9100, createdAt: '2026-10-16T09:00:00Z' });
    expect(turnFromSaved(saved(45, { steps: [{ step: 'rerank', state: 'done', detail: '23 checked' }] })).refine).toBe(true);
    // An ended turn never spins, even if an active step slipped into its view
    expect(turnFromSaved(saved(46, { status: 'cancelled', steps: [{ step: 'web', state: 'active', detail: null }] })).steps).toEqual([]);
  });

  it('shows the saved turns, then the page’s own (their streamed view wins); a refused one stays where it was sent', () => {
    let session = run([send('a'), ...events('a', [{ ...START, data: { ...START.data, thread_id: 12, turn_id: 47 } }])], initialSession(12));
    session = run([send('b'), { type: 'failed', key: 'b', failure: { status: 0, code: 'network', message: '', resetsAt: null } }], run([...events('a', [DONE])], session));
    const thread = { turns: [saved(44), saved(47, { status: 'streaming', steps: [], items: [] })] };
    expect(visibleTurns(thread, session.live).map((turn) => [turn.key, turn.phase])).toEqual([
      ['turn:44', 'done'],
      ['a', 'done'],
      ['b', 'error'],
    ]);
    expect(visibleTurns(undefined, session.live)).toHaveLength(2);
  });
});
