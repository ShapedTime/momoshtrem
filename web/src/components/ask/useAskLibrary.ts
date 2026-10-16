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

const withKey = (keys: ReadonlySet<string>, key: string): ReadonlySet<string> => new Set(keys).add(key);

function withoutKey(keys: ReadonlySet<string>, key: string): ReadonlySet<string> {
  const next = new Set(keys);
  next.delete(key);
  return next;
}

/**
 * The library side of Ask's cards. Adding is For You's (`useAddRecTitle`:
 * the same refetches; its toast offers Find a torrent here), tracked per
 * title, so two quick adds each keep their spinner. An item's `library` is
 * what it was when Noir picked it; `/library?ids=` for the answered turns
 * says what it is now, so a title added from its "…" sheet, or removed
 * elsewhere, shows that too. When a turn ends its ids change, and the last
 * lookup answers until the new one lands. A title added here is "In
 * library" with Find a torrent, which opens the torrent sheet over the
 * conversation.
 */
export function useAskLibrary(answered: readonly AskItem[]): AskLibrary {
  const sheets = useSheet();
  const findTorrent = useCallback((item: TitleRef) => sheets.open('torrent', torrentSheetParams(item)), [sheets]);
  const addTitle = useAddRecTitle({ findTorrent });
  const [added, setAdded] = useState<ReadonlySet<string>>(new Set());
  const [adding, setAdding] = useState<ReadonlySet<string>>(new Set());
  const lookup = useLibraryLookup(
    answered.map((item): LibraryRef => ({ media_type: item.media_type, tmdb_id: item.tmdb_id })),
    { keepPrevious: true }
  );
  // The lookup has answered for these titles: the previous titles' answer standing in only knows some of them
  const settled = !lookup.isPending && !lookup.error && !lookup.isPlaceholderData;
  const lookedUp = new Set(answered.map(keyOf));

  const stateOf = (item: AskItem): AskLibraryState => {
    if (added.has(keyOf(item))) return { kind: 'added' };
    const now = lookup.get(item.media_type, item.tmdb_id);
    if (now) return item.library ? { kind: 'in', stage: now.availability_stage } : { kind: 'added' };
    // A card still streaming, or a lookup that hasn't answered (or can't): the item's own state stands
    if (settled && lookedUp.has(keyOf(item))) return { kind: 'none' };
    return item.library ? { kind: 'in', stage: item.library.availability_stage } : { kind: 'none' };
  };

  // mutate's own callbacks fire only for the newest call, so each add follows its own promise
  const add = (item: AskItem) => {
    const key = keyOf(item);
    setAdding((keys) => withKey(keys, key));
    addTitle
      .mutateAsync({ media_type: item.media_type, tmdb_id: item.tmdb_id, title: item.title, seasons: item.seasons })
      .then(() => setAdded((keys) => withKey(keys, key)))
      // useAddRecTitle's error toast says what went wrong
      .catch(() => undefined)
      .finally(() => setAdding((keys) => withoutKey(keys, key)));
  };

  return {
    stateOf,
    adding: (item) => adding.has(keyOf(item)),
    add,
    findTorrent,
  };
}
