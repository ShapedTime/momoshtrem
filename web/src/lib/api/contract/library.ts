import type { MediaType, Timestamp } from './common';
import type { AvailabilityStage } from './titles';

/** Library (B1; #7). */

export type LibraryStatusFilter = 'ready' | 'incomplete' | 'needs_review' | 'watching';
export type LibrarySort = 'added' | 'title' | 'last_watched' | 'size';

/** GET /library query */
export interface LibraryParams {
  type?: MediaType;
  status?: LibraryStatusFilter;
  sort?: LibrarySort;
  q?: string;
  page?: number;
}

/** One row of GET /library (a Paginated<LibraryItem>). */
export interface LibraryItem {
  id: number;
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
  availability_stage: AvailabilityStage;
  /** Shows only. */
  episodes_total: number | null;
  episodes_ready: number | null;
  episodes_watched: number | null;
  needs_review: number;
  size_bytes: number;
  added_at: Timestamp;
  last_watched_at: Timestamp | null;
}
