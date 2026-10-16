import type { AiFeature, AiSettings, AiSettingsPatch, AiUsage, ClassifierModel, RecModel, RecSchedule, RunSummary } from '@/lib/api/contract';

/**
 * Settings › AI & picks (screens/settings/ai-*; PLAN §6 "AI settings and
 * usage"): the draft the section edits and the partial PUT it sends, the
 * budget input, the spend bar and its projection, the schedule's help and
 * the words around Run now. Pure, so Jest covers it; components/settings/AiSection
 * renders it, and the settings nav reads its line here.
 */

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export const REC_MODELS: readonly RecModel[] = ['claude-opus-5-5', 'claude-sonnet-5-5'];
export const SCHEDULES: readonly RecSchedule[] = ['nightly', 'weekly', 'manual'];
/** Always all three, in the contract's order; the spend bar keeps each one's colour. */
export const FEATURES: readonly AiFeature[] = ['recs', 'ask', 'classifier'];

export const MODEL_NAMES: Record<RecModel | ClassifierModel, string> = {
  'claude-opus-5-5': 'Opus 5.5',
  'claude-sonnet-5-5': 'Sonnet 5.5',
  'claude-haiku-5-5': 'Haiku 5.5',
};

export const SCHEDULE_LABELS: Record<RecSchedule, string> = { nightly: 'Nightly', weekly: 'Weekly', manual: 'Manual' };

export const FEATURE_LABELS: Record<AiFeature, string> = {
  recs: 'Recommendations',
  ask: 'Ask momoshtrem',
  classifier: 'Mapping classifier',
};

/** The model cards (the design's words). */
export const MODEL_CARDS: Record<RecModel, { tag: string; blurb: string; accent: boolean }> = {
  'claude-opus-5-5': { tag: 'Deeper', blurb: 'Sharper reasons, reads more of your history.', accent: true },
  'claude-sonnet-5-5': { tag: 'Cheaper', blurb: 'Good enough most nights, a bit more generic.', accent: false },
};

/**
 * What the classifier costs a torrent on each model. Measured on Haiku 5.5
 * (#5): about $0.0007 a torrent when every file is asked about, far less in
 * the default mode. Sonnet 5.5 and Opus 5.5 cost 20× and 40× Haiku 5.5's
 * token prices (momoshtrem/internal/aicore/pricing.go).
 */
const CLASSIFIER_COST: Record<ClassifierModel, string> = {
  'claude-haiku-5-5': 'under $0.001',
  'claude-sonnet-5-5': 'about $0.01',
  'claude-opus-5-5': 'about $0.03',
};

// ---- The draft and the partial PUT ------------------------------------------------------

/** What the section edits: the settings, with the budget as typed. The classifier's model isn't edited here. */
export interface AiDraft {
  rec_enabled: boolean;
  rec_model: RecModel;
  rec_schedule: RecSchedule;
  /** The budget input's text: "15.00", or empty for no cap. */
  budget: string;
  classifier_enabled: boolean;
  auto_apply_high_confidence: boolean;
}

export type AiField = keyof AiSettings;

/** How the section names each field, e.g. in "Check “Budget” above." */
export const FIELD_LABELS: Record<AiField, string> = {
  rec_enabled: 'Write personal picks',
  rec_model: 'Model',
  rec_schedule: 'Schedule',
  monthly_budget_usd: 'Budget',
  classifier_enabled: 'Ask AI about tricky files',
  classifier_model: 'Classifier model',
  auto_apply_high_confidence: 'Auto-apply high-confidence mappings',
};

export function isAiField(value: unknown): value is AiField {
  return typeof value === 'string' && value in FIELD_LABELS;
}

/** The budget as the input shows it: 15 → "15.00", null (no cap) → "". */
export function formatBudgetInput(value: number | null): string {
  return value === null ? '' : value.toFixed(2);
}

export function draftOf(settings: AiSettings): AiDraft {
  return {
    rec_enabled: settings.rec_enabled,
    rec_model: settings.rec_model,
    rec_schedule: settings.rec_schedule,
    budget: formatBudgetInput(settings.monthly_budget_usd),
    classifier_enabled: settings.classifier_enabled,
    auto_apply_high_confidence: settings.auto_apply_high_confidence,
  };
}

export const BUDGET_FORMAT_ERROR = 'Enter an amount in dollars, like 15 or 7.50, or leave it empty for no cap.';

export type BudgetInput = { ok: true; value: number | null } | { ok: false; error: string };

