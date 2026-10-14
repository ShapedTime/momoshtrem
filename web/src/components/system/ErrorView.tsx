'use client';

import { RefreshCw, TriangleAlert } from 'lucide-react';
import { Button, ButtonLink } from '@/components/ui';

interface ErrorViewProps {
  /** What failed, in plain words. */
  title?: string;
  /** What still works and what to do. */
  message?: string;
  /** Next's error digest (matches the server log line). */
  digest?: string;
  onRetry?: () => void;
}

/**
 * A page that failed to render (HANDOFF errors: calm and specific, never cute
 * when something broke). Says what failed, what's fine, and one way forward.
 */
export function ErrorView({
  title = 'This page didn’t load',
  message = 'Something went wrong while showing it. Your library, downloads and Infuse aren’t affected.',
  digest,
  onRetry,
}: ErrorViewProps) {
  return (
    <div className="page flex min-h-[60vh] flex-col items-center justify-center gap-4 py-12 text-center" role="alert">
      <span className="grid size-14 place-items-center rounded-full border border-error-line bg-error-bg text-error">
        <TriangleAlert size={26} aria-hidden="true" />
      </span>
      <h1 className="t-title-2">{title}</h1>
      <p className="max-w-[48ch] text-fg-2">{message}</p>
      {digest && <p className="t-mono text-fg-3">Error {digest}</p>}
      <div className="flex flex-wrap justify-center gap-2">
        {onRetry && (
          <Button variant="primary" onClick={onRetry}>
            <RefreshCw size={16} aria-hidden="true" /> Try again
          </Button>
        )}
        <ButtonLink href="/">Go to For You</ButtonLink>
      </div>
    </div>
  );
}
