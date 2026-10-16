import type {
  PackType,
  ReleaseAudio,
  ReleaseCodec,
  ReleaseHdr,
  ReleaseResolution,
  ReleaseSource,
  SearchFacets,
  SearchSort,
  SearchTorrentsParams,
} from '@/lib/api/contract';
import { sizeLabel } from './size';

/**
 * The filter part of a torrent search (PLAN §6 "Search"): repeatable values
 * (OR within one, AND across them), a seeders floor and a size range per
 * episode. The search sheet keeps them in its session draft; #93 edits them.
 */
export type SearchFilters = Pick<
  SearchTorrentsParams,
  'resolution' | 'source' | 'codec' | 'hdr' | 'audio' | 'pack' | 'indexer' | 'group' | 'min_seeders' | 'min_size_gb' | 'max_size_gb'
>;

export type FacetKey = keyof SearchFacets;

/** The repeatable filters, in the facets' order. */
export const FACET_KEYS: FacetKey[] = ['resolution', 'source', 'codec', 'hdr', 'audio', 'pack', 'indexer', 'group'];

/** Allowed values in facet order; `indexer` and `group` are open lists, ordered by count. */
export const FACET_VALUES: {
  resolution: ReleaseResolution[];
  source: ReleaseSource[];
  codec: ReleaseCodec[];
  hdr: ReleaseHdr[];
  audio: ReleaseAudio[];
  pack: PackType[];
} = {
  resolution: ['2160p', '1080p', '720p', '576p', '480p', 'unknown'],
  source: ['REMUX', 'BluRay', 'WEB-DL', 'WEBRip', 'WEB', 'HDTV', 'DVD', 'CAM', 'unknown'],
  codec: ['x265', 'x264', 'AV1', 'other', 'unknown'],
  hdr: ['DV', 'HDR10+', 'HDR10', 'HDR', 'SDR'],
  audio: ['Atmos', 'TrueHD', 'DTS-HD', 'DTS', 'DD+', 'DD', 'AAC', 'FLAC', 'Opus'],
  pack: ['episode', 'multi_episode', 'season', 'multi_season', 'complete', 'unknown'],
};

/** How many filters narrow the search (each selected value, the seeders floor and each size bound). */
export function activeFilterCount(filters: SearchFilters): number {
  const values = FACET_KEYS.reduce((sum, key) => sum + (filters[key]?.length ?? 0), 0);
  const numbers = [filters.min_seeders, filters.min_size_gb, filters.max_size_gb].filter((n) => n !== undefined).length;
  return values + numbers;
}

// ---- Values and their words ---------------------------------------------------------------

const selectedIn = (filters: SearchFilters, key: FacetKey): readonly string[] => filters[key] ?? [];

export const isSelected = (filters: SearchFilters, key: FacetKey, value: string): boolean => selectedIn(filters, key).includes(value);

/** Select a value, or drop it when it was selected; a facet with nothing left is dropped. */
export function toggleValue(filters: SearchFilters, key: FacetKey, value: string): SearchFilters {
  const current = selectedIn(filters, key);
  const values = current.includes(value) ? current.filter((v) => v !== value) : [...current, value];
  const next: Record<string, unknown> = { ...filters };
  if (values.length > 0) next[key] = values;
  else delete next[key];
  return next as SearchFilters;
}

/** The same filters, whatever the order they were picked in. */
export function sameFilters(a: SearchFilters, b: SearchFilters): boolean {
  const canonical = (filters: SearchFilters) =>
    JSON.stringify(
      Object.entries(filters)
        .filter(([, value]) => value !== undefined && !(Array.isArray(value) && value.length === 0))
        .map(([key, value]) => [key, Array.isArray(value) ? [...value].sort() : value])
        .sort(([x], [y]) => String(x).localeCompare(String(y)))
    );
  return canonical(a) === canonical(b);
}

/** The Filters sheet's groups, in order, with their legends. Pack type is TV only. */
export const FACET_LEGEND: Record<FacetKey, string> = {
  resolution: 'Resolution',
  source: 'Source',
  codec: 'Codec',
  hdr: 'HDR / Dolby Vision',
  audio: 'Audio',
  pack: 'Pack type',
  indexer: 'Indexer',
  group: 'Release group',
};

/** Values that read differently from the contract's; the rest (2160p, WEB-DL, Atmos, an indexer) read as they are. */
const VALUE_LABEL: Partial<Record<FacetKey, Record<string, string>>> = {
  resolution: { unknown: 'Unknown' },
  source: { unknown: 'Unknown' },
  codec: { x265: 'x265 / HEVC', x264: 'x264 / AVC', other: 'Other', unknown: 'Unknown' },
  hdr: { DV: 'Dolby Vision' },
  pack: { episode: 'Episode', multi_episode: 'Episode range', season: 'Season', multi_season: 'Seasons', complete: 'Complete', unknown: 'Unknown' },
};

/** A value's chip in the Filters sheet, under its group's legend: "Dolby Vision", "Episode range", "Unknown". */
export const valueLabel = (key: FacetKey, value: string): string => VALUE_LABEL[key]?.[value] ?? value;

