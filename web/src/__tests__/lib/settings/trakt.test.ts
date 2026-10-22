import type { TraktIntegration, TraktSync } from '@/lib/api/contract';
import {
  CODE_LIFETIME_SECONDS,
  connectErrorCopy,
  disconnectConfirmCopy,
  displayCode,
  displayUrl,
  expiryPct,
  formatCountdown,
  importSteps,
  isFirstSync,
  lastSyncedLine,
  lastSyncLine,
  reauthBody,
  secondsLeft,
  spokenCode,
  spokenTimeLeft,
  syncedToast,
  syncErrorCopy,
  timeAgo,
  traktEvent,
  traktNavSummary,
  traktScreen,
  waitingLine,
  watchlistHelp,
} from '@/lib/settings/trakt';

const NOW = Date.parse('2026-10-16T21:44:00Z');
const minutesAgo = (minutes: number) => new Date(NOW - minutes * 60_000).toISOString();
const CODE = { user_code: '7F3A91C2', verification_url: 'https://trakt.tv/activate', expires_at: new Date(NOW + 14 * 60_000 + 59_000).toISOString(), interval: 5 };

function sync(overrides: Partial<TraktSync> = {}): TraktSync {
  return {
    status: 'idle',
    kind: 'incremental',
    step: null,
    progress: { plays: 2184, plays_total: 2184, ratings: 312, watchlist: 41, shows: 18 },
    error: null,
    started_at: minutesAgo(5),
    finished_at: minutesAgo(4),
    ...overrides,
  };
}

function integration(overrides: Partial<TraktIntegration> = {}): TraktIntegration {
  return {
    configured: true,
    state: 'connected',
    connected: true,
    username: 'teymur',
    pending: null,
    connect_error: null,
    reauth_reason: null,
    last_sync_at: minutesAgo(4),
    next_sync_at: minutesAgo(-26),
    sync: sync(),
    counts: { history: 2184, ratings: 312, watchlist: 41, watchlist_in_library: 6 },
    settings: { use_ratings: true, show_watchlist: true, include_paused: false },
    ...overrides,
  };
}

const off = (overrides: Partial<TraktIntegration> = {}) =>
  integration({ state: 'off', connected: false, username: null, last_sync_at: null, next_sync_at: null, sync: sync({ kind: null, started_at: null, finished_at: null }), ...overrides });
const pending = (overrides: Partial<TraktIntegration> = {}) => off({ state: 'pending', pending: CODE, ...overrides });

describe('traktScreen', () => {
  it('maps each state to its screen', () => {
    expect(traktScreen(off(), null)).toBe('off');
    expect(traktScreen(off({ configured: false }), null)).toBe('unconfigured');
    expect(traktScreen(integration(), null)).toBe('connected');
    expect(traktScreen(integration({ state: 'reauth', connected: false }), null)).toBe('reauth');
  });

  it('a pending code shows the code, and waiting only once "I’ve approved it" was pressed for it', () => {
    expect(traktScreen(pending(), null)).toBe('code');
    expect(traktScreen(pending(), '7F3A91C2')).toBe('waiting');
    expect(traktScreen(pending(), 'OTHERCODE')).toBe('code');
    // A code pending from reauth keeps the account
    expect(traktScreen(pending({ username: 'teymur' }), null)).toBe('code');
  });

  it('without the code itself, falls back to where the account is', () => {
    expect(traktScreen(pending({ pending: null }), null)).toBe('off');
    expect(traktScreen(pending({ pending: null, username: 'teymur' }), null)).toBe('reauth');
  });
});

describe('the device code', () => {
  it('counts down to the expiry in m:ss and never below zero', () => {
    expect(secondsLeft(CODE.expires_at, NOW)).toBe(899);
    expect(formatCountdown(899)).toBe('14:59');
    expect(formatCountdown(65)).toBe('1:05');
    expect(formatCountdown(5)).toBe('0:05');
    expect(secondsLeft(CODE.expires_at, NOW + 20 * 60_000)).toBe(0);
    expect(formatCountdown(0)).toBe('0:00');
  });

  it('the expiry bar runs over Trakt’s 10 minutes; a longer code starts full', () => {
    expect(expiryPct(CODE_LIFETIME_SECONDS)).toBe(100);
    expect(expiryPct(300)).toBe(50);
    expect(expiryPct(0)).toBe(0);
    expect(expiryPct(899)).toBe(100);
  });

  it('reads in two halves, and one character at a time for screen readers', () => {
    expect(displayCode('7F3A91C2')).toBe('7F3A-91C2');
    expect(displayCode('ABC123')).toBe('ABC123');
    expect(spokenCode('7F3A91C2')).toBe('7 F 3 A, 9 1 C 2');
    expect(displayUrl('https://trakt.tv/activate')).toBe('trakt.tv/activate');
    expect(displayUrl('https://trakt.tv/activate/')).toBe('trakt.tv/activate');
  });

  it('the waiting line ticks; screen readers hear it in minutes', () => {
    expect(waitingLine(5, '7F3A91C2', 820)).toBe('Checking every 5 seconds · code 7F3A-91C2 expires in 13:40');
    expect(waitingLine(1, '7F3A91C2', 0)).toBe('Checking every second · code 7F3A-91C2 expired');
    expect(spokenTimeLeft(820)).toBe('in about 14 minutes');
    expect(spokenTimeLeft(70)).toBe('in about 1 minute');
    expect(spokenTimeLeft(30)).toBe('in less than a minute');
  });
});

