import type { RecommendationsHome, RunProgress, TraktIntegration, TraktSync } from '@/lib/api/contract';
import { firstPicksRun, firstSyncDetail, firstSyncSteps, forYouState, importedSoFar, traktLinked, writingShelvesStep } from '@/lib/for-you/state';

const PROGRESS = { plays: 2184, plays_total: 2184, ratings: 312, watchlist: 41, shows: 6 };

function sync(overrides: Partial<TraktSync> = {}): TraktSync {
  return { status: 'idle', kind: 'incremental', step: null, progress: PROGRESS, error: null, started_at: null, finished_at: null, ...overrides };
}

function trakt(overrides: Partial<TraktIntegration> = {}): TraktIntegration {
  return {
    configured: true,
    state: 'connected',
    connected: true,
    username: 'teymur',
    pending: null,
    connect_error: null,
    reauth_reason: null,
    last_sync_at: null,
    next_sync_at: null,
    sync: sync(),
    counts: { history: 2184, ratings: 312, watchlist: 41, watchlist_in_library: 6 },
    settings: { use_ratings: true, show_watchlist: true, include_paused: false },
    ...overrides,
  };
}

const off = trakt({ state: 'off', connected: false, username: null, sync: sync({ kind: null }) });

describe('forYouState', () => {
  it('waits for the integration, and without Trakt for the library too', () => {
    expect(forYouState({ trakt: undefined, libraryTotal: 3 })).toBe('loading');
    expect(forYouState({ trakt: off, libraryTotal: undefined })).toBe('loading');
    expect(forYouState({ trakt: trakt(), libraryTotal: undefined })).toBe('default');
  });

  it('first time: no Trakt and an empty library; otherwise Trakt off', () => {
    expect(forYouState({ trakt: off, libraryTotal: 0 })).toBe('first_time');
    expect(forYouState({ trakt: off, libraryTotal: 9 })).toBe('trakt_off');
    // A code waiting for its first approval has nothing imported yet
    expect(forYouState({ trakt: { ...off, state: 'pending' }, libraryTotal: 9 })).toBe('trakt_off');
    // The library didn't answer: not a first visit we can be sure of
    expect(forYouState({ trakt: off, libraryTotal: null })).toBe('trakt_off');
    // The integration didn't answer: carry on without Trakt
    expect(forYouState({ trakt: null, libraryTotal: 9 })).toBe('trakt_off');
  });

  it('first sync while the initial import runs or after it failed; the personal page otherwise', () => {
    expect(forYouState({ trakt: trakt({ sync: sync({ status: 'syncing', kind: 'initial', step: 'history' }) }), libraryTotal: 0 })).toBe('first_sync');
    expect(forYouState({ trakt: trakt({ sync: sync({ status: 'error', kind: 'initial', step: 'watchlist' }) }), libraryTotal: 0 })).toBe('first_sync');
    expect(forYouState({ trakt: trakt({ sync: sync({ status: 'idle', kind: 'initial' }) }), libraryTotal: 0 })).toBe('default');
    expect(forYouState({ trakt: trakt({ sync: sync({ status: 'syncing', kind: 'incremental', step: 'history' }) }), libraryTotal: 0 })).toBe('default');
  });

  it('reauth keeps the imported data, so the personal page stays', () => {
    const reauth = trakt({ state: 'reauth', connected: false });
    expect(traktLinked(reauth)).toBe(true);
    expect(traktLinked({ state: 'pending', username: 'teymur' })).toBe(true);
    expect(traktLinked({ state: 'pending', username: null })).toBe(false);
    expect(forYouState({ trakt: reauth, libraryTotal: 0 })).toBe('default');
  });
});

