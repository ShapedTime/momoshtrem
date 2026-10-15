import { serviceRows, summarize, versionLabel } from '@/components/settings/status-model';
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

  it('shows the backend version short on its row', () => {
    const status = { ...systemStatus(), version: '9000f05e8c1b2d3a4f5e6d7c8b9a0f1e2d3c4b5a' };
    expect(serviceRows(status)[0].detail).toMatch(/^9000f05 · up /);
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
