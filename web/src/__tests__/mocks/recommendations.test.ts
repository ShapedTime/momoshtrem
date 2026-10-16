/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import { askStream, recApi, settingsApi, systemApi, titlesApi, traktApi } from '@/lib/api/endpoints';
import type { AskEvent, RecommendationsHome, RecItem } from '@/lib/api/contract';
import { setMockScenario } from '@/mocks/scenario';
import { RUN_DONE_TICK, THREAD_MAX_TURNS, recTitleRef } from '@/mocks/fixtures/recommendations';

/** The recommendations mock (#152): a state machine per page, and every `recs-*` / `ask-*` scenario. */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(() => resetMockState());
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const failure = (promise: Promise<unknown>) =>
  promise.then(
    () => null,
    (error: unknown) => (isApiError(error) ? { status: error.status, code: error.code, ...(error.resetsAt ? { resetsAt: error.resetsAt } : {}) } : error)
  );

const items = (home: RecommendationsHome): RecItem[] => home.shelves.flatMap((shelf) => shelf.items);
const titles = (home: RecommendationsHome) => items(home).map((item) => item.title);
const SILO = recTitleRef('silo');
const PAST_LIVES = recTitleRef('pastlives');

async function collect(message: string, threadId?: number): Promise<AskEvent[]> {
  const events: AskEvent[] = [];
  for await (const event of askStream({ message, thread_id: threadId })) events.push(event);
  return events;
}

/** Reads /home until no run is active (at most a run's worth of reads). */
async function readUntilIdle(): Promise<RecommendationsHome> {
  let home = await recApi.home();
  for (let i = 0; i <= RUN_DONE_TICK && home.active_run; i++) home = await recApi.home();
  return home;
}

describe('GET /recommendations/home', () => {
  it('serves last night’s picks: the design’s shelves, three hero picks and live library state', async () => {
    const home = await recApi.home();
    expect(home).toMatchObject({ enabled: true, active_run: null, run: { id: 42, kind: 'scheduled', status: 'succeeded', model: 'claude-opus-5-5', cost_usd: 0.36 } });
    expect(home.shelves.map((s) => [s.slug, s.kind])).toEqual([
      ['because-severance', 'because'],
      ['new-this-month', 'new'],
      ['anime', 'anime'],
      ['hidden-gems', 'gems'],
      ['coming-soon', 'coming_soon'],
    ]);
    expect(home.shelves.every((s) => s.items.length >= 6 && s.items.length <= 15)).toBe(true);
    expect(home.hero_picks.map((p) => [p.kicker, p.tmdb_id])).toEqual([
      ['Tonight’s pick', PAST_LIVES.tmdb_id],
      ['New for you', recTitleRef('pluribus').tmdb_id],
      ['Ready in Infuse', recTitleRef('dune2').tmdb_id],
    ]);

    const byTitle = new Map(items(home).map((item) => [item.title, item]));
    // Facts are the run's snapshot
    expect(byTitle.get('The Good Place')).toMatchObject({ year: 2016, year_end: 2020, seasons: 4, runtime_minutes: 22, vote_average: 8.1, poster_path: null });
    expect(byTitle.get('Paprika')).toMatchObject({ media_type: 'movie', seasons: null, runtime_minutes: 90, year_end: null });
    expect(home.shelves[4].items[0]).toMatchObject({ title: 'Severance', badge: '2026', reason: 'Season 3 in production' });
    // Library and watchlist are read live; the liked title says so
    expect(byTitle.get('One Battle After Another')).toMatchObject({ on_watchlist: true, library: { availability_stage: 'needs_torrent' } });
    expect(byTitle.get('Silo')).toMatchObject({ library: null, on_watchlist: false, feedback: null });
    expect(byTitle.get('Pluribus')?.feedback).toBe('like');
    expect(home.hero_picks[1].feedback).toBe('like');
  });

  it('keeps every reason, match tag, shelf and hero list within the contract’s limits', async () => {
    const home = await recApi.home();
    expect(items(home).every((item) => item.reason.length <= 140)).toBe(true);
    expect(home.hero_picks.every((pick) => pick.reason.length <= 140)).toBe(true);
    for (const message of ['something like Severance but lighter', 'a 90-minute thriller', 'anime like Frieren', 'films with my parents']) {
      const found = (await collect(message)).flatMap((e) => (e.event === 'item' ? [e.data] : []));
      expect(found.length).toBeGreaterThan(0);
      expect(found.every((item) => item.match_tag.length <= 32 && item.reason.length <= 140)).toBe(true);
    }
  });
});

