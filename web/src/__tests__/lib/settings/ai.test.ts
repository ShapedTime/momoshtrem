import type { AiSettings, AiUsage, RunSummary } from '@/lib/api/contract';
import {
  aiChanges,
  aiNavSummary,
  budgetMonth,
  classifierHelp,
  draftOf,
  formatBudgetInput,
  formatDuration,
  hasChanges,
  nextRunPhrase,
  parseBudget,
  picksHelp,
  runEstimate,
  runNowProblem,
  runProgressLine,
  runToastBody,
  scheduleHelp,
  spendBar,
  spendLine,
  spendProjection,
} from '@/lib/settings/ai';
import { AI_SETTINGS } from '@/mocks/fixtures/settings';

// Local time, so the month and "tonight" don't depend on the machine's zone
const local = (month: number, day: number, hour = 0, minute = 0) => new Date(2026, month - 1, day, hour, minute).getTime();
const NOW = local(10, 16, 21, 44);
const iso = (ms: number) => new Date(ms).toISOString();

const SETTINGS: AiSettings = { ...AI_SETTINGS };

const LAST_RUN: RunSummary = {
  id: 42,
  kind: 'scheduled',
  status: 'succeeded',
  model: 'claude-opus-5-5',
  started_at: iso(local(10, 16, 3)),
  finished_at: iso(local(10, 16, 3) + 41_000),
  cost_usd: 0.36,
  skip_reason: null,
  error: null,
};

function usage(overrides: Partial<AiUsage> = {}): AiUsage {
  return {
    configured: true,
    month: '2026-10',
    resets_at: iso(local(11, 1)),
    total_usd: 3.12,
    budget_usd: 15,
    budget_reached: false,
    by_feature: [
      { feature: 'recs', cost_usd: 2.18, requests: 41 },
      { feature: 'ask', cost_usd: 0.53, requests: 23 },
      { feature: 'classifier', cost_usd: 0.41, requests: 96 },
    ],
    recs: { last_run: LAST_RUN, active_run: null, next_run_at: iso(local(10, 17, 3)) },
    estimates: { run_usd: { 'claude-opus-5-5': 0.36, 'claude-sonnet-5-5': null }, ask_usd: 0.05 },
    asks_today: 2,
    ask_daily_limit: 30,
    ...overrides,
  };
}

describe('the draft and the partial PUT', () => {
  it('shows the budget with cents, and no cap as empty', () => {
    expect(formatBudgetInput(15)).toBe('15.00');
    expect(formatBudgetInput(7.5)).toBe('7.50');
    expect(formatBudgetInput(null)).toBe('');
    expect(draftOf(SETTINGS)).toEqual({
      rec_enabled: true,
      rec_model: 'claude-opus-5-5',
      rec_schedule: 'nightly',
      budget: '15.00',
      classifier_enabled: true,
      auto_apply_high_confidence: true,
    });
  });

  it('reads dollars; empty is no cap; leaves the range to the server', () => {
    expect(parseBudget('')).toEqual({ ok: true, value: null });
    expect(parseBudget('   ')).toEqual({ ok: true, value: null });
    expect(parseBudget('20')).toEqual({ ok: true, value: 20 });
    expect(parseBudget(' $7.50 ')).toEqual({ ok: true, value: 7.5 });
    expect(parseBudget('.5')).toEqual({ ok: true, value: 0.5 });
    expect(parseBudget('-5')).toEqual({ ok: true, value: -5 });
    expect(parseBudget('ten')).toMatchObject({ ok: false });
    expect(parseBudget('1,000')).toMatchObject({ ok: false });
    expect(parseBudget('1.2.3')).toMatchObject({ ok: false });
  });

  it('sends only what changed', () => {
    const draft = draftOf(SETTINGS);
    expect(aiChanges(draft, SETTINGS)).toEqual({ patch: {}, budgetError: null });
    expect(hasChanges(aiChanges(draft, SETTINGS))).toBe(false);
    // The same amount, typed differently, isn't a change
    expect(hasChanges(aiChanges({ ...draft, budget: '15' }, SETTINGS))).toBe(false);

    const changed = aiChanges({ ...draft, rec_schedule: 'weekly', rec_model: 'claude-sonnet-5-5', budget: '' }, SETTINGS);
    expect(changed).toEqual({ patch: { rec_schedule: 'weekly', rec_model: 'claude-sonnet-5-5', monthly_budget_usd: null }, budgetError: null });
    expect(aiChanges({ ...draft, classifier_enabled: false, auto_apply_high_confidence: false, rec_enabled: false }, SETTINGS).patch).toEqual({
      rec_enabled: false,
      classifier_enabled: false,
      auto_apply_high_confidence: false,
    });
  });

  it('holds a budget that does not parse, and counts it as a change', () => {
    const result = aiChanges({ ...draftOf(SETTINGS), budget: 'lots', rec_schedule: 'manual' }, SETTINGS);
    expect(result.patch).toEqual({ rec_schedule: 'manual' });
    expect(result.budgetError).toMatch(/leave it empty for no cap/);
    expect(hasChanges(aiChanges({ ...draftOf(SETTINGS), budget: 'lots' }, SETTINGS))).toBe(true);
  });
});

