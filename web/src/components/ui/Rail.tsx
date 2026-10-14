'use client';

import Link from 'next/link';
import { type ReactNode, useCallback, useEffect, useId, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { buttonClassName } from './Button';
import { cx } from '@/lib/utils/cx';

interface RailProps {
  title: string;
  subtitle?: string;
  /** posters (2.6 / 4.4 / 7 / 8 visible) or wide landscape cards. */
  kind?: 'posters' | 'wide';
  seeAllHref?: string;
  /** Extra controls in the section head (left of the arrows). */
  actions?: ReactNode;
  className?: string;
  children: ReactNode;
}

/**
 * Horizontal carousel. Swipe on touch (the next card always peeks); arrow
 * buttons on fine pointers, disabled at either end, scrolling 85% of the rail.
 */
export function Rail({ title, subtitle, kind = 'posters', seeAllHref, actions, className, children }: RailProps) {
  const headingId = useId();
  const railRef = useRef<HTMLDivElement>(null);
  const [atStart, setAtStart] = useState(true);
  const [atEnd, setAtEnd] = useState(false);

  const update = useCallback(() => {
    const rail = railRef.current;
    if (!rail) return;
    setAtStart(rail.scrollLeft <= 4);
    setAtEnd(rail.scrollLeft + rail.clientWidth >= rail.scrollWidth - 4);
  }, []);

  useEffect(() => {
    const rail = railRef.current;
    if (!rail) return;
    update();
    rail.addEventListener('scroll', update, { passive: true });
    const observer = new ResizeObserver(update);
    observer.observe(rail);
    return () => {
      rail.removeEventListener('scroll', update);
      observer.disconnect();
    };
  }, [update]);

  function scrollBy(direction: 1 | -1) {
    const rail = railRef.current;
    if (!rail) return;
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    rail.scrollBy({ left: direction * rail.clientWidth * 0.85, behavior: reduce ? 'auto' : 'smooth' });
  }

  return (
    <section className={cx('section rail-wrap', className)} aria-labelledby={headingId}>
      <div className="section-head">
        <div className="titles">
          <h2 className="t-title-2" id={headingId}>
            {title}
          </h2>
          {subtitle && <p className="t-small text-fg-3">{subtitle}</p>}
        </div>
        <div className="actions">
          {actions}
          {seeAllHref && (
            <Link className={buttonClassName({ variant: 'ghost', size: 'sm' })} href={seeAllHref}>
              See all
            </Link>
          )}
          <div className="rail-arrows">
            <button
              type="button"
              className={buttonClassName({ variant: 'secondary', size: 'sm', iconOnly: true })}
              aria-label={`Scroll ${title} left`}
              disabled={atStart}
              onClick={() => scrollBy(-1)}
            >
              <ChevronLeft size={18} aria-hidden="true" />
            </button>
            <button
              type="button"
              className={buttonClassName({ variant: 'secondary', size: 'sm', iconOnly: true })}
              aria-label={`Scroll ${title} right`}
              disabled={atEnd}
              onClick={() => scrollBy(1)}
            >
              <ChevronRight size={18} aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
      <div ref={railRef} className={cx('rail', `rail--${kind}`)} role="list" aria-labelledby={headingId}>
        {children}
      </div>
    </section>
  );
}

/** Wrap each rail child so the list semantics hold. */
export function RailItem({ children }: { children: ReactNode }) {
  return <div role="listitem">{children}</div>;
}
