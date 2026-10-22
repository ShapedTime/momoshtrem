/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import { askStream, membersApi, recApi, settingsApi, systemApi, traktApi } from '@/lib/api/endpoints';
import type { AskEvent } from '@/lib/api/contract';
import { setMockMember } from '@/mocks/fixtures/members';
import { setMockScenario } from '@/mocks/scenario';

/** The household mock (#239): the members' CRUD, and Trakt, picks and Ask per member. */

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
    (error: unknown) => (isApiError(error) ? { status: error.status, code: error.code, ...(error.field ? { field: error.field } : {}) } : error)
  );

async function ask(message: string): Promise<AskEvent[]> {
  const events: AskEvent[] = [];
  for await (const event of askStream({ message })) events.push(event);
  return events;
}

const shelfTitles = async () => (await recApi.home()).shelves.map((shelf) => shelf.title);

describe('the household', () => {
  it('is Teymur and Sam, each with their Trakt', async () => {
    expect((await membersApi.list()).items).toMatchObject([
      { id: 1, name: 'Teymur', color: 'teal', trakt: { state: 'connected', username: 'teymur' } },
      { id: 2, name: 'Sam', color: 'rose', trakt: { state: 'connected', username: 'sam' } },
    ]);
  });

  it('adds someone, trimmed, with the first free colour when none is sent', async () => {
    const alex = await membersApi.create({ name: '  Alex  ' });
    expect(alex).toMatchObject({ id: 3, name: 'Alex', color: 'orchid', trakt: { state: 'off', username: null } });
    expect(await membersApi.create({ name: 'Mina', color: 'lime' })).toMatchObject({ id: 4, color: 'lime' });
    expect((await membersApi.list()).items.map((m) => m.name)).toEqual(['Teymur', 'Sam', 'Alex', 'Mina']);
  });

  it('checks names and colours as the contract says', async () => {
    expect(await failure(membersApi.create({ name: '   ' }))).toEqual({ status: 422, code: 'validation_error', field: 'name' });
    expect(await failure(membersApi.create({ name: 'x'.repeat(33) }))).toEqual({ status: 422, code: 'validation_error', field: 'name' });
    expect(await membersApi.create({ name: '🦊'.repeat(32) })).toMatchObject({ id: 3 }); // code points, not UTF-16 units
    expect(await failure(membersApi.create({ name: 'Jo', color: 'mauve' as 'rose' }))).toEqual({ status: 422, code: 'validation_error', field: 'color' });
    expect(await failure(membersApi.create({ name: 'sAM' }))).toEqual({ status: 409, code: 'member_name_taken' });
  });

  it('a body of the wrong JSON type, null included, is a 400 before any value check', async () => {
    const wrong = (body: unknown) => body as { name: string };
    expect(await failure(membersApi.create(wrong({ name: 5 })))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(membersApi.create(wrong({ name: null })))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(membersApi.create(wrong({ name: 'Jo', color: null })))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(membersApi.create(wrong(['Jo'])))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(membersApi.create(wrong({})))).toEqual({ status: 422, code: 'validation_error', field: 'name' });
  });

  it('PATCH checks in the contract’s order: 400, 422, 404, 409', async () => {
    expect(await failure(membersApi.update(9, { name: 5 as unknown as string }))).toEqual({ status: 400, code: 'bad_request' });
    expect(await failure(membersApi.update(9, { color: 'mauve' as 'rose' }))).toEqual({ status: 422, code: 'validation_error', field: 'color' });
    expect(await failure(membersApi.update(9, { name: 'Sam' }))).toEqual({ status: 404, code: 'not_found' });
    expect(await failure(membersApi.update(1, { name: 'Sam' }))).toEqual({ status: 409, code: 'member_name_taken' });
    expect(await membersApi.update(1, {})).toMatchObject({ id: 1, name: 'Teymur', color: 'teal' });
  });

  it('caps the household at six', async () => {
    setMockScenario('members-full');
    expect((await membersApi.list()).items).toHaveLength(6);
    expect(await failure(membersApi.create({ name: 'Seventh' }))).toEqual({ status: 409, code: 'member_limit' });
  });

  it('renames and recolours, with the same rules', async () => {
    expect(await membersApi.update(2, { name: 'Samira', color: 'sand' })).toMatchObject({ id: 2, name: 'Samira', color: 'sand' });
    expect(await membersApi.update(2, { name: 'samira' })).toMatchObject({ name: 'samira' }); // their own name, recased
    expect(await failure(membersApi.update(2, { name: 'TEYMUR' }))).toEqual({ status: 409, code: 'member_name_taken' });
    expect(await failure(membersApi.update(2, { color: 'mauve' as 'rose' }))).toEqual({ status: 422, code: 'validation_error', field: 'color' });
    expect(await failure(membersApi.update(9, { name: 'Nobody' }))).toEqual({ status: 404, code: 'not_found' });
  });

  it('removes someone, but never the last one', async () => {
    expect(await failure(membersApi.remove(9))).toEqual({ status: 404, code: 'not_found' });
    await membersApi.remove(2);
    expect((await membersApi.list()).items.map((m) => m.id)).toEqual([1]);
    expect(await failure(membersApi.remove(1))).toEqual({ status: 409, code: 'last_member' });
    setMockScenario('members-one');
    expect(await failure(membersApi.remove(1))).toEqual({ status: 409, code: 'last_member' });
  });

  it('members-absent is a backend before households: 404, and the header is ignored', async () => {
    setMockScenario('members-absent');
    expect(await failure(membersApi.list())).toEqual({ status: 404, code: 'not_found' });
    setMockMember(2);
    expect((await traktApi.integration()).username).toBe('teymur');
    setMockMember(9);
    expect((await traktApi.upNext()).items.length).toBeGreaterThan(0);
  });

  it('members-error answers 500', async () => {
    setMockScenario('members-error');
    expect(await failure(membersApi.list())).toEqual({ status: 500, code: 'internal_error' });
  });
});

