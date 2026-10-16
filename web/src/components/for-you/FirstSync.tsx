'use client';

import { RefreshCw } from 'lucide-react';
import { Banner, Button, Loader, Steps } from '@/components/ui';
import { useTraktSync } from '@/hooks/useTraktActions';
import type { RunProgress, TraktIntegration } from '@/lib/api/contract';
import { firstSyncDetail, firstSyncSteps, writingShelvesStep } from '@/lib/for-you/state';
import { syncErrorCopy } from '@/lib/settings/trakt';

/**
 * The first import after connecting Trakt (HANDOFF §4.3: named steps, never
 * just a spinner): Noir's eyes while it runs, what's in so far, and the
 * steps. When it stops with an error, the eyes go and Retry picks it up at
 * the step that failed, with the same words as Settings › Trakt. Once the
 * import is in, Noir's first run adds "Writing your shelves" (#155).
 */
export function FirstSync({ trakt, firstRun = null }: { trakt: TraktIntegration; firstRun?: RunProgress | null }) {
  const { sync } = trakt;
  const failed = sync.status === 'error';
  // The answer goes into the integration's cache, so polling resumes at once
  const retry = useTraktSync();
  const stopped = syncErrorCopy(sync);

  const steps = [...firstSyncSteps(sync), ...(firstRun ? [writingShelvesStep(firstRun)] : [])].map(({ label, detail, state }) => ({
    label,
    detail: detail ?? undefined,
    state,
  }));

  return (
    <section className="page sync-hero" aria-label="First sync with Trakt">
      {failed ? (
        <div className="flex w-full max-w-[560px] flex-col gap-4 text-left">
          <h2 className="t-title-2">First sync with Trakt</h2>
          <Banner
            tone="error"
            role="alert"
            title={stopped.title}
            action={
              <Button size="sm" onClick={() => retry.mutate()} loading={retry.isPending}>
                <RefreshCw size={16} aria-hidden="true" /> Retry
              </Button>
            }
          >
            {retry.isError ? `${retry.error.message} What’s imported so far stays.` : stopped.body}
          </Banner>
        </div>
      ) : (
        <Loader title="First sync with Trakt" detail={firstSyncDetail(sync, firstRun !== null)} />
      )}
      <Steps steps={steps} className="sync-steps" />
    </section>
  );
}
