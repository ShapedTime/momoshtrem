import type { CSSProperties, HTMLAttributes, ReactNode } from 'react';
import { LoaderCircle } from 'lucide-react';
import { CatEyes, EmptyCat, type EmptyCatVariant } from '@/components/brand';
import { cx } from '@/lib/utils/cx';

/* ---- Skeleton ----------------------------------------------------------- */

type SkeletonShape = 'block' | 'line' | 'poster' | 'wide' | 'circle';

interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  /** Same box as the final content, so nothing jumps when it arrives. */
  shape?: SkeletonShape;
  width?: string | number;
  height?: string | number;
}

/** Shimmering placeholder; the shimmer stops under reduced motion. */
export function Skeleton({ shape = 'block', width, height, className, style, ...props }: SkeletonProps) {
  const dimensions: CSSProperties = {
    width: typeof width === 'number' ? `${width}px` : width,
    height: typeof height === 'number' ? `${height}px` : height,
    ...style,
  };
  return (
    <div
      aria-hidden="true"
      className={cx(
        'skel',
        shape === 'line' && 'skel-line',
        shape === 'poster' && 'skel-poster',
        shape === 'wide' && 'skel-wide',
        shape === 'circle' && 'rounded-full',
        className
      )}
      style={dimensions}
      {...props}
    />
  );
}

export function SkeletonText({ lines = 1, className }: { lines?: number; className?: string }) {
  return (
    <div className={cx('flex flex-col gap-2', className)} aria-hidden="true">
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} shape="line" width={i === lines - 1 && lines > 1 ? '75%' : '100%'} />
      ))}
    </div>
  );
}

/**
 * Loading region: announces "Loading…" once and fades the skeletons in after
 * 150 ms, so cached loads never flash (HANDOFF §4.3).
 */
export function LoadingRegion({ label = 'Loading', className, children }: { label?: string; className?: string; children: ReactNode }) {
  return (
    <div role="status" aria-live="polite" className={cx('skel-delay', className)}>
      <span className="sr-only">{label}…</span>
      {children}
    </div>
  );
}

/** Poster grid placeholder: poster + two lines per card. */
export function PosterSkeletons({ count = 6 }: { count?: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex flex-col gap-2" aria-hidden="true">
          <Skeleton shape="poster" />
          <Skeleton shape="line" width="80%" />
          <Skeleton shape="line" width="45%" />
        </div>
      ))}
    </>
  );
}

/** Landscape card placeholder (Up next, new episodes). */
export function WideSkeletons({ count = 3 }: { count?: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex flex-col gap-3" aria-hidden="true">
          <Skeleton shape="wide" />
          <Skeleton shape="line" width="40%" />
          <Skeleton shape="line" width="75%" />
        </div>
      ))}
    </>
  );
}

/* ---- EmptyState --------------------------------------------------------- */

interface EmptyStateProps {
  /** Noir's pose: sit (awake), sleep (nothing yet), blank, peek (filtered out / 404). */
  cat?: EmptyCatVariant;
  title: string;
  /** One plain sentence about why it's empty. */
  children?: ReactNode;
  /** The next step. */
  actions?: ReactNode;
  className?: string;
  headingLevel?: 'h1' | 'h2' | 'h3';
  /** A small mark on the cat, e.g. a check for "nothing left to do". Decorative. */
  badge?: ReactNode;
}

export function EmptyState({ cat = 'sit', title, children, actions, className, headingLevel: Heading = 'h3', badge }: EmptyStateProps) {
  return (
    <div className={cx('empty', className)}>
      {badge ? (
        <div className="relative">
          <EmptyCat variant={cat} size={112} />
          <span className="empty-badge" aria-hidden="true">
            {badge}
          </span>
        </div>
      ) : (
        <EmptyCat variant={cat} size={112} />
      )}
      <Heading className="t-title-3">{title}</Heading>
      {children && <p>{children}</p>}
      {actions && <div className="actions">{actions}</div>}
    </div>
  );
}

/* ---- Loader (long waits) ------------------------------------------------ */

/** Noir's eyes in the dark: only while the user is actually waiting on a long job. */
export function Loader({ title = 'Loading…', detail, className }: { title?: string; detail?: ReactNode; className?: string }) {
  return (
    <div className={cx('loader', className)} role="status" aria-live="polite">
      <div className="cat-dark">
        <CatEyes width={76} />
      </div>
      <div className="flex flex-col gap-1">
        <p className="font-medium text-fg-1">{title}</p>
        {detail && <p className="t-small text-fg-3">{detail}</p>}
      </div>
    </div>
  );
}

/* ---- Spinner (short waits, inline) -------------------------------------- */

export function Spinner({ size = 18, label, className }: { size?: number; label?: string; className?: string }) {
  return (
    <span role={label ? 'status' : undefined} className={cx('inline-flex', className)}>
      <LoaderCircle size={size} aria-hidden="true" className="animate-spin motion-reduce:animate-none" />
      {label && <span className="sr-only">{label}</span>}
    </span>
  );
}
