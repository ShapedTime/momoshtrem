'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { useToast } from '@/components/ui';
import type { AssignHint, AssignmentJobDetail, MediaType, Title } from '@/lib/api/contract';
import { assignmentApi, assignmentKeys, libraryKeys, subtitleKeys, titleKeys, titlesApi, waitForJob } from '@/lib/api/endpoints';
import type { SourceContext } from '@/lib/source/context';

/** What adding a torrent led to. */
type AssignOutcome =
  | { kind: 'review'; jobId: number }
  | { kind: 'applied'; addedToLibrary: boolean }
  | { kind: 'failed'; detail: AssignmentJobDetail };

interface AssignVariables {
  magnetUri: string;
  indexer?: string;
}

/** What a torrent is added to: the title, added to the library first when it isn't there yet. */
export interface AssignTarget {
  mediaType: MediaType;
  tmdbId: number;
  /** The title's library id; null adds it to the library first. */
  libraryId: number | null;
  /** A show's season or episode, for its mapping job. */
  hint?: AssignHint;
}

/** An episode search hints its season and episode; a season search its season. */
export function assignHint(context: SourceContext): AssignHint | undefined {
  if (context.kind === 'movie' || context.season === null) return undefined;
  return context.episode === null ? { season: context.season } : { season: context.season, episode: context.episode };
}

/** The search sheet's title and context (`?sheet=torrent`) as what its Add assigns to. */
export function assignTarget(title: Title, context: SourceContext): AssignTarget {
  const hint = assignHint(context);
  return { mediaType: title.media_type, tmdbId: title.tmdb_id, libraryId: title.library.library_id, ...(hint && { hint }) };
}

const isAbort = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

/** How long the search waits on a movie's job before handing over to its page (which shows the progress). */
export const MOVIE_WAIT_MS = 40_000;

/**
 * Adding a torrent from the search sheet starts a mapping job
 * (PLAN §6 "Assignment"), after adding the title to the library when it
 * isn't there yet.
 * - A show's job is reviewed at /mapping/{id}: the search closes and goes there.
 * - A movie's job is waited for here. One plausible file applies on its own
 *   (the success toast, and the title refreshes); otherwise the review picks
 *   one. A slow job (40 s) or a failed read goes to /mapping/{id} too, whose
 *   progress view keeps polling.
 */
export function useAssignTorrent(target: AssignTarget, { onApplied, onLeave }: { onApplied?: () => void; onLeave: () => void }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [added, setAdded] = useState<ReadonlySet<string>>(() => new Set());
  const movie = target.mediaType === 'movie';

  // Waiting on a movie job stops when the search goes away
  const waiting = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    waiting.current = controller;
    return () => controller.abort();
  }, []);

  const mutation = useMutation({
    mutationFn: async ({ magnetUri, indexer }: AssignVariables): Promise<AssignOutcome> => {
      const addedToLibrary = target.libraryId === null;
      const libraryId = target.libraryId ?? (await (movie ? titlesApi.addMovie(target.tmdbId) : titlesApi.addShow(target.tmdbId))).id;
      if (!movie) {
        const { hint } = target;
        const { job_id: jobId } = await assignmentApi.assignShow(libraryId, { magnet_uri: magnetUri, ...(hint && { hint }), ...(indexer && { indexer }) });
        return { kind: 'review', jobId };
      }
      const { job_id: jobId } = await assignmentApi.assignMovie(libraryId, { magnet_uri: magnetUri, ...(indexer && { indexer }) });
      let detail: AssignmentJobDetail;
      try {
        detail = await waitForJob(queryClient, jobId, { signal: waiting.current?.signal, timeoutMs: MOVIE_WAIT_MS });
      } catch (error) {
        if (isAbort(error)) throw error;
        // Slow (no peers yet) or the read failed: the job exists, and its page keeps polling it
        return { kind: 'review', jobId };
      }
      if (detail.job.status === 'applied') return { kind: 'applied', addedToLibrary };
      if (detail.job.status === 'failed' || detail.job.status === 'cancelled') return { kind: 'failed', detail };
      return { kind: 'review', jobId };
    },
    // Subtitles too: a new file brings its own, and matches were graded against the old release
    onSettled: () =>
      Promise.all(
        [titleKeys.all, libraryKeys.all, assignmentKeys.lists, subtitleKeys.all].map((queryKey) => queryClient.invalidateQueries({ queryKey }))
      ),
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
