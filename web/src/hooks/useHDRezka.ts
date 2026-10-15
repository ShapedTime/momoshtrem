'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import type { AssignMovieStreamRequest, SystemStatus } from '@/lib/api/contract';
import { hdrezkaApi, hdrezkaKeys, libraryKeys, statusQuery, subtitleKeys, titleKeys } from '@/lib/api/endpoints';

const hdrezkaConfigured = (status: SystemStatus) => status.hdrezka?.configured === true;

/**
 * Whether HDRezka is set up on the backend (#63, opt-in). Every HDRezka entry
 * point checks it: while it's off, or the status hasn't loaded, none shows.
 */
export function useHDRezkaEnabled(): boolean {
  const { data } = useQuery({ ...statusQuery(), select: hdrezkaConfigured });
  return data === true;
}

/**
 * Stream a movie from HDRezka: assigns, then refetches the title (now ready,
 * with `library.stream`), the library and the subtitles (graded against the
 * old file), and says so. Errors are the caller's to show: the sheet keeps
 * them inline.
 */
export function useStreamMovie(movieName: string) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useMutation({
    mutationFn: ({ movieId, body }: { movieId: number; body: AssignMovieStreamRequest }) => hdrezkaApi.assignMovie(movieId, body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: subtitleKeys.all }),
        queryClient.invalidateQueries({ queryKey: hdrezkaKeys.all }),
      ]);
      toast({ tone: 'ready', title: 'Streaming from HDRezka', body: `${movieName} · ready in Infuse` });
    },
  });
}
