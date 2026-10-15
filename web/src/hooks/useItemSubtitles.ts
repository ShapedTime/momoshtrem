'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2 } from 'lucide-react';
import { useConfirm, useToast } from '@/components/ui';
import type { SubtitleSearchState, SubtitlesPanelProps } from '@/components/title/SubtitlesPanel';
import type { Subtitle, SubtitleCandidate, SubtitleItemType } from '@/lib/api/contract';
import { itemSubtitlesQuery, subtitleKeys, subtitleSearchQuery, subtitlesApi } from '@/lib/api/endpoints';
import { defaultSubtitleLanguage } from '@/lib/title/subtitles';

/**
 * Everything the SubtitlesPanel shows and does for one movie or episode:
 * the stored list, a one-language OpenSubtitles search, Get and Remove.
 * `id` is the library id; null keeps every query off (nothing assigned yet).
 * `releaseKey` identifies the assigned release (e.g. its info hash): a new
 * one clears the search, whose match grades were for the old release.
 */
export function useItemSubtitles(type: SubtitleItemType, id: number | null, releaseKey: string | null = null): SubtitlesPanelProps {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();
  const itemId = id ?? 0;

  const storedQuery = useQuery({ ...itemSubtitlesQuery(type, itemId), enabled: id !== null });
  const stored = storedQuery.data?.items;

  const [chosen, setChosen] = useState<string | null>(null);
  const [searched, setSearched] = useState<string | null>(null);
  // Once searched, the picker stays on that language: Get adds it to the stored list, which moves the default on
  const language = chosen ?? searched ?? defaultSubtitleLanguage(stored ?? []);

  const [searchedRelease, setSearchedRelease] = useState(releaseKey);
  if (releaseKey !== searchedRelease) {
    setSearchedRelease(releaseKey);
    setSearched(null);
  }
  const searchQuery = useQuery({ ...subtitleSearchQuery(type, itemId, searched ? [searched] : []), enabled: id !== null && searched !== null });

  const refetchStored = () => queryClient.invalidateQueries({ queryKey: subtitleKeys.item(type, itemId) });

  const download = useMutation({
    mutationFn: (candidate: SubtitleCandidate) =>
      subtitlesApi.download(type, itemId, { file_id: candidate.file_id, language_code: candidate.language_code, release_name: candidate.release_name }),
    onSuccess: async (subtitle) => {
      await refetchStored();
      toast({ tone: 'ready', title: `${subtitle.language_name} subtitles added`, body: 'Infuse offers them with the video.' });
    },
    onError: (error, candidate) => toast({ tone: 'error', title: `Couldn’t get the ${candidate.language_name} subtitles`, body: error.message }),
  });

  const remove = useMutation({
    mutationFn: (subtitle: Subtitle) => subtitlesApi.remove(type, itemId, subtitle.id),
    onSuccess: async (_, subtitle) => {
      await refetchStored();
      toast({ tone: 'info', icon: Trash2, title: `Removed the ${subtitle.language_name} subtitles` });
    },
    onError: (error, subtitle) => toast({ tone: 'error', title: `Couldn’t remove the ${subtitle.language_name} subtitles`, body: error.message }),
  });

  let search: SubtitleSearchState = { status: 'idle' };
  if (searched !== null) {
    if (searchQuery.isFetching || searchQuery.isPending) search = { status: 'pending', language: searched };
    else if (searchQuery.isError) {
      search = { status: 'error', language: searched, message: searchQuery.error.message, retry: () => void searchQuery.refetch() };
    } else search = { status: 'success', language: searched, items: searchQuery.data.items };
  }

  return {
    stored,
    storedError: storedQuery.isError,
    onRetryStored: () => void storedQuery.refetch(),
    onRemove: async (subtitle) => {
      const confirmed = await confirm({
        title: `Remove the ${subtitle.language_name} subtitles?`,
        description:
          subtitle.source === 'hdrezka'
            ? 'Infuse stops offering them. Picking this HDRezka voice-over again brings them back.'
            : 'Infuse stops offering them. You can get them again from OpenSubtitles any time.',
        confirmLabel: 'Remove subtitles',
      });
      if (confirmed) remove.mutate(subtitle);
    },
    removingId: remove.isPending ? (remove.variables?.id ?? null) : null,
    language,
    onLanguageChange: setChosen,
    onSearch: () => {
      if (searched === language) void searchQuery.refetch();
      else setSearched(language);
    },
    search,
    onGet: (candidate) => download.mutate(candidate),
    gettingFileId: download.isPending ? (download.variables?.file_id ?? null) : null,
  };
}