describe('runs', () => {
  it('refresh walks a run through its steps, one per read, then /home serves its picks', async () => {
    const { run_id } = await recApi.refresh();
    expect(run_id).toBe(43);
    expect((await recApi.refresh()).run_id).toBe(43); // joins the active run
    expect((await recApi.run(run_id)).status).toBe('running'); // reading the run moves it too

    const seen: string[] = [];
    let home = await recApi.home();
    while (home.active_run) {
      const { steps, shelves_done } = home.active_run;
      const active = steps.find((s) => s.state === 'active');
      seen.push(`${active?.step}:${shelves_done}`);
      expect(steps.filter((s) => s.state === 'active')).toHaveLength(1);
      expect(home.run?.id).toBe(42); // the old picks stay meanwhile
      home = await recApi.home();
    }
    expect(seen).toEqual(['web:0', 'tmdb:0', 'write:2', 'write:4']);
    expect(home.run).toMatchObject({ id: 43, kind: 'manual', status: 'succeeded' });
    expect(home.shelves[0].title).toBe('Because you loved The Bear');

    const detail = await recApi.run(43);
    expect(detail.steps.map((s) => [s.step, s.state])).toEqual([
      ['history', 'done'],
      ['web', 'done'],
      ['tmdb', 'done'],
      ['write', 'done'],
    ]);
    expect(detail).toMatchObject({ shelves_done: 5, shelves_target: 5, usage: { api_calls: 9, web_searches: 6, cost_usd: 0.36 } });
    expect((await recApi.runs(2)).items.map((r) => r.id)).toEqual([43, 42]);
    expect((await settingsApi.aiUsage()).by_feature[0]).toEqual({ feature: 'recs', cost_usd: 2.54, requests: 50 });
  });

  it('lists runs newest first, with skips and failures', async () => {
    const { items: runs } = await recApi.runs();
    expect(runs.map((r) => [r.id, r.status])).toEqual([
      [42, 'succeeded'],
      [41, 'succeeded'],
      [40, 'failed'],
      [39, 'succeeded'],
      [38, 'skipped'],
    ]);
    expect(runs[4]).toMatchObject({ kind: 'scheduled', skip_reason: 'no_history', started_at: null, cost_usd: 0 });
    expect(runs[2].error).toMatch(/Anthropic API/);
    expect(await failure(recApi.runs(51))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(recApi.run(999))).toEqual({ status: 404, code: 'not_found' });
  });

  it('refresh needs Trakt history: 409 no_history before the first import', async () => {
    setMockScenario('trakt-off');
    expect(await failure(recApi.refresh())).toEqual({ status: 409, code: 'no_history' });
  });

  it('queues the first_sync run once Trakt’s first import finishes', async () => {
    setMockScenario('trakt-off');
    expect(await recApi.home()).toMatchObject({ enabled: true, run: null, active_run: null, shelves: [] });
    await traktApi.connect();
    for (let i = 0; i < 8; i++) await traktApi.integration(); // approve, then import every step
    const home = await recApi.home();
    expect(home.active_run).toMatchObject({ kind: 'first_sync', status: 'queued' });
    expect((await readUntilIdle()).run).toMatchObject({ kind: 'first_sync', status: 'succeeded' });
  });
});

