'use client';

import { cx } from '@/lib/utils/cx';

interface RangeProps {
  min: number;
  max: number;
  step?: number;
  /** The two thumbs, low first. */
  value: [number, number];
  onValueChange: (value: [number, number]) => void;
  /** Each thumb's accessible name: ["Minimum size", "Maximum size"]. */
  labels: [string, string];
  /** What a thumb's value says, read out and shown under the track ("200 MB", "No maximum"). */
  valueText: (value: number, thumb: 'min' | 'max') => string;
  className?: string;
}

/**
 * A dual-thumb range (HANDOFF §3 `Range`): two native range inputs over one
 * track, so each thumb is a real slider (arrows, Page Up/Down, Home/End, a
 * screen reader's value text). The thumbs never cross; the low one comes to
 * the top once it's in the upper half, so two thumbs at the top end can part.
 */
export function Range({ min, max, step = 1, value, onValueChange, labels, valueText, className }: RangeProps) {
  const [low, high] = value;
  const percent = (n: number) => ((n - min) / (max - min)) * 100;
  return (
    <div className={cx('range', className)}>
      <div className="range-track">
        <div className="range-fill" style={{ left: `${percent(low)}%`, right: `${100 - percent(high)}%` }} />
        <input
          type="range"
          className={cx('range-input', low > (min + max) / 2 && 'is-top')}
          min={min}
          max={max}
          step={step}
          value={low}
          aria-label={labels[0]}
          aria-valuetext={valueText(low, 'min')}
          onChange={(event) => onValueChange([Math.min(Number(event.target.value), high), high])}
        />
        <input
          type="range"
          className="range-input"
          min={min}
          max={max}
          step={step}
          value={high}
          aria-label={labels[1]}
          aria-valuetext={valueText(high, 'max')}
          onChange={(event) => onValueChange([low, Math.max(Number(event.target.value), low)])}
        />
      </div>
      <div className="range-values" aria-hidden="true">
        <span>{valueText(low, 'min')}</span>
        <span>{valueText(high, 'max')}</span>
      </div>
    </div>
  );
}
