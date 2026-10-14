import type { EpisodeOrdering, Title, WatchState } from '@/lib/api/contract';
import { formatCount, formatDate, formatMonthYear, formatRuntime, languageName, plural } from './format';

/** One row of the Details panel. */
export interface TitleFact {
  label: string;
  value: string;
}

const ORDERING: Record<EpisodeOrdering, string> = {
  aired: 'Aired',
  absolute: 'Absolute',
  episode_group: 'TMDB episode group',
};

const rating = (title: Title) =>
  title.vote_average === null ? null : `${title.vote_average.toFixed(1)}${title.vote_count ? ` · ${formatCount(title.vote_count)} votes` : ''}`;

/** The Details panel: who made it, how long, when, in what language, and TMDB's rating. Empty values are left out. */
export function titleFacts(title: Title): TitleFact[] {
  const facts: Array<[string, string | null]> =
    title.media_type === 'movie'
      ? [
          [title.directors.length > 1 ? 'Directors' : 'Director', title.directors.join(', ') || null],
          ['Runtime', formatRuntime(title.runtime)],
          ['Released', title.year === null ? null : String(title.year)],
          ['Original language', title.original_language && languageName(title.original_language)],
          ['TMDB', rating(title)],
        ]
      : [
          [title.networks.length > 1 ? 'Networks' : 'Network', title.networks.join(', ') || null],
          ['Seasons', String(title.seasons.filter((season) => season.season_number > 0).length)],
          ['Status', title.status],
          ['Episode ordering', ORDERING[title.episode_ordering]],
          ['TMDB', rating(title)],
        ];
  return facts.flatMap(([label, value]) => (value ? [{ label, value }] : []));
}

/** "Directed by …" / "Created by …", or null without crew. */
export function creditLine(title: Title): string | null {
  const crew = title.media_type === 'movie' ? title.directors : title.creators;
  if (!crew.length) return null;
  return `${title.media_type === 'movie' ? 'Directed' : 'Created'} by ${crew.join(', ')}`;
}

/**
 * The hero's Trakt line: "Watched Mar 14, 2024 · You rated it 9/10",
 * "Watched 3 times · last Jan 2023", "Watching · 26 of 38 episodes".
 * Empty when there's nothing to say.
 */
export function watchSummary(state: WatchState): string[] {
  const parts: string[] = [];
  if (state.progress && state.progress.aired > 0) {
    const { completed, aired } = state.progress;
    parts.push(completed >= aired ? `Watched all ${plural(aired, 'episode')}` : `Watching · ${completed} of ${plural(aired, 'episode')}`);
    if (state.watched_at) parts.push(`last watched ${formatDate(state.watched_at)}`);
  } else if (state.plays > 1) {
    parts.push(`Watched ${state.plays} times`);
    if (state.watched_at) parts.push(`last ${formatMonthYear(state.watched_at)}`);
  } else if (state.watched_at) {
    parts.push(`Watched ${formatDate(state.watched_at)}`);
  }
  if (state.rating !== null) parts.push(`You rated it ${state.rating}/10`);
  return parts;
}

/** The confirm dialog's consequence sentence for "Remove from library": what goes and what stays. */
export function removeConsequence(title: Title): string {
  if (!title.library.webdav_path) return 'Nothing is assigned to it yet, so Infuse isn’t affected.';
  if (title.media_type === 'movie') {
    return 'Infuse loses it right away and its torrent is unassigned. The torrent itself stays in Activity until you remove it there.';
  }
  const ready = title.library.episodes_ready ?? 0;
  return `Infuse loses its ${plural(ready, 'episode')} right away and their torrents are unassigned. The torrents themselves stay in Activity until you remove them there.`;
}
