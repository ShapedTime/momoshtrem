import { Check, CheckCheck, ChevronDown } from 'lucide-react';
import type { AskStep } from '@/lib/api/contract';
import { traceLabel } from '@/lib/ask/copy';
import { ASK_STEPS, REFINE_STEPS, stepRows, type StepRow } from '@/lib/recommendations/steps';
import { cx } from '@/lib/utils/cx';

const STATE_WORDS = { done: '(done)', active: '(in progress)', pending: '(pending)' } as const;

function StepList({ rows, className }: { rows: StepRow[]; className?: string }) {
  return (
    <ol className={cx('steps', className)}>
      {rows.map((row) => (
        <li
          key={row.step}
          className={cx('step', row.state === 'done' && 'is-done', row.state === 'active' && 'is-active')}
          aria-current={row.state === 'active' ? 'step' : undefined}
        >
          <span className="ic" aria-hidden="true">
            {row.state === 'done' && <Check size={14} strokeWidth={2.5} />}
          </span>
          <span>{row.label}</span>
          {row.state === 'done' && row.detail && <span className="detail">{row.detail}</span>}
          <span className="sr-only">{STATE_WORDS[row.state]}</span>
        </li>
      ))}
    </ol>
  );
}

/** While Noir works (ask.js `stepsLive`): the steps reached, then the ones still to come. A refinement re-ranks instead. */
export function AskSteps({ steps, refine }: { steps: readonly AskStep[]; refine: boolean }) {
  const order = refine || steps.some((step) => step.step === 'rerank') ? REFINE_STEPS : ASK_STEPS;
  return <StepList rows={stepRows(steps, order)} className="ask-steps" />;
}

/** Once it ended (ask.js `trace`): "History, web and TMDB checked · 9 s", which opens to the steps and their tallies. */
export function AskTrace({ steps, elapsedMs }: { steps: readonly AskStep[]; elapsedMs: number | null }) {
  return (
    <details className="ask-trace">
      <summary>
        <CheckCheck size={15} aria-hidden="true" />
        <span>{traceLabel(steps, elapsedMs)}</span>
        <ChevronDown size={15} className="chev" aria-hidden="true" />
      </summary>
      <StepList rows={stepRows(steps, [])} />
    </details>
  );
}
