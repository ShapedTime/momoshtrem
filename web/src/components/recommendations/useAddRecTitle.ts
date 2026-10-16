'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import type { MediaType } from '@/lib/api/contract';
import { libraryKeys, recKeys, titleKeys, titlesApi, traktKeys } from '@/lib/api/endpoints';

interface AddTarget {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
}

/**
 * "Add to library" from a recommendation's feedback sheet. Then every card
 * that reads the library refetches: the shelves' pills (/recommendations/home
 * joins it live), title pages, the library and /me.
 */
export function useAddRecTitle() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useMutation({
    mutationFn: ({ media_type, tmdb_id }: AddTarget) => (media_type === 'movie' ? titlesApi.addMovie(tmdb_id) : titlesApi.addShow(tmdb_id)),
    onSuccess: async (_, { title }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: recKeys.home }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
        queryClient.invalidateQueries({ queryKey: traktKeys.me }),
      ]);
      toast({ tone: 'ready', title: 'Added to library', body: `${title} · next, find a torrent` });
    },
    onError: (error, { title }) => toast({ tone: 'error', title: `Couldn’t add ${title}`, body: error.message }),
  });
}
