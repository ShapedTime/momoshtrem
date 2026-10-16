import type { SearchScope, SearchSort, SearchTorrentsParams, ShowTitle, Title } from '@/lib/api/contract';
import { contextMediaType, type SourceContext } from '@/lib/source/context';
import { episodeCode } from '@/lib/title/episodes';
import type { SearchFilters } from './filters';

/** What the torrent search sends (PLAN §6 "Search") and shows before the server answers. */

/** Results per page: the backend's default. */
export const PAGE_SIZE = 25;

/**
 * "Frieren: Beyond Journey’s End" → "Frieren Beyond Journeys End": release
 * names drop punctuation, so the query does too (the plan strips `: ' ! ,`).
 */
export function normaliseTitle(title: string): string {
  return title
    .replace(/\s+[-–—]\s+/g, ' ')
    .replace(/[:'’‘!,?."“”]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

/** The part before a colon or dash: "Frieren", "Dune". Release names often use only that. */
export function shortTitle(title: string): string {
  return normaliseTitle(title.split(/\s*:\s*|\s+[-–—]\s+/)[0]);
}

/** The scope a context starts on: an episode's, a season's packs, or anything for a whole show. Null for movies. */
export function defaultScope(context: SourceContext): SearchScope | null {
  if (context.kind === 'movie') return null;
  if (context.season !== null && context.episode !== null) return 'episode';
  return context.season !== null ? 'season' : 'any';
}

/** The scope switch's options: Episode · Season pack · Any for an episode, Season pack · Any for a season, none otherwise. */
export function scopeOptions(context: SourceContext): SearchScope[] {
  const scope = defaultScope(context);
  if (scope === 'episode') return ['episode', 'season', 'any'];
  if (scope === 'season') return ['season', 'any'];
  return [];
}

export const SCOPE_LABEL: Record<SearchScope, string> = { episode: 'Episode', season: 'Season pack', any: 'Any' };

/**
 * An episode's absolute number from its season's range, when the library
 * numbers the show that way (anime); null otherwise.
 */
export function absoluteNumber(show: Pick<ShowTitle, 'seasons'>, season: number, episode: number): number | null {
  const summary = show.seasons.find((s) => s.season_number === season);
  return summary?.absolute_from != null ? summary.absolute_from + episode - 1 : null;
}

/**
 * The field's first frame, before the server answers with its own primary
 * query: "Title Year" for a movie, "Title S01E29" for an episode (anime: the
 * short title and the absolute number, "Frieren 29"), "Title S01" for a
 * season, else the title.
 */
export function defaultQuery(context: SourceContext, title: Title): string {
  const name = normaliseTitle(title.title);
  if (context.kind === 'movie' || title.media_type === 'movie') return title.year ? `${name} ${title.year}` : name;
  const { season, episode } = context;
  if (season !== null && episode !== null) {
    const absolute = title.is_anime ? absoluteNumber(title, season, episode) : null;
    return absolute !== null ? `${shortTitle(title.title)} ${absolute}` : `${name} ${episodeCode(season, episode)}`;
  }
  if (season !== null) return `${name} S${String(season).padStart(2, '0')}`;
  return name;
}

interface SearchState {
  scope: SearchScope | null;
  sort: SearchSort;
  filters: SearchFilters;
  /** The text to send as `q`: only what the user typed and submitted; empty sends none. */
  q: string;
}

/** The GET /search/torrents params for a context and the sheet's state (page 1; paging adds `page`). */
export function searchParams(context: SourceContext, { scope, sort, filters, q }: SearchState): SearchTorrentsParams {
  const params: SearchTorrentsParams = { media_type: contextMediaType(context), tmdb_id: context.tmdbId };
  if (context.kind === 'show') {
    if (context.season !== null) params.season = context.season;
    if (context.season !== null && context.episode !== null) params.episode = context.episode;
    if (scope) params.scope = scope;
  }
  if (q.trim()) params.q = q.trim();
  return { ...params, ...filters, sort, page_size: PAGE_SIZE };
}

/**
 * Whether two searches run the same plan (the backend caches per media
 * type, title, scope, season, episode and `q`): then only filters, sort or
 * paging differ, and the old results can stay up while the new ones load.
 */
export function samePlan(a: SearchTorrentsParams, b: SearchTorrentsParams): boolean {
  return (
    a.media_type === b.media_type &&
    a.tmdb_id === b.tmdb_id &&
    a.season === b.season &&
    a.episode === b.episode &&
    a.scope === b.scope &&
    (a.q ?? '') === (b.q ?? '')
  );
}
