import type {
  PackType,
  ReleaseAudio,
  ReleaseCodec,
  ReleaseHdr,
  ReleaseResolution,
  ReleaseSource,
  SearchFacets,
  SearchTorrentsParams,
} from '@/lib/api/contract';

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
