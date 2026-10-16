import type { LibraryItem, MissingRun } from '@/lib/api/contract';
import { activityHref, type FindTarget, libraryActions } from '@/lib/library/actions';
import { hasNoFile, librarySize, watchLine } from '@/lib/library/display';
import { STATUS, type StatusKey } from '@/lib/status';
import { formatNumber, plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';

/**
 * What a library title says in search results and the palette
 * (js/screens/search.js `libInfo`, `statusText`): its state, pill, line and
 * the one action that fits. Pure, so Jest checks every state.
 */

export type LibraryRowState = 'needs_review' | 'downloading' | 'needs_torrent' | 'partial' | 'ready';

/** Review first (it blocks), then a download, then nothing to play, then missing episodes. */
export function libraryRowState(item: LibraryItem): LibraryRowState {
  if (item.needs_review > 0 && item.review_job_id !== null) return 'needs_review';
  if (item.availability_stage === 'downloading') return 'downloading';
  if (hasNoFile(item) || (item.media_type === 'movie' && item.availability_stage === 'needs_torrent')) return 'needs_torrent';
  if (item.media_type === 'tv' && (item.missing !== null || item.availability_stage === 'needs_torrent')) return 'partial';
  return 'ready';
}

export interface RowPill {
  status: StatusKey;
  label: string;
  pct?: number;
}

const episodeCount = (item: LibraryItem) => `${formatNumber(item.episodes_ready ?? 0)}/${formatNumber(item.episodes_total ?? 0)}`;

/** The status pill: full words in a search row, short ones in the palette ("Review", "42%", "No torrent"). */
export function libraryRowPill(item: LibraryItem, { short = false }: { short?: boolean } = {}): RowPill {
  switch (libraryRowState(item)) {
    case 'needs_review':
      return { status: 'needs_review', label: short ? 'Review' : STATUS.needs_review.label };
    case 'downloading': {
      const pct = item.download?.pct;
      if (pct === undefined) return { status: 'downloading', label: STATUS.downloading.label };
      return { status: 'downloading', label: short ? `${pct}%` : `${STATUS.downloading.label} ${pct}%`, pct };
    }
    case 'needs_torrent':
      return { status: 'needs_torrent', label: short ? 'No torrent' : STATUS.needs_torrent.label };
    case 'partial':
      return { status: 'partial', label: short ? episodeCount(item) : `${episodeCount(item)} ready` };
    case 'ready':
      if (item.media_type === 'movie') return { status: 'ready_infuse', label: short ? STATUS.ready.label : STATUS.ready_infuse.label };
      return { status: 'ready', label: short ? STATUS.ready.label : `${episodeCount(item)} ready` };
  }
}

/** A search row's line after "2024 · Movie": "2160p · 18.4 GB", "10 episodes missing", "2 files to check"… */
export function libraryRowSub(item: LibraryItem): string {
  const ready = item.episodes_ready ?? 0;
  const total = item.episodes_total ?? 0;
  switch (libraryRowState(item)) {
    case 'needs_review': {
      const files = `${plural(item.needs_review, 'file')} to check`;
      return item.media_type === 'tv' ? `${formatNumber(ready)} of ${formatNumber(total)} ready · ${files}` : files;
    }
    // Torrents stream on demand: no finish time to promise (HANDOFF §6), and it plays now
    case 'downloading':
      return 'Plays while it downloads';
    case 'needs_torrent':
      return 'In your library, no torrent yet';
    case 'partial': {
      const missing = total - ready;
      return missing > 0 ? `${plural(missing, 'episode')} missing` : 'Some episodes have no torrent';
    }
    case 'ready':
      if (item.media_type === 'tv') return watchLine(item);
      return [item.resolution, item.size_bytes > 0 ? librarySize(item.size_bytes) : null].filter(Boolean).join(' · ');
  }
}

/** The palette's status words: "Ready in Infuse", "14/19 ready", "Needs review"… */
export function libraryStatusText(item: LibraryItem): string {
  switch (libraryRowState(item)) {
    case 'needs_review':
      return STATUS.needs_review.label;
    case 'downloading':
      return STATUS.downloading.label;
    case 'needs_torrent':
      return STATUS.needs_torrent.label;
    case 'partial':
      return `${episodeCount(item)} ready`;
    case 'ready':
      return item.media_type === 'movie' ? STATUS.ready_infuse.label : STATUS.ready.label;
  }
}

/**
 * Where Find goes: the torrent sheet on the missing episodes (or the movie),
 * as the library menu does; the title page without the sheet, or when
 * there's nothing to scope the search to.
 */
export function findTarget(item: LibraryItem, torrentSheet: boolean): FindTarget {
  const find = libraryActions(item, { torrentSheet }).find((action) => action.kind === 'find');
  return find?.kind === 'find' ? find.target : { kind: 'page', href: titleHref(item.media_type, item.tmdb_id) };
}

export type LibraryRowAction =
  | { kind: 'play'; label: string }
  | { kind: 'find'; label: string; target: FindTarget }
  | { kind: 'activity'; label: string; href: string }
  | { kind: 'review'; label: string; href: string };

/** The row's one action: Play (how to play in Infuse), Find missing, Activity, Review or Find. */
export function libraryRowAction(item: LibraryItem, { torrentSheet }: { torrentSheet: boolean }): LibraryRowAction {
  switch (libraryRowState(item)) {
    case 'ready':
      return { kind: 'play', label: 'Play' };
    case 'downloading':
      return { kind: 'activity', label: 'Activity', href: activityHref() };
    case 'needs_review':
      return { kind: 'review', label: 'Review', href: `/mapping/${item.review_job_id ?? ''}` };
    case 'partial':
      return { kind: 'find', label: 'Find missing', target: findTarget(item, torrentSheet) };
    case 'needs_torrent':
      return { kind: 'find', label: 'Find', target: findTarget(item, torrentSheet) };
  }
}

/**
 * Something it should have has no file: a movie without a torrent, a show
 * with missing episodes or no file at all. Whatever else is going on (a
 * review, a download), so the palette can offer Find next to Review.
 */
export function lacksFile(item: LibraryItem): boolean {
  if (item.media_type === 'movie') return item.availability_stage === 'needs_torrent';
  return item.missing !== null || hasNoFile(item);
}

/** The palette's ⌘↵ on a library title: find what it lacks; nothing when it lacks nothing. */
export function librarySecondary(item: LibraryItem, { torrentSheet }: { torrentSheet: boolean }): { label: string; target: FindTarget } | null {
  if (!lacksFile(item)) return null;
  const label = item.media_type === 'tv' && item.missing ? 'Find missing' : 'Find a torrent';
  return { label, target: findTarget(item, torrentSheet) };
}

/** "episodes 29–38", "episode 1122" (absolute), or "S02E08–E10" across seasons. */
export function missingSubject(missing: MissingRun): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  const many = missing.count > 1;
  if (missing.absolute_from !== null) {
    return many ? `episodes ${missing.absolute_from}–${missing.absolute_to ?? missing.absolute_from}` : `episode ${missing.absolute_from}`;
  }
  if (missing.season_number === 1) return many ? `episodes ${missing.from}–${missing.to}` : `episode ${missing.from}`;
  const first = `S${pad(missing.season_number)}E${pad(missing.from)}`;
  return many ? `${first}–E${pad(missing.to)}` : first;
}