/** Out of its group (filtered-empty), a value says what it is: "Season packs", "Unknown codec". */
const STANDALONE_LABEL: Partial<Record<FacetKey, Record<string, string>>> = {
  codec: { x265: 'x265', x264: 'x264', other: 'Other codec' },
  pack: {
    episode: 'Single episodes',
    multi_episode: 'Episode ranges',
    season: 'Season packs',
    multi_season: 'Multi-season packs',
    complete: 'Complete series',
    unknown: 'Unknown pack',
  },
};

function standaloneLabel(key: FacetKey, value: string): string {
  const own = STANDALONE_LABEL[key]?.[value];
  if (own) return own;
  return value === 'unknown' ? `Unknown ${key}` : valueLabel(key, value);
}

/** One active filter: a facet's value, the seeders floor or a size bound. */
export type ActiveFilter =
  | { key: FacetKey; value: string; label: string }
  | { key: 'min_seeders' | 'min_size_gb' | 'max_size_gb'; label: string };

/** Every active filter as a removable chip, in the sheet's order: "2160p", "Dolby Vision", "20+ seeders", "Up to 4 GB". */
export function activeFilters(filters: SearchFilters): ActiveFilter[] {
  const chips: ActiveFilter[] = [];
  for (const key of FACET_KEYS) for (const value of selectedIn(filters, key)) chips.push({ key, value, label: standaloneLabel(key, value) });
  if (filters.min_size_gb !== undefined) chips.push({ key: 'min_size_gb', label: `From ${sizeLabel(filters.min_size_gb)}` });
  if (filters.max_size_gb !== undefined) chips.push({ key: 'max_size_gb', label: `Up to ${sizeLabel(filters.max_size_gb)}` });
  if (filters.min_seeders !== undefined) chips.push({ key: 'min_seeders', label: `${filters.min_seeders}+ seeders` });
  return chips;
}

/** The filters without one of them. */
export function removeFilter(filters: SearchFilters, chip: ActiveFilter): SearchFilters {
  if ('value' in chip) return toggleValue(filters, chip.key, chip.value);
  const next = { ...filters };
  delete next[chip.key];
  return next;
}

// ---- The head's chips ------------------------------------------------------------------------

type QuickKey = Extract<FacetKey, 'resolution' | 'codec' | 'hdr'>;

interface QuickChip {
  key: QuickKey;
  value: string;
  label: string;
  /** Only from 640 px, as in the prototype. */
  wide: boolean;
}

/** One tap each under the search: resolution, then (on wider screens) codec and Dolby Vision. */
export const QUICK_CHIPS: readonly QuickChip[] = [
  { key: 'resolution', value: '2160p', label: '2160p', wide: false },
  { key: 'resolution', value: '1080p', label: '1080p', wide: false },
  { key: 'codec', value: 'x265', label: 'x265', wide: true },
  { key: 'hdr', value: 'DV', label: 'Dolby Vision', wide: true },
];

export interface QuickChipState extends QuickChip {
  /** What selecting it would give (disjunctive); undefined before the first answer. */
  count: number | undefined;
  pressed: boolean;
  /** Nothing to find with it, and it isn't on. */
  disabled: boolean;
}

/** A facet value's count; 0 when the facet doesn't list it. */
export const facetCount = (facets: SearchFacets, key: FacetKey, value: string): number =>
  (facets[key] as Array<{ value: string; count: number }>).find((bucket) => bucket.value === value)?.count ?? 0;

/** The quick chips with their counts from the answer's facets. */
export function quickChipStates(facets: SearchFacets | undefined, filters: SearchFilters): QuickChipState[] {
  return QUICK_CHIPS.map((chip) => {
    const count = facets ? facetCount(facets, chip.key, chip.value) : undefined;
    const pressed = isSelected(filters, chip.key, chip.value);
    return { ...chip, count, pressed, disabled: count === 0 && !pressed };
  });
}

// ---- Sort and seeders -------------------------------------------------------------------------

/** The Sort chip's word. */
export const SORT_LABEL: Record<SearchSort, string> = { best: 'Best', seeders: 'Seeders', size: 'Size', newest: 'Newest' };

/** The Sort sheet's cards, in order. */
export const SORT_OPTIONS: ReadonlyArray<{ value: SearchSort; title: string; description: string }> = [
  { value: 'best', title: 'Best match', description: 'Quality profile, health and size together' },
  { value: 'seeders', title: 'Seeders', description: 'Fastest to start' },
  { value: 'size', title: 'Size', description: 'Smallest first' },
  { value: 'newest', title: 'Newest', description: 'Most recently published' },
];

/** Minimum seeders, as the Filters sheet offers it: Any, 5+, 20+, 100+. */
export const SEEDER_FLOORS: readonly number[] = [0, 5, 20, 100];

/** A floor of 0 is no floor. */
export function withMinSeeders(filters: SearchFilters, floor: number): SearchFilters {
  const next = { ...filters };
  if (floor > 0) next.min_seeders = floor;
  else delete next.min_seeders;
  return next;
}