/**
 * Read the budget input: empty is no cap (null); "15", "7.50", "$20" are
 * dollars. Anything else can't be sent. The range (0 or more) is the
 * server's to check: its 422 shows on the field.
 */
export function parseBudget(text: string): BudgetInput {
  const value = text.trim().replace(/^\$\s*/, '');
  if (value === '') return { ok: true, value: null };
  if (!/^-?(\d+\.?\d*|\.\d+)$/.test(value)) return { ok: false, error: BUDGET_FORMAT_ERROR };
  return { ok: true, value: Number(value) };
}

export interface AiChanges {
  /** Only the fields that differ from what was loaded. */
  patch: AiSettingsPatch;
  /** The budget input doesn't parse: nothing can be saved until it does. */
  budgetError: string | null;
}

/** What Save would send: a partial PUT of the changed fields. */
export function aiChanges(draft: AiDraft, saved: AiSettings): AiChanges {
  const patch: AiSettingsPatch = {};
  if (draft.rec_enabled !== saved.rec_enabled) patch.rec_enabled = draft.rec_enabled;
  if (draft.rec_model !== saved.rec_model) patch.rec_model = draft.rec_model;
  if (draft.rec_schedule !== saved.rec_schedule) patch.rec_schedule = draft.rec_schedule;
  if (draft.classifier_enabled !== saved.classifier_enabled) patch.classifier_enabled = draft.classifier_enabled;
  if (draft.auto_apply_high_confidence !== saved.auto_apply_high_confidence) patch.auto_apply_high_confidence = draft.auto_apply_high_confidence;
  const budget = parseBudget(draft.budget);
  if (!budget.ok) return { patch, budgetError: budget.error };
  if (budget.value !== saved.monthly_budget_usd) patch.monthly_budget_usd = budget.value;
  return { patch, budgetError: null };
}

/** Something differs from what was loaded (a budget that doesn't parse counts). */
export const hasChanges = ({ patch, budgetError }: AiChanges): boolean => budgetError !== null || Object.keys(patch).length > 0;

// ---- Money and the month ----------------------------------------------------------------

const USD = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** "$3.12", "$1,250.00". */
export function usd(value: number): string {
  return USD.format(value);
}

const MONTH_NAME = new Intl.DateTimeFormat('en-US', { month: 'long' });
const DAY_MONTH = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const WEEKDAY = new Intl.DateTimeFormat('en-US', { weekday: 'long' });
const CLOCK = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' });

export interface BudgetMonth {
  /** "October" */
  name: string;
  /** "Nov 1": when the budget starts over. */
  resets: string;
  /** The month's bounds in this browser's time, for the projection. */
  start: number;
  end: number;
}

/**
 * The usage's month ("2026-10"). Its dates come from the month itself, not
 * from `resets_at`: that's midnight server-local, which a browser in another
 * time zone would show as the 31st.
 */
export function budgetMonth(month: string): BudgetMonth {
  const [year, index] = month.split('-').map(Number);
  const start = new Date(year, index - 1, 1);
  const next = new Date(year, index, 1);
  return { name: MONTH_NAME.format(start), resets: DAY_MONTH.format(next), start: start.getTime(), end: next.getTime() };
}

// ---- The spend bar -------------------------------------------------------------------------

export interface SpendSegment {
  feature: AiFeature;
  label: string;
  cost: number;
  /** Width on the bar, 0–100: a share of the budget, or of the spend without a cap or past it. */
  pct: number;
}

export interface SpendBar {
  segments: SpendSegment[];
  /** The bar's accessible name: "Spent $3.12 of $15.00: Recommendations $2.18, …". */
  label: string;
}

export function spendBar(usage: Pick<AiUsage, 'by_feature' | 'total_usd' | 'budget_usd'>): SpendBar {
  const { total_usd: total, budget_usd: budget } = usage;
  const scale = budget !== null && budget > total ? budget : total;
  const segments = FEATURES.map((feature) => {
    const cost = usage.by_feature.find((row) => row.feature === feature)?.cost_usd ?? 0;
    return { feature, label: FEATURE_LABELS[feature], cost, pct: scale > 0 ? (cost / scale) * 100 : 0 };
  });
  const spent = budget !== null ? `Spent ${usd(total)} of ${usd(budget)}` : `Spent ${usd(total)} this month, no cap`;
  return { segments, label: `${spent}: ${segments.map((s) => `${s.label} ${usd(s.cost)}`).join(', ')}` };
}

