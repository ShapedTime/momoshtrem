import type {
  AiFeature,
  AiSettings,
  AiSettingsPatch,
  AiUsage,
  ClassifierModel,
  Member,
  RecModel,
  RecSchedule,
  RunSummary,
  TraktState,
} from '@/lib/api/contract';
import { possessive } from '@/lib/members/display';

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

const BUDGET_INPUT = new Intl.NumberFormat('en-US', { useGrouping: false, minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** The budget as the input shows it: 15 → "15.00", null (no cap) → "" (never "1e+21"). */
export function formatBudgetInput(value: number | null): string {
  return value === null ? '' : BUDGET_INPUT.format(value);
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

/** What's on screen: the saved settings with this page's edits on top (untouched fields follow the server). */
export function withEdits(saved: AiSettings, edits: Partial<AiDraft>): AiDraft {
  return { ...draftOf(saved), ...edits };
}

/** A month's budget above this is a typo, not a cap. */
export const BUDGET_MAX = 10_000;

export const BUDGET_ERRORS = {
  format: 'Enter an amount in dollars, like 15 or 7.50, or leave it empty for no cap.',
  negative: 'The budget can’t be negative. Enter 0 or more, or leave it empty for no cap.',
  tooHigh: `Keep it to ${BUDGET_MAX.toLocaleString('en-US')} dollars or less, or leave it empty for no cap.`,
  /** A 422 on the budget from the server, in words (its own message is for developers). */
  server: 'momoshtrem didn’t accept that budget. Enter 0 or more, or leave it empty for no cap.',
} as const;

export type BudgetInput = { ok: true; value: number | null } | { ok: false; error: string };

/**
 * Read the budget input: empty is no cap (null); "15", "7.50", "$20" are
 * dollars, rounded to cents, from 0 to BUDGET_MAX. Anything else stays here
 * with a reason.
 */
export function parseBudget(text: string): BudgetInput {
  const value = text.trim().replace(/^\$\s*/, '');
  if (value === '') return { ok: true, value: null };
  if (!/^-?(\d+\.?\d*|\.\d+)$/.test(value)) return { ok: false, error: BUDGET_ERRORS.format };
  const amount = Number(value);
  if (!Number.isFinite(amount)) return { ok: false, error: BUDGET_ERRORS.tooHigh };
  if (amount < 0) return { ok: false, error: BUDGET_ERRORS.negative };
  if (amount > BUDGET_MAX) return { ok: false, error: BUDGET_ERRORS.tooHigh };
  return { ok: true, value: Math.round(amount * 100) / 100 };
}

export interface AiChanges {
  /** Only the edited fields that differ from what was loaded. */
  patch: AiSettingsPatch;
  /** The budget input doesn't parse: nothing can be saved until it does. */
  budgetError: string | null;
}

/**
 * What Save would send: a partial PUT of the fields this page edited and
 * that differ from the server's. Untouched fields are never sent, so a
 * change made elsewhere meanwhile stays. A budget still showing the saved
 * value as the input formats it is unchanged, so 7.125 isn't re-sent as 7.13.
 */
export function aiChanges(edits: Partial<AiDraft>, saved: AiSettings): AiChanges {
  const patch: AiSettingsPatch = {};
  if (edits.rec_enabled !== undefined && edits.rec_enabled !== saved.rec_enabled) patch.rec_enabled = edits.rec_enabled;
  if (edits.rec_model !== undefined && edits.rec_model !== saved.rec_model) patch.rec_model = edits.rec_model;
  if (edits.rec_schedule !== undefined && edits.rec_schedule !== saved.rec_schedule) patch.rec_schedule = edits.rec_schedule;
  if (edits.classifier_enabled !== undefined && edits.classifier_enabled !== saved.classifier_enabled) patch.classifier_enabled = edits.classifier_enabled;
  if (edits.auto_apply_high_confidence !== undefined && edits.auto_apply_high_confidence !== saved.auto_apply_high_confidence) {
    patch.auto_apply_high_confidence = edits.auto_apply_high_confidence;
  }
  if (edits.budget !== undefined && edits.budget !== formatBudgetInput(saved.monthly_budget_usd)) {
    const budget = parseBudget(edits.budget);
    if (!budget.ok) return { patch, budgetError: budget.error };
    if (budget.value !== saved.monthly_budget_usd) patch.monthly_budget_usd = budget.value;
  }
  return { patch, budgetError: null };
}

/** A 422's message for the field it names: the budget's in words, others as the server says. */
export function fieldErrorCopy(field: AiField, message: string): string {
  return field === 'monthly_budget_usd' ? BUDGET_ERRORS.server : message;
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

// ---- Picks and Ask by person (#239) ------------------------------------------------------

export interface MemberSpend {
  memberId: number;
  name: string;
  /** Their colour key from the household list, joined by id; null when the list doesn't have them (a neutral avatar). */
  color: string | null;
  cost: number;
}

/**
 * The budget's "Picks and Ask by person": the usage's `by_member` rows (every
 * current member, in id order), each with their colour. Null when there's no
 * split to show: one person, or a backend from before household members.
 */
export function memberSpend(usage: Pick<AiUsage, 'by_member'>, members: ReadonlyArray<Pick<Member, 'id' | 'color'>>): MemberSpend[] | null {
  const rows = usage.by_member;
  if (!rows || rows.length < 2) return null;
  return rows.map((row) => ({
    memberId: row.member_id,
    name: row.name,
    color: members.find((m) => m.id === row.member_id)?.color ?? null,
    cost: row.cost_usd,
  }));
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

/**
 * Personal picks with two or more people (#239): the runs are the viewer's,
 * the settings the household's. Null alone, or without household members.
 */
export function picksMemberNote(current: Pick<Member, 'name'> | null, people: number): string | null {
  return current && people >= 2 ? `Runs and Run now are ${possessive(current.name)}. The schedule, model and budget are shared.` : null;
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

const startOfDay = (ms: number) => new Date(ms).setHours(0, 0, 0, 0);

/**
 * When the next run starts, day and time both in this browser's time (the
 * server's 03:00 is another hour in another zone): "tonight at 03:00",
 * "today at 14:00", "tomorrow at 07:00", "on Sunday at 03:00".
 */
export function nextRunPhrase(at: string, now: number): string {
  const date = new Date(at);
  const time = CLOCK.format(date);
  const hour = date.getHours();
  if (date.getTime() - now <= DAY && (hour >= 18 || hour < 6)) return `tonight at ${time}`;
  const days = Math.round((startOfDay(date.getTime()) - startOfDay(now)) / DAY);
  if (days <= 0) return `today at ${time}`;
  if (days === 1) return `tomorrow at ${time}`;
  return `on ${WEEKDAY.format(date)} at ${time}`;
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
  /** The viewer's Trakt (#239); null while it's unknown. Only a connected member's runs are scheduled. */
  trakt: TraktState | null;
  now: number;
}

/** The slots, in the server's time: said only when the next run's own time isn't known. */
const SLOTS = 'Nightly runs at 03:00 server time, weekly on Sunday.';

/**
 * The schedule's help (the design's "Nightly runs at 03:00, weekly on
 * Sunday. Next run tonight; the last one took 41 s and cost $0.36."), from
 * the usage's `next_run_at` and last run. A known next run says its own day
 * and time in this browser's time ("Next run tonight at 03:00; …"); without
 * one, the slots are said in server time. The schedule queues runs only for
 * members with Trakt connected (#239), so without it the help says to connect
 * rather than promise a run.
 */
export function scheduleHelp({ draft, saved, configured, recs, trakt, now }: ScheduleHelpInput): string {
  const unsaved = draft.rec_enabled !== saved.rec_enabled || draft.rec_schedule !== saved.rec_schedule;
  const last = lastRunClause(recs?.last_run ?? null);
  const withLast = (lead: string) => (last ? `${lead}; ${last}.` : `${lead}.`);
  let lead: string | null = null;
  if (configured && !draft.rec_enabled) lead = 'Picks are off, so nothing runs';
  else if (configured && draft.rec_schedule === 'manual') lead = 'Manual: only when you press Run now';
  else if (configured && trakt !== null && trakt !== 'connected') {
    lead = `${trakt === 'reauth' ? 'Sign in to Trakt again' : 'Connect Trakt'} to get ${draft.rec_schedule} picks`;
  }
  if (lead) return `${SLOTS} ${withLast(lead)}`;
  if (configured && !unsaved && recs?.next_run_at) return withLast(`Next run ${nextRunPhrase(recs.next_run_at, now)}`);
  return last ? `${SLOTS} ${capitalize(last.replace('the last one', 'the last run'))}.` : SLOTS;
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
    default:
      // Including 503 unavailable: without a key Run now is disabled, so it's the server's own reason
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
