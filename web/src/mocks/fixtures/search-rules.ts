import type {
  FacetBucket,
  IndexerReport,
  MediaType,
  MissingRun,
  QualityProfile,
  Release,
  ReleaseResolution,
  SearchFacets,
  SearchMissingRun,
  SearchQuery,
  SearchScope,
  SearchSort,
  SearchTorrentsParams,
  SearchTorrentsResponse,
  Timestamp,
} from '@/lib/api/contract';
import { FACET_KEYS, FACET_VALUES, type FacetKey, type SearchFilters } from '@/lib/search/filters';

/**
 * The backend's read side of GET /search/torrents (PLAN §6 "Search"), once and
 * generically, for every fixture: scope, the profile's hides, filters (OR
 * within one, AND across), disjunctive facets, sort, paging, `best_id` and
 * `missing`. The fixtures (search.ts) only say what the indexers found.
 */

/** In-season TMDB episodes, inclusive. */
export interface Coverage {
  season: number;
  from: number;
  to: number;
}

/** A release as an indexer found it, plus what the mock knows that the contract doesn't carry. */
export interface MockRelease {
  release: Release;
  /** The TMDB episodes it holds; empty for movies. */
  covers: Coverage[];
  /** `needs_torrent_fetch`: what resolve answers. */
  resolves?: { magnet_uri: string; info_hash: string };
}

// ---- Params ---------------------------------------------------------------------------

const SCOPES: readonly SearchScope[] = ['episode', 'season', 'any'];
const SORTS: readonly SearchSort[] = ['best', 'seeders', 'size', 'newest'];
const MAX_PAGE_SIZE = 100;

export interface ParsedSearch {
  params: SearchTorrentsParams;
  /** The scope in effect: the context's default when none was sent; null for movies. */
  scope: SearchScope | null;
  filters: SearchFilters;
  refresh: boolean;
}

/** Validates GET /search/torrents like the backend: a message for 400 `bad_request`, or the parsed request. */
export function parseSearchParams(query: URLSearchParams): ParsedSearch | { error: string } {
  const mediaType = query.get('media_type');
  const tmdbId = Number(query.get('tmdb_id'));
  if ((mediaType !== 'movie' && mediaType !== 'tv') || !Number.isInteger(tmdbId) || tmdbId <= 0) {
    return { error: 'Send media_type (movie or tv) and tmdb_id.' };
  }
  // Absent: undefined; malformed: NaN
  const number = (name: string, { integer = true, min = 0 } = {}): number | undefined => {
    const raw = query.get(name);
    if (raw === null || raw === '') return undefined;
    const n = Number(raw);
    return (integer ? Number.isInteger(n) : Number.isFinite(n)) && n >= min ? n : NaN;
  };
  const numbers = {
    season: number('season'),
    episode: number('episode', { min: 1 }),
    page: number('page', { min: 1 }),
    page_size: number('page_size', { min: 1 }),
    min_seeders: number('min_seeders'),
    min_size_gb: number('min_size_gb', { integer: false }),
    max_size_gb: number('max_size_gb', { integer: false }),
  };
  const malformed = Object.entries(numbers).find(([, value]) => Number.isNaN(value));
  if (malformed) return { error: `${malformed[0]} must be a number.` };

  const scopeParam = query.get('scope');
  const sortParam = query.get('sort') ?? 'best';
  if (scopeParam !== null && !SCOPES.includes(scopeParam as SearchScope)) return { error: `Unknown scope "${scopeParam}".` };
  if (!SORTS.includes(sortParam as SearchSort)) return { error: `Unknown sort "${sortParam}".` };
  const { season, episode, min_seeders, min_size_gb, max_size_gb } = numbers;
  let scope: SearchScope | null = null;
  if (mediaType === 'tv') {
    if (episode !== undefined && season === undefined) return { error: 'An episode needs a season.' };
    scope =(scopeParam as SearchScope | null) ?? (episode !== undefined && season !== undefined ? 'episode' : season !== undefined ? 'season' : 'any');
    if (scope === 'episode' && (episode === undefined || season === undefined)) return { error: 'scope=episode needs a season and an episode.' };
    if (scope === 'season' && season === undefined) return { error: 'scope=season needs a season.' };
  }

  const filters: SearchFilters = {};
  for (const key of FACET_KEYS) {
    const values = query.getAll(key);
    // Kept as sent: an unknown value matches nothing
    if (values.length > 0) (filters as Record<FacetKey, string[]>)[key] = values;
  }
  if (min_seeders !== undefined) filters.min_seeders = min_seeders;
  if (min_size_gb !== undefined) filters.min_size_gb = min_size_gb;
  if (max_size_gb !== undefined) filters.max_size_gb = max_size_gb;

  const q = query.get('q')?.trim() || undefined;
  const params: SearchTorrentsParams = {
    media_type: mediaType,
    tmdb_id: tmdbId,
    ...(season !== undefined ? { season } : {}),
    ...(episode !== undefined ? { episode } : {}),
    ...(scope ? { scope } : {}),
    ...(q ? { q } : {}),
    ...filters,
    sort: sortParam as SearchSort,
    page: numbers.page ?? 1,
    page_size: Math.min(numbers.page_size ?? 25, MAX_PAGE_SIZE),
  };
  return { params, scope, filters, refresh: query.get('refresh') === '1' };
}

