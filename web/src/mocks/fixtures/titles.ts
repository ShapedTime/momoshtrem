import type { EpisodeSummary, MovieTitle, SeasonSummary, ShowTitle, Title } from '@/lib/api/contract';
import { type CatalogEntry, findTitle, iso } from './catalog';

function episodes(season: number, count: number, readyUpTo: number, watchedUpTo: number): EpisodeSummary[] {
  return Array.from({ length: count }, (_, i) => {
    const n = i + 1;
    const state = n <= watchedUpTo ? 'watched' : n <= readyUpTo ? 'ready' : n === readyUpTo + 1 ? 'downloading' : 'missing';
    return {
      id: season * 1000 + n,
      season_number: season,
      episode_number: n,
      absolute_number: season === 0 ? null : n,
      name: season === 0 ? `Special ${n}` : `Episode ${n}`,
      air_date: iso(400 - n * 7).slice(0, 10),
      still_path: null,
      runtime: 24,
      state,
      pct: state === 'downloading' ? 64 : null,
    };
  });
}

function seasonSummary(season: number, name: string, list: EpisodeSummary[]): SeasonSummary {
  return {
    season_number: season,
    name,
    episodes_total: list.length,
    ready: list.filter((e) => e.state === 'ready' || e.state === 'watched').length,
    missing: list.filter((e) => e.state === 'missing').length,
    needs_review: list.filter((e) => e.state === 'needs_review').length,
  };
}

function base(entry: CatalogEntry) {
  return {
    tmdb_id: entry.tmdb_id,
    title: entry.title,
    original_title: entry.original_title,
    year: entry.year,
    overview: entry.overview,
    poster_path: null,
    backdrop_path: null,
    backdrop_dominant_color: '#2f8f7f',
    genres: entry.genres,
    vote_average: 8.4,
    cast: [{ name: 'Atsumi Tanezaki', character: 'Frieren', profile_path: null }],
    trailer_youtube_key: null,
    recommendation_reason: 'Because you loved Mushishi: quiet, melancholic and kind.',
  };
}

export function buildTitle(mediaType: 'movie' | 'tv', tmdbId: number): Title | undefined {
  const entry = findTitle(mediaType, tmdbId);
  if (!entry) return undefined;

  if (entry.media_type === 'movie') {
    const movie: MovieTitle = {
      ...base(entry),
      media_type: 'movie',
      runtime: entry.runtime ?? null,
      library: { in_library: true, library_id: tmdbId % 1000, availability_stage: 'ready', webdav_path: `/Movies/${entry.title} (${entry.year})/`, download: null, needs_review: 0 },
      watch_state: { watched_at: null, plays: 0, rating: null },
    };
    return movie;
  }

  const main = episodes(1, 28, 20, 12);
  const specials = episodes(0, 4, 2, 0);
  const show: ShowTitle = {
    ...base(entry),
    media_type: 'tv',
    status: 'Returning Series',
    is_anime: Boolean(entry.is_anime),
    episode_ordering: 'aired',
    episode_group_id: null,
    include_specials: true,
    classifier_mode: 'auto',
    seasons: [seasonSummary(1, 'Season 1', main), seasonSummary(0, 'Specials', specials)],
    episodes: [...main, ...specials],
    library: {
      in_library: true,
      library_id: tmdbId % 1000,
      availability_stage: 'downloading',
      webdav_path: `/Shows/${entry.title}/`,
      download: { pct: 64, speed_bps: 6_400_000, eta_seconds: 540, seeders: 312 },
      needs_review: 0,
    },
    watch_state: { watched_at: iso(2), plays: 12, rating: 9, progress: { completed: 12, aired: 28 } },
  };
  return show;
}