describe('who’s asking', () => {
  it('without a member it’s the lowest id, as Go answers without the header', async () => {
    expect((await traktApi.integration()).username).toBe('teymur');
  });

  it('a member nobody has gets 400 unknown_member on personal routes; /members and /status still answer', async () => {
    setMockMember(9);
    for (const call of [traktApi.integration, traktApi.upNext, recApi.home, recApi.threads, settingsApi.aiUsage]) {
      expect(await failure(call())).toEqual({ status: 400, code: 'unknown_member' });
    }
    expect((await membersApi.list()).items).toHaveLength(2);
    expect((await systemApi.status()).trakt).toEqual({ configured: true, connected: false });
    // The web's legacy library routes call the backend without a member
    expect((await fetch('/api/library/shows')).status).toBe(200);
    expect((await fetch('/api/library/movies')).status).toBe(200);
    expect((await fetch('/api/library?page_size=1')).status).toBe(400);
  });

  it('each member has their own Trakt: connecting as someone new leaves Teymur’s alone', async () => {
    await membersApi.create({ name: 'Alex Kim' });
    setMockMember(3);
    expect(await traktApi.integration()).toMatchObject({ state: 'off', username: null });
    expect((await systemApi.status()).trakt.connected).toBe(false);
    await traktApi.connect();
    await traktApi.integration();
    expect(await traktApi.integration()).toMatchObject({ state: 'connected', username: 'alexkim' });
    setMockMember(1);
    const teymur = await traktApi.integration();
    expect(teymur).toMatchObject({ state: 'connected', username: 'teymur' });
    expect(teymur.sync.kind).toBe('incremental');
    expect((await membersApi.list()).items[2].trakt).toEqual({ state: 'connected', username: 'alexkim' });
  });

  it('Sam’s Up next, watchlist and history are his own', async () => {
    setMockMember(1);
    const teymur = { upNext: (await traktApi.upNext()).items.length, plays: (await traktApi.history()).total };
    setMockMember(2);
    const sam = await traktApi.upNext();
    expect(sam.items.map((item) => item.show.title).sort()).toEqual(['Severance', 'The Studio']);
    expect(sam.items.length).toBeLessThan(teymur.upNext);
    expect((await traktApi.watchlist()).items.map((item) => item.title)).toEqual(['Interstellar', 'Frieren: Beyond Journey’s End']);
    const history = await traktApi.history();
    expect(history.total).toBeLessThan(teymur.plays);
    expect(new Set(history.items.map((item) => item.title))).toEqual(new Set(['Severance', 'The Studio']));
    expect((await traktApi.integration()).counts.history).toBe(history.total);
  });

  it('members-fresh: Sam hasn’t connected Trakt, so /status, /home and Ask are empty for him', async () => {
    setMockScenario('members-fresh');
    setMockMember(2);
    expect(await traktApi.integration()).toMatchObject({ state: 'off', username: null });
    expect((await systemApi.status()).trakt.connected).toBe(false);
    expect(await recApi.home()).toMatchObject({ run: null, shelves: [] });
    expect((await recApi.threads()).items).toEqual([]);
    expect((await membersApi.list()).items[1].trakt).toEqual({ state: 'off', username: null });
    setMockMember(1);
    expect((await systemApi.status()).trakt.connected).toBe(true);
  });

  it('picks are per member: Teymur’s design set, Sam’s refreshed one', async () => {
    setMockMember(1);
    expect((await shelfTitles())[0]).toBe('Because you loved Severance');
    setMockMember(2);
    expect((await shelfTitles())[0]).toBe('Because you loved The Bear');
    expect((await recApi.home()).hero_picks[0]).toMatchObject({ kicker: 'Tonight’s pick' });
    expect((await recApi.runs()).items.map((run) => run.id)).toEqual([201]);
    setMockMember(1);
    expect(await failure(recApi.run(201))).toEqual({ status: 404, code: 'not_found' });
  });

  it('Ask threads are per member', async () => {
    setMockMember(1);
    const teymur = (await recApi.threads()).items.map((t) => t.id);
    expect(teymur).toHaveLength(4);
    setMockMember(2);
    expect((await recApi.threads()).items).toEqual([]);
    const events = await ask('something light for tonight');
    expect(events[0]).toMatchObject({ event: 'start', data: { thread_id: 100 } });
    expect((await recApi.threads()).items.map((t) => t.id)).toEqual([100]);
    setMockMember(1);
    expect((await recApi.threads()).items.map((t) => t.id)).toEqual(teymur);
    expect(await failure(recApi.thread(100))).toEqual({ status: 404, code: 'not_found' });
  });

  it('the spend is the household’s; asks today are the viewer’s', async () => {
    setMockMember(1);
    const before = await settingsApi.aiUsage();
    setMockMember(2);
    expect((await settingsApi.aiUsage()).asks_today).toBe(0);
    await ask('a 90-minute thriller for tonight');
    const sam = await settingsApi.aiUsage();
    expect(sam.asks_today).toBe(1);
    expect(sam.total_usd).toBeGreaterThan(before.total_usd);
    setMockMember(1);
    const teymur = await settingsApi.aiUsage();
    expect(teymur.total_usd).toBe(sam.total_usd);
    expect(teymur.asks_today).toBe(before.asks_today);
  });

  it('removing someone drops their Trakt, picks and threads; what they spent stays in the total', async () => {
    setMockMember(2);
    await ask('anime with a Frieren vibe');
    const spent = (await settingsApi.aiUsage()).total_usd;
    setMockMember(1);
    await membersApi.remove(2);
    expect((await settingsApi.aiUsage()).total_usd).toBe(spent);
    setMockMember(2);
    expect(await failure(recApi.threads())).toEqual({ status: 400, code: 'unknown_member' });
  });

  it('trakt-account-in-use: the code ends with account_in_use, back to off', async () => {
    setMockScenario('trakt-account-in-use');
    expect(await traktApi.integration()).toMatchObject({ state: 'off' });
    await traktApi.connect();
    expect((await traktApi.integration()).state).toBe('pending');
    expect(await traktApi.integration()).toMatchObject({ state: 'off', connect_error: 'account_in_use', pending: null, username: null });
  });
});
