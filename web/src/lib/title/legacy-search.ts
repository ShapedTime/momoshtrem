import type { Title } from '@/lib/api/contract';
import type { TorrentSearchContext } from '@/types/jackett';

const pad = (n: number) => String(n).padStart(2, '0');

/**
 * The search the legacy TorrentSearchModal opens with, until the torrent
 * search sheet (#6) takes over: "Title Year" for a movie, "Title S01" for a
 * season, "Title S01E29" for an episode, or the show's title.
 */
export function legacySearchContext(title: Title, scope: { season?: number; episode?: number } = {}): TorrentSearchContext {
  const base = { title: title.title, tmdbId: title.tmdb_id, year: title.year ?? undefined };
  if (title.media_type === 'movie') {
    return { ...base, mediaType: 'movie', query: title.year ? `${title.title} ${title.year}` : title.title };
  }
  const { season, episode } = scope;
  if (season !== undefined && episode !== undefined) {
    return { ...base, mediaType: 'episode', season, episode, query: `${title.title} S${pad(season)}E${pad(episode)}` };
  }
  if (season !== undefined) return { ...base, mediaType: 'tv', season, query: `${title.title} S${pad(season)}` };
  return { ...base, mediaType: 'tv', query: title.title };
}
