import type { TraktIntegration, TraktSync } from '@/lib/api/contract';
import { firstSyncDetail, firstSyncSteps, firstSyncStoppedTitle, forYouState, importedSoFar, traktLinked } from '@/lib/for-you/state';

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
    expect(firstSyncStoppedTitle(failed)).toBe('The first sync stopped at your watchlist');
  });

  it('sums up what’s in so far', () => {
    expect(importedSoFar(PROGRESS)).toBe('2,184 plays, 312 ratings and 41 watchlist items');
    expect(importedSoFar({ ...PROGRESS, ratings: 0, watchlist: 0 })).toBe('2,184 plays');
    expect(importedSoFar({ ...PROGRESS, plays: 0, ratings: 0, watchlist: 0 })).toBeNull();
    expect(firstSyncDetail(sync({ status: 'syncing', kind: 'initial', step: 'history', progress: { ...PROGRESS, plays: 0, ratings: 0, watchlist: 0 } }))).toMatch(/^Reading your history/);
  });
});
