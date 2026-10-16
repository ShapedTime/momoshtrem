import type { AssignmentJob, LibraryItem, MediaType, TorrentState } from '@/lib/api/contract';
import { isJobWorking } from '@/lib/api/contract/assignment';
import { activityHref, type FindTarget } from '@/lib/library/actions';
import { shortTitle } from '@/lib/mapping/review';
import { plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';
import { findTarget, lacksFile, missingSubject } from './library-row';
import { hasEveryWord, queryWords } from './text';

/**
 * The palette's actions (js/screens/search.js `ACTIONS`), in order: review
 * each open mapping job, find what library titles lack, go to Activity,
 * Library and Discover, settings, Infuse setup, log out. Pure: the palette
 * adds icons and runs them. Ask (#9) and Trakt (#8) join when they land.
 */

export type PaletteActionIcon = 'review' | 'find' | 'activity' | 'library' | 'discover' | 'settings' | 'infuse' | 'logout';

export type PaletteActionTarget = { kind: 'href'; href: string } | { kind: 'sheet'; name: string; params?: Record<string, string> } | { kind: 'logout' };

export interface PaletteAction {
  key: string;
  label: string;
  sub: string;
  icon: PaletteActionIcon;
  target: PaletteActionTarget;
}

export interface PaletteActionInput {
  /** GET /assignment-jobs?status=open */
  jobs: AssignmentJob[];
  /** Library titles that aren't ready (`/library?status=incomplete`); the ones lacking a file get a Find. */
  incomplete: LibraryItem[];
  /** GET /torrents/summary `counts`; null when it can't be read. */
  torrentCounts: Record<TorrentState, number> | null;
  /** The library's `summary.total`; null until known. */
  libraryTotal: number | null;
  /** The torrent sheet is registered: Find opens it instead of the title page. */
  torrentSheet: boolean;
}

/** At most this many Find actions, so they don't bury the rest. */
export const MAX_FIND_ACTIONS = 3;

const findAction = (target: FindTarget): PaletteActionTarget =>
  target.kind === 'sheet' ? { kind: 'sheet', name: 'torrent', params: target.params } : { kind: 'href', href: target.href };

function reviewSub(job: AssignmentJob): string {
  if (isJobWorking(job.status)) return 'Mapping files…';
  if (job.counts.needs_review === 0) return 'Ready to apply';
  return `${plural(job.counts.needs_review, 'file')} ${job.counts.needs_review === 1 ? 'needs' : 'need'} a look`;
}

/** "Find Frieren episodes 29–38" (10 episodes missing), "Find a torrent for Interstellar". */
function findCopy(item: LibraryItem): { label: string; sub: string } | null {
  if (!lacksFile(item)) return null;
  if (item.media_type === 'tv' && item.missing) {
    return { label: `Find ${shortTitle(item.title)} ${missingSubject(item.missing)}`, sub: `${plural(item.missing.count, 'episode')} missing` };
  }
  return { label: `Find a torrent for ${item.title}`, sub: 'No torrent yet' };
}

/** "2 downloading · 1 streaming", "Nothing active right now". */
function activitySub(counts: Record<TorrentState, number> | null): string {
  if (!counts) return 'Torrents, speeds and streams';
  const parts = [counts.downloading > 0 ? `${counts.downloading} downloading` : null, counts.streaming > 0 ? `${counts.streaming} streaming` : null].filter(Boolean);
  return parts.length ? parts.join(' · ') : 'Nothing active right now';
}

export function paletteActions({ jobs, incomplete, torrentCounts, libraryTotal, torrentSheet }: PaletteActionInput): PaletteAction[] {
  const reviews = jobs.map(
    (job): PaletteAction => ({
      key: `review:${job.id}`,
      label: `Review ${shortTitle(job.item.title)} mapping`,
      sub: reviewSub(job),
      icon: 'review',
      target: { kind: 'href', href: `/mapping/${job.id}` },
    })
  );
  const finds = incomplete
    .flatMap((item): PaletteAction[] => {
      const copy = findCopy(item);
      return copy ? [{ key: `find:${item.media_type}:${item.tmdb_id}`, ...copy, icon: 'find', target: findAction(findTarget(item, torrentSheet)) }] : [];
    })
    .slice(0, MAX_FIND_ACTIONS);
  return [
    ...reviews,
    ...finds,
    { key: 'activity', label: 'Go to Activity', sub: activitySub(torrentCounts), icon: 'activity', target: { kind: 'href', href: activityHref() } },
    {
      key: 'library',
      label: 'Go to Library',
      sub: libraryTotal === null ? 'Everything you’ve added' : plural(libraryTotal, 'title'),
      icon: 'library',
      target: { kind: 'href', href: '/library' },
    },
    { key: 'discover', label: 'Go to Discover', sub: 'Browse with filters', icon: 'discover', target: { kind: 'href', href: '/discover' } },
    { key: 'settings', label: 'Open settings', sub: 'Integrations, AI, quality, playback', icon: 'settings', target: { kind: 'href', href: '/settings' } },
    { key: 'infuse', label: 'Infuse setup', sub: 'Copy the WebDAV address', icon: 'infuse', target: { kind: 'sheet', name: 'infuse-setup' } },
    { key: 'logout', label: 'Log out', sub: 'Sign out on this device', icon: 'logout', target: { kind: 'logout' } },
  ];
}

/**
 * ⌘↵ on a TMDB title you don't have: its page with torrent search open
 * (`/movie/603?sheet=torrent&movie=603`). Picking a torrent there adds it
 * to the library first.
 */
export function findTorrentHref(item: { media_type: MediaType; tmdb_id: number }): string {
  const params = new URLSearchParams({ sheet: 'torrent', [item.media_type === 'movie' ? 'movie' : 'show']: String(item.tmdb_id) });
  return `${titleHref(item.media_type, item.tmdb_id)}?${params}`;
}

/** The actions whose label and line hold every word of the query ("lib" → Go to Library). */
export function matchingActions(actions: PaletteAction[], query: string): PaletteAction[] {
  if (!queryWords(query).length) return actions;
  return actions.filter((action) => hasEveryWord(`${action.label} ${action.sub}`, query));
}
