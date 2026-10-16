'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { RefreshCw } from 'lucide-react';
import { Banner, Button, Loader, Steps } from '@/components/ui';
import type { TraktIntegration } from '@/lib/api/contract';
import { traktApi, traktKeys } from '@/lib/api/endpoints';
import { firstSyncDetail, firstSyncSteps, firstSyncStoppedTitle } from '@/lib/for-you/state';

/**
 * The first import after connecting Trakt (HANDOFF §4.3: named steps, never
 * just a spinner): Noir's eyes while it runs, what's in so far, and the
 * steps. When it stops with an error, the eyes go and Try again picks it up
 * at the step that failed.
 */
export function FirstSync({ trakt }: { trakt: TraktIntegration }) {
  const queryClient = useQueryClient();
  const { sync } = trakt;
  const failed = sync.status === 'error';
  const retry = useMutation({
    mutationFn: traktApi.sync,
    // The answer is the integration with its sync running again, so polling resumes at once
    onSuccess: (next) => queryClient.setQueryData(traktKeys.integration, next),
  });

  const steps = firstSyncSteps(sync).map(({ label, detail, state }) => ({ label, detail: detail ?? undefined, state }));

  return (
    <section className="page sync-hero" aria-label="First sync with Trakt">
      {failed ? (
        <div className="flex w-full max-w-[560px] flex-col gap-4 text-left">
          <h2 className="t-title-2">First sync with Trakt</h2>
          <Banner
            tone="error"
            role="alert"
            title={firstSyncStoppedTitle(sync)}
            action={
              <Button size="sm" onClick={() => retry.mutate()} loading={retry.isPending}>
                <RefreshCw size={16} aria-hidden="true" /> Try again
              </Button>
            }
          >
            {retry.isError ? retry.error.message : (sync.error ?? 'Trakt didn’t answer.')} What’s imported so far stays.
          </Banner>
        </div>
      ) : (
        <Loader title="First sync with Trakt" detail={firstSyncDetail(sync)} />
      )}
      <Steps steps={steps} className="sync-steps" />
    </section>
  );
}