describe('feedback', () => {
  it('negative feedback hides a title everywhere at once; DELETE (Undo) brings it back', async () => {
    await recApi.feedback({ ...SILO, signal: 'dislike', source: 'shelf' });
    expect(titles(await recApi.home())).not.toContain('Silo');
    await recApi.clearFeedback(SILO.media_type, SILO.tmdb_id);
    await recApi.clearFeedback(SILO.media_type, SILO.tmdb_id); // idempotent
    expect(titles(await recApi.home())).toContain('Silo');

    // The hero skips it and the day's next pick fills in; the title page loses its reason
    expect((await titlesApi.get('movie', PAST_LIVES.tmdb_id)).recommendation_reason).toBe('Quiet, warm and under two hours.');
    await recApi.feedback({ ...PAST_LIVES, signal: 'not_interested', source: 'hero' });
    expect((await recApi.home()).hero_picks.map((p) => p.kicker)).toEqual(['New for you', 'Ready in Infuse', 'Something quiet']);
    expect((await titlesApi.get('movie', PAST_LIVES.tmdb_id)).recommendation_reason).toBeNull();
  });

  it('like shows on the card; Undo of a changed signal restores the previous one', async () => {
    await recApi.feedback({ ...SILO, signal: 'like', source: 'shelf' });
    expect(items(await recApi.home()).find((i) => i.title === 'Silo')?.feedback).toBe('like');
    await recApi.feedback({ ...SILO, signal: 'seen', source: 'shelf' });
    expect(titles(await recApi.home())).not.toContain('Silo');
    await recApi.feedback({ ...SILO, signal: 'like', source: 'shelf' }); // Undo: the previous signal again
    expect(items(await recApi.home()).find((i) => i.title === 'Silo')?.feedback).toBe('like');
  });

  it('answers 400 for a bad body or path', async () => {
    const bad = { ...SILO, signal: 'meh', source: 'shelf' } as unknown as Parameters<typeof recApi.feedback>[0];
    expect(await failure(recApi.feedback(bad))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(recApi.clearFeedback('book' as never, 1))).toEqual({ status: 400, code: 'bad_request' });
  });
});

describe('AI settings', () => {
  it('PUT is partial: it merges the fields sent and ignores unknown ones', async () => {
    expect(await settingsApi.ai()).toEqual({
      rec_enabled: true,
      rec_model: 'claude-opus-5-5',
      rec_schedule: 'nightly',
      monthly_budget_usd: 15,
      classifier_enabled: true,
      classifier_model: 'claude-haiku-5-5',
      auto_apply_high_confidence: true,
    });
    const saved = await settingsApi.putAi({ rec_model: 'claude-sonnet-5-5', monthly_budget_usd: null, ...({ nope: 1 } as object) });
    expect(saved).toMatchObject({ rec_model: 'claude-sonnet-5-5', monthly_budget_usd: null, rec_schedule: 'nightly', classifier_enabled: true });
    expect(saved).not.toHaveProperty('nope');
    expect((await settingsApi.aiUsage()).budget_usd).toBeNull();
  });

  it('answers 422 naming the field', async () => {
    for (const [patch, field] of [
      [{ rec_model: 'gpt-5' }, 'rec_model'],
      [{ rec_schedule: '0 4 * * *' }, 'rec_schedule'],
      [{ monthly_budget_usd: -1 }, 'monthly_budget_usd'],
      [{ rec_enabled: 'yes' }, 'rec_enabled'],
      [{ classifier_model: 'claude-fable-5-1' }, 'classifier_model'],
    ] as const) {
      const error = await settingsApi.putAi(patch as never).catch((e: unknown) => e);
      expect(isApiError(error) && { status: error.status, code: error.code, field: error.field }).toEqual({ status: 422, code: 'validation_error', field });
    }
  });

  it('the usage: this month by feature (always all three), the schedule, estimates and asks', async () => {
    const usage = await settingsApi.aiUsage();
    expect(usage).toMatchObject({ configured: true, total_usd: 3.12, budget_usd: 15, budget_reached: false, asks_today: 2, ask_daily_limit: 30 });
    expect(usage.by_feature.map((f) => f.feature)).toEqual(['recs', 'ask', 'classifier']);
    expect(usage.recs).toMatchObject({ last_run: { id: 42 }, active_run: null });
    expect(new Date(usage.recs.next_run_at ?? '').getHours()).toBe(3);
    expect(usage.estimates).toEqual({ run_usd: { 'claude-opus-5-5': 0.36, 'claude-sonnet-5-5': 0.07 }, ask_usd: 0.02 });
    expect(new Date(usage.resets_at).getDate()).toBe(1);
    expect((await systemApi.status()).ai).toEqual({ configured: true, mtd_cost_usd: 3.12 });

    await settingsApi.putAi({ rec_schedule: 'weekly' });
    expect(new Date((await settingsApi.aiUsage()).recs.next_run_at ?? '').getDay()).toBe(0);
    await settingsApi.putAi({ rec_schedule: 'manual' });
    expect((await settingsApi.aiUsage()).recs.next_run_at).toBeNull();
  });

  it('rec_enabled off empties /home and refuses refresh; on again brings the picks back', async () => {
    await settingsApi.putAi({ rec_enabled: false });
    expect(await recApi.home()).toEqual({ enabled: false, run: null, active_run: null, hero_picks: [], shelves: [] });
    expect(await failure(recApi.refresh())).toEqual({ status: 409, code: 'recs_disabled' });
    await settingsApi.putAi({ rec_enabled: true });
    expect((await recApi.home()).shelves).toHaveLength(5);
  });
});

