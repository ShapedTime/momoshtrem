import { serviceRows, summarize, versionLabel } from '@/components/settings/status-model';
import type { IndexerHealth, SystemStatus, TraktIntegration } from '@/lib/api/contract';
import { degradedIndexers, systemStatus } from '@/mocks/fixtures/system';

describe('System status model', () => {
  it('lists every service, healthy when everything answers', () => {
    const rows = serviceRows(systemStatus());
    expect(rows.map((r) => r.name)).toEqual(['momoshtrem backend', 'WebDAV', 'Jackett', 'HDRezka', 'TMDB', 'Trakt', 'AI', 'OpenSubtitles']);
    expect(summarize(rows)).toMatchObject({ tone: 'ready', title: 'All good', attention: false });
  });

  it('treats services that are not set up as off, not broken', () => {
    const status = { ...systemStatus(), jackett: { configured: false, reachable: false }, ai: { configured: false, mtd_cost_usd: null } };
    const rows = serviceRows(status);
    expect(rows.find((r) => r.id === 'jackett')).toMatchObject({ health: 'off', pill: { status: 'not_configured' } });
    expect(summarize(rows).tone).toBe('ready');
  });

  it('flags a service that is set up but not answering', () => {
    const status = { ...systemStatus(), jackett: { configured: true, reachable: false, error: 'timeout after 3s' } };
    const rows = serviceRows(status);
    expect(rows.find((r) => r.id === 'jackett')).toMatchObject({ health: 'error', detail: 'timeout after 3s', pill: { status: 'unreachable' } });
    expect(summarize(rows)).toMatchObject({ tone: 'error', title: 'Jackett isn’t working', short: '1 problem', attention: true });
  });

  it('shows HDRezka next to Jackett: off unless it’s set up, and left out by an older backend', () => {
    const off = serviceRows({ ...systemStatus(), hdrezka: { configured: false, reachable: false } });
    expect(off.find((r) => r.id === 'hdrezka')).toMatchObject({ health: 'off', pill: { status: 'not_configured' } });
    const blocked = serviceRows({ ...systemStatus(), hdrezka: { configured: true, reachable: false, error: 'HDRezka answered with its anti-bot page' } });
    expect(blocked.find((r) => r.id === 'hdrezka')).toMatchObject({ health: 'error', detail: 'HDRezka answered with its anti-bot page' });
    expect(serviceRows({ ...systemStatus(), hdrezka: undefined }).map((r) => r.id)).not.toContain('hdrezka');
  });

  it('details Trakt from the integration: who and when, a lapsed sign-in, or Connect', () => {
    const now = Date.parse('2026-10-16T21:44:00Z');
    const trakt = (overrides: Partial<TraktIntegration>) =>
      ({ configured: true, state: 'connected', username: 'teymur', last_sync_at: new Date(now - 4 * 60_000).toISOString(), sync: { status: 'idle' }, ...overrides }) as TraktIntegration;
    const row = (integration?: TraktIntegration, status: Partial<SystemStatus['trakt']> = {}) =>
      serviceRows({ ...systemStatus(), trakt: { configured: true, connected: true, ...status } }, integration, now).find((r) => r.id === 'trakt');

    expect(row()).toMatchObject({ detail: 'Connected', health: 'ok', pill: { status: 'ok' } });
    expect(row(trakt({}))).toMatchObject({ detail: 'Connected as teymur · synced 4 min ago', health: 'ok' });
    expect(row(trakt({ sync: { status: 'error' } as TraktIntegration['sync'] }))).toMatchObject({ health: 'warning', pill: { label: 'Sync failed' } });
    expect(row(trakt({ state: 'reauth' }), { connected: false })).toMatchObject({ detail: 'teymur · needs you to sign in again', health: 'warning', pill: { status: 'warning', label: 'Sign in again' } });
    expect(row(trakt({ state: 'off', username: null }), { connected: false })).toMatchObject({ detail: 'Not connected', health: 'off', action: { label: 'Connect', href: '/settings?section=trakt' } });
    expect(row(undefined, { connected: false })).toMatchObject({ detail: 'Not connected', action: { label: 'Connect' } });
    expect(row(undefined, { configured: false, connected: false })).toMatchObject({ health: 'off', pill: { status: 'not_configured' } });

    const reauth = serviceRows({ ...systemStatus(), trakt: { configured: true, connected: false } }, trakt({ state: 'reauth' }), now);
    expect(summarize(reauth)).toMatchObject({ tone: 'warning', body: 'Trakt needs you to sign in again. Your imported history is still here.' });
  });

  it('flags a database outage on the backend row', () => {
    const status = { ...systemStatus(), db: { ok: false, size_bytes: null } };
    expect(serviceRows(status)[0]).toMatchObject({ health: 'error', pill: { status: 'error', label: 'Database down' } });
  });

  it('shows the backend version short on its row', () => {
    const status = { ...systemStatus(), version: '9000f05e8c1b2d3a4f5e6d7c8b9a0f1e2d3c4b5a' };
    expect(serviceRows(status)[0].detail).toMatch(/^9000f05 · up /);
  });
});

