import type { AskStep, AskStepId, RecRunStep, RecStepState, RunStep } from '@/lib/api/contract';

/**
 * Named steps for long AI work (HANDOFF §4.3: "named steps, never just a
 * spinner"): Ask's turns and recommendation runs. The labels follow the
 * design's Ask screen (js/screens/ask.js): present continuous while a step
 * runs, past tense once it's done.
 */

export type StepId = AskStepId | RecRunStep;
export type StepRowState = RecStepState | 'pending';
/** Ask's turn, or a recommendation run (For You's refreshing banner, first sync, Settings). */
export type StepContext = 'ask' | 'run';

interface StepLabels {
  active: string;
  done: string;
}

const LABELS: Record<StepId, StepLabels> = {
  history: { active: 'Reading your history…', done: 'Read your history' },
  web: { active: 'Searching the web for new releases…', done: 'Searched the web for new releases' },
  tmdb: { active: 'Checking TMDB…', done: 'Checked TMDB' },
  rerank: { active: 'Re-ranking the candidates…', done: 'Re-ranked the candidates' },
  write: { active: 'Writing reasons…', done: 'Wrote reasons' },
};

/** A run writes shelves, not reasons for one answer (home.js: "Writing your shelves · 2 of 5"). */
const RUN_LABELS: Partial<Record<StepId, StepLabels>> = {
  write: { active: 'Writing your shelves…', done: 'Wrote your shelves' },
};

/** The order a fresh ask goes through. */
export const ASK_STEPS: readonly AskStepId[] = ['history', 'web', 'tmdb', 'write'];
/** A refinement ("Lighter") re-ranks what the thread already found. */
export const REFINE_STEPS: readonly AskStepId[] = ['rerank', 'write'];
/** A recommendation run's steps, in the order the web lists the pending ones. */
export const RUN_STEPS: readonly RecRunStep[] = ['history', 'web', 'tmdb', 'write'];

/** "Reading your history…" while it runs (and while pending), "Read your history" once done. */
export function stepLabel(step: StepId, state: StepRowState, context: StepContext = 'ask'): string {
  const labels = (context === 'run' && RUN_LABELS[step]) || LABELS[step];
  return state === 'done' ? labels.done : labels.active;
}

export interface StepRow {
  step: StepId;
  state: StepRowState;
  label: string;
  /** As reported: a run's active step has a running tally ("2 of 5 shelves"); Ask sends it with `done` only. Pending steps have none. */
  detail: string | null;
}

/**
 * The rows to render: the reported steps in their order, then the expected
 * ones not reached yet as pending. A step the server reports outside `order`
 * (a refinement's `rerank`) still shows, where it was reported.
 */
export function stepRows(reported: ReadonlyArray<AskStep | RunStep>, order: readonly StepId[], context: StepContext = 'ask'): StepRow[] {
  const seen = new Set(reported.map((s) => s.step));
  const rows: StepRow[] = reported.map((s) => ({
    step: s.step,
    state: s.state,
    label: stepLabel(s.step, s.state, context),
    detail: s.detail,
  }));
  for (const step of order) {
    if (!seen.has(step)) rows.push({ step, state: 'pending', label: stepLabel(step, 'pending', context), detail: null });
  }
  return rows;
}

/**
 * Fold one `status` event into a turn's steps: a step already listed is
 * replaced in place, a new one is appended. Starting a step closes the one
 * that was active (the server sends each step's `done`, but a dropped event
 * mustn't leave two spinners).
 */
export function applyStatus(steps: readonly AskStep[], status: AskStep): AskStep[] {
  const index = steps.findIndex((s) => s.step === status.step);
  const closed = status.state === 'active' ? steps.map((s) => (s.state === 'active' && s.step !== status.step ? { ...s, state: 'done' as const } : s)) : [...steps];
  if (index === -1) return [...closed, status];
  closed[index] = status;
  return closed;
}