/** "$3.12" "of $15.00 this month", or "this month · no cap". */
export function spendLine(usage: Pick<AiUsage, 'total_usd' | 'budget_usd'>): { spent: string; of: string } {
  return { spent: usd(usage.total_usd), of: usage.budget_usd !== null ? `of ${usd(usage.budget_usd)} this month` : 'this month · no cap' };
}

const PAUSES = 'If the budget runs out, AI pauses until the 1st and mapping falls back to rules.';
const NO_CAP = 'There’s no cap, so AI never pauses.';

/**
 * "At this pace…": this month's spend carried to the month's end at the
 * pace so far. Not in the first day (too early to tell), and once the budget
 * is spent it says what that means instead.
 */
export function spendProjection(usage: Pick<AiUsage, 'month' | 'total_usd' | 'budget_usd' | 'budget_reached'>, now: number): { text: string; warn: boolean } {
  const month = budgetMonth(usage.month);
  const { total_usd: total, budget_usd: budget } = usage;
  if (usage.budget_reached) {
    return { text: `This month’s budget is used up: AI is paused until ${month.resets} and mapping runs on rules.`, warn: true };
  }
  const tail = budget === null ? NO_CAP : PAUSES;
  const elapsed = Math.min(now, month.end) - month.start;
  if (total <= 0 || elapsed < DAY) return { text: total <= 0 ? `Nothing spent yet this month. ${tail}` : tail, warn: false };
  const projected = total * ((month.end - month.start) / elapsed);
  if (budget !== null && projected > budget) {
    const reachedAt = month.start + (budget / total) * elapsed;
    return {
      text: `At this pace you’ll reach the ${usd(budget)} budget around ${DAY_MONTH.format(new Date(reachedAt))}. Then AI pauses until the 1st and mapping falls back to rules.`,
      warn: true,
    };
  }
  return { text: `At this pace you’ll spend about ${usd(projected)} by the end of ${month.name}. ${tail}`, warn: false };
}

// ---- Personal picks ------------------------------------------------------------------------

/** "≈ $0.36 a run", from the mean of the model's last runs; none before its first. */
export function runEstimate(estimates: AiUsage['estimates'] | undefined, model: RecModel): string {
  const value = estimates?.run_usd[model];
  return value === null || value === undefined ? 'No estimate until its first run' : `≈ ${usd(value)} a run`;
}

/** Run now's toast: "About a minute. Opus 5.5 · ≈ $0.36" (no estimate before the model's first run). */
export function runToastBody(model: RecModel, estimates: AiUsage['estimates'] | undefined): string {
  const value = estimates?.run_usd[model];
  return `About a minute. ${MODEL_NAMES[model]}${value === null || value === undefined ? '' : ` · ≈ ${usd(value)}`}`;
}

/** The master switch's help: it's what picks need at all (Ask doesn't). */
export function picksHelp(enabled: boolean): string {
  return enabled
    ? 'Noir reads your Trakt history, checks what’s new and writes 4–6 shelves for For You.'
    : 'Off: no shelves on For You and no runs, scheduled or by hand. Ask momoshtrem still works.';
}