describe('the first sync', () => {
  it('names each step: done with its count, the running one active, the rest pending', () => {
    const steps = firstSyncSteps(sync({ status: 'syncing', kind: 'initial', step: 'details' }));
    expect(steps.map(({ label, detail, state }) => [label, detail, state])).toEqual([
      ['Imported your history', '2,184 plays', 'done'],
      ['Imported your ratings', '312 ratings', 'done'],
      ['Imported your watchlist', '41 titles', 'done'],
      ['Found Up next for 6 shows', null, 'done'],
      ['Fetching artwork and episode names', null, 'active'],
    ]);
  });

  it('counts plays while it reads the history', () => {
    const [history, ...rest] = firstSyncSteps(sync({ status: 'syncing', kind: 'initial', step: 'history', progress: { ...PROGRESS, plays: 900, ratings: 0, watchlist: 0, shows: 0 } }));
    expect(history).toMatchObject({ label: 'Importing your history', detail: '900 of 2,184 plays', state: 'active' });
    expect(rest.every((step) => step.state === 'pending')).toBe(true);
  });

  it('after an error, the failed step stops and nothing spins', () => {
    const failed = sync({ status: 'error', kind: 'initial', step: 'watchlist', error: 'Trakt didn’t answer (502).' });
    const steps = firstSyncSteps(failed);
    expect(steps.map((step) => step.state)).toEqual(['done', 'done', 'pending', 'pending', 'pending']);
    expect(steps[2]).toMatchObject({ label: 'Importing your watchlist', detail: 'Stopped here' });
  });

  it('sums up what’s in so far', () => {
    expect(importedSoFar(PROGRESS)).toBe('2,184 plays, 312 ratings and 41 watchlist items');
    expect(importedSoFar({ ...PROGRESS, ratings: 0, watchlist: 0 })).toBe('2,184 plays');
    expect(importedSoFar({ ...PROGRESS, plays: 0, ratings: 0, watchlist: 0 })).toBeNull();
    expect(firstSyncDetail(sync({ status: 'syncing', kind: 'initial', step: 'history', progress: { ...PROGRESS, plays: 0, ratings: 0, watchlist: 0 } }))).toMatch(/^Reading your history/);
  });
});

describe('Noir’s first shelves (#155)', () => {
  const firstRun: RunProgress = {
    id: 1,
    kind: 'first_sync',
    status: 'running',
    model: 'claude-opus-5-5',
    started_at: '2026-10-16T14:00:00Z',
    finished_at: null,
    cost_usd: 0.1,
    skip_reason: null,
    error: null,
    steps: [{ step: 'write', state: 'active', detail: '2 of 5 shelves' }],
    shelves_done: 2,
    shelves_target: 5,
  };
  const home = (overrides: Partial<RecommendationsHome> = {}): RecommendationsHome => ({
    configured: true,
    enabled: true,
    run: null,
    active_run: firstRun,
    hero_picks: [],
    shelves: [],
    ...overrides,
  });

  it('the first-sync run while there are no picks yet', () => {
    expect(firstPicksRun(home())).toBe(firstRun);
    expect(firstPicksRun(undefined)).toBeNull();
    // A refresh over existing picks shows the refreshing banner instead
    expect(firstPicksRun(home({ run: { ...firstRun, id: 0, kind: 'scheduled', status: 'succeeded' } }))).toBeNull();
    expect(firstPicksRun(home({ active_run: { ...firstRun, kind: 'manual' } }))).toBeNull();
    expect(firstPicksRun(home({ active_run: null }))).toBeNull();
    expect(firstPicksRun(home({ enabled: false }))).toBeNull();
  });

  it('keeps For You on the first sync while Noir writes, once the import is done', () => {
    expect(forYouState({ trakt: trakt(), libraryTotal: 3, writingFirstPicks: true })).toBe('first_sync');
    expect(forYouState({ trakt: off, libraryTotal: 3, writingFirstPicks: true })).toBe('trakt_off');
  });

  it('adds "Writing your shelves" with the count, and says it’s building your picks', () => {
    expect(writingShelvesStep(firstRun)).toEqual({ label: 'Writing your shelves', detail: '2 of 5', state: 'active' });
    expect(writingShelvesStep({ shelves_done: 0, shelves_target: 0 }).detail).toBeNull();
    expect(firstSyncDetail(sync(), true)).toBe('Imported 2,184 plays, 312 ratings and 41 watchlist items. Building your picks…');
    expect(firstSyncDetail(sync({ progress: { ...PROGRESS, plays: 0, ratings: 0, watchlist: 0 } }), true)).toBe('Building your picks…');
  });
});