describe('the budget card', () => {
  it('names the month and when it resets from the usage’s month', () => {
    expect(budgetMonth('2026-10')).toMatchObject({ name: 'October', resets: 'Nov 1', start: local(10, 1), end: local(11, 1) });
    expect(budgetMonth('2026-12')).toMatchObject({ name: 'December', resets: 'Jan 1' });
  });

  it('splits the bar by feature over the budget, in a fixed order', () => {
    const bar = spendBar(usage());
    expect(bar.segments.map((s) => [s.feature, s.cost])).toEqual([
      ['recs', 2.18],
      ['ask', 0.53],
      ['classifier', 0.41],
    ]);
    expect(bar.segments[0].pct).toBeCloseTo((2.18 / 15) * 100);
    expect(bar.label).toBe('Spent $3.12 of $15.00: Recommendations $2.18, Ask momoshtrem $0.53, Mapping classifier $0.41');
  });

  it('fills the bar with the spend itself without a cap or past the budget', () => {
    const noCap = spendBar(usage({ budget_usd: null }));
    expect(noCap.segments.reduce((sum, s) => sum + s.pct, 0)).toBeCloseTo(100);
    expect(noCap.label).toMatch(/^Spent \$3\.12 this month, no cap: /);
    const over = spendBar(usage({ budget_usd: 2, budget_reached: true }));
    expect(over.segments.reduce((sum, s) => sum + s.pct, 0)).toBeCloseTo(100);
    const nothing = spendBar(usage({ total_usd: 0, budget_usd: 0, by_feature: [] }));
    expect(nothing.segments.map((s) => s.pct)).toEqual([0, 0, 0]);
  });

  it('says what was spent of what', () => {
    expect(spendLine(usage())).toEqual({ spent: '$3.12', of: 'of $15.00 this month' });
    expect(spendLine(usage({ budget_usd: null }))).toEqual({ spent: '$3.12', of: 'this month · no cap' });
    expect(spendLine(usage({ total_usd: 1250 }))).toEqual({ spent: '$1,250.00', of: 'of $15.00 this month' });
  });

  it('projects the month at the pace so far', () => {
    // Half of October gone (Oct 16, 12:00), $3.10 spent → about $6.20
    const half = local(10, 16, 12);
    expect(spendProjection(usage({ total_usd: 3.1 }), half)).toEqual({
      text: 'At this pace you’ll spend about $6.20 by the end of October. If the budget runs out, AI pauses until the 1st and mapping falls back to rules.',
      warn: false,
    });
    expect(spendProjection(usage({ total_usd: 3.1, budget_usd: null }), half).text).toBe(
      'At this pace you’ll spend about $6.20 by the end of October. There’s no cap, so AI never pauses.'
    );
  });

  it('says when the pace would reach the budget', () => {
    // $10 by Oct 16 12:00 against $15: reached two-thirds of the way past the 15.5 days, around Oct 24
    const result = spendProjection(usage({ total_usd: 10 }), local(10, 16, 12));
    expect(result.warn).toBe(true);
    expect(result.text).toBe('At this pace you’ll reach the $15.00 budget around Oct 24. Then AI pauses until the 1st and mapping falls back to rules.');
  });

  it('skips the projection on the first day and before any spend, and says when the budget is spent', () => {
    expect(spendProjection(usage(), local(10, 1, 9)).text).toBe('If the budget runs out, AI pauses until the 1st and mapping falls back to rules.');
    expect(spendProjection(usage({ total_usd: 0 }), NOW).text).toBe(
      'Nothing spent yet this month. If the budget runs out, AI pauses until the 1st and mapping falls back to rules.'
    );
    expect(spendProjection(usage({ total_usd: 15, budget_reached: true }), NOW)).toEqual({
      text: 'This month’s budget is used up: AI is paused until Nov 1 and mapping runs on rules.',
      warn: true,
    });
  });
});

