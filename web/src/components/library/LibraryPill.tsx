import type { ReactElement } from 'react';
import { StatusPill } from '@/components/ui';
import type { AvailabilityStage, LibraryItem } from '@/lib/api/contract';
import { posterBadge } from '@/lib/library/display';
import type { StatusKey } from '@/lib/status';

/** Library pills on TMDB cards; titles outside the library get none. Short labels for narrow cards. */
const PILL: Record<AvailabilityStage, { status: StatusKey; label?: string } | null> = {
  none: null,
  needs_torrent: { status: 'needs_torrent', label: 'No torrent' },
  downloading: { status: 'downloading' },
  ready: { status: 'ready' },
};

/** Anything with an availability stage: `SimilarTitle.library`, a `LibraryItem` from `useLibraryLookup`. */
type InLibrary = { availability_stage: AvailabilityStage } | null | undefined;

/**
 * Where a title stands in your library, on its poster (More like this,
 * Discover, Search): Ready, Downloading or No torrent; nothing when it isn't
 * in the library.
 */
export function LibraryPill({ library }: { library: InLibrary }) {
  const pill = library ? PILL[library.availability_stage] : null;
  return pill ? <StatusPill status={pill.status} label={pill.label} onArt /> : null;
}

/** The pill for a card's `badge` slot, or undefined so the card draws no badge. */
export function libraryPill(library: InLibrary): ReactElement | undefined {
  return library && PILL[library.availability_stage] ? <LibraryPill library={library} /> : undefined;
}

/**
 * Where a TMDB card's title stands in your library (Discover, Search), as
 * the Library's own cards say it (Review, Playing, 42%, No torrent), else
 * Ready. Short words fit the phone's 104 px cards, where "Downloading"
 * would run off the poster.
 */
export function libraryBadge(item: LibraryItem | undefined): ReactElement | undefined {
  const badge = item ? posterBadge(item) : null;
  return badge ? <StatusPill status={badge.status} label={badge.label} pct={badge.pct} onArt /> : libraryPill(item);
}
