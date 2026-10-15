import type { EpisodeSummary, MediaType, Title } from '@/lib/api/contract';
import { episodeCode, parseEpisodeCode } from '@/lib/title/episodes';
import { formatRuntime } from '@/lib/title/format';

/**
 * What the source sheet (Torrents · HDRezka, #6 and #67) finds a file for,
 * from its URL params: `movie=<tmdb>`, or `show=<tmdb>` with `season=n` or
 * `se=S01E29`. `season` is also the show page's tab, so it stays when the
 * sheet closes.
 */
export type SourceContext =
  | { kind: 'movie'; tmdbId: number; key: string }
  | { kind: 'show'; tmdbId: number; season: number | null; episode: number | null; key: string };

/** The params the sheet removes when it closes from a deep link. */
export const SOURCE_PARAMS = ['movie', 'show', 'se'];

const tmdbParam = (value: string | null) => {
  const n = Number(value);
  return value && Number.isInteger(n) && n > 0 ? n : null;
};

export function sourceContext(params: URLSearchParams): SourceContext | null {
  const movie = tmdbParam(params.get('movie'));
  if (movie !== null) return { kind: 'movie', tmdbId: movie, key: `movie:${movie}` };
  const show = tmdbParam(params.get('show'));
  if (show === null) return null;
  const code = parseEpisodeCode(params.get('se'));
  const seasonParam = Number(params.get('season'));
  const season = code?.season ?? (params.get('season') !== null && Number.isInteger(seasonParam) && seasonParam >= 0 ? seasonParam : null);
  const episode = code?.episode ?? null;
  return { kind: 'show', tmdbId: show, season, episode, key: `show:${show}:${season ?? ''}:${episode ?? ''}` };
}

export const contextMediaType = (context: SourceContext): MediaType => (context.kind === 'movie' ? 'movie' : 'tv');

/** `?sheet=<tab>&…` params that open the source sheet on this context. */
export function sourceParams(context: SourceContext): Record<string, string> {
  if (context.kind === 'movie') return { movie: String(context.tmdbId) };
  const params: Record<string, string> = { show: String(context.tmdbId) };
  if (context.season !== null) params.season = String(context.season);
  if (context.season !== null && context.episode !== null) params.se = episodeCode(context.season, context.episode);
  return params;
}

/** The legacy search's scope for this context (the Torrents tab until #6). */
export function legacyScope(context: SourceContext): { season?: number; episode?: number } {
  if (context.kind === 'movie' || context.season === null) return {};
  return context.episode === null ? { season: context.season } : { season: context.season, episode: context.episode };
}

/**
 * The sheet's kicker and title: "Find a source · Interstellar" over
 * "2014 · 2h 49m", "Season 2" or "S01E29 · Let's go somewhere warm".
 */
export function sourceHeading(context: SourceContext, title: Title | undefined, episode?: Pick<EpisodeSummary, 'name'>): { label: string; heading: string } {
  const label = title?.title ?? '';
  if (context.kind === 'movie') {
    const facts = title?.media_type === 'movie' ? [title.year, formatRuntime(title.runtime)].filter(Boolean).join(' · ') : '';
    return { label, heading: facts || label || 'Movie' };
  }
  if (context.season !== null && context.episode !== null) {
    const code = episodeCode(context.season, context.episode);
    return { label, heading: episode?.name ? `${code} · ${episode.name}` : code };
  }
  if (context.season !== null) return { label, heading: context.season === 0 ? 'Specials' : `Season ${context.season}` };
  return { label, heading: label || 'Show' };
}
