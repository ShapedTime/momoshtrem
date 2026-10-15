'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import type { AssignMovieStreamRequest, StreamEpisodeFailure, StreamEpisodeTarget, StreamPick, SystemStatus } from '@/lib/api/contract';
import { hdrezkaApi, hdrezkaKeys, libraryKeys, statusQuery, subtitleKeys, titleKeys } from '@/lib/api/endpoints';
import { assignInChunks, type ChunkedResult } from '@/lib/hdrezka/episodes';
import type { TranslatorRef } from '@/lib/hdrezka/pick';
import { rememberVoiceOver } from '@/lib/hdrezka/remembered';
import { plural } from '@/lib/title/format';

const hdrezkaConfigured = (status: SystemStatus) => status.hdrezka?.configured === true;

/**
 * Whether HDRezka is set up on the backend (#63, opt-in). Every HDRezka entry
 * point checks it: while it's off, or the status hasn't loaded, none shows.
 */
export function useHDRezkaEnabled(): boolean {
  const { data } = useQuery({ ...statusQuery(), select: hdrezkaConfigured });
  return data === true;
}

/** A new file changes the title (and its seasons), the library, subtitle grades and HDRezka's episode annotations. */
function useFilesChanged() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: titleKeys.all }),
      queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
      queryClient.invalidateQueries({ queryKey: subtitleKeys.all }),
      queryClient.invalidateQueries({ queryKey: hdrezkaKeys.all }),
    ]);
}

/**
 * Stream a movie from HDRezka: assigns, then refetches the title (now ready,
 * with `library.stream`), the library and the subtitles (graded against the
 * old file), remembers the voice-over for next time, and says so. Errors are
 * the caller's to show: the sheet keeps them inline.
 */
export function useStreamMovie(movieName: string) {
  const filesChanged = useFilesChanged();
  const { toast } = useToast();
  return useMutation({
    mutationFn: ({ movieId, body }: { movieId: number; body: AssignMovieStreamRequest }) => hdrezkaApi.assignMovie(movieId, body),
    onSuccess: async (_, { body }) => {
      rememberVoiceOver({ id: body.translator_id, camrip: body.camrip, ads: body.ads, director: body.director });
      await filesChanged();
      toast({ tone: 'ready', title: 'Streaming from HDRezka', body: `${movieName} · ready in Infuse` });
    },
  });
}

export interface EpisodesProgress {
  done: number;
  total: number;
}

/**
 * Stream a show's episodes from HDRezka, 30 a request (`assignInChunks`):
 * progress while it runs, then the episodes that failed, to retry on their
 * own. Everything assigned so far is refetched either way; the toast only
 * comes when all of them made it.
 */
export function useStreamEpisodes(showName: string) {
  const filesChanged = useFilesChanged();
  const { toast } = useToast();
  const [progress, setProgress] = useState<EpisodesProgress | null>(null);
  const [failed, setFailed] = useState<StreamEpisodeFailure[]>([]);
  const [lastAssigned, setLastAssigned] = useState(0);

  const run = async (showId: number, pick: StreamPick, targets: StreamEpisodeTarget[], voiceOver: TranslatorRef): Promise<ChunkedResult> => {
    setFailed([]);
    const result = await assignInChunks(
      targets,
      (items) => hdrezkaApi.assignShow(showId, { ...pick, items }),
      (done, total) => setProgress({ done, total })
    );
    if (result.assigned > 0) {
      rememberVoiceOver(voiceOver);
      await filesChanged();
    }
    setProgress(null);
    setFailed(result.failed);
    setLastAssigned(result.assigned);
    if (result.failed.length === 0) {
      toast({ tone: 'ready', title: 'Streaming from HDRezka', body: `${plural(result.assigned, 'episode')} of ${showName} · ready in Infuse` });
    }
    return result;
  };

  /** `lastAssigned`: how many episodes the last run streamed (some, while others failed). */
  return { run, progress, running: progress !== null, failed, lastAssigned };
}
