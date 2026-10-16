'use client';

import { type PointerEvent, useRef, useState } from 'react';
import { formatRate } from '@/lib/activity/format';
import { sampleAt, SPARK_HEIGHT, SPARK_SCALE_BPS, SPARK_WIDTH, sparkPaths, sparkTip, sparkY } from '@/lib/activity/sparkline';
import { cx } from '@/lib/utils/cx';

interface SparklineProps {
  /** Total download speed, oldest first. */
  samples: readonly number[];
  intervalSeconds: number;
  /** Grey and still: nothing moving, or offline (stale data). */
  flat?: boolean;
}

/**
 * The last minute of download speed on a fixed 0–40 MB/s axis (activity.js
 * `spark`), with the value under the pointer on hover or touch.
 */
export function Sparkline({ samples, intervalSeconds, flat }: SparklineProps) {
  const box = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<{ index: number; x: number; y: number; width: number } | null>(null);
  const { line, area } = sparkPaths(samples);
  const scaleLabel = `${SPARK_SCALE_BPS / 1_000_000} MB/s`;
  const latest = samples.at(-1) ?? 0;

  const show = (event: PointerEvent<HTMLDivElement>) => {
    const rect = box.current?.getBoundingClientRect();
    if (!rect || flat || samples.length < 2) return;
    const index = sampleAt(event.clientX - rect.left, rect.width, samples.length);
    const x = (index / (samples.length - 1)) * rect.width;
    setHover({ index, x, y: sparkY(samples[index], rect.height), width: rect.width });
  };

  return (
    <div
      ref={box}
      className={cx('act-spark', flat && 'is-flat')}
      onPointerMove={show}
      onPointerDown={show}
      onPointerLeave={() => setHover(null)}
    >
      <svg viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`} preserveAspectRatio="none" role="img" aria-label={`Total download speed over the last minute, now ${formatRate(latest)}, on a scale of 0 to ${scaleLabel}`}>
        <line x1="0" y1="0.5" x2={SPARK_WIDTH} y2="0.5" className="grid" vectorEffect="non-scaling-stroke" />
        <line x1="0" y1={SPARK_HEIGHT / 2} x2={SPARK_WIDTH} y2={SPARK_HEIGHT / 2} className="grid" vectorEffect="non-scaling-stroke" />
        <path className="area" d={area} />
        <path className="line" d={line} vectorEffect="non-scaling-stroke" />
      </svg>
      <span className="scale-top" aria-hidden="true">
        {scaleLabel}
      </span>
      {hover && (
        <>
          <span className="hover-dot" style={{ transform: `translate(${hover.x}px, ${hover.y}px)` }} aria-hidden="true" />
          <span className="hover-tip" style={{ left: Math.max(0, Math.min(hover.width - 120, hover.x - 60)) }} aria-hidden="true">
            {sparkTip(hover.index, samples, intervalSeconds)}
          </span>
        </>
      )}
    </div>
  );
}
