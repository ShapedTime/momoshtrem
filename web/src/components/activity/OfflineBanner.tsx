'use client';

import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, WifiOff } from 'lucide-react';
import { Button } from '@/components/ui';
import { formatTimeOfDay } from '@/lib/activity/format';

/** The first retry comes 12 s after the failure, then every 30 s (activity.js `retryCountdown`). */
export const FIRST_RETRY_MS = 12_000;
export const NEXT_RETRY_MS = 30_000;

interface OfflineBannerProps {
  /** When the failure that started this was reported (the query's errorUpdatedAt). */
  since: number;
  /** When it last answered (the query's dataUpdatedAt), or null when nothing is cached. */
  lastKnown: number | null;
  /** Ask again; settles when the answer (or the failure) is in. */
  onRetry: () => Promise<unknown>;
}

/**
 * "Can't reach momoshtrem" (activity.js `offlineBanner`): what the page shows
 * instead, what still works, and a retry countdown (12 s, then "Checking…",
 * then 30 s) with Retry now. The banner owns the retries: the page stops
 * polling while it's up.
 */
export function OfflineBanner({ since, lastKnown, onRetry }: OfflineBannerProps) {
  const [dueAt, setDueAt] = useState<number | null>(since + FIRST_RETRY_MS);
  const [attempts, setAttempts] = useState(0);
  const [now, setNow] = useState(since);

  const retry = useCallback(() => {
    setDueAt(null);
    setAttempts((count) => count + 1);
    void onRetry().finally(() => setDueAt(Date.now() + NEXT_RETRY_MS));
  }, [onRetry]);

  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(tick);
  }, []);

  useEffect(() => {
    if (dueAt === null) return;
    const timer = window.setTimeout(retry, Math.max(0, dueAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [dueAt, retry]);

  const seconds = dueAt === null ? 0 : Math.max(1, Math.ceil((dueAt - now) / 1000));
  const countdown = dueAt === null ? 'Checking…' : `${attempts > 0 ? 'Still not answering. ' : ''}Retrying in ${seconds} s`;

  return (
    <div className="banner act-offline" role="alert">
      <WifiOff size={20} aria-hidden="true" />
      <div className="grow">
        <strong>Can’t reach momoshtrem</strong>
        <span>
          {lastKnown !== null && `Showing what it said at ${formatTimeOfDay(lastKnown)}. `}
          Downloads may still be running on the server; Infuse can still play ready titles if WebDAV is up.
        </span>
        <span className="text-fg-3" aria-live="off">
          {countdown}
        </span>
      </div>
      <div className="actions">
        <Button size="sm" onClick={retry} loading={dueAt === null}>
          <RefreshCw size={15} aria-hidden="true" /> Retry now
        </Button>
      </div>
    </div>
  );
}