/** "41 s", "2 min 5 s". */
export function formatDuration(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds} s`;
  const rest = seconds % 60;
  return `${Math.floor(seconds / 60)} min${rest ? ` ${rest} s` : ''}`;
}

/** "tonight", "at 14:00" (today, in daylight), "on Sunday". */
export function nextRunPhrase(at: string, now: number): string {
  const date = new Date(at);
  const hour = date.getHours();
  if (date.getTime() - now <= DAY) return hour >= 18 || hour < 6 ? 'tonight' : `at ${CLOCK.format(date)}`;
  return `on ${WEEKDAY.format(date)}`;
}

const sentence = (text: string) => text.replace(/[.\s]+$/, '');
const capitalize = (text: string) => text.charAt(0).toUpperCase() + text.slice(1);

/** "the last one took 41 s and cost $0.36"; null before the first. */
function lastRunClause(run: RunSummary | null): string | null {
  if (!run) return null;
  switch (run.status) {
    case 'succeeded': {
      const took = run.started_at && run.finished_at ? `took ${formatDuration(Date.parse(run.finished_at) - Date.parse(run.started_at))} and ` : '';
      return `the last one ${took}cost ${usd(run.cost_usd)}`;
    }
    case 'failed':
      return `the last one didn’t finish${run.error ? `: ${sentence(run.error)}` : ''}`;
    case 'skipped':
      return `the last one was skipped: ${run.skip_reason === 'budget_exceeded' ? 'the budget was used up' : 'there wasn’t enough Trakt history yet'}`;
    default:
      return null;
  }
}

export interface ScheduleHelpInput {
  /** What's on screen, and what's saved: the next run is the saved schedule's. */
  draft: Pick<AiDraft, 'rec_enabled' | 'rec_schedule'>;
  saved: Pick<AiSettings, 'rec_enabled' | 'rec_schedule'>;
  configured: boolean;
  recs: AiUsage['recs'] | undefined;
  now: number;
}

/**
 * The schedule's help (the design's "Nightly runs at 03:00, weekly on
 * Sunday. Next run tonight; the last one took 41 s and cost $0.36."), from
 * the usage's `next_run_at` and last run.
 */
export function scheduleHelp({ draft, saved, configured, recs, now }: ScheduleHelpInput): string {
  const unsaved = draft.rec_enabled !== saved.rec_enabled || draft.rec_schedule !== saved.rec_schedule;
  let next: string | null = null;
  if (!configured) next = null;
  else if (!draft.rec_enabled) next = 'Picks are off, so nothing runs';
  else if (draft.rec_schedule === 'manual') next = 'Manual: only when you press Run now';
  else if (!unsaved && recs?.next_run_at) next = `Next run ${nextRunPhrase(recs.next_run_at, now)}`;
  const last = lastRunClause(recs?.last_run ?? null);
  const parts = ['Nightly runs at 03:00, weekly on Sunday.'];
  if (next && last) parts.push(`${next}; ${last}.`);
  else if (next) parts.push(`${next}.`);
  else if (last) parts.push(`${capitalize(last.replace('the last one', 'the last run'))}.`);
  return parts.join(' ');
}

/** "Opus 5.5"; a model this page doesn't know (a fallback's) as the server names it. */
export function modelName(model: string): string {
  return (MODEL_NAMES as Record<string, string>)[model] ?? model;
}

/** The run's line while it works: "Opus 5.5 · $0.12 so far" (its steps say the rest). */
export function runProgressLine(run: Pick<RunSummary, 'model' | 'status' | 'cost_usd'>): string {
  return run.status === 'queued' ? `${modelName(run.model)} · starting` : `${modelName(run.model)} · ${usd(run.cost_usd)} so far`;
}

export interface RunNowProblem {
  title: string;
  body: string;
  /** Where to fix it, when that's elsewhere. */
  link?: { label: string; href: string };
}

/** Why Run now didn't start (POST /recommendations/refresh's errors), shown in the card. */
export function runNowProblem(error: { status?: number; code?: string; message: string }, resets: string | null): RunNowProblem {
  switch (error.code) {
    case 'recs_disabled':
      return { title: 'Personal picks are off', body: 'Turn on “Write personal picks” and save, then run them.' };
    case 'no_history':
      return {
        title: 'Not enough history yet',
        body: 'Picks need Trakt connected and at least 5 titles you loved or finished.',
        link: { label: 'Trakt settings', href: '/settings?section=trakt' },
      };
    case 'budget_exceeded':
      return {
        title: 'This month’s budget is used up',
        body: `AI is paused${resets ? ` until ${resets}` : ' until the 1st'}. Raise the budget below to run now.`,
      };
    case 'unavailable':
      return { title: 'No Anthropic key', body: 'Picks need ANTHROPIC_API_KEY on the server.' };
    default:
      return { title: 'Couldn’t start a run', body: `${sentence(error.message)}. Try again in a moment.` };
  }
}

// ---- Episode mapping --------------------------------------------------------------------

/** The classifier switch's help, from the model it really uses. */
export function classifierHelp(model: ClassifierModel): string {
  return `Only for files the rules can’t place: absolute numbering, specials, odd names. ${MODEL_NAMES[model]}, ${CLASSIFIER_COST[model]} a torrent.`;
}

// ---- The settings nav ---------------------------------------------------------------------

/** The settings nav's line under AI & picks: "Nightly · $3.12 of $15.00". A spent budget gets the attention dot. */
export function aiNavSummary(settings: AiSettings | undefined, usage: AiUsage | undefined): { text: string; attention?: boolean } {
  if (!settings || !usage) return { text: '' };
  if (!usage.configured) return { text: 'No Anthropic key' };
  const lead = settings.rec_enabled ? SCHEDULE_LABELS[settings.rec_schedule] : 'Picks off';
  const spend = usage.budget_usd !== null ? `${usd(usage.total_usd)} of ${usd(usage.budget_usd)}` : `${usd(usage.total_usd)} this month`;
  return { text: `${lead} · ${spend}`, attention: usage.budget_reached || undefined };
}
