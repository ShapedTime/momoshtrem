'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';
import { useConfirm, useToast } from '@/components/ui';
import type { MediaType, Title } from '@/lib/api/contract';
import { libraryKeys, subtitleKeys, titleKeys, titleQuery, titlesApi } from '@/lib/api/endpoints';
import { removeConsequence } from '@/lib/title/facts';

/** How often a downloading title refreshes its progress (paused in hidden tabs). */
export const DOWNLOAD_POLL_MS = 5000;

/** `refetchInterval` for a title: every 5 s while it's downloading, otherwise not at all. */
export const pollWhileDownloading = (title: Title | undefined): number | false =>
  title?.library.availability_stage === 'downloading' ? DOWNLOAD_POLL_MS : false;

/** A title page's data: polls while the title is downloading, so progress and the stage move on their own. */
export function useTitle(mediaType: MediaType, tmdbId: number) {
  return useQuery({
    ...titleQuery(mediaType, tmdbId),
    refetchInterval: (query) => pollWhileDownloading(query.state.data),
  });
}

/**
 * Add, refresh and remove for a title page, with toasts. Each refetches the
 * title pages and the library before it settles, so the panel moves to its
 * next stage as the button stops spinning.
 */
export function useTitleActions(title: Title) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();
  const { media_type: mediaType, tmdb_id: tmdbId, title: name } = title;
  const libraryId = title.library.library_id;

  const refetch = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: titleKeys.all }),
      queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
    ]);
  const failed = (verb: string) => (error: Error) => toast({ tone: 'error', title: `Couldn’t ${verb} ${name}`, body: error.message });

  const add = useMutation({
    mutationFn: () => (mediaType === 'movie' ? titlesApi.addMovie(tmdbId) : titlesApi.addShow(tmdbId)),
    onSuccess: async () => {
      await refetch();
      toast({ tone: 'ready', title: 'Added to library', body: `${name} · next, find a torrent` });
    },
    onError: failed('add'),
  });

  const refresh = useMutation({
    mutationFn: (id: number) => (mediaType === 'movie' ? titlesApi.refreshMovie(id) : titlesApi.refreshShow(id)),
    onSuccess: async () => {
      await refetch();
      toast({ tone: 'info', title: 'Refreshed from TMDB', body: name });
    },
    onError: failed('refresh'),
  });

  const remove = useMutation({
    mutationFn: (id: number) => (mediaType === 'movie' ? titlesApi.removeMovie(id) : titlesApi.removeShow(id)),
    onSuccess: async () => {
      await refetch();
      toast({ tone: 'info', icon: Trash2, title: `Removed ${name}`, body: 'It’s no longer in your library or Infuse.' });
    },
    onError: failed('remove'),
  });

  return {
    add: () => add.mutate(),
    adding: add.isPending,
    refresh: () => {
      if (libraryId !== null) refresh.mutate(libraryId);
    },
    /** Confirms first, saying what goes and what stays. */
    remove: async () => {
      if (libraryId === null) return;
      const confirmed = await confirm({
        title: `Remove ${name} from your library?`,
        description: removeConsequence(title),
        confirmLabel: mediaType === 'movie' ? 'Remove movie' : 'Remove show',
      });
      if (confirmed) remove.mutate(libraryId);
    },
  };
}

/**
 * "Remove file from episode": confirms, unassigns the file (the episode
 * stays and shows as missing), then refetches the title, its seasons and the
 * library.
 */
export function useRemoveEpisodeFile() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();
  const unassign = useMutation({
    mutationFn: (episodeId: number) => titlesApi.unassignEpisode(episodeId),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: subtitleKeys.all }),
      ]),
  });

  return {
    removing: unassign.isPending,
    /** Resolves true once the file is gone. */
    remove: async (episodeId: number, code: string): Promise<boolean> => {
      const confirmed = await confirm({
        title: `Remove the file from ${code}?`,
        description: 'Infuse loses this episode right away and it shows as missing until you find another torrent. The torrent itself stays in Activity.',
        confirmLabel: 'Remove file',
      });
      if (!confirmed) return false;
      try {
        await unassign.mutateAsync(episodeId);
        toast({ tone: 'info', icon: Trash2, title: `Removed the file from ${code}` });
        return true;
      } catch (error) {
        toast({ tone: 'error', title: `Couldn’t remove the file from ${code}`, body: error instanceof Error ? error.message : undefined });
        return false;
      }
    },
  };
}