/** The cache key of a search: filters, sort and paging aren't part of it. */
export const planKey = ({ media_type, tmdb_id, season, episode, scope, q }: SearchTorrentsParams) =>
  [media_type, tmdb_id, scope ?? '', season ?? '', episode ?? '', q ?? ''].join(':');

// ---- What a scope keeps -------------------------------------------------------------------

/** Episodes a release holds, at least one (a size per episode divides by it). */
export const episodesCovered = (entry: Pick<MockRelease, 'covers'>) =>
  Math.max(1, entry.covers.reduce((sum, range) => sum + range.to - range.from + 1, 0));

/**
 * episode: releases holding the episode, packs included; season: anything but
 * a single episode with at least one episode of the season; any: everything.
 */
export function inScope(entry: MockRelease, scope: SearchScope | null, season?: number, episode?: number): boolean {
  if (scope === null || scope === 'any') return true;
  if (scope === 'episode') return entry.covers.some((range) => range.season === season && episode !== undefined && range.from <= episode && episode <= range.to);
  return entry.release.attrs.pack?.type !== 'episode' && entry.covers.some((range) => range.season === season);
}

const tokens = (text: string) => text.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, ' ').trim().split(' ').filter(Boolean);

/** A typed `q` keeps the releases whose name holds every word of it. */
export function matchesQuery(release: Pick<Release, 'raw_title'>, q: string): boolean {
  const name = tokens(release.raw_title).join(' ');
  return tokens(q).every((word) => name.includes(word));
}

// ---- The quality profile ---------------------------------------------------------------

const RESOLUTION_RANK: Record<ReleaseResolution, number> = { '2160p': 5, '1080p': 4, '720p': 3, '576p': 2, '480p': 1, unknown: 0 };

const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** Hidden by the profile: an avoid term as a whole word (case-insensitive), or above `max_resolution`. */
export function profileHides(release: Pick<Release, 'raw_title' | 'attrs'>, profile: Pick<QualityProfile, 'avoid_terms' | 'max_resolution'>): boolean {
  const avoided = profile.avoid_terms.some((term) => new RegExp(`(^|[^a-z0-9])${escape(term)}([^a-z0-9]|$)`, 'i').test(release.raw_title));
  const tooBig = profile.max_resolution !== null && RESOLUTION_RANK[release.attrs.resolution] > RESOLUTION_RANK[profile.max_resolution];
  return avoided || tooBig;
}

