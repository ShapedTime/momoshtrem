'use client';

import { TorrentSearchModal } from '@/components/torrent';
import { closeLegacySearch, useLegacySearchHost } from '@/hooks/useLegacySearch';
import { useSheet } from '@/lib/sheets/useSheet';

interface LegacySearchHostProps {
  /** A movie's torrent was assigned on the spot (a show's goes to its mapping review). */
  onAdded: () => void;
  /** The search closed (something may have been added). */
  onClose: () => void;
}

/** The legacy torrent search for a title page, opened with `openLegacySearch` (until #6). */
export function LegacySearchHost({ onAdded, onClose }: LegacySearchHostProps) {
  const context = useLegacySearchHost();
  const { current: sheet } = useSheet();
  // A sheet traps focus: wait until it has closed (Replace torrent in the episode sheet)
  if (!context || sheet) return null;
  return (
    <TorrentSearchModal
      isOpen
      context={context}
      onAdded={onAdded}
      onClose={() => {
        closeLegacySearch();
        onClose();
      }}
    />
  );
}
