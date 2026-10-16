import type { AssignmentJob, TraktIntegration } from '@/lib/api/contract';
import { askAction, findTorrentHref, matchingActions, type PaletteActionInput, paletteActions } from '@/lib/title-search/actions';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

function job(id: number, title: string, overrides: Partial<AssignmentJob> = {}): AssignmentJob {
  return {
    id,
    status: 'needs_review',
    dry_run: false,
    item: { id: 1, media_type: 'tv', tmdb_id: 209867, title, year: 2023, poster_path: null },
    counts: { files: 16, to_apply: 12, pending: 2, conflicts: 0, needs_review: 2, matched: 12, specials: 0, ignored: 1, unmatched: 1 },
    ...overrides,
  } as AssignmentJob;
}

const input = (overrides: Partial<PaletteActionInput> = {}): PaletteActionInput => ({
  jobs: [],
  incomplete: [],
  torrentCounts: null,
  libraryTotal: null,
  torrentSheet: false,
  ...overrides,
});

const labels = (actions: ReturnType<typeof paletteActions>) => actions.map((action) => action.label);

describe('paletteActions', () => {
  it('always offers the destinations, settings, Infuse setup and log out', () => {
    expect(labels(paletteActions(input()))).toEqual(['Go to Activity', 'Go to Library', 'Go to Discover', 'Open settings', 'Infuse setup', 'Log out']);
    const infuse = paletteActions(input()).find((action) => action.key === 'infuse');
    expect(infuse).toMatchObject({ sub: 'Copy the WebDAV address', target: { kind: 'sheet', name: 'infuse-setup' } });
  });

  it('reviews each open mapping job first', () => {
    const actions = paletteActions(input({ jobs: [job(101, 'Frieren: Beyond Journey’s End'), job(102, 'One Piece', { counts: { ...job(0, '').counts, needs_review: 1 } })] }));
    expect(actions.slice(0, 2).map((action) => [action.label, action.sub, action.target])).toEqual([
      ['Review Frieren mapping', '2 files need a look', { kind: 'href', href: '/mapping/101' }],
      ['Review One Piece mapping', '1 file needs a look', { kind: 'href', href: '/mapping/102' }],
    ]);
  });

  it('says when a job is still working or ready to apply', () => {
    expect(paletteActions(input({ jobs: [job(1, 'Bluey', { status: 'classifying' })] }))[0].sub).toBe('Mapping files…');
    expect(paletteActions(input({ jobs: [job(1, 'Bluey', { counts: { ...job(0, '').counts, needs_review: 0 } })] }))[0].sub).toBe('Ready to apply');
  });

  it('finds what library titles lack, a few at most, on the title page until the sheet is live', () => {
    const movie = libraryItem({ tmdb_id: 1054867, title: 'One Battle After Another', availability_stage: 'needs_torrent', webdav_path: null, webdav_url: null });
    const downloading = libraryItem({ tmdb_id: 157336, title: 'Interstellar', availability_stage: 'downloading', download: { pct: 42, info_hash: 'h' } });
    const extra = (n: number) => libraryShow({ tmdb_id: 900 + n, title: `Show ${n}` });
    const actions = paletteActions(input({ incomplete: [libraryShow(), downloading, movie, extra(1), extra(2)] }));
    expect(actions.slice(0, 3).map((action) => [action.label, action.sub, action.target])).toEqual([
      ['Find Frieren episodes 29–38', '10 episodes missing', { kind: 'href', href: '/tv/209867?season=1' }],
      ['Find a torrent for One Battle After Another', 'No torrent yet', { kind: 'href', href: '/movie/1054867' }],
      ['Find Show 1 episodes 29–38', '10 episodes missing', { kind: 'href', href: '/tv/901?season=1' }],
    ]);
    expect(actions[3].label).toBe('Go to Activity');

    const sheet = paletteActions(input({ incomplete: [libraryShow()], torrentSheet: true }))[0];
    expect(sheet.target).toEqual({ kind: 'sheet', name: 'torrent', params: { show: '209867', season: '1', se: 'S01E29' } });
  });

  it('puts live counts on Activity and Library', () => {
    const actions = paletteActions(input({ torrentCounts: { streaming: 1, downloading: 2, paused: 0, idle: 3 }, libraryTotal: 11 }));
    expect(actions.find((action) => action.key === 'activity')?.sub).toBe('2 downloading · 1 streaming');
    expect(actions.find((action) => action.key === 'library')?.sub).toBe('11 titles');
    const quiet = paletteActions(input({ torrentCounts: { streaming: 0, downloading: 0, paused: 1, idle: 3 } }));
    expect(quiet.find((action) => action.key === 'activity')?.sub).toBe('Nothing active right now');
    expect(quiet.find((action) => action.key === 'library')?.sub).toBe('Everything you’ve added');
  });
});