// ---- Filters and facets ------------------------------------------------------------------

/** A release's values for a facet; `hdr` and `audio` have several, and an empty `hdr` is SDR. */
export function facetValues(release: Release, key: FacetKey): string[] {
  const { attrs } = release;
  switch (key) {
    case 'resolution':
    case 'source':
    case 'codec':
      return [attrs[key]];
    case 'hdr':
      return attrs.hdr.length > 0 ? attrs.hdr : ['SDR'];
    case 'audio':
      return attrs.audio;
    case 'pack':
      return attrs.pack ? [attrs.pack.type] : [];
    case 'indexer':
      return [release.indexer];
    case 'group':
      return attrs.group ? [attrs.group] : [];
  }
}

/** The size the size filters compare: per episode for TV, the whole release for movies, in GB. */
const sizeGb = (entry: MockRelease, mediaType: MediaType) => entry.release.size_bytes / (mediaType === 'tv' ? episodesCovered(entry) : 1) / 1e9;

/** Every filter holds (`skip`: leave one facet out, for its disjunctive counts). */
export function matchesFilters(entry: MockRelease, mediaType: MediaType, filters: SearchFilters, skip?: FacetKey): boolean {
  for (const key of FACET_KEYS) {
    const selected: readonly string[] | undefined = filters[key];
    if (key === skip || !selected?.length) continue;
    if (!facetValues(entry.release, key).some((value) => selected.includes(value))) return false;
  }
  if (filters.min_seeders !== undefined && entry.release.seeders < filters.min_seeders) return false;
  const size = sizeGb(entry, mediaType);
  if (filters.min_size_gb !== undefined && size < filters.min_size_gb) return false;
  if (filters.max_size_gb !== undefined && size > filters.max_size_gb) return false;
  return true;
}

const GROUP_LIMIT = 20;

/**
 * Disjunctive facets: each counted under every other active filter, so a count
 * is what selecting that value would give. Buckets hold every value in the
 * unfiltered scope plus any selected one, in allowed-value order (`indexer`
 * and `group` by count; `group` the top 20).
 */
export function searchFacets(entries: MockRelease[], mediaType: MediaType, filters: SearchFilters): SearchFacets {
  const facet = (key: FacetKey): FacetBucket[] => {
    const counts = new Map<string, number>();
    for (const entry of entries) {
      if (!matchesFilters(entry, mediaType, filters, key)) continue;
      for (const value of facetValues(entry.release, key)) counts.set(value, (counts.get(value) ?? 0) + 1);
    }
    const present = new Set([...entries.flatMap((entry) => facetValues(entry.release, key)), ...(filters[key] ?? [])]);
    const buckets = [...present].map((value) => ({ value, count: counts.get(value) ?? 0 }));
    if (key === 'indexer' || key === 'group') {
      buckets.sort((a, b) => b.count - a.count || a.value.localeCompare(b.value));
      if (key === 'indexer') return buckets;
      const selected = new Set<string>(filters.group ?? []);
      return buckets.filter((bucket, i) => i < GROUP_LIMIT || selected.has(bucket.value));
    }
    const order: readonly string[] = FACET_VALUES[key];
    return buckets.sort((a, b) => order.indexOf(a.value) - order.indexOf(b.value));
  };
  return {
    resolution: facet('resolution') as SearchFacets['resolution'],
    source: facet('source') as SearchFacets['source'],
    codec: facet('codec') as SearchFacets['codec'],
    hdr: facet('hdr') as SearchFacets['hdr'],
    audio: facet('audio') as SearchFacets['audio'],
    pack: facet('pack') as SearchFacets['pack'],
    indexer: facet('indexer'),
    group: facet('group'),
  };
}

// ---- Best match, sort and missing ----------------------------------------------------------

