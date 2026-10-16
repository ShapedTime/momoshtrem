'use client';

import { useState } from 'react';
import { Button, FilterChip, Range, SegmentedControl, Sheet } from '@/components/ui';
import { useFilterCount } from '@/hooks/useReleaseSearch';
import type { MediaType, SearchFacets, SearchTorrentsParams, SearchTorrentsResponse } from '@/lib/api/contract';
import { showResultsLabel } from '@/lib/search/copy';
import {
  FACET_KEYS,
  FACET_LEGEND,
  type FacetKey,
  isSelected,
  SEEDER_FLOORS,
  type SearchFilters,
  toggleValue,
  valueLabel,
  withMinSeeders,
} from '@/lib/search/filters';
import { rangeText, SIZE_SCALE, SIZE_STEPS, sizeRange, withSizeRange } from '@/lib/search/size';

/** Facets before the size and seeders, and after them. */
const BEFORE: FacetKey[] = ['resolution', 'source', 'codec', 'hdr', 'audio', 'pack'];
const AFTER: FacetKey[] = FACET_KEYS.filter((key) => !BEFORE.includes(key));

function FacetGroup({ facet, facets, draft, onToggle }: { facet: FacetKey; facets: SearchFacets | undefined; draft: SearchFilters; onToggle: (key: FacetKey, value: string) => void }) {
  const buckets: Array<{ value: string; count: number }> = facets?.[facet] ?? [];
  if (buckets.length === 0) return null;
  return (
    <fieldset className="ts-fgroup">
      <legend className="label">{FACET_LEGEND[facet]}</legend>
      <div className="fchips">
        {buckets.map(({ value, count }) => {
          const pressed = isSelected(draft, facet, value);
          return (
            <FilterChip key={value} pressed={pressed} count={count} disabled={count === 0 && !pressed} onPressedChange={() => onToggle(facet, value)}>
              {valueLabel(facet, value)}
            </FilterChip>
          );
        })}
      </div>
    </fieldset>
  );
}

interface FiltersSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mediaType: MediaType;
  /** What's applied now; the sheet starts its draft from it. */
  filters: SearchFilters;
  /** The search's params with other filters, for "Show N results". */
  paramsFor: (filters: SearchFilters) => SearchTorrentsParams;
  /** The results' latest page 1. */
  current: SearchTorrentsResponse | undefined;
  onApply: (filters: SearchFilters) => void;
}

/**
 * Every filter (HANDOFF §5.2), nested over the search: each facet with what
 * it would give, the size per episode (a movie's whole size) on a log scale,
 * a seeders floor, then indexers and release groups. It edits a draft: the
 * foot says how many results the draft finds and applies it; Cancel, Esc and
 * the close button leave the search as it was. Reset clears the draft.
 * Mount it again (a new `key`) each time it opens, so the draft starts over.
 */
export function FiltersSheet({ open, onOpenChange, mediaType, filters, paramsFor, current, onApply }: FiltersSheetProps) {
  const [draft, setDraft] = useState<SearchFilters>(filters);
  const { total, facets, counting } = useFilterCount({ paramsFor, applied: filters, draft, current, enabled: open });
  const toggle = (key: FacetKey, value: string) => setDraft((d) => toggleValue(d, key, value));
  const scale = SIZE_SCALE[mediaType];
  const group = (facet: FacetKey) => <FacetGroup key={facet} facet={facet} facets={facets} draft={draft} onToggle={toggle} />;

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      nested
      className="ts-filters"
      title="Filters"
      titleAction={
        <Button variant="ghost" size="sm" onClick={() => setDraft({})}>
          Reset
        </Button>
      }
      footer={
        <>
          <Button onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button variant="primary" className="grow" aria-busy={counting} onClick={() => onApply(draft)}>
            {showResultsLabel(total)}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-6">
        {BEFORE.filter((facet) => facet !== 'pack' || mediaType === 'tv').map(group)}
        <fieldset className="ts-fgroup">
          <legend className="label">{mediaType === 'tv' ? 'Size per episode' : 'Size'}</legend>
          <Range
            min={0}
            max={SIZE_STEPS}
            value={sizeRange(draft, scale)}
            onValueChange={(range) => setDraft((d) => withSizeRange(d, range, scale))}
            labels={['Minimum size', 'Maximum size']}
            valueText={(position, thumb) => rangeText(position, thumb, scale)}
          />
        </fieldset>
        <fieldset className="ts-fgroup">
          <legend className="label">Minimum seeders</legend>
          <SegmentedControl
            block
            aria-label="Minimum seeders"
            options={SEEDER_FLOORS.map((floor) => ({ value: String(floor), label: floor === 0 ? 'Any' : `${floor}+` }))}
            value={String(draft.min_seeders ?? 0)}
            onChange={(value) => setDraft((d) => withMinSeeders(d, Number(value)))}
          />
        </fieldset>
        {AFTER.map(group)}
      </div>
    </Sheet>
  );
}