describe('personal picks', () => {
  it('estimates a run per model, with a fallback before its first', () => {
    const { estimates } = usage();
    expect(runEstimate(estimates, 'claude-opus-5-5')).toBe('≈ $0.36 a run');
    expect(runEstimate(estimates, 'claude-sonnet-5-5')).toBe('No estimate until its first run');
    expect(runEstimate(undefined, 'claude-opus-5-5')).toBe('No estimate until its first run');
    expect(runToastBody('claude-opus-5-5', estimates)).toBe('About a minute. Opus 5.5 · ≈ $0.36');
    expect(runToastBody('claude-sonnet-5-5', estimates)).toBe('About a minute. Sonnet 5.5');
  });

  it('says what the master switch does', () => {
    expect(picksHelp(true)).toMatch(/writes 4–6 shelves for For You/);
    expect(picksHelp(false)).toBe('Off: no shelves on For You and no runs, scheduled or by hand. Ask momoshtrem still works.');
  });

  it('formats durations and the next run', () => {
    expect(formatDuration(41_000)).toBe('41 s');
    expect(formatDuration(120_000)).toBe('2 min');
    expect(formatDuration(125_400)).toBe('2 min 5 s');
    expect(nextRunPhrase(iso(local(10, 17, 3)), NOW)).toBe('tonight');
    expect(nextRunPhrase(iso(local(10, 17, 14)), local(10, 17, 9))).toBe('at 14:00');
    expect(nextRunPhrase(iso(local(10, 18, 3)), NOW)).toBe('on Sunday');
  });

  const help = (overrides: Partial<Parameters<typeof scheduleHelp>[0]> = {}) =>
    scheduleHelp({
      draft: { rec_enabled: true, rec_schedule: 'nightly' },
      saved: { rec_enabled: true, rec_schedule: 'nightly' },
      configured: true,
      recs: usage().recs,
      now: NOW,
      ...overrides,
    });

  it('builds the schedule help from the next run and the last one', () => {
    expect(help()).toBe('Nightly runs at 03:00, weekly on Sunday. Next run tonight; the last one took 41 s and cost $0.36.');
    // A schedule change on screen: the next run is known once it's saved
    expect(help({ draft: { rec_enabled: true, rec_schedule: 'weekly' } })).toBe(
      'Nightly runs at 03:00, weekly on Sunday. The last run took 41 s and cost $0.36.'
    );
    expect(help({ draft: { rec_enabled: true, rec_schedule: 'manual' } })).toBe(
      'Nightly runs at 03:00, weekly on Sunday. Manual: only when you press Run now; the last one took 41 s and cost $0.36.'
    );
    expect(help({ draft: { rec_enabled: false, rec_schedule: 'nightly' } })).toMatch(/Picks are off, so nothing runs; the last one/);
    expect(help({ recs: { last_run: null, active_run: null, next_run_at: null } })).toBe('Nightly runs at 03:00, weekly on Sunday.');
    expect(help({ configured: false, recs: { last_run: null, active_run: null, next_run_at: null } })).toBe('Nightly runs at 03:00, weekly on Sunday.');
  });

  it('says when the last run failed or was skipped', () => {
    const failed = { ...LAST_RUN, status: 'failed' as const, error: 'The Anthropic API didn’t answer (overloaded).' };
    expect(help({ recs: { last_run: failed, active_run: null, next_run_at: null } })).toBe(
      'Nightly runs at 03:00, weekly on Sunday. The last run didn’t finish: The Anthropic API didn’t answer (overloaded).'
    );
    const skipped = { ...LAST_RUN, status: 'skipped' as const, started_at: null, skip_reason: 'budget_exceeded' as const };
    expect(help({ recs: { last_run: skipped, active_run: null, next_run_at: iso(local(10, 17, 3)) } })).toBe(
      'Nightly runs at 03:00, weekly on Sunday. Next run tonight; the last one was skipped: the budget was used up.'
    );
  });

  it('describes a run while it works', () => {
    expect(runProgressLine({ model: 'claude-opus-5-5', status: 'queued', cost_usd: 0 })).toBe('Opus 5.5 · starting');
    expect(runProgressLine({ model: 'claude-sonnet-5-5', status: 'running', cost_usd: 0.12 })).toBe('Sonnet 5.5 · $0.12 so far');
    expect(runProgressLine({ model: 'claude-opus-5', status: 'running', cost_usd: 0.2 })).toBe('claude-opus-5 · $0.20 so far');
  });

  it('explains why Run now did not start', () => {
    expect(runNowProblem({ status: 409, code: 'recs_disabled', message: 'x' }, 'Nov 1').title).toBe('Personal picks are off');
    expect(runNowProblem({ status: 409, code: 'no_history', message: 'x' }, 'Nov 1')).toMatchObject({
      title: 'Not enough history yet',
      link: { href: '/settings?section=trakt' },
    });
    expect(runNowProblem({ status: 429, code: 'budget_exceeded', message: 'x' }, 'Nov 1').body).toBe(
      'AI is paused until Nov 1. Raise the budget below to run now.'
    );
    expect(runNowProblem({ status: 500, code: 'internal', message: 'Something broke.' }, null)).toEqual({
      title: 'Couldn’t start a run',
      body: 'Something broke. Try again in a moment.',
    });
  });
});

