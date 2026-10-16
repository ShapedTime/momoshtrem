'use client';

import { Sheet } from '@/components/ui';
import type { SearchSort } from '@/lib/api/contract';
import { SORT_OPTIONS } from '@/lib/search/filters';

interface SortSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sort: SearchSort;
  /** Picking one applies it; the sheet closes. */
  onPick: (sort: SearchSort) => void;
}

/**
 * The orders (HANDOFF §5.2), nested over the search: a card each, with what
 * it's good for. Picking one closes the sheet. The cards are pressed buttons,
 * like the scope switch, so a keyboard picks with Enter or Space and arrows
 * never apply an order on the way past it.
 */
export function SortSheet({ open, onOpenChange, sort, onPick }: SortSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange} nested size="narrow" title="Sort by">
      <div role="group" aria-label="Sort by" className="flex flex-col gap-2">
        {SORT_OPTIONS.map((option) => (
          <button
            key={option.value}
            type="button"
            className="radio-card ts-sort-card"
            aria-pressed={option.value === sort}
            onClick={() => onPick(option.value)}
          >
            <span className="ts-radio" aria-hidden="true" />
            <span className="flex flex-col text-left">
              <span className="font-semibold text-fg-1">{option.title}</span>
              <span className="t-small text-fg-2">{option.description}</span>
            </span>
          </button>
        ))}
      </div>
    </Sheet>
  );
}
