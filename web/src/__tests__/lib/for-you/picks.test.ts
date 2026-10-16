import type { RunProgress, RunSummary } from '@/lib/api/contract';
import { failedRefresh, picksFootLine, refreshSummary } from '@/lib/for-you/picks';

/** The runs behind For You's picks (#155): the refreshing banner, the failed-refresh note and the footer. */

function run(overrides: Partial<RunProgress> = {}): RunProgress {
  return {
    id: 43,
    kind: 'manual',
    status: 'running',
    model: 'claude-opus-5-5',
    started_at: '2026-10-16T14:00:00Z',
    finished_at: null,
    cost_usd: 0.12,
    skip_reason: null,
    error: null,
    steps: [],
    shelves_done: 0,
    shelves_target: 0,
    ...overrides,
  };
}

describe('refreshSummary', () => {
  it('queued: starting', () => {
    expect(refreshSummary(run({ status: 'queued' }))).toBe('Starting in a moment');
    expect(refreshSummary(run({ status: 'running', steps: [] }))).toBe('Starting in a moment');
  });

  it('the step under way with its tally, then the shelves written', () => {
    expect(
      refreshSummary(
        run({
          steps: [
            { step: 'history', state: 'done', detail: '2,184 plays · 312 ratings' },
            { step: 'tmdb', state: 'active', detail: '148 candidates' },
          ],
          shelves_target: 5,
        })
      )
    ).toBe('Checking TMDB… · 148 candidates · 0 of 5 shelves done');
  });

  it('writing: the shelf count once, not the step’s own tally too', () => {
    expect(
      refreshSummary(
        run({
          steps: [
            { step: 'history', state: 'done', detail: null },
            { step: 'write', state: 'active', detail: '2 of 5 shelves' },
          ],
          shelves_done: 2,
          shelves_target: 5,
        })
      )
    ).toBe('Writing your shelves… · 2 of 5 shelves done');
  });

  it('between steps, the last one reached', () => {
    expect(refreshSummary(run({ steps: [{ step: 'history', state: 'done', detail: null }] }))).toBe('Reading your history…');
  });
});

describe('failedRefresh', () => {
  const picks = run({ id: 42, status: 'succeeded' }) as RunSummary;

  it('a newer failed run than the picks on screen', () => {
    const failed = run({ id: 43, status: 'failed', error: 'The Anthropic API didn’t answer.' });
    expect(failedRefresh(failed, picks)).toBe(failed);
    expect(failedRefresh(failed, null)).toBe(failed);
  });

  it('nothing for the picks’ own run, an older failure, a skip, or no runs', () => {
    expect(failedRefresh(picks, picks)).toBeNull();
    expect(failedRefresh(run({ id: 40, status: 'failed' }), picks)).toBeNull();
    expect(failedRefresh(run({ id: 43, status: 'skipped' }), picks)).toBeNull();
    expect(failedRefresh(undefined, picks)).toBeNull();
  });
});

describe('picksFootLine', () => {
  const now = new Date(2026, 9, 16, 21, 30);
  const at = (date: Date) => ({ started_at: date.toISOString(), finished_at: null });

  it('when the run started, in local time', () => {
    expect(picksFootLine(at(new Date(2026, 9, 16, 3, 0)), now)).toBe('Picks written by Noir at 03:00 from your Trakt history.');
    expect(picksFootLine(at(new Date(2026, 9, 15, 3, 0)), now)).toBe('Picks written by Noir yesterday at 03:00 from your Trakt history.');
    expect(picksFootLine(at(new Date(2026, 9, 11, 14, 5)), now)).toBe('Picks written by Noir on Oct 11 at 14:05 from your Trakt history.');
  });

  it('falls back to when it finished, then to no time', () => {
    expect(picksFootLine({ started_at: null, finished_at: new Date(2026, 9, 16, 3, 1).toISOString() }, now)).toBe(
      'Picks written by Noir at 03:01 from your Trakt history.'
    );
    expect(picksFootLine({ started_at: null, finished_at: null }, now)).toBe('Picks written by Noir from your Trakt history.');
  });
});