describe('episode mapping and the nav', () => {
  it('names the classifier’s real model and cost', () => {
    expect(classifierHelp('claude-haiku-5-5')).toBe(
      'Only for files the rules can’t place: absolute numbering, specials, odd names. Haiku 5.5, under $0.001 a torrent.'
    );
    expect(classifierHelp('claude-sonnet-5-5')).toMatch(/Sonnet 5\.5, about \$0\.01 a torrent\.$/);
  });

  it('summarises the schedule and the spend', () => {
    expect(aiNavSummary(undefined, usage())).toEqual({ text: '' });
    expect(aiNavSummary(SETTINGS, undefined)).toEqual({ text: '' });
    expect(aiNavSummary(SETTINGS, usage())).toEqual({ text: 'Nightly · $3.12 of $15.00', attention: undefined });
    expect(aiNavSummary({ ...SETTINGS, rec_schedule: 'weekly' }, usage({ budget_usd: null })).text).toBe('Weekly · $3.12 this month');
    expect(aiNavSummary({ ...SETTINGS, rec_enabled: false }, usage()).text).toBe('Picks off · $3.12 of $15.00');
    expect(aiNavSummary(SETTINGS, usage({ total_usd: 15, budget_reached: true }))).toEqual({ text: 'Nightly · $15.00 of $15.00', attention: true });
    expect(aiNavSummary(SETTINGS, usage({ configured: false }))).toEqual({ text: 'No Anthropic key' });
  });
});
