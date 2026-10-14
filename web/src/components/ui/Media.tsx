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
  dismissed?: boolean;
  className?: string;
}

/** Title and year are always visible (never hover-only), clamped to 2 lines. */
export function PosterCard({ href, title, subtitle, reason, posterSrc, badge, watchedPct, footer, dismissed, className }: PosterCardProps) {
  return (
    <article className={cx('pcard', dismissed && 'is-dismissed', className)}>
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
      <div className="meta">
        <Link href={href} className="title" tabIndex={-1}>
          {title}
        </Link>
        {reason ? <p className="reason">{reason}</p> : subtitle ? <p className="sub">{subtitle}</p> : null}
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
