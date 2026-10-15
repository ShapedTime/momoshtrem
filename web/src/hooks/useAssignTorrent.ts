'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { useToast } from '@/components/ui';
import type { AssignHint, AssignmentJobDetail } from '@/lib/api/contract';
import { assignmentApi, assignmentKeys, libraryKeys, titleKeys, titlesApi, waitForJob } from '@/lib/api/endpoints';
import type { TorrentSearchContext } from '@/types/jackett';

/** What adding a torrent led to. */
type AssignOutcome =
  | { kind: 'review'; jobId: number }
  | { kind: 'applied'; addedToLibrary: boolean }
  | { kind: 'failed'; detail: AssignmentJobDetail };

interface AssignVariables {
  magnetUri: string;
  indexer?: string;
}

/** An episode search hints its season and episode; a season search its season. */
export function assignHint(context: TorrentSearchContext): AssignHint | undefined {
  if (context.mediaType === 'movie' || context.season === undefined) return undefined;
  return context.mediaType === 'episode' && context.episode !== undefined ? { season: context.season, episode: context.episode } : { season: context.season };
}

const isAbort = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

/**
 * Adding a torrent from the legacy search (until #6) starts a mapping job
 * (PLAN §6 "Assignment"), after adding the title to the library when it
 * isn't there yet.
 * - A show's job is reviewed at /mapping/{id}: the search closes and goes there.
 * - A movie's job is waited for here. One plausible file applies on its own
 *   (the success toast, and the title refreshes); otherwise the review picks one.
 */
export function useAssignTorrent(
  context: TorrentSearchContext,
  { onApplied, onLeave }: { onApplied?: () => void; onLeave: () => void }
) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [added, setAdded] = useState<ReadonlySet<string>>(() => new Set());
  const movie = context.mediaType === 'movie';

  // Waiting on a movie job stops when the search goes away
  const waiting = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    waiting.current = controller;
    return () => controller.abort();
  }, []);

  const mutation = useMutation({
    mutationFn: async ({ magnetUri, indexer }: AssignVariables): Promise<AssignOutcome> => {
      const addedToLibrary = context.libraryId == null;
      const libraryId = context.libraryId ?? (await (movie ? titlesApi.addMovie(context.tmdbId) : titlesApi.addShow(context.tmdbId))).id;
      if (!movie) {
        const hint = assignHint(context);
        const { job_id: jobId } = await assignmentApi.assignShow(libraryId, { magnet_uri: magnetUri, ...(hint && { hint }), ...(indexer && { indexer }) });
        return { kind: 'review', jobId };
      }
      const { job_id: jobId } = await assignmentApi.assignMovie(libraryId, { magnet_uri: magnetUri, ...(indexer && { indexer }) });
      const detail = await waitForJob(queryClient, jobId, waiting.current?.signal);
      if (detail.job.status === 'applied') return { kind: 'applied', addedToLibrary };
      if (detail.job.status === 'failed' || detail.job.status === 'cancelled') return { kind: 'failed', detail };
      return { kind: 'review', jobId };
    },
    onSettled: () =>
      Promise.all([titleKeys.all, libraryKeys.all, assignmentKeys.lists].map((queryKey) => queryClient.invalidateQueries({ queryKey }))),
  });
  const { mutate, isPending, variables } = mutation;

  const add = useCallback(
    (magnetUri: string, indexer?: string) => {
      if (isPending) return;
      mutate(
        { magnetUri, indexer },
        {
          // Per call, so nothing happens once the search has closed
          onSuccess: (outcome) => {
            if (outcome.kind === 'review') {
              onLeave();
              router.push(`/mapping/${outcome.jobId}`);
            } else if (outcome.kind === 'applied') {
              setAdded((current) => new Set(current).add(magnetUri));
              toast({ tone: 'ready', title: outcome.addedToLibrary ? 'Added to library and assigned torrent' : 'Torrent assigned to movie' });
              onApplied?.();
            } else {
              const cancelled = outcome.detail.job.status === 'cancelled';
              toast({
                tone: 'error',
                title: cancelled ? 'The mapping was discarded' : 'Couldn’t map the torrent',
                body: cancelled ? 'Nothing was added to Infuse.' : (outcome.detail.job.error ?? undefined),
              });
            }
          },
          onError: (error) => {
            if (!isAbort(error)) toast({ tone: 'error', title: 'Couldn’t add the torrent', body: error.message });
          },
        }
      );
    },
    [isPending, mutate, onLeave, router, toast, onApplied]
  );

  return {
    add,
    isAdding: (magnetUri: string) => isPending && variables?.magnetUri === magnetUri,
    isAdded: (magnetUri: string) => added.has(magnetUri),
  };
}
