import type { MediaType, Timestamp } from './common';

/** Torrent search (B4; #6). */

export type PackType = 'episode' | 'multi_episode' | 'season' | 'multi_season' | 'complete';
export type SearchSort = 'best' | 'seeders' | 'size' | 'newest';
export type ReleaseFlag = 'cam' | 'no_seeders' | 'size_outlier' | 'needs_torrent_fetch';

/** GET /search/torrents query */
export interface SearchTorrentsParams {
  media_type?: MediaType;
  tmdb_id?: number;
  season?: number;
  episode?: number;
  q?: string;
  resolution?: string[];
  source?: string[];
  codec?: string[];
  hdr?: string[];
  pack?: PackType[];
  min_seeders?: number;
  max_size_gb?: number;
  indexer?: string[];
  sort?: SearchSort;
  page?: number;
  page_size?: number;
}

export interface ReleaseAttributes {
  resolution: string | null;
  source: string | null;
  codec: string | null;
  hdr: string[];
  audio: string[];
  group: string | null;
  languages: string[];
  edition: string | null;
  repack: boolean;
  pack: { type: PackType; seasons: number[]; episodes: number[] };
}

export interface Release {
  id: string;
  raw_title: string;
  display_title: string;
  info_hash: string | null;
  magnet_uri: string | null;
  size_bytes: number;
  seeders: number;
  leechers: number;
  published_at: Timestamp | null;
  indexer: string;
  attrs: ReleaseAttributes;
  score: number;
  score_reasons: string[];
  flags: ReleaseFlag[];
}

/** Facet counts under the current filters (HANDOFF). */
export type FacetCounts = Record<string, number>;

export interface IndexerReport {
  name: string;
  status: 'ok' | 'timeout' | 'error';
  count: number;
  ms: number;
}

/** GET /search/torrents */
export interface SearchTorrentsResponse {
  items: Release[];
  facets: { resolution: FacetCounts; source: FacetCounts; codec: FacetCounts; hdr: FacetCounts; pack: FacetCounts; indexer: FacetCounts };
  total: number;
  page: number;
  page_size: number;
  /** What was sent to Jackett. */
  queries: string[];
  /** Per-indexer outcome (HANDOFF). */
  indexers: IndexerReport[];
  cached_at: Timestamp;
  warnings: string[];
}

/** POST /search/torrents/inspect (202 → JobAccepted, a dry-run assignment job) */
export interface InspectRequest {
  release_id?: string;
  magnet_uri?: string;
  media_type: MediaType;
  tmdb_id: number;
}

/** GET/PUT /settings/quality-profile */
export interface QualityProfile {
  preferred_resolution: string;
  min_resolution: string;
  max_resolution: string;
  prefer_codecs: string[];
  prefer_hdr: boolean;
  max_gb_per_hour: number;
  avoid_terms: string[];
  preferred_groups: string[];
}
