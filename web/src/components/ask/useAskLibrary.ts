'use client';

import { useCallback, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui';
import type { AskItem, MediaType } from '@/lib/api/contract';
import { libraryKeys, recKeys, titleKeys, titlesApi, traktKeys } from '@/lib/api/endpoints';
import { showSourceParams } from '@/lib/source/context';
import { useSheet } from '@/lib/sheets/useSheet';
import { plural } from '@/lib/title/format';

type TitleRef = Pick<AskItem, 'media_type' | 'tmdb_id'>;

export const askTitleKey = (item: TitleRef): string => `${item.media_type}:${item.tmdb_id}`;

/** Find a torrent for a title Ask found: a movie, or a show's season-1 packs (Season pack scope; flow 2 in js/manifest.js). */
export function torrentSheetParams(item: { media_type: MediaType; tmdb_id: number }): Record<string, string> {
  return item.media_type === 'movie' ? { movie: String(item.tmdb_id) } : showSourceParams(item.tmdb_id, { season: 1 });
}

export interface AskLibrary {
  /** Titles added from this page: "In library" + Find a torrent, whatever the item says. */
  added: ReadonlySet<string>;
  adding: ReadonlySet<string>;
  add: (item: AskItem) => void;
  findTorrent: (item: TitleRef) => void;
}

/**
 * Ask's cards add to the library in place (POST /movies or /shows, as a
 * title page does) and then offer Find a torrent, which opens the torrent
 * sheet over the conversation.
 */
export function useAskLibrary(): AskLibrary {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const sheets = useSheet();
  const [added, setAdded] = useState<ReadonlySet<string>>(new Set());
  const [adding, setAdding] = useState<ReadonlySet<string>>(new Set());

  const findTorrent = useCallback((item: TitleRef) => sheets.open('torrent', torrentSheetParams(item)), [sheets]);

  const add = useCallback(
    async (item: AskItem) => {
      const key = askTitleKey(item);
      setAdding((keys) => new Set(keys).add(key));
      try {
        await (item.media_type === 'movie' ? titlesApi.addMovie(item.tmdb_id) : titlesApi.addShow(item.tmdb_id));
        setAdded((keys) => new Set(keys).add(key));
        toast({
          tone: 'ready',
          title: 'Added to library',
          body: item.seasons ? `${item.title} · ${plural(item.seasons, 'season')}` : item.title,
          action: { label: 'Find a torrent', onClick: () => findTorrent(item) },
        });
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: titleKeys.all }),
          queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
          queryClient.invalidateQueries({ queryKey: traktKeys.me }),
          queryClient.invalidateQueries({ queryKey: recKeys.home }),
        ]);
      } catch (error) {
        toast({ tone: 'error', title: `Couldn’t add ${item.title}`, body: error instanceof Error ? error.message : undefined });
      } finally {
        setAdding((keys) => {
          const next = new Set(keys);
          next.delete(key);
          return next;
        });
      }
    },
    [findTorrent, queryClient, toast]
  );

  return { added, adding, add: (item) => void add(item), findTorrent };
}