describe('Ask momoshtrem', () => {
  it('streams start → status… → text → items → done with the design’s step details', async () => {
    const events = await collect('something like Severance but lighter');
    expect(events[0]).toEqual({ event: 'start', data: { thread_id: 13, turn_id: 46, title: 'Something like Severance but lighter' } });
    const statuses = events.flatMap((e) => (e.event === 'status' ? [`${e.data.step}:${e.data.state}`] : []));
    expect(statuses).toEqual(['history:active', 'history:done', 'web:active', 'web:done', 'tmdb:active', 'tmdb:done', 'write:active', 'write:done']);
    expect(events.find((e) => e.event === 'status' && e.data.state === 'done')).toMatchObject({ data: { detail: '2,184 plays · you loved Severance, The Bear, Ted Lasso' } });
    const intro = events.flatMap((e) => (e.event === 'text' ? [e.data.delta] : [])).join('');
    expect(intro).toMatch(/^Five picks with the same/);
    expect(events.filter((e) => e.event === 'item')).toHaveLength(5);
    expect(events.at(-1)).toMatchObject({ event: 'done', data: { thread_id: 13, turn_id: 46, usage: { cost_usd: 0.02, web_searches: 2 } } });
  });

  it('threads grow as you ask; a refinement chip re-ranks', async () => {
    await collect('a cosy show to fall asleep to');
    const { items: threads } = await recApi.threads();
    expect(threads[0]).toMatchObject({ id: 13, title: 'A cosy show to fall asleep to', turn_count: 1 });
    expect(threads.map((t) => t.id)).toEqual([13, 12, 11, 10, 9]);

    const refined = await collect('Only anime', 13);
    expect(refined.flatMap((e) => (e.event === 'status' ? [e.data.step] : []))).toEqual(['rerank', 'rerank', 'write', 'write']);
    const thread = await recApi.thread(13);
    expect(thread).toMatchObject({ turn_count: 2, model: 'claude-opus-5-5' });
    expect(thread.turns[1]).toMatchObject({ message: 'Only anime', status: 'done', intro: 'Anime with the same “something is off here” hook, kept light.', elapsed_ms: 4200 });
    expect(thread.turns[1].items.map((i) => [i.title, i.match_tag])).toEqual([
      ['Dan Da Dan', 'Weird · funny'],
      ['Mushishi', 'Gentle mystery'],
      ['Paprika', 'One evening'],
    ]);
    expect(thread.turns[1].steps.every((s) => s.state === 'done')).toBe(true);
    expect((await settingsApi.aiUsage()).asks_today).toBe(4);
  });

  it('thread reads join feedback: a hidden title leaves the thread too', async () => {
    const before = await recApi.thread(12);
    expect(before.turns[0].items.map((i) => i.title)).toContain('The Good Place');
    await recApi.feedback({ ...recTitleRef('goodplace'), signal: 'seen', source: 'ask' });
    expect((await recApi.thread(12)).turns[0].items.map((i) => i.title)).not.toContain('The Good Place');
  });

  it('answers JSON errors before the stream: 400, 404 and 409 thread_full', async () => {
    expect(await failure(collect(''))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(collect('x'.repeat(501)))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(collect('more like this', 999))).toEqual({ status: 404, code: 'not_found' });
    for (let i = 0; i < THREAD_MAX_TURNS - 2; i++) await collect('Lighter', 12);
    expect(await failure(collect('Lighter', 12))).toEqual({ status: 409, code: 'thread_full' });
  });
});

