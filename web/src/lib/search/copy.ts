import { isApiError } from '@/lib/api/client';
import type {
  IndexerReport,
  SearchMissingRun,
  SearchQuery,
  SearchQueryLabel,
  SearchScope,
  SearchSort,
  SearchTorrentsResponse,
  Title,
} from '@/lib/api/contract';
import { shorterQuery } from '@/lib/hdrezka/copy';
import { formatNumber, languageName, plural } from '@/lib/title/format';
import { shortTitle } from './query';

/** The torrent search sheet's words (HANDOFF §5.2): status lines, banners, empty states and labels. */

/** Names while they fit on a line, else a count: "Nyaa, TorrentLeech, The Pirate Bay" / "5 indexers". */
export function indexerList(names: string[]): string {
  const joined = names.join(', ');
  return names.length <= 3 && joined.length <= 40 ? joined : plural(names.length, 'indexer');
}

/** "27 results · Nyaa, TorrentLeech, The Pirate Bay": what's in view and the indexers that answered. */
export function statusLine(response: Pick<SearchTorrentsResponse, 'total' | 'indexers' | 'complete'>): string {
  const count = `${plural(response.total, 'result')}${response.complete ? '' : ' so far'}`;
  const answered = response.indexers.filter((indexer) => indexer.status === 'ok').map((indexer) => indexer.name);
  return answered.length > 0 ? `${count} · ${indexerList(answered)}` : count;
}

/** Labels that explain a query; the others ("original title", "absolute number") read plainly from the query. */
const EXPLAINED: ReadonlySet<SearchQueryLabel> = new Set(['scene numbering', 'whole show']);

/** "Also searched: Sousou no Frieren 29 · Frieren S02E01 (scene numbering)", or null with one query. */
export function alsoSearched(queries: SearchQuery[]): string | null {
  const extra = queries.slice(1).map(({ q, label }) => (label && EXPLAINED.has(label) ? `${q} (${label})` : q));
  return extra.length > 0 ? `Also searched: ${extra.join(' · ')}` : null;
}

const seconds = (ms: number) => `${Math.max(1, Math.round(ms / 1000))} s`;

