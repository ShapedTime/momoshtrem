import {
  CAT_EYE_LEFT,
  CAT_EYE_RIGHT,
  CAT_HEAD,
  CAT_PUPIL_LEFT,
  CAT_PUPIL_RIGHT,
  WORDMARK_PATH,
  WORDMARK_WIDTH,
} from './geometry';

function Pupils({ fill, rx = 1.25, ry = 3.1 }: { fill: string; rx?: number; ry?: number }) {
  return (
    <>
      <ellipse cx={CAT_PUPIL_LEFT[0]} cy={CAT_PUPIL_LEFT[1]} rx={rx} ry={ry} fill={fill} />
      <ellipse cx={CAT_PUPIL_RIGHT[0]} cy={CAT_PUPIL_RIGHT[1]} rx={rx} ry={ry} fill={fill} />
    </>
  );
}

interface MarkProps {
  size?: number;
  /** Head colour; defaults to the primary text colour (bone). */
  head?: string;
  className?: string;
}

/** Colour mark: bone head, amber eyes. Decorative; label the surrounding link. */
export function Mark({ size = 28, head = 'var(--text-1)', className }: MarkProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" focusable="false" className={className}>
      <path d={CAT_HEAD} fill={head} />
      <path d={CAT_EYE_LEFT} fill="var(--accent)" />
      <path d={CAT_EYE_RIGHT} fill="var(--accent)" />
      <Pupils fill="var(--ink-1)" />
    </svg>
  );
}

interface MarkMonoProps {
  size?: number;
  fill?: string;
  className?: string;
}

/** Single-colour mark with eye cut-outs. */
export function MarkMono({ size = 28, fill = 'currentColor', className }: MarkMonoProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" focusable="false" className={className}>
      <path fillRule="evenodd" d={`${CAT_HEAD} ${CAT_EYE_LEFT} ${CAT_EYE_RIGHT}`} fill={fill} />
      <Pupils fill={fill} />
    </svg>
  );
}

interface WordmarkProps {
  height?: number;
  fill?: string;
  className?: string;
}

/** "momoshtrem" set in the brand's Geist Semibold outlines. */
export function Wordmark({ height = 20, fill = 'currentColor', className }: WordmarkProps) {
  const scale = 34 / 710;
  const width = (WORDMARK_WIDTH - 70) * scale;
  return (
    <svg height={height} viewBox={`0 0 ${width.toFixed(1)} 36`} role="img" aria-label="momoshtrem" className={className}>
      <g transform={`translate(${(-70 * scale).toFixed(2)} 35) scale(${scale.toFixed(5)})`} fill={fill}>
        <path d={WORDMARK_PATH} />
      </g>
    </svg>
  );
}

/** Mark + wordmark, gap = 0.32 × height. */
export function Lockup({ height = 28, className }: { height?: number; className?: string }) {
  return (
    <span className={`inline-flex items-center ${className ?? ''}`} style={{ gap: Math.round(height * 0.32) }}>
      <Mark size={height} />
      <Wordmark height={Math.round(height * 0.62)} fill="var(--text-1)" />
    </span>
  );
}

export { Pupils };
