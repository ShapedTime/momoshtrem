'use client';

import { useEffect, useRef } from 'react';
import { ArrowUpDown, CircleAlert, LayoutGrid, Rows3, X } from 'lucide-react';
import { FilterChip, IconButton, SearchInput, SegmentedControl } from '@/components/ui';
import type { LibraryCounts, LibrarySort } from '@/lib/api/contract';
import { FILTERS, type LibraryFilter, type LibraryLayout, SORTS } from '@/lib/library/view';
import { plural } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

interface LibraryToolbarProps {
  q: string;
  onQueryChange: (q: string) => void;
  onClearSearch: () => void;
  filter: LibraryFilter;
  onFilterChange: (filter: LibraryFilter) => void;
  counts: LibraryCounts;
  /** Titles under the current filter and search. */
  total: number;
  sort: LibrarySort;
  onSortChange: (sort: LibrarySort) => void;
  view: LibraryLayout;
  onViewChange: (view: LibraryLayout) => void;
}

/**
 * Search, filter chips with counts, sort and the grid/list switch
 * (library.css `.lib-tools`). Phone: search full width, chips scroll
 * sideways, then the count, sort and layout share a row.
 */
export function LibraryToolbar({ q, onQueryChange, onClearSearch, filter, onFilterChange, counts, total, sort, onSortChange, view, onViewChange }: LibraryToolbarProps) {
  const chipsRef = useRef<HTMLDivElement>(null);

  // Keep the active chip in view on the phone's scrolling row
  useEffect(() => {
    const row = chipsRef.current;
    const chip = row?.querySelector<HTMLElement>('[aria-pressed="true"]');
    if (!row || !chip || row.scrollWidth <= row.clientWidth) return;
    const left = chip.offsetLeft - row.offsetLeft;
    const right = left + chip.offsetWidth;
    if (left < row.scrollLeft || right > row.scrollLeft + row.clientWidth) row.scrollLeft = Math.max(0, right - row.clientWidth + 24);
  }, [filter]);

  return (
    <div className="lib-tools">
      <div className="lib-search">
        <SearchInput
          aria-label="Search your library"
          placeholder="Search your library"
          autoComplete="off"
          enterKeyHint="search"
          value={q}
          onChange={(event) => onQueryChange(event.target.value)}
          end={
            q ? (
              <IconButton aria-label="Clear search" size="sm" onClick={onClearSearch}>
                <X size={16} aria-hidden="true" />
              </IconButton>
            ) : undefined
          }
        />
      </div>
      <div ref={chipsRef} className="fchips fchips--scroll lib-chips" role="group" aria-label="Filter">
        {FILTERS.map((definition) => {
          const count = definition.count(counts);
          const attention = definition.value === 'review' && count > 0;
          return (
            <FilterChip
              key={definition.value}
              pressed={filter === definition.value}
              count={count}
              onPressedChange={() => onFilterChange(definition.value)}
              className={cx(attention && 'fchip--review')}
            >
              {attention && <CircleAlert size={14} aria-hidden="true" />}
              {definition.label}
            </FilterChip>
          );
        })}
      </div>
      <p className="lib-count t-small text-fg-3">{plural(total, 'title')}</p>
      <div className="lib-viewbar">
        <label className="lib-sort">
          <span className="sr-only">Sort by</span>
          <ArrowUpDown size={16} aria-hidden="true" />
          <select className="input" value={sort} onChange={(event) => onSortChange(event.target.value as LibrarySort)}>
            {SORTS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <SegmentedControl<LibraryLayout>
          aria-label="Layout"
          value={view}
          onChange={onViewChange}
          options={[
            { value: 'grid', label: <LayoutGrid size={16} aria-hidden="true" />, ariaLabel: 'Grid' },
            { value: 'list', label: <Rows3 size={16} aria-hidden="true" />, ariaLabel: 'List' },
          ]}
        />
      </div>
    </div>
  );
}
