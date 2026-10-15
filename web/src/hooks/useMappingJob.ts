'use client';

import { useCallback, useRef } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { EyeOff } from 'lucide-react';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { useConfirm, useToast } from '@/components/ui';
import type { AssignmentJob, AssignmentJobDetail, ProposalChange, ReclassifyRequest, Title } from '@/lib/api/contract';
import { isApiError } from '@/lib/api/client';
import {
  assignmentApi,
  assignmentKeys,
  invalidateAfterJob,
  jobMutationScope,
  libraryKeys,
  putJobDetail,
  setJobDetail,
  titleKeys,
} from '@/lib/api/endpoints';
import { applyErrorMessage, readyToastTitle, shortTitle, titleHref, withDecisions } from '@/lib/mapping/review';
import { useSheet } from '@/lib/sheets/useSheet';

/** "How to play in Infuse" for the job's title: its WebDAV path when the title page has loaded it. */
export function useHowToPlay(job: Pick<AssignmentJob, 'item'>) {
  const queryClient = useQueryClient();
  const sheets = useSheet();
  const { media_type: mediaType, tmdb_id: tmdbId } = job.item;
  return useCallback(() => {
    const title = queryClient.getQueryData<Title>(titleKeys.detail(mediaType, tmdbId));
    const library = title?.library;
    sheets.open('infuse-setup', infuseSetupParams({ path: library?.webdav_path ?? null, url: library?.webdav_url ?? null }));
  }, [queryClient, sheets, mediaType, tmdbId]);
}

/**
 * A mapping job's writes (PLAN §6 "Assignment"): batch PATCH, apply and
 * discard, in one mutation scope so they run in order. Decisions show at
 * once (the answer replaces them); only the latest answer lands in the
 * cache, and a failed change refetches the job.
 */
export function useMappingActions(job: AssignmentJob) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const { toast } = useToast();
  const confirm = useConfirm();
  const howToPlay = useHowToPlay(job);
  const latest = useRef(0);
  const jobId = job.id;
  const scope = jobMutationScope(jobId);
  const refetchJob = useCallback(() => queryClient.invalidateQueries({ queryKey: assignmentKeys.job(jobId) }), [queryClient, jobId]);

  const patch = useMutation({
    scope,
    mutationFn: ({ items }: { items: ProposalChange[]; seq: number }) => assignmentApi.patchProposals(jobId, { items }),
    onSuccess: (detail, { seq }) => {
      if (seq === latest.current) void setJobDetail(queryClient, detail);
    },
    onError: (error) => {
      toast({ tone: 'error', title: 'Couldn’t save that change', body: error.message });
      void refetchJob();
    },
  });
  const { mutateAsync: patchAsync } = patch;

  /**
   * Shows decisions in the same tick (so focus can move to what replaces the
   * button you pressed), then resolves with the server's detail, or null when
   * it failed (a toast says so).
   */
  const change = useCallback(
    async (items: ProposalChange[]): Promise<AssignmentJobDetail | null> => {
      const key = assignmentKeys.job(jobId);
      latest.current += 1;
      void queryClient.cancelQueries({ queryKey: key });
      queryClient.setQueryData<AssignmentJobDetail>(key, (detail) => detail && { ...detail, proposals: withDecisions(detail.proposals, items) });
      try {
        return await patchAsync({ items, seq: latest.current });
      } catch {
        return null;
      }
    },
    [patchAsync, queryClient, jobId]
  );

  const applyMutation = useMutation({
    scope,
    mutationFn: () => assignmentApi.apply(jobId),
    onSuccess: (detail) => {
      putJobDetail(queryClient, detail);
      void invalidateAfterJob(queryClient, jobId);
      toast({
        tone: 'ready',
        sticky: true,
        title: readyToastTitle(shortTitle(detail.job.item.title), detail.proposals),
        body: 'They’ll appear in Infuse after its next library refresh.',
        action: { label: 'How to play', onClick: howToPlay },
      });
    },
  });
  const { mutateAsync: applyAsync } = applyMutation;

  /**
   * Apply. When the server still says no (a 409), `onRefused` gets the line
   * for under the button; other failures are a toast whose Retry runs this again.
   */
  const apply = useCallback(
    async function run(onRefused: (message: string) => void): Promise<void> {
      try {
        await applyAsync();
      } catch (error) {
        void refetchJob();
        if (isApiError(error) && error.status === 409) {
          onRefused(applyErrorMessage(error.code));
          return;
        }
        toast({
          tone: 'error',
          title: 'Couldn’t apply the mapping',
          body: error instanceof Error ? error.message : undefined,
          action: { label: 'Retry', onClick: () => void run(onRefused) },
        });
      }
    },
    [applyAsync, refetchJob, toast]
  );

  const discardMutation = useMutation({
    scope,
    mutationFn: () => assignmentApi.discard(jobId),
    onSuccess: () => {
      // Leave first: refetching the job here would flash "This mapping was discarded" on the way out
      router.push(titleHref(job));
      toast({ tone: 'info', icon: EyeOff, title: 'Mapping discarded', body: 'Nothing was added to Infuse.' });
      void invalidateAfterJob(queryClient, jobId, { refetchJob: false });
    },
    onError: (error) => {
      toast({ tone: 'error', title: 'Couldn’t discard the mapping', body: error.message });
      void refetchJob();
    },
  });
  const { mutate: discardNow } = discardMutation;

  /** Confirms first: the job closes and nothing reaches Infuse. */
  const discard = useCallback(async () => {
    const confirmed = await confirm({
      title: 'Discard this mapping?',
      description: 'Nothing is added to Infuse. The torrent is removed unless another title or mapping uses it.',
      confirmLabel: 'Discard mapping',
    });
    if (confirmed) discardNow();
  }, [confirm, discardNow]);

  return { change, apply, applying: applyMutation.isPending, discard, discarding: discardMutation.isPending };
}

