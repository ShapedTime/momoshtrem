import { serviceRows, summarize } from '@/components/settings/status-model';
import { systemStatus } from '@/mocks/fixtures/system';

describe('System status model', () => {
  it('lists every service, healthy when everything answers', () => {
    const rows = serviceRows(systemStatus());
    expect(rows.map((r) => r.name)).toEqual(['momoshtrem backend', 'WebDAV', 'Jackett', 'TMDB', 'Trakt', 'AI', 'OpenSubtitles']);
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

  it('flags a database outage on the backend row', () => {
    const status = { ...systemStatus(), db: { ok: false, size_bytes: null } };
    expect(serviceRows(status)[0]).toMatchObject({ health: 'error', pill: { status: 'error', label: 'Database down' } });
  });
});
