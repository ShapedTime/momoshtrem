'use client';

import { SlidersHorizontal, X } from 'lucide-react';
import { Button, FilterChip } from '@/components/ui';
import { type ActiveFilter, clearFilters, DECADES, decadeOf, type DiscoverFilters, filterBadgeCount, toggleDecade } from '@/lib/discover/filters';
import { genreBySlug } from '@/lib/discover/genres';
import { applyMood, MOODS, selectedMood } from '@/lib/discover/moods';
import { cx } from '@/lib/utils/cx';

interface ChipsProps {
  filters: DiscoverFilters;
  onChange: (next: DiscoverFilters) => void;
}

/**
 * The mood row: each mood fills in its preset, and shows as selected only
 * while the filters equal it. Tapping the selected one clears the filters.
 */
export function DiscoverMoods({ filters, onChange }: ChipsProps) {
  const selected = selectedMood(filters)?.id;
  return (
    <div className="disc-moods fchips fchips--scroll" role="group" aria-label="Moods">
      {MOODS.map((mood) => (
        <FilterChip
          key={mood.id}
          pressed={selected === mood.id}
          onPressedChange={() => onChange(selected === mood.id ? clearFilters(filters) : applyMood(filters, mood.id))}
        >
          {mood.label}
        </FilterChip>
      ))}
    </div>
  );
}

const TWENTIES = DECADES[0];

/**
 * Phone: Filters (with how many are on) opens the sheet; the quick chips and
 * the selected genres apply at once. The row scrolls sideways.
 */
export function DiscoverQuickChips({ filters, onChange, onOpenFilters }: ChipsProps & { onOpenFilters: () => void }) {
  const count = filterBadgeCount(filters);
  const set = (patch: Partial<DiscoverFilters>) => onChange({ ...filters, ...patch });
  return (
    <div className="disc-pchips fchips fchips--scroll" role="group" aria-label="Quick filters">
      <button type="button" className={cx('fchip disc-filters-btn', count > 0 && 'is-on')} aria-haspopup="dialog" onClick={onOpenFilters}>
        <SlidersHorizontal size={16} aria-hidden="true" />
        Filters
        {count > 0 && <span className="n">{count}</span>}
      </button>
      <FilterChip pressed={filters.type === 'tv'} onPressedChange={() => set({ type: filters.type === 'tv' ? 'all' : 'tv' })}>
        Shows
      </FilterChip>
      <FilterChip pressed={filters.type === 'movie'} onPressedChange={() => set({ type: filters.type === 'movie' ? 'all' : 'movie' })}>
        Movies
      </FilterChip>
      <FilterChip pressed={filters.anime} onPressedChange={(anime) => set({ anime })}>
        Anime
      </FilterChip>
      <FilterChip pressed={filters.rating === 8} onPressedChange={(on) => set({ rating: on ? 8 : null })}>
        Rated 8+
      </FilterChip>
      <FilterChip pressed={decadeOf(filters) === TWENTIES} onPressedChange={() => onChange(toggleDecade(filters, TWENTIES))}>
        {TWENTIES.label}
      </FilterChip>
      {filters.genres.map((slug) => (
        <FilterChip key={slug} pressed onRemove={() => set({ genres: filters.genres.filter((genre) => genre !== slug) })}>
          {genreBySlug(slug).label}
        </FilterChip>
      ))}
    </div>
  );
}

/** The active filters as removable chips, plus Clear all (which keeps the type and sort). */
export function DiscoverActiveFilters({ active, onRemove, onClearAll }: { active: ActiveFilter[]; onRemove: (filter: ActiveFilter) => void; onClearAll: () => void }) {
  if (!active.length) return null;
  return (
    <div className="disc-active fchips" role="group" aria-label="Active filters">
      {active.map((filter) => (
        <button key={filter.key} type="button" className="fchip is-on" aria-label={`Remove filter: ${filter.label}`} onClick={() => onRemove(filter)}>
          {filter.label}
          <span className="x" aria-hidden="true">
            <X size={12} strokeWidth={2.4} />
          </span>
        </button>
      ))}
      <Button variant="ghost" size="sm" onClick={onClearAll}>
        Clear all
      </Button>
    </div>
  );
}
