import { useId } from 'react';
import { CAT_EYE_LEFT, CAT_EYE_RIGHT, CAT_HEAD } from './geometry';
import { Pupils } from './Mark';

interface CatEyesProps {
  width?: number;
  /** Don't blink (the eyes are still under reduced motion regardless). */
  still?: boolean;
  className?: string;
}

/** Noir's eyes in the dark: the loader. Blinks slowly; static under reduced motion. */
export function CatEyes({ width = 72, still = false, className }: CatEyesProps) {
  return (
    <svg
      className={`cat-eyes--svg ${className ?? ''}`}
      width={width}
      height={Number((width * 0.3).toFixed(1))}
      viewBox="13 31 38 12"
      aria-hidden="true"
      focusable="false"
    >
      <g className="glow">
        <g className={still ? undefined : 'lid'}>
          <path d={CAT_EYE_LEFT} fill="#ffc95e" />
          <path d={CAT_EYE_RIGHT} fill="#ffc95e" />
          <Pupils fill="#0b0d12" />
        </g>
      </g>
    </svg>
  );
}

/** Round avatar with Noir's face: the face of "Ask momoshtrem" and the account menu. */
export function CatAvatar({ size = 32, className }: { size?: number; className?: string }) {
  const gradientId = useId();
  return (
    <span className={`cat-avatar ${className ?? ''}`} style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" focusable="false">
        <defs>
          <radialGradient id={gradientId} cx="0.5" cy="0.8" r="0.7">
            <stop offset="0" stopColor="#3a2a12" />
            <stop offset="1" stopColor="#11141b" />
          </radialGradient>
        </defs>
        <circle cx="32" cy="32" r="32" fill={`url(#${gradientId})`} />
        <g transform="translate(9 11) scale(0.72)">
          <path d={CAT_HEAD} fill="#07080b" />
          <path d={CAT_EYE_LEFT} fill="#ffc95e" />
          <path d={CAT_EYE_RIGHT} fill="#ffc95e" />
          <Pupils fill="#07080b" />
        </g>
      </svg>
    </span>
  );
}

export type EmptyCatVariant = 'sit' | 'sleep' | 'blank' | 'peek';

const CAT_BODY = 'M41 66C33 76 25 96 25 116c0 6 4 10 10 10h50c6 0 10-4 10-10 0-20-8-40-16-50z';
const CAT_TAIL = 'M90 122c14 0 22-6 22-16 0-8-6-12-11-10';

/**
 * Noir's silhouette for empty and error states. Filled with currentColor
 * (surface-4 inside .empty), with amber eyes when awake.
 */
export function EmptyCat({ variant = 'sit', size = 112, className }: { variant?: EmptyCatVariant; size?: number; className?: string }) {
  if (variant === 'peek') {
    return (
      <svg className={`art ${className ?? ''}`} width={size} height={size * 0.6} viewBox="0 0 120 72" aria-hidden="true" focusable="false">
        <g transform="translate(28 2)">
          <path d={CAT_HEAD} fill="currentColor" />
          <path d={CAT_EYE_LEFT} fill="var(--accent)" />
          <path d={CAT_EYE_RIGHT} fill="var(--accent)" />
        </g>
        <rect x="4" y="52" width="112" height="3" rx="1.5" fill="var(--line-3)" />
      </svg>
    );
  }

  return (
    <svg className={`art ${className ?? ''}`} width={size} height={size * 1.12} viewBox="0 0 120 134" aria-hidden="true" focusable="false">
      <path d={CAT_TAIL} fill="none" stroke="currentColor" strokeWidth="7" strokeLinecap="round" />
      <path d={CAT_BODY} fill="currentColor" />
      <g transform="translate(28 18)">
        <path d={CAT_HEAD} fill="currentColor" />
        {variant === 'sit' && (
          <>
            <path d={CAT_EYE_LEFT} fill="var(--accent)" />
            <path d={CAT_EYE_RIGHT} fill="var(--accent)" />
            <Pupils fill="var(--ink-1)" rx={1.2} ry={3} />
          </>
        )}
        {variant === 'sleep' && (
          <g fill="none" stroke="var(--text-3)" strokeWidth="1.8" strokeLinecap="round">
            <path d="M16 38q6 4 12 0" />
            <path d="M36 38q6 4 12 0" />
          </g>
        )}
      </g>
    </svg>
  );
}
