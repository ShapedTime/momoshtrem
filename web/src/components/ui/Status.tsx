import type { ReactNode } from 'react';
import { Check } from 'lucide-react';
import { STATUS, type StatusKey } from '@/lib/status';
import { cx } from '@/lib/utils/cx';

/* ---- RingMeter ---------------------------------------------------------- */

interface RingMeterProps {
  /** 0–100 */
  value: number;
  size?: number;
  className?: string;
  label?: string;
}

/** Circular progress. Decorative unless given a `label`. */
export function RingMeter({ value, size = 28, className, label }: RingMeterProps) {
  const radius = 10;
  const circumference = 2 * Math.PI * radius;
  const pct = Math.min(100, Math.max(0, value));
  return (
    <svg
      className={cx('ring-meter', className)}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      <circle className="bg" cx="12" cy="12" r={radius} />
      <circle
        className="fg"
        cx="12"
        cy="12"
        r={radius}
        strokeDasharray={circumference.toFixed(1)}
        strokeDashoffset={(circumference * (1 - pct / 100)).toFixed(1)}
      />
    </svg>
  );
}

/* ---- StatusPill --------------------------------------------------------- */

interface StatusPillProps {
  status: StatusKey;
  /** Overrides the vocabulary label (e.g. "64%", "Airs Fri"). Keep it short. */
  label?: string;
  /** Shows a mini ring instead of the icon (downloads). */
  pct?: number;
  size?: 'md' | 'lg';
  /** Over artwork: darker glass base. */
  onArt?: boolean;
  className?: string;
}

/** Status is never colour alone: always icon + word. */
export function StatusPill({ status, label, pct, size = 'md', onArt, className }: StatusPillProps) {
  const def = STATUS[status];
  const Icon = def.icon;
  return (
    <span className={cx('pill', `pill--${def.tone}`, size === 'lg' && 'pill--lg', onArt && 'pill--on-art', className)}>
      {pct !== undefined ? <RingMeter value={pct} size={13} className="mini-ring" /> : <Icon size={13} strokeWidth={2.2} aria-hidden="true" />}
      <span>{label ?? def.label}</span>
    </span>
  );
}

/* ---- Confidence --------------------------------------------------------- */

export type ConfidenceLevel = 'high' | 'medium' | 'low' | 'none';

const CONFIDENCE_LABEL: Record<ConfidenceLevel, string> = {
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  none: 'No match',
};

/** Three bars plus a word (mapping review, inspect). */
export function Confidence({ level }: { level: ConfidenceLevel }) {
  const label = CONFIDENCE_LABEL[level];
  return (
    <span className={cx('conf', `conf--${level}`)} title={`Confidence: ${label}`}>
      <i aria-hidden="true">
        <b />
        <b />
        <b />
      </i>
      <span className="sr-only">Confidence: </span>
      {label}
    </span>
  );
}

/* ---- Progress ----------------------------------------------------------- */

interface ProgressProps {
  /** 0–100; ignored when indeterminate. */
  value?: number;
  tone?: 'downloading' | 'accent' | 'ready' | 'watched';
  size?: 'md' | 'lg';
  /** Only for short waits where we can't know how far along it is. */
  indeterminate?: boolean;
  /** Accessible name, e.g. "Frieren S01E29–E38 download". */
  label: string;
  className?: string;
}

export function Progress({ value = 0, tone = 'downloading', size = 'md', indeterminate, label, className }: ProgressProps) {
  const pct = Math.min(100, Math.max(0, value));
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={indeterminate ? undefined : 0}
      aria-valuemax={indeterminate ? undefined : 100}
      aria-valuenow={indeterminate ? undefined : Math.round(pct)}
      className={cx(
        'progress',
        tone !== 'downloading' && `progress--${tone}`,
        size === 'lg' && 'progress--lg',
        indeterminate && 'progress--indeterminate',
        className
      )}
    >
      <span style={{ width: indeterminate ? undefined : `${pct}%` }} />
    </div>
  );
}

/* ---- Steps -------------------------------------------------------------- */

export interface Step {
  label: ReactNode;
  /** e.g. "12 of 16" */
  detail?: ReactNode;
  state: 'done' | 'active' | 'pending';
}

/** Named phases for long jobs (mapping, Ask, Trakt sync): never just a spinner. */
export function Steps({ steps, inline, className }: { steps: Step[]; inline?: boolean; className?: string }) {
  return (
    <ol className={cx('steps', inline && 'steps--inline', className)}>
      {steps.map((step, index) => (
        <li
          key={index}
          className={cx('step', step.state === 'done' && 'is-done', step.state === 'active' && 'is-active')}
          aria-current={step.state === 'active' ? 'step' : undefined}
        >
          <span className="ic" aria-hidden="true">
            {step.state === 'done' && <Check size={inline ? 11 : 14} strokeWidth={2.6} />}
          </span>
          <span>
            {step.label}
            {step.detail && <span className="detail"> {step.detail}</span>}
          </span>
          <span className="sr-only">{step.state === 'done' ? '(done)' : step.state === 'active' ? '(in progress)' : '(pending)'}</span>
        </li>
      ))}
    </ol>
  );
}