describe('sync lines', () => {
  it('says how long ago', () => {
    expect(timeAgo(minutesAgo(0.5), NOW)).toBe('just now');
    expect(timeAgo(minutesAgo(4), NOW)).toBe('4 min ago');
    expect(timeAgo(minutesAgo(125), NOW)).toBe('2 h ago');
    expect(timeAgo(minutesAgo(26 * 60), NOW)).toBe('yesterday');
    expect(timeAgo(minutesAgo(3 * 24 * 60), NOW)).toBe('3 days ago');
  });

  it('under the username: the last sync and the schedule, or what runs', () => {
    expect(lastSyncLine(integration(), NOW)).toBe('Last sync 4 min ago · syncs every 30 min');
    expect(lastSyncLine(integration({ sync: sync({ status: 'syncing', step: 'history' }) }), NOW)).toBe('Syncing now…');
    expect(lastSyncLine(integration({ last_sync_at: null, sync: sync({ status: 'syncing', kind: 'initial', step: 'ratings' }) }), NOW)).toBe('First sync running…');
    expect(lastSyncLine(integration({ last_sync_at: null, sync: sync({ status: 'error', kind: 'initial' }) }), NOW)).toBe('Not synced yet · syncs every 30 min');
    // Nothing syncs until you sign in again
    expect(lastSyncLine(integration({ state: 'reauth', last_sync_at: minutesAgo(2 * 24 * 60) }), NOW)).toBe('Last sync 2 days ago');
  });

  it('the palette’s line', () => {
    expect(lastSyncedLine(integration(), NOW)).toBe('Last synced 4 min ago');
    expect(lastSyncedLine(integration({ sync: sync({ status: 'syncing' }) }), NOW)).toBe('Syncing now…');
    expect(lastSyncedLine(integration({ last_sync_at: null }), NOW)).toBe('Not synced yet');
  });
});

describe('the first import', () => {
  it('names its steps, done up to the running one, with what each brought', () => {
    const steps = importSteps(sync({ status: 'syncing', kind: 'initial', step: 'watchlist', progress: { plays: 2184, plays_total: 2184, ratings: 312, watchlist: 0, shows: 0 } }));
    expect(steps.map((s) => [s.label, s.state, s.detail])).toEqual([
      ['History', 'done', '2,184 plays'],
      ['Ratings', 'done', '312'],
      ['Watchlist', 'active', undefined],
      ['Show progress', 'pending', undefined],
      ['Artwork', 'pending', undefined],
    ]);
  });

  it('history shows how far it got once Trakt’s total is known', () => {
    const running = importSteps(sync({ status: 'syncing', kind: 'initial', step: 'history', progress: { plays: 1200, plays_total: 2184, ratings: 0, watchlist: 0, shows: 0 } }));
    expect(running[0]).toMatchObject({ state: 'active', detail: '1,200 of 2,184' });
    const unknown = importSteps(sync({ status: 'syncing', kind: 'initial', step: 'history', progress: { plays: 0, plays_total: 0, ratings: 0, watchlist: 0, shows: 0 } }));
    expect(unknown[0].detail).toBeUndefined();
  });

  it('is the banner only while the initial import runs', () => {
    expect(isFirstSync(integration({ sync: sync({ status: 'syncing', kind: 'initial', step: 'details' }) }))).toBe(true);
    expect(isFirstSync(integration({ sync: sync({ status: 'syncing', kind: 'incremental', step: 'history' }) }))).toBe(false);
    expect(isFirstSync(integration())).toBe(false);
  });

  it('a failed sync says where Retry picks up', () => {
    expect(syncErrorCopy(sync({ status: 'error', kind: 'initial', step: 'watchlist', error: 'Trakt didn’t answer (502).' }))).toEqual({
      title: 'The first import didn’t finish',
      body: 'Trakt didn’t answer (502). Retry picks up from your watchlist.',
    });
    expect(syncErrorCopy(sync({ status: 'error', kind: 'incremental', step: 'history', error: null })).title).toBe('The last sync didn’t finish');
  });
});

