import type { MediaType, Paginated, Timestamp } from './common';
import type { AvailabilityStage, MissingRun } from './titles';

/** Library (B1; #7, #97). */

export type LibraryStatusFilter = 'ready' | 'incomplete' | 'needs_review' | 'watching';
export type LibrarySort = 'added' | 'title' | 'last_watched' | 'size';

/** GET /library query */
export interface LibraryParams {
  type?: MediaType;
  status?: LibraryStatusFilter;
  sort?: LibrarySort;
  q?: string;
  /**
   * Look titles up by TMDB id: `movie:603,tv:1399` (at most 200; `libraryIds`
   * builds it). ANDed with the other filters; the answer is page 1 of all of them.
   */
  ids?: string;
  page?: number;
  /** 50 by default, max 200. */
  page_size?: number;
}

/** A title by TMDB id, for `ids`. */
export interface LibraryRef {
  media_type: MediaType;
  tmdb_id: number;
}

/** What a library title is fetching: files a download (#77) wants that aren't all on disk. */
export interface LibraryDownload {
  /** Shows: combined over the files being fetched. */
  pct: number;
  /** One of the torrents being fetched ("See download"). */
  info_hash: string;
}

/** One row of GET /library. */
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
  /** Regular episodes watched on Trakt (B5); 0 without Trakt. */
  episodes_watched: number | null;
  needs_review: number;
  size_bytes: number;
  added_at: Timestamp;
  /** The movie's last play, or the show's last watched episode; null without Trakt. */
  last_watched_at: Timestamp | null;
  /** Movies: minutes. Null for shows and when unknown. */
  runtime: number | null;
  /** Movies: the assigned file's, e.g. "2160p". */
  resolution: string | null;
  /** As `TitleLibraryState`: null until something is assigned. */
  webdav_path: string | null;
  webdav_url: string | null;
  /** The mapping job to review (PLAN §6 review rules). */
  review_job_id: number | null;
  /** While a file is downloading: a download wants it (#77) and it isn't all on disk. */
  download: LibraryDownload | null;
  /** Shows: the first frontier run, as `TitleLibraryState.missing`. */
  missing: MissingRun | null;
  /** A streaming session plays it now (a show: any of its episodes; #99). */
  streaming: boolean;
}

/** Filter counts: they honour `q` and `ids`, never `type` or `status`. */
export interface LibraryCounts {
  all: number;
  movie: number;
  tv: number;
  /** Shows with 0 < episodes_watched < episodes_total that aren't dropped or hidden on Trakt. */
  watching: number;
  ready: number;
  incomplete: number;
  needs_review: number;
}

/** The whole library, whatever the params. */
export interface LibrarySummary {
  total: number;
  size_bytes: number;
  ready: number;
}

/** GET /library */
export interface LibraryPage extends Paginated<LibraryItem> {
  counts: LibraryCounts;
  summary: LibrarySummary;
}
