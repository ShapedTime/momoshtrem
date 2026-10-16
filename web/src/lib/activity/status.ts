import type { StreamingSession, TorrentListItem } from '@/lib/api/contract';
import { isWeak } from './health';
import { isActive } from './sections';

/**
 * The line under "Activity" (activity.js `verdict`): is everything working?
 * Built from the data, never canned.
 */
export interface StatusLine {
  /** Bold lead-in ("Everything's working."), or null. */
  lead: string | null;
  text: string;
}

/** Who is streaming, by name when the User-Agent says. */
function player(session: StreamingSession): string {
  return session.client && session.client !== 'Unknown' ? session.client : 'A player';
}

/**
 * - nothing streaming or downloading, and no session (HDRezka's count): "All quiet…";
 * - otherwise "Everything's working.", plus "Infuse is streaming smoothly" when a
 *   session plays, and "{Title} is slow because it has few peers" for a weak torrent.
 *
 * `now` is the "Streaming now" session (paused torrents' sessions already skipped).
 */
export function statusLine(items: readonly TorrentListItem[], now: StreamingSession | null): StatusLine {
  if (!items.some(isActive) && !now) return { lead: null, text: 'All quiet. Nothing in the queue, and the server is fine.' };
  const weak = items.find(isWeak);
  const weakName = weak ? (weak.titles[0]?.title ?? 'One torrent') : null;
  const streaming = now ? `${player(now)} is streaming smoothly` : null;
  const slow = weakName ? `${weakName} is slow because it has few peers` : null;
  if (streaming && slow) return { lead: 'Everything’s working.', text: `${streaming}; ${slow}.` };
  if (streaming) return { lead: 'Everything’s working.', text: `${streaming}.` };
  if (slow) return { lead: 'Everything’s working.', text: `${slow}.` };
  return { lead: 'Everything’s working.', text: '' };
}

/** While the first answer is on its way. */
export const LOADING_LINE = 'Checking on your torrents…';