/**
 * How starting a re-check went: running (or already running; `detail` is the
 * job as refetched right after, which may already be done), no AI (503), or
 * another failure.
 */
export type RecheckStart =
  | { kind: 'started'; detail: AssignmentJobDetail | undefined }
  | { kind: 'unavailable' }
  | { kind: 'failed'; message: string };

/**
 * "Re-check with AI" (PLAN §6 "Re-check"): POST reclassify (202), then refetch
 * the job so it polls while `resolving`. It shares the job's mutation scope,
 * so it starts after any PATCH still on its way. A 409 `job_working` means a
 * re-check is already running: wait for that one.
 */
export function useRecheck(jobId: number) {
  const queryClient = useQueryClient();
  const { mutateAsync } = useMutation({
    scope: jobMutationScope(jobId),
    mutationFn: (body: ReclassifyRequest) => assignmentApi.reclassify(jobId, body),
  });

  const start = useCallback(
    async (body: ReclassifyRequest, files: number): Promise<RecheckStart> => {
      const queryKey = assignmentKeys.job(jobId);
      let failure: RecheckStart | null = null;
      try {
        await mutateAsync(body);
        // Accepted: the job is resolving now, as the server just made it. Polling starts even if the
        // refetch below fails, and a settled copy can't end the run before it began.
        queryClient.setQueryData<AssignmentJobDetail>(queryKey, (detail) => detail && startedRecheck(detail, body, files));
      } catch (error) {
        if (isApiError(error) && error.status === 503) failure = { kind: 'unavailable' };
        else if (isApiError(error) && error.status === 409 && error.code === 'job_working')
          // One is already running (another tab): wait for that one, with its own hint and count
          queryClient.setQueryData<AssignmentJobDetail>(queryKey, (detail) => detail && { ...detail, job: { ...detail.job, status: 'resolving' } });
        else failure = { kind: 'failed', message: error instanceof Error ? error.message : 'Something went wrong.' };
      }
      // After a 503 the job says whether AI is still there
      await queryClient.invalidateQueries({ queryKey });
      void queryClient.invalidateQueries({ queryKey: assignmentKeys.lists });
      return failure ?? { kind: 'started', detail: queryClient.getQueryData<AssignmentJobDetail>(queryKey) };
    },
    [mutateAsync, queryClient, jobId]
  );

  /** The run is over: new targets can make or end a conflict, which the title and library count. */
  const finished = useCallback(
    () => Promise.all([assignmentKeys.lists, titleKeys.all, libraryKeys.all].map((queryKey) => queryClient.invalidateQueries({ queryKey }))),
    [queryClient]
  );

  return { start, finished };
}

/** What a 202 did on the server (PLAN §6 "Re-check"): resolving, ai_error and the Updated marks cleared, the hint replaced. */
function startedRecheck(detail: AssignmentJobDetail, body: ReclassifyRequest, files: number): AssignmentJobDetail {
  const { season, episode } = detail.job.hint ?? {};
  const hint = { ...(season !== undefined && { season }), ...(episode !== undefined && { episode }), ...(body.hint && { text: body.hint }) };
  return {
    job: {
      ...detail.job,
      status: 'resolving',
      ai_error: null,
      hint: Object.keys(hint).length ? hint : null,
      progress: { ...detail.job.progress, ai_files: files },
    },
    proposals: detail.proposals.map((p) => (p.updated || p.hinted ? { ...p, updated: false, hinted: false } : p)),
  };
}
