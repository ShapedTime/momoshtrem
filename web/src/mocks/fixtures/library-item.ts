import type { LibraryItem } from '@/lib/api/contract';

/** Hand-made `LibraryItem`s for unit tests and the gallery (the mock /library builds its own from the seeds). */

/** A ready movie with a file; override what a test is about. */
export function libraryItem(overrides: Partial<LibraryItem> = {}): LibraryItem {
  return {
    id: 1,
    media_type: 'movie',
    tmdb_id: 693134,
    title: 'Dune: Part Two',
    year: 2024,
    poster_path: null,
    availability_stage: 'ready',
    episodes_total: null,
    episodes_ready: null,
    episodes_watched: null,
    needs_review: 0,
    size_bytes: 18_400_000_000,
    added_at: '2026-10-01T10:00:00Z',
    last_watched_at: null,
    runtime: 166,
    resolution: '2160p',
    webdav_path: '/Movies/Dune Part Two (2024)/Dune Part Two (2024).mkv',
    webdav_url: 'https://momoshtrem.local:36911/Movies/Dune%20Part%20Two%20(2024)/Dune%20Part%20Two%20(2024).mkv',
    review_job_id: null,
    download: null,
    missing: null,
    streaming: false,
    ...overrides,
  };
}

/** A show in the library: 28 of 38 episodes ready, 26 watched. */
export function libraryShow(overrides: Partial<LibraryItem> = {}): LibraryItem {
  return libraryItem({
    id: 2,
    media_type: 'tv',
    tmdb_id: 209867,
    title: 'Frieren: Beyond Journey’s End',
    year: 2023,
    availability_stage: 'needs_torrent',
    episodes_total: 38,
    episodes_ready: 28,
    episodes_watched: 26,
    size_bytes: 39_200_000_000,
    runtime: null,
    resolution: null,
    webdav_path: '/TV Shows/Frieren (2023)/',
    webdav_url: 'https://momoshtrem.local:36911/TV%20Shows/Frieren%20(2023)/',
    missing: { season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10 },
    ...overrides,
  });
}
