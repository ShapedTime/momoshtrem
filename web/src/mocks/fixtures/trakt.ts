import type { HistoryItem, Paginated, TraktIntegration, UpNextItem, WatchlistItem } from '@/lib/api/contract';
import { CATALOG, iso } from './catalog';

export const TRAKT: TraktIntegration = {
  configured: true,
  connected: true,
  username: 'teymur',
  last_sync_at: iso(0),
  sync_status: 'idle',
  counts: { history: 1284, ratings: 212, watchlist: 37 },
};

const shows = CATALOG.filter((entry) => entry.media_type === 'tv');
const availability = ['downloading', 'ready', 'needs_torrent', 'airs_on'] as const;

export const UP_NEXT: UpNextItem[] = shows.map((entry, i) => ({
  show: { tmdb_id: entry.tmdb_id, title: entry.title, poster_path: null, backdrop_path: null },
  next_episode: { season: 1, episode: 13 + i, absolute_number: entry.is_anime ? 13 + i : null, name: `Episode ${13 + i}`, still_path: null },
  availability: availability[i % availability.length],
  airs_at: availability[i % availability.length] === 'airs_on' ? iso(-3) : null,
  progress: { completed: 12 + i, aired: 28 },
  last_watched_at: iso(i + 1),
}));

export const WATCHLIST: WatchlistItem[] = CATALOG.slice(3).map((entry, i) => ({
  media_type: entry.media_type,
  tmdb_id: entry.tmdb_id,
  title: entry.title,
  year: entry.year,
  poster_path: null,
  listed_at: iso(i * 3),
  in_library: i % 2 === 0,
  availability: i % 2 === 0 ? 'ready' : 'not_in_library',
}));

export function history(page = 1): Paginated<HistoryItem> {
  const items: HistoryItem[] = Array.from({ length: 20 }, (_, i) => ({
    media_type: 'tv',
    tmdb_id: 209867,
    title: 'Frieren: Beyond Journey’s End',
    season: 1,
    episode: 12 - (i % 12),
    watched_at: iso(i + (page - 1) * 20),
  }));
  return { items, total: 1284, page, page_size: 20 };
}
