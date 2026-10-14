import type { LibraryItem } from '@/lib/api/contract';
import { buildTitle, type MockLibrary } from './titles';

/** GET /library rows for what the page's library holds, newest first (same stages as the title pages). */
export function libraryItems(library: MockLibrary): LibraryItem[] {
  return [...library.values()]
    .flatMap((row): LibraryItem[] => {
      const title = buildTitle(row.media_type, row.tmdb_id, library);
      if (!title) return [];
      const { library: state, watch_state } = title;
      return [
        {
          id: row.library_id,
          media_type: title.media_type,
          tmdb_id: title.tmdb_id,
          title: title.title,
          year: title.year,
          poster_path: null,
          availability_stage: state.availability_stage,
          episodes_total: state.episodes_total,
          episodes_ready: state.episodes_ready,
          episodes_watched: title.media_type === 'tv' ? (watch_state?.progress?.completed ?? 0) : null,
          needs_review: state.needs_review,
          size_bytes: state.torrent?.size_bytes ?? (state.episodes_ready ?? 0) * 1_400_000_000,
          added_at: row.added_at,
          last_watched_at: watch_state?.watched_at ?? null,
        },
      ];
    })
    .sort((a, b) => b.added_at.localeCompare(a.added_at));
}