describe('scenarios', () => {
  it('recs-none: no run yet (For You keeps trending); refresh starts the first one', async () => {
    setMockScenario('recs-none');
    expect(await recApi.home()).toEqual({ enabled: true, run: null, active_run: null, hero_picks: [], shelves: [] });
    expect((await recApi.runs()).items).toEqual([]);
    expect((await settingsApi.aiUsage()).recs.last_run).toBeNull();
    await recApi.refresh();
    expect((await readUntilIdle()).shelves.length).toBe(5);
  });

  it('recs-first-run: the first_sync run holds at “Writing your shelves”, 2 of 5', async () => {
    setMockScenario('recs-first-run');
    for (let i = 0; i < 3; i++) {
      const home = await recApi.home();
      expect(home).toMatchObject({ run: null, shelves: [], active_run: { kind: 'first_sync', status: 'running', shelves_done: 2, shelves_target: 5 } });
      expect(home.active_run?.steps.at(-1)).toEqual({ step: 'write', state: 'active', detail: '2 of 5 shelves' });
    }
  });

  it('recs-refreshing: last night’s picks with a manual run held; refresh joins it', async () => {
    setMockScenario('recs-refreshing');
    const home = await recApi.home();
    expect(home).toMatchObject({ run: { id: 42 }, active_run: { id: 43, kind: 'manual', shelves_done: 2 } });
    expect(home.shelves).toHaveLength(5);
    expect((await recApi.refresh()).run_id).toBe(43);
    expect((await settingsApi.aiUsage()).recs.active_run).toMatchObject({ id: 43 });
  });

  it('recs-failed: the last refresh failed, the old picks stay, and the next one fails at TMDB', async () => {
    setMockScenario('recs-failed');
    expect((await recApi.runs(1)).items[0]).toMatchObject({ id: 43, kind: 'manual', status: 'failed', error: expect.stringMatching(/Your last picks stay/) });
    expect((await settingsApi.aiUsage()).recs.last_run).toMatchObject({ id: 43, status: 'failed' });
    const { run_id } = await recApi.refresh();
    const home = await readUntilIdle();
    expect(home.run?.id).toBe(42);
    const failed = await recApi.run(run_id);
    expect(failed).toMatchObject({ status: 'failed', error: expect.any(String) });
    expect(failed.steps.map((s) => s.step)).toEqual(['history', 'web', 'tmdb']);
  });

  it('recs-off: picks are off, Ask still answers', async () => {
    setMockScenario('recs-off');
    expect((await settingsApi.ai()).rec_enabled).toBe(false);
    expect((await recApi.home()).enabled).toBe(false);
    expect(await failure(recApi.refresh())).toEqual({ status: 409, code: 'recs_disabled' });
    expect((await settingsApi.aiUsage()).recs.next_run_at).toBeNull();
    expect((await collect('a 90-minute thriller for tonight')).at(-1)?.event).toBe('done');
  });

  it('ai-off: no key: /home is disabled, refresh and Ask answer 503, settings still read and save', async () => {
    setMockScenario('ai-off');
    expect((await recApi.home()).enabled).toBe(false);
    expect(await failure(recApi.refresh())).toEqual({ status: 503, code: 'unavailable' });
    expect(await failure(collect('anything'))).toEqual({ status: 503, code: 'unavailable' });
    expect(await settingsApi.putAi({ monthly_budget_usd: 20 })).toMatchObject({ monthly_budget_usd: 20 });
    expect(await settingsApi.aiUsage()).toMatchObject({ configured: false, budget_usd: 20 });
    expect((await systemApi.status()).ai.configured).toBe(false);
  });

  it('ask-budget: the $5 is spent: Ask and refresh answer 429 budget_exceeded with resets_at', async () => {
    setMockScenario('ask-budget');
    const usage = await settingsApi.aiUsage();
    expect(usage).toMatchObject({ budget_usd: 5, total_usd: 5, budget_reached: true });
    expect(await failure(collect('a 90-minute thriller for tonight'))).toEqual({ status: 429, code: 'budget_exceeded', resetsAt: usage.resets_at });
    expect(await failure(recApi.refresh())).toEqual({ status: 429, code: 'budget_exceeded', resetsAt: usage.resets_at });
  });

  it('ask-limit: 30 asks today: 429 ask_limit, back at midnight', async () => {
    setMockScenario('ask-limit');
    expect((await settingsApi.aiUsage()).asks_today).toBe(30);
    const error = (await failure(collect('one more'))) as { status: number; code: string; resetsAt: string };
    expect(error).toMatchObject({ status: 429, code: 'ask_limit' });
    const midnight = new Date(error.resetsAt);
    expect([midnight.getHours(), midnight.getMinutes()]).toEqual([0, 0]);
    expect(midnight.getTime()).toBeGreaterThan(Date.now());
  });

  it('ask-error: the stream ends with an error event, and the thread keeps the failed turn', async () => {
    setMockScenario('ask-error');
    const events = await collect('something like Severance but lighter');
    expect(events.map((e) => e.event)).toEqual(['start', 'status', 'status', 'status', 'error']);
    expect(events.at(-1)).toEqual({ event: 'error', data: { code: 'upstream', message: expect.any(String) } });
    const start = events[0].event === 'start' ? events[0].data : null;
    const turn = (await recApi.thread(start!.thread_id)).turns[0];
    expect(turn).toMatchObject({ status: 'error', error: { code: 'upstream' }, items: [] });
    expect(turn.steps.map((s) => `${s.step}:${s.state}`)).toEqual(['history:done', 'web:active']);
  });

  it('ask-hold: the stream never finishes; another ask is busy; Stop cancels the turn', async () => {
    setMockScenario('ask-hold');
    const controller = new AbortController();
    const stream = askStream({ message: 'something like Severance but lighter' }, controller.signal);
    const received: AskEvent[] = [];
    for (let i = 0; i < 4; i++) received.push((await stream.next()).value as AskEvent);
    expect(received.map((e) => (e.event === 'status' ? `${e.data.step}:${e.data.state}` : e.event))).toEqual(['start', 'history:active', 'history:done', 'web:active']);
    const threadId = received[0].event === 'start' ? received[0].data.thread_id : 0;
    expect((await recApi.thread(threadId)).turns[0]).toMatchObject({ status: 'streaming', steps: [], items: [], elapsed_ms: null });
    expect(await failure(collect('something else'))).toEqual({ status: 409, code: 'ask_busy' });

    controller.abort();
    await stream.return(undefined);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect((await recApi.thread(threadId)).turns[0]).toMatchObject({ status: 'cancelled' });
    expect((await settingsApi.aiUsage()).by_feature[1].cost_usd).toBe(0.54); // the usage already spent
  });
});
