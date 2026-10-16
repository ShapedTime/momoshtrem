import type { MediaType, Paginated, Timestamp } from './common';
import type { AssignHint } from './assignment';
import type { MissingRun } from './titles';

/** Torrent search and the quality profile (PLAN §6 "Search"; #6). */

// Allowed values: each is both a facet's values and its filter's values, in facet order.

export type ReleaseResolution = '2160p' | '1080p' | '720p' | '576p' | '480p' | 'unknown';
/** CAM also covers TS, HDTS, TELESYNC, TC and SCR. */
export type ReleaseSource = 'REMUX' | 'BluRay' | 'WEB-DL' | 'WEBRip' | 'WEB' | 'HDTV' | 'DVD' | 'CAM' | 'unknown';
/** x265 covers HEVC and H.265, x264 covers AVC and H.264. */
export type ReleaseCodec = 'x265' | 'x264' | 'AV1' | 'other' | 'unknown';
/** The facet's and filter's SDR also covers releases whose `hdr` is empty. */
export type ReleaseHdr = 'DV' | 'HDR10+' | 'HDR10' | 'HDR' | 'SDR';
export type ReleaseAudio = 'Atmos' | 'TrueHD' | 'DTS-HD' | 'DTS' | 'DD+' | 'DD' | 'AAC' | 'FLAC' | 'Opus';
export type ReleaseChannels = '7.1' | '5.1' | '2.0';
export type PackType = 'episode' | 'multi_episode' | 'season' | 'multi_season' | 'complete' | 'unknown';

/** TV only. Defaults from the context: `episode` with an episode, `season` with only a season, else `any`. */
export type SearchScope = 'episode' | 'season' | 'any';
/** `seeders` most first, `size` smallest first, `newest` by `published_at`. */
export type SearchSort = 'best' | 'seeders' | 'size' | 'newest';
/** `needs_torrent_fetch`: no magnet yet, so Add and Copy magnet resolve first. */
export type ReleaseFlag = 'cam' | 'no_seeders' | 'low_seeders' | 'size_outlier' | 'over_size_limit' | 'needs_torrent_fetch';

/** GET /search/torrents query. Filters repeat: OR within one, AND across them; an unknown value matches nothing. */
export interface SearchTorrentsParams {
  media_type: MediaType;
  tmdb_id: number;
  season?: number;
  episode?: number;
  scope?: SearchScope;
  /** Only when the user edited the field: replaces the generated text queries, and ID queries are skipped. */
  q?: string;
  resolution?: ReleaseResolution[];
  source?: ReleaseSource[];
  codec?: ReleaseCodec[];
  hdr?: ReleaseHdr[];
  audio?: ReleaseAudio[];
  pack?: PackType[];
  indexer?: string[];
  group?: string[];
  min_seeders?: number;
  /** Per episode for TV (size ÷ episodes covered), the whole release for movies. */
  min_size_gb?: number;
  max_size_gb?: number;
  /** Default `best`. */
  sort?: SearchSort;
  page?: number;
  /** Default 25, max 100. */
  page_size?: number;
  /** Retry: re-runs the search, at most once per 10 s. Polling leaves it out. */
  refresh?: 1;
}

/** A release's numbering as its name gives it, not TMDB's. */
export interface ReleasePack {
  type: PackType;
  /** e.g. "S01E29", "Season 1", "S01–S04". */
  label: string;
  seasons: number[];
  /** Null for whole seasons. */
  episodes: { from: number; to: number } | null;
}

export interface ReleaseAttributes {
  resolution: ReleaseResolution;
  source: ReleaseSource;
  codec: ReleaseCodec;
  bit_depth: 10 | 12 | null;
  /** Only what the name says. */
  hdr: ReleaseHdr[];
  audio: ReleaseAudio[];
  channels: ReleaseChannels | null;
  /** "multi", "dual", or ISO 639-1 codes. */
  languages: string[];
  subs: 'multi' | 'hardcoded' | null;
  group: string | null;
  edition: string | null;
  /** REPACK or PROPER. */
  repack: boolean;
  /** Null for movies. */
  pack: ReleasePack | null;
}

