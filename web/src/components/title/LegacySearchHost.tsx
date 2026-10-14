'use client';

import { useEffect } from 'react';
import { TorrentSearchModal } from '@/components/torrent';
import { closeLegacySearch, useLegacySearch } from '@/hooks/useLegacySearch';
import { useSheet } from '@/lib/sheets/useSheet';

interface LegacySearchHostProps {
  /** A torrent was added and assigned. */
  onAdded: () => void;
  /** The search closed (something may have been added). */
  onClose: () => void;
}

/** The legacy torrent search for a title page, opened with `openLegacySearch` (until #6). */
export function LegacySearchHost({ onAdded, onClose }: LegacySearchHostProps) {
  const context = useLegacySearch();
  const { current: sheet } = useSheet();
  // Leaving the page closes it, so the next title doesn't open with it
  useEffect(() => closeLegacySearch, []);
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
