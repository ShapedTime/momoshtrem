'use client';

import Image from 'next/image';
import Link from 'next/link';
import { type CSSProperties, type ReactNode, useState } from 'react';
import { cx } from '@/lib/utils/cx';

export type MediaShape = 'poster' | 'wide' | 'square';

interface MediaProps {
  shape?: MediaShape;
  /** Absolute image URL (TMDB). Without one, the surface-2 placeholder shows. */
  src?: string | null;
  alt?: string;
  /** `sizes` for next/image; defaults suit a poster grid. */
  sizes?: string;
  priority?: boolean;
  /** Shared-element name for the poster → detail view transition. */
  viewTransitionName?: string;
  className?: string;
  /** Overlays: badges, progress bars. */
  children?: ReactNode;
}

/** Poster (2:3), backdrop/still (16:9) or round profile, with a placeholder and a fade-in. */
export function Media({ shape = 'poster', src, alt = '', sizes, priority, viewTransitionName, className, children }: MediaProps) {
  const [loaded, setLoaded] = useState(false);
  const style: CSSProperties | undefined = viewTransitionName ? { viewTransitionName } : undefined;
  return (
    <div className={cx('media', `media--${shape}`, className)} style={style}>
      {src && (
        <Image
          src={src}
          alt={alt}
          fill
          sizes={sizes ?? (shape === 'wide' ? '(min-width: 1024px) 25vw, 80vw' : '(min-width: 1440px) 184px, (min-width: 1024px) 168px, (min-width: 640px) 140px, 33vw')}
          priority={priority}
          onLoad={() => setLoaded(true)}
          className={cx('transition-opacity duration-(--d-base) ease-out', loaded ? 'opacity-100' : 'opacity-0')}
        />
      )}
      {children}
    </div>
  );
}

interface PosterCardProps {
  href: string;
  title: string;
  /** Drawn in place of `title`, e.g. with a search match marked; `title` stays the link's name. */
  titleContent?: ReactNode;
  /** "2024 · Series", "2h 12m"; one line. */
  subtitle?: string;
  /** Why it's recommended (replaces the subtitle). */
  reason?: string;
  posterSrc?: string | null;
  /** Top-left status pill (use `onArt`). */
  badge?: ReactNode;
  /** 0–100 watched progress bar on the artwork. */
  watchedPct?: number;
  /** Inline feedback buttons under the meta. */
  footer?: ReactNode;
  /**
   * Overflow control at the artwork's top right, visible without hover: a
   * 32 px "…" button (`pcard-action` gives it a 44 px hit area) that opens an
   * ActionMenu. The pill makes room for it, and on narrow cards moves to the
   * bottom so it never hides under it.
   */
  action?: ReactNode;
  /** Lines under the title instead of the subtitle, e.g. a status and a watch line. */
  lines?: ReactNode;
  dismissed?: boolean;
  className?: string;
}

/** Title and year are always visible (never hover-only), clamped to 2 lines. */
export function PosterCard({ href, title, titleContent, subtitle, reason, posterSrc, badge, watchedPct, footer, action, lines, dismissed, className }: PosterCardProps) {
  return (
    <article className={cx('pcard', dismissed && 'is-dismissed', action !== undefined && 'has-action', className)}>
      <Link href={href} className="flex flex-col gap-2 rounded-md" aria-label={subtitle ? `${title}, ${subtitle}` : title}>
        <Media shape="poster" src={posterSrc} alt="">
          {badge && <span className="badge-tl">{badge}</span>}
          {watchedPct !== undefined && (
            <div className="progress progress--watched" aria-hidden="true">
              <span style={{ width: `${watchedPct}%` }} />
            </div>
          )}
        </Media>
      </Link>
      {action && <div className="pcard-action">{action}</div>}
      <div className="meta">
        <Link href={href} className="title" tabIndex={-1}>
          {titleContent ?? title}
        </Link>
        {lines ?? (reason ? <p className="reason">{reason}</p> : subtitle ? <p className="sub">{subtitle}</p> : null)}
      </div>
      {footer && <div className="feedback">{footer}</div>}
    </article>
  );
}

/** Fluid poster grid: auto-fill columns, never fixed card widths (3 columns ≤ 380 px). */
export function PosterGrid({ className, children, ...rest }: { className?: string; children: ReactNode; 'aria-label'?: string }) {
  return (
    <div className={cx('poster-grid', className)} {...rest}>
      {children}
    </div>
  );
}