export interface Release {
  /** Stable per indexer result across searches; resolve and inspect accept it for 1 h. */
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
  /** 0–100. */
  score: number;
  /** Positive only, most important first; the UI shows at most 4. */
  score_reasons: string[];
  /** e.g. 'Scene "Season 2" = TMDB S01E29–E38'. */
  notes: string[];
  /** e.g. "Only 4 seeders · may be slow". */
  warnings: string[];
  flags: ReleaseFlag[];
}

/** What selecting `value` would give, under every other active filter. */
export interface FacetBucket<T extends string = string> {
  value: T;
  count: number;
}

/** Disjunctive facets: every value in the unfiltered scope plus any selected one, in allowed-value order (`indexer` and `group` by count). */
export interface SearchFacets {
  resolution: FacetBucket<ReleaseResolution>[];
  source: FacetBucket<ReleaseSource>[];
  codec: FacetBucket<ReleaseCodec>[];
  hdr: FacetBucket<ReleaseHdr>[];
  audio: FacetBucket<ReleaseAudio>[];
  /** Empty for movies. */
  pack: FacetBucket<PackType>[];
  indexer: FacetBucket[];
  /** Top 20. */
  group: FacetBucket[];
}

/** Null for the primary query; `whole show` is the fallback when the exact queries found nothing. */
export type SearchQueryLabel = 'original title' | 'absolute number' | 'scene numbering' | 'whole show';

/** A text query sent to the indexers. */
export interface SearchQuery {
  q: string;
  label: SearchQueryLabel | null;
}

/** One indexer over all the plan's queries: `pending` while any request runs, `ok` when every one answered. */
export interface IndexerReport {
  name: string;
  status: 'pending' | 'ok' | 'timeout' | 'error';
  count: number;
  /** Time so far while pending. */
  ms: number;
  error?: string;
}

/** The title's `library.missing` run, when the context falls inside it. */
export interface SearchMissingRun extends MissingRun {
  /** The best match covers the whole run. */
  best_covers: boolean;
}

/** GET /search/torrents. Poll the same URL every second while `complete` is false. */
export interface SearchTorrentsResponse extends Paginated<Release> {
  /** In scope, after the profile hides and before filters. */
  total_unfiltered: number;
  /** Hidden by the profile (avoid terms, above max_resolution). */
  hidden: number;
  /** `sort=best` puts it first on page 1. */
  best_id: string | null;
  facets: SearchFacets;
  /** Primary first: the field shows `queries[0].q`. */
  queries: SearchQuery[];
  indexers: IndexerReport[];
  complete: boolean;
  missing: SearchMissingRun | null;
  /** When the search started. */
  cached_at: Timestamp;
  /** Problems that aren't about one indexer. */
  warnings: string[];
}

/** POST /search/torrents/resolve: before Add or Copy magnet when a release has no magnet. */
export interface ResolveRequest {
  release_id: string;
}

export interface ResolveResponse {
  magnet_uri: string;
  info_hash: string;
}

/**
 * POST /search/torrents/inspect → 202 JobAccepted (200 when inspected in the
 * last hour): a read-only dry-run assignment job. For a title not in the
 * library, `item.id` is null and targets carry TMDB episode ids.
 */
export type InspectRequest = {
  media_type: MediaType;
  tmdb_id: number;
  hint?: Pick<AssignHint, 'season' | 'episode'>;
} & ({ release_id: string } | { magnet_uri: string });

/** GET/PUT /settings/quality-profile. PUT takes the whole profile; 422 names a bad field. */
export interface QualityProfile {
  /** Default "1080p". */
  preferred_resolution: '720p' | '1080p' | '2160p';
  /** Null: no limit (the default). At least `preferred_resolution`; above it is hidden. */
  max_resolution: '1080p' | '2160p' | null;
  /** 1–20, step 0.5; default 8. Soft: a warning and a penalty, never hidden. */
  max_gb_per_hour: number;
  prefer_x265: boolean;
  prefer_hdr: boolean;
  prefer_dolby_vision: boolean;
  /** ≤ 30 terms of 1–30 characters; a whole-word, case-insensitive match hides the release. */
  avoid_terms: string[];
  /** ≤ 30; a small boost that never overrides seeders or the size limit. */
  preferred_groups: string[];
}
