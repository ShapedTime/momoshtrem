import type { LibraryItem } from '@/lib/api/contract';
import { CATALOG, iso } from './catalog';

export const LIBRARY: LibraryItem[] = CATALOG.map((entry, i) => ({
  id: i + 1,
  media_type: entry.media_type,
  tmdb_id: entry.tmdb_id,
  title: entry.title,
  year: entry.year,
  poster_path: null,
  availability_stage: (['downloading', 'ready', 'ready', 'needs_torrent', 'ready', 'none', 'ready'] as const)[i] ?? 'ready',
  episodes_total: entry.media_type === 'tv' ? 28 : null,
  episodes_ready: entry.media_type === 'tv' ? 20 : null,
  episodes_watched: entry.media_type === 'tv' ? 12 : null,
  needs_review: i === 2 ? 3 : 0,
  size_bytes: (i + 1) * 4_300_000_000,
  added_at: iso(i * 9),
  last_watched_at: i % 2 ? iso(i) : null,
}));