/** "A", "A and B", "A, B and C". */
const prose = (names: string[]) => (names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`);

/** "Searching 3 indexers…" (or just "Searching indexers…" before the first answer). */
export const searchingTitle = (indexers: IndexerReport[]) =>
  indexers.length > 0 ? `Searching ${plural(indexers.length, 'indexer')}…` : 'Searching indexers…';

/** "Frieren 29 · also Sousou no Frieren 29". */
export function searchingDetail(q: string, queries: SearchQuery[]): string {
  const also = queries.find((query, i) => i > 0 && query.q !== q)?.q;
  return also ? `${q} · also ${also}` : q;
}

/** One indexer in the searching list: "26 results", "waiting · 6 s", "timed out after 8 s", or its error. */
export function indexerProgress(report: IndexerReport): string {
  switch (report.status) {
    case 'ok':
      return plural(report.count, 'result');
    case 'pending':
      return `waiting · ${seconds(report.ms)}`;
    case 'timeout':
      return `timed out after ${seconds(report.ms)}`;
    case 'error':
      return report.error ?? 'failed';
  }
}

/**
 * The banner when indexers timed out or failed: "2 of 3 indexers answered" /
 * "The Pirate Bay timed out after 8 s. Results may be incomplete." Null when
 * every indexer answered (or some are still pending).
 */
export function partialProblem(indexers: IndexerReport[]): { title: string; body: string; none: boolean } | null {
  if (indexers.some((indexer) => indexer.status === 'pending')) return null;
  const timeouts = indexers.filter((indexer) => indexer.status === 'timeout');
  const errors = indexers.filter((indexer) => indexer.status === 'error');
  if (timeouts.length + errors.length === 0) return null;
  const sentences: string[] = [];
  if (timeouts.length > 0) {
    sentences.push(`${prose(timeouts.map((indexer) => indexer.name))} timed out after ${seconds(Math.max(...timeouts.map((indexer) => indexer.ms)))}.`);
  }
  for (const { name, error } of errors) sentences.push(error ? `${name} failed: ${error.replace(/\.?$/, '.')}` : `${name} failed.`);
  const answered = indexers.length - timeouts.length - errors.length;
  if (answered === 0) {
    const title = indexers.length === 1 ? `${indexers[0].name} didn’t answer` : `None of the ${plural(indexers.length, 'indexer')} answered`;
    return { title, body: [...sentences, 'Try again in a moment.'].join(' '), none: true };
  }
  return { title: `${answered} of ${plural(indexers.length, 'indexer')} answered`, body: [...sentences, 'Results may be incomplete.'].join(' '), none: false };
}

/**
 * "10 episodes are missing" / "A pack gets 29–38 in one go. The best match
 * below already does." Absolute numbers for a show ordered that way.
 */
export function missingCopy(missing: SearchMissingRun, absolute: boolean): { title: string; body: string } {
  const range =
    absolute && missing.absolute_from !== null ? `${missing.absolute_from}–${missing.absolute_to ?? missing.absolute_from}` : `${missing.from}–${missing.to}`;
  return {
    title: `${missing.count} episodes are missing`,
    body: `A pack gets ${range} in one go.${missing.best_covers ? ' The best match below already does.' : ''}`,
  };
}

export const noResultsTitle = (q: string) => `No releases for “${q}”`;

export const noResultsBody = (indexers: number) => `${plural(indexers, 'indexer')} searched. Release names are often shorter than titles.`;

export interface NoResultsSuggestions {
  /** Search this shorter text: the short title, else the query without its last word. */
  shorter: string | null;
  /** A wider scope: "Search any pack, not just episode 29". */
  wider: { scope: SearchScope; label: string } | null;
  /** The query labelled "original title" (romaji), when it wasn't the one shown. */
  original: { q: string; label: string } | null;
}

/** Ways to broaden a search that found nothing (HANDOFF §5.2). */
export function noResultsSuggestions(input: {
  q: string;
  title: Pick<Title, 'title' | 'original_language'>;
  scope: SearchScope | null;
  episode: number | null;
  queries: SearchQuery[];
}): NoResultsSuggestions {
  const { q, title, scope, episode, queries } = input;
  const short = shortTitle(title.title);
  const shorter = short && short.toLowerCase() !== q.trim().toLowerCase() ? short : shorterQuery(q);
  const wider =
    scope === 'episode'
      ? { scope: 'any' as const, label: episode !== null ? `Search any pack, not just episode ${episode}` : 'Search any pack, not just this episode' }
      : scope === 'season'
        ? { scope: 'any' as const, label: 'Search any release, not just season packs' }
        : null;
  const romaji = queries.find((query) => query.label === 'original title' && query.q !== q);
  const language = title.original_language && title.original_language !== 'en' ? languageName(title.original_language) : null;
  return {
    shorter,
    wider,
    original: romaji ? { q: romaji.q, label: language ? `Use the ${language} title` : 'Use the original title' } : null,
  };
}

export interface SearchProblem {
  title: string;
  /** The backend's message, which names the cause. */
  body: string;
  /** Trying again can help (not when Jackett isn't set up or the title doesn't exist). */
  retry: boolean;
}

/** A search that failed as a whole, by its contract code (PLAN §6 "Search", errors). */
export function searchProblem(error: unknown): SearchProblem {
  const message = error instanceof Error ? error.message : '';
  switch (isApiError(error) ? error.code : null) {
    case 'unavailable':
      return { title: 'Torrent search is off', body: message || 'Jackett isn’t set up on the server: set JACKETT_URL and JACKETT_API_KEY.', retry: false };
    case 'jackett_unavailable':
      return { title: 'Jackett isn’t answering', body: message || 'It may be down, or it rejects the API key.', retry: true };
    case 'tmdb_unavailable':
      return { title: 'TMDB isn’t answering', body: message || 'Type your own search to look without it.', retry: true };
    case 'not_found':
      return { title: 'TMDB doesn’t know this title', body: message || 'Open it again from its page.', retry: false };
    case 'bad_request':
      return { title: 'This search can’t run', body: message || 'Open it again from the title’s page.', retry: false };
    default:
      return { title: 'Couldn’t search for torrents', body: message || 'Something went wrong on the way. Try again.', retry: true };
  }
}

/** What Add does, as its accessible name: it adds the title to the library first when needed. */
export function addLabel(title: Pick<Title, 'media_type' | 'library'>): string {
  if (!title.library.in_library) return 'Add to library & assign';
  return title.media_type === 'tv' ? 'Add & review mapping' : 'Add & assign to movie';
}

const SORT_WORD: Record<SearchSort, string> = { best: 'best', seeders: 'seeders', size: 'size', newest: 'newest' };

/** "Other results · sorted by best" under the best match; "Results · sorted by seeders" without one. */
export const resultsHeading = (hasBest: boolean, sort: SearchSort) => `${hasBest ? 'Other results' : 'Results'} · sorted by ${SORT_WORD[sort]}`;

/** "Showing 25 of 45". */
export const pagerText = (shown: number, total: number) => `Showing ${formatNumber(shown)} of ${formatNumber(total)}`;
