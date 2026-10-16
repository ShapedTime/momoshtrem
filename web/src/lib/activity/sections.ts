import type { StreamingSession, TorrentListItem, TorrentState } from '@/lib/api/contract';

/**
 * Activity's sections (activity.js `groups`): Streaming now, Downloading,
 * Paused, then Idle. Torrents stream on demand, so the fourth is Idle, not
 * Seeding (HANDOFF §6).
 */

export const SECTION_ORDER: readonly TorrentState[] = ['streaming', 'downloading', 'paused', 'idle'];

const rank = (state: TorrentState) => SECTION_ORDER.indexOf(state);
const newestFirst = (a: string | null, b: string | null) => {
  if (a === b) return 0;
  if (a === null) return 1;
  if (b === null) return -1;
  return b.localeCompare(a);
};

/** The backend's order (PLAN §6): by state, then the last read newest first (nulls last), then added newest first. */
export function sortTorrents(items: readonly TorrentListItem[]): TorrentListItem[] {
  return [...items].sort(
    (a, b) => rank(a.state) - rank(b.state) || newestFirst(a.last_active_at, b.last_active_at) || newestFirst(a.added_at, b.added_at)
  );
}

export type TorrentSections = Record<TorrentState, TorrentListItem[]>;

/** One list per state, each in the backend's order. */
export function groupTorrents(items: readonly TorrentListItem[]): TorrentSections {
  const sections: TorrentSections = { streaming: [], downloading: [], paused: [], idle: [] };
  for (const item of sortTorrents(items)) sections[item.state].push(item);
  return sections;
}

/** Streaming or downloading: what Pause all pauses and the badges count. */
export const isActive = (torrent: Pick<TorrentListItem, 'state'>) => torrent.state === 'streaming' || torrent.state === 'downloading';

/**
 * Nothing to show but the empty state: no torrents at all, or only idle ones
 * (they sit below it, full width).
 */
export const isQuiet = (items: readonly TorrentListItem[]) => items.every((item) => item.state === 'idle');

/**
 * The session the "Streaming now" tile shows: the most recently read one,
 * skipping those of paused torrents (a read doesn't wake them; the session
 * lingers for 30 s), and of torrents `items` doesn't have: removed ones (a
 * session outlives its torrent by 30 s) and ones waiting out their Undo, which
 * the caller leaves out. HDRezka's always count.
 */
export function streamingNow(sessions: readonly StreamingSession[], items: readonly TorrentListItem[]): StreamingSession | null {
  const states = new Map(items.map((item) => [item.info_hash, item.state]));
  return (
    sessions.find((session) => {
      if (session.source !== 'torrent' || !session.info_hash) return true;
      const state = states.get(session.info_hash);
      return state !== undefined && state !== 'paused';
    }) ?? null
  );
}