describe('Jackett’s indexers', () => {
  const withIndexers = (indexers: IndexerHealth[] | undefined): SystemStatus => {
    const status = systemStatus();
    return { ...status, jackett: { ...status.jackett, indexers } };
  };
  const jackettRow = (status: SystemStatus) => serviceRows(status).find((r) => r.id === 'jackett')!;

  it('counts the indexers that answered, and lists each one', () => {
    const row = jackettRow(systemStatus());
    expect(row).toMatchObject({ detail: '2 of 3 indexers OK', health: 'ok', pill: { status: 'ok' } });
    expect(row.indexers).toEqual([
      { name: 'Nyaa', state: 'ok', text: '210 ms' },
      { name: 'TorrentLeech', state: 'ok', text: '340 ms' },
      { name: 'The Pirate Bay', state: 'unknown', text: 'Not searched yet' },
    ]);
  });

  it('is Degraded while an indexer times out or fails, with how long and why', () => {
    const status = systemStatus();
    const rows = serviceRows({ ...status, jackett: { ...status.jackett, indexers: degradedIndexers(Date.parse(status.checked_at)) } });
    const row = rows.find((r) => r.id === 'jackett')!;
    expect(row).toMatchObject({ detail: '2 of 4 indexers OK', health: 'warning', pill: { status: 'warning', label: 'Degraded' } });
    expect(row.indexers?.slice(2)).toEqual([
      { name: 'The Pirate Bay', state: 'failing', text: 'Timing out for 2 h' },
      { name: 'RuTracker', state: 'failing', text: 'Failing for 5 min: jackett error: Login failed' },
    ]);
    expect(summarize(rows)).toMatchObject({
      tone: 'warning',
      title: 'Mostly fine',
      body: '2 indexers are not answering. Search still works with the others.',
      short: '1 warning',
      attention: true,
    });
  });

  it('names a lone failing indexer in the banner, and says when none is left', () => {
    const checkedAt = Date.parse(systemStatus().checked_at);
    const since = new Date(checkedAt - 3 * 86_400_000).toISOString();
    const one = serviceRows(
      withIndexers([
        { name: 'Nyaa', status: 'ok', ms: 210, since: null },
        { name: 'The Pirate Bay', status: 'timeout', ms: 8000, since },
      ])
    );
    expect(one.find((r) => r.id === 'jackett')?.indexers?.[1].text).toBe('Timing out for 3 days');
    expect(summarize(one).body).toBe('The Pirate Bay is timing out. Search still works with the others.');

    const none = serviceRows(withIndexers([{ name: 'Nyaa', status: 'error', ms: 50, since: null, error: 'jackett rejected the API key (HTTP 401)' }]));
    expect(none.find((r) => r.id === 'jackett')?.indexers?.[0].text).toBe('Failing: jackett rejected the API key (HTTP 401)');
    expect(summarize(none).body).toBe('Nyaa is failing. Torrent search finds nothing until one answers.');
  });

  it('warns when Jackett has no indexers at all', () => {
    expect(jackettRow(withIndexers([]))).toMatchObject({ health: 'warning', pill: { status: 'warning', label: 'No indexers' } });
  });

  it('keeps the plain row for a backend older than #6, or when Jackett isn’t answering', () => {
    expect(jackettRow(withIndexers(undefined))).toMatchObject({ detail: 'Answering · 210 ms', health: 'ok' });
    expect(jackettRow(withIndexers(undefined)).indexers).toBeUndefined();
    const down = jackettRow({ ...systemStatus(), jackett: { configured: true, reachable: false, error: 'timeout after 3s' } });
    expect(down).toMatchObject({ health: 'error', detail: 'timeout after 3s' });
    expect(down.indexers).toBeUndefined();
  });
});

describe('versionLabel', () => {
  it('shortens a commit SHA to 7 characters', () => {
    expect(versionLabel('9000f05e8c1b2d3a4f5e6d7c8b9a0f1e2d3c4b5a')).toBe('9000f05');
  });

  it('prefixes a release number with v', () => {
    expect(versionLabel('2.0.0')).toBe('v2.0.0');
  });

  it('leaves anything else alone', () => {
    expect(versionLabel('dev')).toBe('dev');
  });
});
