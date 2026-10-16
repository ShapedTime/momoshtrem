import type { LibraryItem, LibrarySummary, Timestamp } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';
import { formatBytes, formatNumber, formatRuntime, formatShortDate, plural } from '@/lib/title/format';

/**
 * What a library card and row say (docs/redesign/design-cinematic/js/screens/library.js):
 * the poster pill, the status line, the watch line and the facts. Pure, so
 * Jest checks every state.
 */

/** Nothing is assigned yet: no file anywhere, so Infuse doesn't list it. */
export const hasNoFile = (item: Pick<LibraryItem, 'webdav_path' | 'availability_stage'>) =>
  item.webdav_path === null && item.availability_stage === 'needs_torrent';

export interface PosterBadge {
  status: StatusKey;
  label: string;
  pct?: number;
}

/**
 * The poster's pill, only when something wants attention; "ready" lives in
 * the status line. Review, then Playing, then the download, then No torrent.
 */
export function posterBadge(item: LibraryItem): PosterBadge | null {
  if (item.needs_review > 0) return { status: 'needs_review', label: 'Review' };
  if (item.streaming) return { status: 'streaming', label: 'Playing' };
  if (item.download) return { status: 'downloading', label: `${item.download.pct}%`, pct: item.download.pct };
  if (hasNoFile(item)) return { status: 'needs_torrent', label: 'No torrent' };
  return null;
}

/** The status line under the title. */
export type StatusLine =
  | { kind: 'needs_torrent' }
  | { kind: 'episodes'; ready: number; total: number; complete: boolean }
  | { kind: 'downloading'; pct: number }
  | { kind: 'ready' };

export function statusLine(item: LibraryItem): StatusLine {
  if (hasNoFile(item)) return { kind: 'needs_torrent' };
  if (item.media_type === 'tv') {
    const ready = item.episodes_ready ?? 0;
    const total = item.episodes_total ?? 0;
    return { kind: 'episodes', ready, total, complete: total > 0 && ready >= total };
  }
  if (item.download) return { kind: 'downloading', pct: item.download.pct };
  return { kind: 'ready' };
}

/** How much is watched, 0–100: shows by episode, movies all or nothing (until B5 knows more). */
export function watchedPct(item: LibraryItem): number {
  if (item.media_type === 'tv') {
    const total = item.episodes_total ?? 0;
    return total > 0 ? Math.min(100, Math.round(((item.episodes_watched ?? 0) / total) * 100)) : 0;
  }
  return item.last_watched_at ? 100 : 0;
}

/**
 * The watch line: "28 of 38 watched", "Watched all", "Not started"; movies
 * "Watched" or the runtime. While Trakt isn't linked a show has none: every
 * one would read "Not started".
 */
export function watchLine(item: LibraryItem, { trakt = true }: { trakt?: boolean } = {}): string {
  if (item.media_type === 'tv') {
    if (!trakt) return '';
    const total = item.episodes_total ?? 0;
    const watched = item.episodes_watched ?? 0;
    if (total > 0 && watched >= total) return 'Watched all';
    return watched > 0 ? `${formatNumber(watched)} of ${formatNumber(total)} watched` : 'Not started';
  }
  return item.last_watched_at ? 'Watched' : (formatRuntime(item.runtime) ?? '');
}

/** Sizes on the page: "63.1 GB", "1.45 TB"; "—" for nothing. */
export function librarySize(bytes: number): string {
  if (bytes <= 0) return '—';
  return bytes >= 1e12 ? `${(bytes / 1e12).toFixed(2)} TB` : formatBytes(bytes);
}

/** "15 titles · 2.00 TB · 7 ready in Infuse". */
export function totalsLine(summary: LibrarySummary): string {
  const parts = [plural(summary.total, 'title')];
  if (summary.size_bytes > 0) parts.push(librarySize(summary.size_bytes));
  parts.push(`${formatNumber(summary.ready)} ready in Infuse`);
  return parts.join(' · ');
}

const DAY = 86_400_000;

/** The list's Last watched column: "Now", "Today", "Yesterday", "5 days ago", "3 weeks ago", "Aug 30"; null when never. */
export function lastWatchedLabel(item: Pick<LibraryItem, 'last_watched_at' | 'streaming'>, now: Date = new Date()): string | null {
  if (item.streaming) return 'Now';
  if (!item.last_watched_at) return null;
  return relativeDay(item.last_watched_at, now);
}

function relativeDay(timestamp: Timestamp, now: Date): string {
  const startOf = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const days = Math.round((startOf(now) - startOf(new Date(timestamp))) / DAY);
  if (days <= 0) return 'Today';
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days} days ago`;
  if (days < 28) return `${Math.floor(days / 7)} weeks ago`.replace(/^1 weeks/, '1 week');
  return formatShortDate(timestamp, now);
}

/** "Show · 2023" / "Movie · 2014". */
export function kindLine(item: Pick<LibraryItem, 'media_type' | 'year'>): string {
  return [item.media_type === 'tv' ? 'Show' : 'Movie', item.year].filter(Boolean).join(' · ');
}

/** The remove confirmation: what goes and what stays (STYLE_GUIDE §9). */
export function removeCopy(item: LibraryItem): { title: string; body: string; warning: string | null; checkbox: string | null } {
  const mappings = item.media_type === 'tv' ? (item.episodes_ready ?? 0) : item.webdav_path ? 1 : 0;
  const what =
    item.media_type === 'tv'
      ? mappings > 0
        ? `the show and its ${plural(mappings, 'episode mapping')}`
        : 'the show'
      : mappings > 0
        ? 'the movie and its mapping'
        : 'the movie';
  const body =
    mappings > 0 || item.webdav_path
      ? `This removes ${what}. Infuse stops showing it. Torrents other titles use are kept.`
      : `This removes ${what}. Nothing is assigned to it yet, so Infuse isn’t affected.`;
  return {
    title: `Remove ${item.title} from your library?`,
    body,
    warning: item.streaming ? 'It’s playing in Infuse right now; playback stops.' : null,
    checkbox: item.size_bytes > 0 ? `Also delete downloaded data (up to ${librarySize(item.size_bytes)})` : null,
  };
}