/** The top score among the filtered releases, flagged neither CAM nor seedless, scoring at least 50. */
export function bestReleaseId(releases: Release[]): string | null {
  const candidates = releases.filter((r) => r.score >= 50 && !r.flags.includes('cam') && !r.flags.includes('no_seeders'));
  candidates.sort((a, b) => b.score - a.score || b.seeders - a.seeders);
  return candidates[0]?.id ?? null;
}

const time = (timestamp: Timestamp | null) => (timestamp ? Date.parse(timestamp) : -Infinity);

/** best: by score, the best match first; seeders: most first; size: smallest first; newest: by date, unknown last. */
export function sortReleases(releases: Release[], sort: SearchSort, bestId: string | null): Release[] {
  const compare: Record<SearchSort, (a: Release, b: Release) => number> = {
    best: (a, b) => Number(b.id === bestId) - Number(a.id === bestId) || b.score - a.score || b.seeders - a.seeders,
    seeders: (a, b) => b.seeders - a.seeders || b.score - a.score,
    size: (a, b) => a.size_bytes - b.size_bytes,
    newest: (a, b) => time(b.published_at) - time(a.published_at),
  };
  return [...releases].sort(compare[sort]);
}

/**
 * The title's missing run when the context falls inside it (its season, and
 * its episode if any), for runs of more than one; `best_covers` when the best
 * match holds the whole run.
 */
export function missingFor(
  run: MissingRun | null,
  context: { season?: number; episode?: number },
  best: MockRelease | undefined
): SearchMissingRun | null {
  if (!run || run.count < 2 || context.season !== run.season_number) return null;
  if (context.episode !== undefined && (context.episode < run.from || context.episode > run.to)) return null;
  const covers = best?.covers.some((range) => range.season === run.season_number && range.from <= run.from && range.to >= run.to) ?? false;
  return { ...run, best_covers: covers };
}

// ---- The answer ---------------------------------------------------------------------------

export interface SearchAnswerInput {
  parsed: ParsedSearch;
  /** What the indexers found so far (the answered ones), before scope. */
  found: MockRelease[];
  profile: Pick<QualityProfile, 'avoid_terms' | 'max_resolution'>;
  queries: SearchQuery[];
  /** Each indexer's state; the counts come from what it found in scope. */
  indexers: Array<Omit<IndexerReport, 'count'>>;
  complete: boolean;
  missingRun: MissingRun | null;
  cachedAt: Timestamp;
  warnings?: string[];
}

/** A whole SearchTorrentsResponse for one request. */
export function searchAnswer({ parsed, found, profile, queries, indexers, complete, missingRun, cachedAt, warnings = [] }: SearchAnswerInput): SearchTorrentsResponse {
  const { params, scope, filters } = parsed;
  const scoped = found.filter((entry) => inScope(entry, scope, params.season, params.episode));
  const visible = scoped.filter((entry) => !profileHides(entry.release, profile));
  const filtered = visible.filter((entry) => matchesFilters(entry, params.media_type, filters));
  const bestId = bestReleaseId(filtered.map((entry) => entry.release));
  const sorted = sortReleases(
    filtered.map((entry) => entry.release),
    params.sort ?? 'best',
    bestId
  );
  const page = params.page ?? 1;
  const pageSize = params.page_size ?? 25;
  return {
    items: sorted.slice((page - 1) * pageSize, page * pageSize),
    total: filtered.length,
    page,
    page_size: pageSize,
    total_unfiltered: visible.length,
    hidden: scoped.length - visible.length,
    best_id: bestId,
    facets: searchFacets(visible, params.media_type, filters),
    queries,
    indexers: indexers.map((indexer) => ({
      ...indexer,
      count: indexer.status === 'ok' ? scoped.filter((entry) => entry.release.indexer === indexer.name).length : 0,
    })),
    complete,
    missing: params.media_type === 'tv' ? missingFor(missingRun, params, filtered.find((entry) => entry.release.id === bestId)) : null,
    cached_at: cachedAt,
    warnings,
  };
}