describe('copy', () => {
  it('a denied code mentions Trakt’s 2-app limit for free accounts; the rest stay calm', () => {
    expect(connectErrorCopy('denied')).toMatchObject({ tone: 'warning', body: expect.stringContaining('Free Trakt accounts can connect only 2 apps') });
    expect(connectErrorCopy('expired')).toMatchObject({ tone: 'neutral', title: 'That code expired' });
    expect(connectErrorCopy('failed').body).toMatch(/Connect again/);
  });

  it('an account another member connected says each person connects their own (#239)', () => {
    expect(connectErrorCopy('account_in_use')).toEqual({
      tone: 'warning',
      title: 'That Trakt account is already connected',
      body: 'Someone else in this household connected it. Each person connects their own: sign in to trakt.tv with your account, then connect again.',
    });
  });

  it('the disconnect confirm names whose Trakt it is when there are two or more people', () => {
    expect(disconnectConfirmCopy(null)).toEqual({
      title: 'Disconnect Trakt?',
      description: 'Your imported history, ratings and watchlist will be deleted from momoshtrem. Nothing changes on Trakt. Personal picks fall back to trending.',
    });
    expect(disconnectConfirmCopy('Teymur')).toEqual({
      title: 'Disconnect Teymur’s Trakt?',
      description: 'Teymur’s imported history, ratings and watchlist will be deleted from momoshtrem. Nothing changes on Trakt. Teymur’s picks fall back to trending.',
    });
  });

  it('reauth says the server’s reason, or what stays', () => {
    expect(reauthBody(integration({ state: 'reauth', reauth_reason: 'Trakt stopped accepting the sign-in.' }))).toBe('Trakt stopped accepting the sign-in.');
    expect(reauthBody(integration({ state: 'reauth' }))).toMatch(/still here/);
  });

  it('the watchlist switch counts what’s listed and what’s in the library', () => {
    expect(watchlistHelp({ history: 0, ratings: 0, watchlist: 41, watchlist_in_library: 6 })).toBe('41 titles · 6 already in your library');
    expect(watchlistHelp({ history: 0, ratings: 0, watchlist: 1, watchlist_in_library: 0 })).toBe('1 title');
    expect(watchlistHelp({ history: 0, ratings: 0, watchlist: 0, watchlist_in_library: 0 })).toBe('Nothing on your Trakt watchlist yet');
  });

  it('the settings nav names the account, or what it waits for', () => {
    expect(traktNavSummary(undefined)).toEqual({ text: '' });
    expect(traktNavSummary(integration())).toEqual({ text: 'Connected as teymur' });
    expect(traktNavSummary(off())).toEqual({ text: 'Not connected' });
    expect(traktNavSummary(off({ configured: false }))).toEqual({ text: 'Not set up' });
    expect(traktNavSummary(pending())).toEqual({ text: 'Waiting for approval' });
    expect(traktNavSummary(integration({ state: 'reauth' }))).toEqual({ text: 'Sign in again', attention: true });
    expect(traktNavSummary(integration({ sync: sync({ status: 'syncing', kind: 'initial', step: 'history' }) }))).toEqual({ text: 'First sync running' });
    expect(traktNavSummary(integration({ sync: sync({ status: 'error' }) }))).toEqual({ text: 'Last sync failed', attention: true });
  });
});

describe('toasts', () => {
  it('a poll says when a code was approved, or a sync finished', () => {
    const syncing = integration({ sync: sync({ status: 'syncing', kind: 'initial', step: 'history' }) });
    expect(traktEvent(pending(), syncing)).toBe('connected');
    expect(traktEvent(syncing, integration({ sync: sync({ kind: 'initial' }) }))).toBe('synced');
    expect(traktEvent(syncing, syncing)).toBeNull();
    expect(traktEvent(integration(), integration())).toBeNull();
    expect(traktEvent(syncing, integration({ sync: sync({ status: 'error' }) }))).toBeNull();
    expect(traktEvent(pending(), off({ connect_error: 'denied' }))).toBeNull();
  });

  it('"Synced with Trakt" says what the first import brought, or what’s new', () => {
    const before = { history: 2181, ratings: 311, watchlist: 41, watchlist_in_library: 6 };
    expect(syncedToast(integration({ sync: sync({ kind: 'initial' }) }), before).body).toBe('Imported 2,184 plays, 312 ratings and 41 watchlist titles.');
    expect(syncedToast(integration(), before)).toEqual({ title: 'Synced with Trakt', body: '3 new plays, 1 new rating.' });
    expect(syncedToast(integration(), integration().counts).body).toBe('Nothing new since the last sync.');
  });
});
