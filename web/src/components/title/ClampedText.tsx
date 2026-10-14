'use client';

import { useEffect, useId, useRef, useState } from 'react';
import { cx } from '@/lib/utils/cx';

/**
 * Overviews (HANDOFF §5.1): 3 lines on phone, 4 on desktop, with "More" to
 * read the rest. The toggle only shows when the text is actually cut off.
 */
export function ClampedText({ text, className }: { text: string; className?: string }) {
  const id = useId();
  const ref = useRef<HTMLParagraphElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [clamped, setClamped] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element || expanded || typeof ResizeObserver === 'undefined') return;
    const measure = () => setClamped(element.scrollHeight > element.clientHeight + 1);
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [expanded, text]);

  return (
    <>
      <p ref={ref} id={id} className={cx(className, !expanded && 'clamp-3')}>
        {text}
      </p>
      {(clamped || expanded) && (
        <button type="button" className="more-toggle" aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Less' : 'More'}
        </button>
      )}
    </>
  );
}
