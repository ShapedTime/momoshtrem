import type { SearchScope, SearchSort, SearchTorrentsParams, ShowTitle, Title } from '@/lib/api/contract';
import { contextMediaType, type SourceContext } from '@/lib/source/context';
import { episodeCode } from '@/lib/title/episodes';
import type { SearchFilters } from './filters';

/** What the torrent search sends (PLAN §6 "Search") and shows before the server answers. */

/** Results per page: the backend's default. */
export const PAGE_SIZE = 25;

/**
 * A title as release names write it, like the backend's query plan
 * (`internal/search/plan.go`, queryTitle): marks dropped ("Shōgun" is
 * "Shogun"), "&" as "and", apostrophes gone ("Journey’s" is "Journeys"),
 * other punctuation a space; a hyphen inside a word stays ("Spider-Man").
 */
export function normaliseTitle(title: string): string {
  return title
    .normalize('NFD')
    .replace(/\p{Mn}/gu, '')
    .normalize('NFC')
    .replace(/&/g, ' and ')
    .replace(/['’‘`´]/g, '')
    .replace(/[^\p{L}\p{N}-]|(?<![\p{L}\p{N}])-|-(?![\p{L}\p{N}])/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

/** The part before a colon ("Frieren"), when it has at least three letters or digits ("Re:Zero" stays whole). */
export function shortTitle(title: string): string {
  const colon = title.indexOf(':');
  const before = colon < 0 ? '' : title.slice(0, colon);
  return normaliseTitle((before.match(/[\p{L}\p{N}]/gu)?.length ?? 0) >= 3 ? before : title);
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
 * An episode's absolute number: from its season's range when the library
 * numbers the show that way, else counted over TMDB's regular seasons (a
 * title outside the library). Null for specials and unknown seasons.
 */
export function absoluteNumber(show: Pick<ShowTitle, 'seasons'>, season: number, episode: number): number | null {
  const summary = show.seasons.find((s) => s.season_number === season);
  if (!summary || season === 0) return null;
  if (summary.absolute_from !== null) return summary.absolute_from + episode - 1;
  return show.seasons.filter((s) => s.season_number > 0 && s.season_number < season).reduce((sum, s) => sum + s.episodes_total, 0) + episode;
}

const pad = (n: number) => String(n).padStart(2, '0');

/**
 * The field's first frame, before the server answers: the plan's primary
 * query, so the two match. "Title Year" for a movie; for a show by scope,
 * "Title S01E29" (episode), "Title S01" (season) or "Title" (any). Anime uses
 * the short title and numbers an episode by its absolute number ("Frieren 29").
 */
export function defaultQuery(context: SourceContext, title: Title, scope: SearchScope | null = defaultScope(context)): string {
  if (context.kind === 'movie' || title.media_type === 'movie') {
    const name = normaliseTitle(title.title);
    return title.year ? `${name} ${title.year}` : name;
  }
  const name = title.is_anime ? shortTitle(title.title) : normaliseTitle(title.title);
  const { season, episode } = context;
  if (scope === 'episode' && season !== null && episode !== null) {
    const absolute = title.is_anime ? absoluteNumber(title, season, episode) : null;
    return `${name} ${absolute !== null ? pad(absolute) : episodeCode(season, episode)}`;
  }
  if (scope === 'season' && season !== null) return `${name} S${pad(season)}`;
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