describe('the Trakt action', () => {
  const NOW = Date.parse('2026-10-16T21:44:00Z');
  const trakt = (overrides: Partial<TraktIntegration> = {}): TraktIntegration =>
    ({
      configured: true,
      state: 'connected',
      username: 'teymur',
      last_sync_at: new Date(NOW - 4 * 60_000).toISOString(),
      sync: { status: 'idle' },
      ...overrides,
    }) as TraktIntegration;
  const traktAction = (value: TraktIntegration | null) => paletteActions(input({ trakt: value, now: NOW })).find((action) => action.key === 'trakt');

  it('syncs now in place while connected, between Discover and settings', () => {
    expect(traktAction(trakt())).toEqual({ key: 'trakt', label: 'Sync Trakt now', sub: 'Last synced 4 min ago', icon: 'sync', target: { kind: 'trakt-sync' } });
    expect(labels(paletteActions(input({ trakt: trakt() }))).slice(2, 5)).toEqual(['Go to Discover', 'Sync Trakt now', 'Open settings']);
  });

  it('opens Settings › Trakt to connect, or to sign in again', () => {
    const section = { kind: 'href', href: '/settings?section=trakt' };
    expect(traktAction(trakt({ state: 'off' }))).toMatchObject({ label: 'Connect Trakt', sub: 'Personal picks and Up next', target: section });
    expect(traktAction(trakt({ state: 'off', configured: false }))).toMatchObject({ label: 'Connect Trakt', target: section });
    expect(traktAction(trakt({ state: 'pending' }))).toMatchObject({ label: 'Connect Trakt', sub: 'A code is waiting for you on trakt.tv' });
    expect(traktAction(trakt({ state: 'reauth' }))).toMatchObject({ label: 'Reconnect Trakt', sub: 'Trakt needs you to sign in again', target: section });
  });

  it('is left out until the integration is known', () => {
    expect(traktAction(null)).toBeUndefined();
  });
});

describe('findTorrentHref', () => {
  it('offers Ask momoshtrem after Discover only with an Anthropic key (#156)', () => {
    expect(labels(paletteActions(input({ ask: true }))).slice(2, 5)).toEqual(['Go to Discover', 'Ask momoshtrem', 'Open settings']);
    expect(paletteActions(input({ ask: true })).find((action) => action.key === 'ask')).toEqual({
      key: 'ask',
      label: 'Ask momoshtrem',
      sub: 'Conversational picks from your history',
      icon: 'ask',
      target: { kind: 'href', href: '/ask' },
    });
    expect(paletteActions(input()).some((action) => action.key === 'ask')).toBe(false);
  });

  it('asks Noir about what you typed', () => {
    expect(askAction('  a cosy show  ')).toEqual({
      key: 'ask-query',
      label: 'Ask momoshtrem: “a cosy show”',
      sub: 'Picks with a reason for each',
      icon: 'ask',
      target: { kind: 'href', href: '/ask?q=a%20cosy%20show' },
    });
  });

  it('opens a title’s page with torrent search on it', () => {
    expect(findTorrentHref({ media_type: 'movie', tmdb_id: 666277 })).toBe('/movie/666277?sheet=torrent&movie=666277');
    expect(findTorrentHref({ media_type: 'tv', tmdb_id: 1396 })).toBe('/tv/1396?sheet=torrent&show=1396');
  });
});

describe('matchingActions', () => {
  const actions = paletteActions(input({ jobs: [job(101, 'Frieren: Beyond Journey’s End')], incomplete: [libraryShow()], libraryTotal: 11 }));

  it('keeps the actions whose label and line hold every word', () => {
    expect(labels(matchingActions(actions, 'frieren'))).toEqual(['Review Frieren mapping', 'Find Frieren episodes 29–38']);
    expect(labels(matchingActions(actions, 'lib'))).toEqual(['Go to Library']);
    expect(labels(matchingActions(actions, '11 titles'))).toEqual(['Go to Library']);
    expect(labels(matchingActions(actions, 'webdav'))).toEqual(['Infuse setup']);
    expect(labels(matchingActions(actions, 'frieren review'))).toEqual(['Review Frieren mapping']);
  });

  it('ignores case, accents and word order', () => {
    expect(labels(matchingActions(actions, 'DISCOVER go'))).toEqual(['Go to Discover']);
    expect(labels(matchingActions(actions, 'séttings'))).toEqual(['Open settings']);
  });

  it('keeps every action for a blank query and none for a miss', () => {
    expect(matchingActions(actions, '  ')).toBe(actions);
    expect(matchingActions(actions, 'zzxq')).toEqual([]);
  });
});
