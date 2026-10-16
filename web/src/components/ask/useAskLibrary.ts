'use client';

import { useCallback, useState } from 'react';
import { useAddRecTitle } from '@/components/recommendations';
import { useLibraryLookup } from '@/hooks/useLibraryLookup';
import type { AskItem, AvailabilityStage, LibraryRef, MediaType } from '@/lib/api/contract';
import { showSourceParams } from '@/lib/source/context';
import { useSheet } from '@/lib/sheets/useSheet';

type TitleRef = Pick<AskItem, 'media_type' | 'tmdb_id'>;

const keyOf = (item: TitleRef): string => `${item.media_type}:${item.tmdb_id}`;

/** Find a torrent for a title Ask found: a movie, or a show's season-1 packs (Season pack scope; flow 2 in js/manifest.js). */
export function torrentSheetParams(item: { media_type: MediaType; tmdb_id: number }): Record<string, string> {
  return item.media_type === 'movie' ? { movie: String(item.tmdb_id) } : showSourceParams(item.tmdb_id, { season: 1 });
}

/** Where a card's title stands: not in the library, added since Noir picked it (Find a torrent next), or in it (its stage). */
export type AskLibraryState = { kind: 'none' } | { kind: 'added' } | { kind: 'in'; stage: AvailabilityStage };

export interface AskLibrary {
  stateOf: (item: AskItem) => AskLibraryState;
  adding: (item: AskItem) => boolean;
  add: (item: AskItem) => void;
  findTorrent: (item: TitleRef) => void;
}

/**
 * The library side of Ask's cards. Adding is For You's (`useAddRecTitle`:
 * the same refetches; its toast offers Find a torrent here). An item's `library` is what it was when
 * Noir picked it; `/library?ids=` for the answered turns says what it is
 * now, so a title added from its "…" sheet, or removed elsewhere, shows that
 * too. A title added here is "In library" with Find a torrent, which opens
 * the torrent sheet over the conversation.
 */
export function useAskLibrary(answered: readonly AskItem[]): AskLibrary {
  const sheets = useSheet();
  const findTorrent = useCallback((item: TitleRef) => sheets.open('torrent', torrentSheetParams(item)), [sheets]);
  const addTitle = useAddRecTitle({ findTorrent });
  const [added, setAdded] = useState<ReadonlySet<string>>(new Set());
  const lookup = useLibraryLookup(answered.map((item): LibraryRef => ({ media_type: item.media_type, tmdb_id: item.tmdb_id })));
  const settled = !lookup.isPending && !lookup.error;
  const lookedUp = new Set(answered.map(keyOf));

  const stateOf = (item: AskItem): AskLibraryState => {
    if (added.has(keyOf(item))) return { kind: 'added' };
    const now = lookup.get(item.media_type, item.tmdb_id);
    if (now) return item.library ? { kind: 'in', stage: now.availability_stage } : { kind: 'added' };
    // A card still streaming, or a lookup that hasn't answered (or can't): the item's own state stands
    if (settled && lookedUp.has(keyOf(item))) return { kind: 'none' };
    return item.library ? { kind: 'in', stage: item.library.availability_stage } : { kind: 'none' };
  };

  const pending = addTitle.isPending && addTitle.variables ? keyOf(addTitle.variables) : null;

  return {
    stateOf,
    adding: (item) => pending === keyOf(item),
    add: (item) =>
      addTitle.mutate(
        { media_type: item.media_type, tmdb_id: item.tmdb_id, title: item.title },
        { onSuccess: () => setAdded((keys) => new Set(keys).add(keyOf(item))) }
      ),
    findTorrent,
  };
}
