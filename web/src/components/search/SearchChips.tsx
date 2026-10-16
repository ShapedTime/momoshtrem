import { ArrowUpDown, SlidersHorizontal } from 'lucide-react';
import { FilterChip } from '@/components/ui';
import type { SearchFacets, SearchSort } from '@/lib/api/contract';
import { activeFilterCount, type FacetKey, quickChipStates, type SearchFilters, SORT_LABEL } from '@/lib/search/filters';
import { cx } from '@/lib/utils/cx';

interface SearchChipsProps {
  /** The latest answer's disjunctive facets; undefined before the first one. */
  facets: SearchFacets | undefined;
  filters: SearchFilters;
  sort: SearchSort;
  onToggle: (key: FacetKey, value: string) => void;
  onOpenFilters: () => void;
  onOpenSort: () => void;
}

/**
 * Under the scope (HANDOFF §5.2): Filters (with how many are on), Sort, and
 * quick filters with what each would give. 2160p and 1080p always; x265 and
 * Dolby Vision from 640 px. A chip that would find nothing is disabled unless
 * it's on. Below 640 px the Sort chip is its icon, still named.
 */
export function SearchChips({ facets, filters, sort, onToggle, onOpenFilters, onOpenSort }: SearchChipsProps) {
  const active = activeFilterCount(filters);
  return (
    <div className="ts-chips" role="group" aria-label="Filter and sort">
      <FilterChip
        pressed={false}
        count={active > 0 ? active : undefined}
        onOpenMenu={onOpenFilters}
        aria-label={active > 0 ? `Filters, ${active} on` : 'Filters'}
      >
        <SlidersHorizontal size={16} aria-hidden="true" />
        Filters
      </FilterChip>
      <FilterChip pressed={false} onOpenMenu={onOpenSort} className="ts-sort" aria-label={`Sort: ${SORT_LABEL[sort]}`}>
        <ArrowUpDown size={16} aria-hidden="true" />
        <span className="ts-sort-label">{SORT_LABEL[sort]}</span>
      </FilterChip>
      {quickChipStates(facets, filters).map((chip) => (
        <FilterChip
          key={`${chip.key}:${chip.value}`}
          className={cx(chip.wide && 'ts-wide')}
          pressed={chip.pressed}
          count={chip.count}
          disabled={chip.disabled}
          onPressedChange={() => onToggle(chip.key, chip.value)}
        >
          {chip.label}
        </FilterChip>
      ))}
    </div>
  );
}
