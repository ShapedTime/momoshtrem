import { applyStatus, ASK_STEPS, REFINE_STEPS, RUN_STEPS, stepLabel, stepRows } from '@/lib/recommendations/steps';
import type { AskStep } from '@/lib/api/contract';

describe('stepLabel', () => {
  it.each([
    ['history', 'Reading your history…', 'Read your history'],
    ['web', 'Searching the web for new releases…', 'Searched the web for new releases'],
    ['tmdb', 'Checking TMDB…', 'Checked TMDB'],
    ['rerank', 'Re-ranking the candidates…', 'Re-ranked the candidates'],
    ['write', 'Writing reasons…', 'Wrote reasons'],
  ] as const)('%s: the design’s Ask labels', (step, active, done) => {
    expect(stepLabel(step, 'active')).toBe(active);
    expect(stepLabel(step, 'pending')).toBe(active);
    expect(stepLabel(step, 'done')).toBe(done);
  });

  it('a run writes shelves', () => {
    expect(stepLabel('write', 'active', 'run')).toBe('Writing your shelves…');
    expect(stepLabel('write', 'done', 'run')).toBe('Wrote your shelves');
    expect(stepLabel('history', 'done', 'run')).toBe('Read your history');
  });
});

describe('stepRows', () => {
  it('lists what the server reported, then the rest as pending; details only once done', () => {
    const rows = stepRows(
      [
        { step: 'history', state: 'done', detail: '2,184 plays' },
        { step: 'web', state: 'active', detail: '3 searches so far' },
      ],
      ASK_STEPS
    );
    expect(rows.map((r) => [r.step, r.state, r.label, r.detail])).toEqual([
      ['history', 'done', 'Read your history', '2,184 plays'],
      ['web', 'active', 'Searching the web for new releases…', null],
      ['tmdb', 'pending', 'Checking TMDB…', null],
      ['write', 'pending', 'Writing reasons…', null],
    ]);
  });

  it('a refinement shows its own steps; a run’s rows use the run labels', () => {
    expect(stepRows([], REFINE_STEPS).map((r) => r.label)).toEqual(['Re-ranking the candidates…', 'Writing reasons…']);
    expect(stepRows([{ step: 'history', state: 'active', detail: null }], RUN_STEPS, 'run').at(-1)?.label).toBe('Writing your shelves…');
  });

  it('keeps a reported step that isn’t in the order, where it was reported', () => {
    expect(stepRows([{ step: 'rerank', state: 'done', detail: '23 checked' }], ['write']).map((r) => r.step)).toEqual(['rerank', 'write']);
  });
});

describe('applyStatus', () => {
  const history: AskStep = { step: 'history', state: 'active', detail: null };

  it('appends a new step and replaces a known one in place', () => {
    const started = applyStatus([], history);
    expect(started).toEqual([history]);
    const done = applyStatus(started, { step: 'history', state: 'done', detail: '2,184 plays' });
    expect(done).toEqual([{ step: 'history', state: 'done', detail: '2,184 plays' }]);
    expect(started).toEqual([history]); // never mutates
  });

  it('starting a step closes the one left active', () => {
    const steps = applyStatus([history], { step: 'web', state: 'active', detail: null });
    expect(steps.map((s) => `${s.step}:${s.state}`)).toEqual(['history:done', 'web:active']);
  });
});
