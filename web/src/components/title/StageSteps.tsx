import { STAGE_STEPS } from '@/lib/title/stage-copy';
import { cx } from '@/lib/utils/cx';

/** Four segments (done green, current amber) and "Add · Source · Download · Ready", the current step in bold. */
export function StageSteps({ step }: { step: number }) {
  return (
    <>
      <div className="state-steps" aria-hidden="true">
        {STAGE_STEPS.map((name, i) => (
          <i key={name} className={cx(i < step && 'done', i === step && 'on')} />
        ))}
      </div>
      <p className="state-label">
        <span>
          {STAGE_STEPS.map((name, i) => (
            <span key={name}>
              {i > 0 && ' · '}
              {i === step ? <b aria-current="step">{name}</b> : name}
            </span>
          ))}
        </span>
        <span>
          Step {step + 1} of {STAGE_STEPS.length}
        </span>
      </p>
    </>
  );
}
