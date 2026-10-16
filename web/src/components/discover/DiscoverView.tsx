'use client';

import { useCallback, useEffect, useRef } from 'react';
import { useQueryState } from 'nuqs';
import type { DiscoverFilters } from '@/lib/discover/filters';
import { FILTER_SHEET_KEY, filterSheetParser } from '@/lib/discover/url';
import { DiscoverMoods, DiscoverQuickChips } from './DiscoverChips';
import { DiscoverFilterPanel } from './DiscoverFilterPanel';
import { DiscoverFilterSheet } from './DiscoverFilterSheet';
import { DiscoverResults } from './DiscoverResults';
import { useDiscoverFilters } from './useDiscoverFilters';

/**
 * /discover (PLAN F6, #100; screens/discover, js/screens/discover.js):
 * everything on TMDB, filtered in the URL. Desktop has a sticky filter
 * sidebar that applies live; phone has quick chips and a filter sheet that
 * stages changes. Moods are filter presets.
 */
export function DiscoverView() {
  const [filters, setFilters] = useDiscoverFilters();
  const sheet = useFilterSheet(setFilters);

  return (
    <div className="disc-page">
      <header className="page disc-head">
        <h1 className="t-display">Discover</h1>
        <p className="disc-intro text-fg-2">Everything on TMDB, filtered your way. The link keeps your filters, so a view can be bookmarked.</p>
      </header>

      <div className="page disc-top">
        <DiscoverMoods filters={filters} onChange={setFilters} />
        <DiscoverQuickChips filters={filters} onChange={setFilters} onOpenFilters={sheet.open} />
      </div>

      <div className="page disc-layout">
        <aside className="disc-side" aria-label="Filters">
          <div className="disc-side-in">
            <DiscoverFilterPanel filters={filters} onChange={setFilters} variant="side" />
          </div>
        </aside>
        <section className="disc-main" aria-labelledby="disc-count">
          <DiscoverResults filters={filters} onChange={setFilters} />
        </section>
      </div>

      <DiscoverFilterSheet open={sheet.isOpen} filters={filters} onClose={sheet.close} onApply={sheet.apply} />
    </div>
  );
}

/**
 * `?filters=1`. Opening pushes a history entry, so Back closes the sheet;
 * closing goes back again when this page opened it (a deep link just drops
 * the key). Apply goes back first and then applies, so Back afterwards
 * leaves Discover instead of undoing the filters.
 */
function useFilterSheet(setFilters: (next: DiscoverFilters) => void) {
  const [flag, setFlag] = useQueryState(FILTER_SHEET_KEY, filterSheetParser.withOptions({ history: 'push', scroll: false }));
  const isOpen = flag === true;
  const openedHere = useRef(false);
  const pending = useRef<DiscoverFilters | null>(null);

  useEffect(() => {
    if (isOpen) return;
    openedHere.current = false;
    if (!pending.current) return;
    setFilters(pending.current);
    pending.current = null;
    window.scrollTo({ top: 0 });
  }, [isOpen, setFilters]);

  const open = useCallback(() => {
    openedHere.current = true;
    void setFlag(true);
  }, [setFlag]);

  const close = useCallback(() => {
    if (openedHere.current) {
      openedHere.current = false; // a second close (a double tap) mustn't go back twice
      window.history.back();
    } else void setFlag(null, { history: 'replace' });
  }, [setFlag]);

  const apply = useCallback(
    (next: DiscoverFilters) => {
      pending.current = next;
      close();
    },
    [close]
  );

  return { isOpen, open, close, apply };
}
