import type { LibraryItem } from '@/lib/api/contract';
import { SHOWS } from './title-seeds';
import { buildTitle, episodesWatched, libraryKey, type MockLibrary } from './titles';

/**
 * Titles Infuse is playing right now in the mock (`streaming` is false on the
 * backend until #99), so the Playing pill shows: The Studio.
 */
const STREAMING = new Set([libraryKey('tv', 247767)]);

/** The mock's size for an episode file. */
const EPISODE_BYTES = 1_400_000_000;

/** GET /library rows for what the page's library holds (same stages as the title pages). Unsorted: queryLibrary sorts. */
export function libraryItems(library: MockLibrary): LibraryItem[] {
  return [...library.values()].flatMap((row): LibraryItem[] => {
    const title = buildTitle(row.media_type, row.tmdb_id, library);
    if (!title) return [];
    const { library: state, watch_state } = title;
    const movie = title.media_type === 'movie' ? title : null;
    const showKey = SHOWS[row.tmdb_id]?.key ?? 0;
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
        episodes_watched: title.media_type === 'tv' ? episodesWatched(title.tmdb_id, library) : null,
        needs_review: state.needs_review,
        size_bytes: state.torrent?.size_bytes ?? state.stream?.size_bytes ?? (state.episodes_ready ?? 0) * EPISODE_BYTES,
        added_at: row.added_at,
        last_watched_at: watch_state?.watched_at ?? null,
        runtime: movie ? movie.runtime : null,
        resolution: state.torrent?.resolution ?? state.stream?.quality ?? null,
        webdav_path: state.webdav_path,
        webdav_url: state.webdav_url,
        review_job_id: state.review_job_id,
        download: state.download && {
          pct: state.download.pct,
          // Shows: the seeded release's torrent (fixtures/titles.ts episodeFile)
          info_hash: state.torrent?.info_hash ?? `seed-${showKey}`,
        },
        missing: state.missing,
        streaming: STREAMING.has(libraryKey(row.media_type, row.tmdb_id)),
      },
    ];
  });
}
