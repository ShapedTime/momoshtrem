'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import type { MediaType } from '@/lib/api/contract';
import { libraryKeys, recKeys, titleKeys, titlesApi, traktKeys } from '@/lib/api/endpoints';
import { plural } from '@/lib/title/format';

interface AddTarget {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  /** A show's season count, for Ask's toast ("The Good Place · 4 seasons"). */
  seasons?: number | null;
}

interface AddOptions {
  /** Ask's cards (#156): the toast offers Find a torrent, the next step. */
  findTorrent?: (target: AddTarget) => void;
}

/**
 * "Add to library" from a recommendation's feedback sheet, or an Ask card.
 * Then every card that reads the library refetches: the shelves' pills
 * (/recommendations/home joins it live), title pages, the library and /me.
 */
export function useAddRecTitle({ findTorrent }: AddOptions = {}) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useMutation({
    mutationFn: ({ media_type, tmdb_id }: AddTarget) => (media_type === 'movie' ? titlesApi.addMovie(tmdb_id) : titlesApi.addShow(tmdb_id)),
    onSuccess: async (_, target) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: recKeys.home }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
        queryClient.invalidateQueries({ queryKey: traktKeys.me }),
      ]);
      toast(
        findTorrent
          ? {
              tone: 'ready',
              title: 'Added to library',
              body: target.seasons ? `${target.title} · ${plural(target.seasons, 'season')}` : target.title,
              action: { label: 'Find a torrent', onClick: () => findTorrent(target) },
            }
          : { tone: 'ready', title: 'Added to library', body: `${target.title} · next, find a torrent` }
      );
    },
    onError: (error, { title }) => toast({ tone: 'error', title: `Couldn’t add ${title}`, body: error.message }),
  });
}
