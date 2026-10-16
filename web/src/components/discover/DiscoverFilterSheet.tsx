'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button, Sheet } from '@/components/ui';
import { useDebounce } from '@/hooks/useDebounce';
import { discoverCountQuery } from '@/lib/api/endpoints';
import { countLabel, DEFAULT_FILTERS, type DiscoverFilters, noun } from '@/lib/discover/filters';
import { DiscoverFilterPanel } from './DiscoverFilterPanel';

/** The staged filters settle this long before the sheet counts them. */
const COUNT_DEBOUNCE_MS = 300;

interface DiscoverFilterSheetProps {
  open: boolean;
  filters: DiscoverFilters;
  onClose: () => void;
  onApply: (next: DiscoverFilters) => void;
}

/**
 * Phone filters (`?filters=1`, page-local, not in the sheet registry): the
 * whole panel plus Sort, full height. Changes are staged until "Show 42
 * shows", which counts the staged filters (page 1, debounced) and turns into
 * a disabled "No shows match" at zero. Reset keeps the sort.
 */
export function DiscoverFilterSheet({ open, filters, onClose, onApply }: DiscoverFilterSheetProps) {
  const [draft, setDraft] = useState(filters);
  // Start from the applied filters each time the sheet opens
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setDraft(filters);
  }

  const counted = useDebounce(draft, COUNT_DEBOUNCE_MS);
  const count = useQuery({ ...discoverCountQuery(counted), enabled: open });
  const total = count.data;
  const nothing = total === 0;
  const label = total === undefined ? 'Show results' : nothing ? `No ${noun(draft.type, 2)} match` : `Show ${countLabel(draft.type, total)}`;

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => !next && onClose()}
      title="Filters"
      closeLabel="Close filters"
      size="full"
      className="disc-sheet"
      footer={
        <>
          <Button variant="ghost" onClick={() => setDraft({ ...DEFAULT_FILTERS, sort: draft.sort })}>
            Reset
          </Button>
          <Button variant="primary" className="disc-apply" disabled={nothing} onClick={() => onApply(draft)} aria-live="polite">
            {label}
          </Button>
        </>
      }
    >
      <DiscoverFilterPanel filters={draft} onChange={setDraft} variant="sheet" />
    </Sheet>
  );
}
