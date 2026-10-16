import type { SearchFacets } from '@/lib/api/contract';
import {
  activeFilterCount,
  activeFilters,
  quickChipStates,
  removeFilter,
  sameFilters,
  toggleValue,
  valueLabel,
  withMinSeeders,
} from '@/lib/search/filters';

const facets = (patch: Partial<SearchFacets> = {}): SearchFacets => ({
  resolution: [
    { value: '2160p', count: 4 },
    { value: '1080p', count: 31 },
  ],
  source: [],
  codec: [{ value: 'x265', count: 16 }],
  hdr: [{ value: 'SDR', count: 40 }],
  audio: [],
  pack: [],
  indexer: [],
  group: [],
  ...patch,
});

describe('toggleValue', () => {
  it('adds a value, and drops it again with its facet once empty', () => {
    const on = toggleValue({}, 'resolution', '2160p');
    expect(on).toEqual({ resolution: ['2160p'] });
    expect(toggleValue(on, 'resolution', '1080p')).toEqual({ resolution: ['2160p', '1080p'] });
    expect(toggleValue(on, 'resolution', '2160p')).toEqual({});
  });

  it('leaves the other filters alone', () => {
    expect(toggleValue({ min_seeders: 5, hdr: ['DV'] }, 'hdr', 'DV')).toEqual({ min_seeders: 5 });
  });
});

describe('sameFilters', () => {
  it('ignores the order values were picked in and empty facets', () => {
    expect(sameFilters({ resolution: ['2160p', '1080p'], min_seeders: 5 }, { min_seeders: 5, resolution: ['1080p', '2160p'], hdr: [] })).toBe(true);
    expect(sameFilters({ resolution: ['2160p'] }, { resolution: ['1080p'] })).toBe(false);
    expect(sameFilters({}, { max_size_gb: 4 })).toBe(false);
  });
});

describe('labels', () => {
  it('reads values in the sheet, under their group', () => {
    expect(valueLabel('hdr', 'DV')).toBe('Dolby Vision');
    expect(valueLabel('codec', 'x265')).toBe('x265 / HEVC');
    expect(valueLabel('pack', 'multi_episode')).toBe('Episode range');
    expect(valueLabel('resolution', 'unknown')).toBe('Unknown');
    expect(valueLabel('indexer', 'Nyaa')).toBe('Nyaa');
  });

  it('names every active filter on its own, in the sheet’s order', () => {
    const chips = activeFilters({ hdr: ['DV'], resolution: ['2160p'], codec: ['unknown'], pack: ['season'], min_seeders: 20, max_size_gb: 4, min_size_gb: 0.2 });
    expect(chips.map((chip) => chip.label)).toEqual(['2160p', 'Unknown codec', 'Dolby Vision', 'Season packs', 'From 200 MB', 'Up to 4 GB', '20+ seeders']);
    expect(activeFilterCount({ hdr: ['DV'], resolution: ['2160p'], codec: ['unknown'], pack: ['season'], min_seeders: 20, max_size_gb: 4, min_size_gb: 0.2 })).toBe(chips.length);
  });

  it('removes one chip’s filter', () => {
    const filters = { resolution: ['2160p' as const], hdr: ['DV' as const], min_seeders: 20 };
    const [resolution, hdr, seeders] = activeFilters(filters);
    expect(removeFilter(filters, resolution)).toEqual({ hdr: ['DV'], min_seeders: 20 });
    expect(removeFilter(filters, hdr)).toEqual({ resolution: ['2160p'], min_seeders: 20 });
    expect(removeFilter(filters, seeders)).toEqual({ resolution: ['2160p'], hdr: ['DV'] });
  });
});

describe('quickChipStates', () => {
  it('counts each chip from the facets; one that would find nothing is disabled unless it’s on', () => {
    const chips = quickChipStates(facets(), { hdr: ['DV'] });
    expect(chips.map(({ label, count, pressed, disabled, wide }) => ({ label, count, pressed, disabled, wide }))).toEqual([
      { label: '2160p', count: 4, pressed: false, disabled: false, wide: false },
      { label: '1080p', count: 31, pressed: false, disabled: false, wide: false },
      { label: 'x265', count: 16, pressed: false, disabled: false, wide: true },
      // Not in the facets: 0, but on, so it can be turned off
      { label: 'Dolby Vision', count: 0, pressed: true, disabled: false, wide: true },
    ]);
    expect(quickChipStates(facets(), {}).at(-1)).toMatchObject({ count: 0, pressed: false, disabled: true });
  });

  it('has no counts and disables nothing before the first answer', () => {
    expect(quickChipStates(undefined, {}).every((chip) => chip.count === undefined && !chip.disabled)).toBe(true);
  });
});

describe('withMinSeeders', () => {
  it('sets a floor, and Any drops it', () => {
    expect(withMinSeeders({}, 20)).toEqual({ min_seeders: 20 });
    expect(withMinSeeders({ min_seeders: 20, resolution: ['1080p'] }, 0)).toEqual({ resolution: ['1080p'] });
  });
});
