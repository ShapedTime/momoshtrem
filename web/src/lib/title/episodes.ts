import type { EpisodeOrdering, EpisodeSummary, SeasonSummary, ShowTitle } from '@/lib/api/contract';

/** Episode lists on show pages: codes, labels, the selected season and the next-up row. */

const pad = (n: number) => String(n).padStart(2, '0');

/** 1, 5 → "S01E05". */
export const episodeCode = (season: number, episode: number) => `S${pad(season)}E${pad(episode)}`;

/** "S01E05" (any case) → {season: 1, episode: 5}; null for anything else. */
export function parseEpisodeCode(code: string | null): { season: number; episode: number } | null {
  const match = code ? /^s(\d{1,3})e(\d{1,4})$/i.exec(code.trim()) : null;
  return match ? { season: Number(match[1]), episode: Number(match[2]) } : null;
}

/**
 * "#1100 · S22E15" with absolute ordering, else "S22E15". The absolute number
 * is left out where it's the in-season one anyway (Frieren S01E29 is #29).
 */
export function episodeLabel(episode: Pick<EpisodeSummary, 'season_number' | 'episode_number' | 'absolute_number'>, ordering: EpisodeOrdering): string {
  const code = episodeCode(episode.season_number, episode.episode_number);
  const absolute = episode.absolute_number;
  return ordering === 'absolute' && absolute !== null && absolute !== episode.episode_number ? `#${absolute} · ${code}` : code;
}

/** Absolute ordering where some season's numbers run on from earlier ones (One Piece's #1086 is S22E01). */
export function showsAbsoluteNumbers(show: Pick<ShowTitle, 'episode_ordering' | 'seasons'>): boolean {
  return (
    show.episode_ordering === 'absolute' &&
    show.seasons.some((season) => season.season_number > 0 && season.absolute_from !== null && season.absolute_from !== 1)
  );
}

/** Tab label: "Specials", TMDB's name ("Wano Country"), or "Season 3". */
export function seasonLabel(season: Pick<SeasonSummary, 'season_number' | 'name'>): string {
  if (season.season_number === 0) return 'Specials';
  return season.name.trim() || `Season ${season.season_number}`;
}

/** The season `?season=` names when the show has it, else the show's default, else its first. */
export function selectedSeason(param: string | null, show: Pick<ShowTitle, 'seasons' | 'default_season'>): number | null {
  const requested = param !== null && /^\d+$/.test(param) ? Number(param) : null;
  if (requested !== null && show.seasons.some((season) => season.season_number === requested)) return requested;
  return show.default_season ?? show.seasons[0]?.season_number ?? null;
}

/**
 * The row marked "Up next": the first episode of the frontier (the aired run
 * after the last file), when it's in this season. Null otherwise; once
 * Trakt (#8) knows what was watched this can point at the next unwatched one.
 */
export function nextUpEpisodeId(show: Pick<ShowTitle, 'library'>, seasonNumber: number, episodes: EpisodeSummary[]): number | null {
  const missing = show.library.missing;
  if (!missing || missing.season_number !== seasonNumber) return null;
  return episodes.find((episode) => episode.episode_number === missing.from)?.id ?? null;
}

/** "28 of 38 ready", or "Absolute numbering · 1,124 episodes · 1,088 ready" when the numbers run across seasons. */
export function episodesSummary(show: Pick<ShowTitle, 'episode_ordering' | 'seasons' | 'library'>): string | null {
  const { episodes_total: total, episodes_ready: ready } = show.library;
  if (total === null || ready === null) return null;
  if (showsAbsoluteNumbers(show)) {
    return `Absolute numbering · ${total.toLocaleString('en-US')} episodes · ${ready.toLocaleString('en-US')} ready`;
  }
  return `${ready.toLocaleString('en-US')} of ${total.toLocaleString('en-US')} ready`;
}
