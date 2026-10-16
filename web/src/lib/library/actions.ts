import type { LibraryItem } from '@/lib/api/contract';
import { NAV } from '@/lib/nav';
import { showSourceParams } from '@/lib/source/context';
import { formatNumber, plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';

/**
 * The "…" menu of a library title, in order (library.js `menuItems`): open
 * it, review its mapping, find what it lacks, copy its WebDAV path, see its
 * download, and remove it. Pure; the component adds icons and handlers.
 */

/** Find a torrent: the torrent sheet once it exists (#6), else the title page. */
export type FindTarget = { kind: 'sheet'; params: Record<string, string> } | { kind: 'page'; href: string };

export type LibraryAction =
  | { kind: 'open'; label: string; href: string }
  | { kind: 'review'; label: string; href: string; hint: string }
  | { kind: 'find'; label: string; target: FindTarget; hint?: string }
  | { kind: 'copy'; label: string; url: string }
  | { kind: 'download'; label: string; href: string }
  | { kind: 'remove'; label: string };

/** Activity, or one torrent's detail there. */
export const activityHref = (infoHash?: string) => {
  const base = NAV.find((item) => item.id === 'activity')?.href ?? '/activity';
  return infoHash ? `${base}/${infoHash}` : base;
};

export function libraryActions(item: LibraryItem, { torrentSheet }: { torrentSheet: boolean }): LibraryAction[] {
  const page = titleHref(item.media_type, item.tmdb_id);
  const actions: LibraryAction[] = [{ kind: 'open', label: 'Open details', href: page }];
  if (item.needs_review > 0 && item.review_job_id !== null) {
    actions.push({ kind: 'review', label: 'Review mapping', href: `/mapping/${item.review_job_id}`, hint: plural(item.needs_review, 'file') });
  }
  if (item.media_type === 'tv' && item.missing) {
    const { season_number: season, from } = item.missing;
    actions.push({
      kind: 'find',
      label: 'Find missing episodes',
      hint: formatNumber(item.missing.count),
      target: torrentSheet
        ? { kind: 'sheet', params: showSourceParams(item.tmdb_id, { season, episode: from }) }
        : { kind: 'page', href: `${page}?season=${season}` },
    });
  } else if (item.media_type === 'movie' && item.availability_stage === 'needs_torrent') {
    actions.push({
      kind: 'find',
      label: 'Find a torrent',
      target: torrentSheet ? { kind: 'sheet', params: { movie: String(item.tmdb_id) } } : { kind: 'page', href: page },
    });
  }
  if (item.webdav_url) actions.push({ kind: 'copy', label: 'Copy WebDAV path', url: item.webdav_url });
  if (item.download) actions.push({ kind: 'download', label: 'See download', href: activityHref(item.download.info_hash) });
  actions.push({ kind: 'remove', label: 'Remove from library…' });
  return actions;
}
