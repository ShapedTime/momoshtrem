import type { StreamingSession, TorrentListItem } from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';

/**
 * Activity's words and numbers (activity.js): labels, clocks, speeds and the
 * "Streaming now" lines. Pure, so Jest covers them.
 */

const pad = (n: number) => String(n).padStart(2, '0');

/** 1122 → "18:42"; 3725 → "1:02:05". */
export function formatClock(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** (2, 5) → "S02E05". */
export const episodeCode = (season: number, episode: number) => `S${pad(season)}E${pad(episode)}`;

/** A transfer rate: 2_300_000 → "2.3 MB/s"; under 1 KB/s reads "0 KB/s", not "0 B/s". */
export function formatRate(bytesPerSecond: number): string {
  return bytesPerSecond < 1000 ? '0 KB/s' : `${formatBytes(bytesPerSecond)}/s`;
}

/** A speed as a big number and its unit: 29_800_000 → {value: "29.8", unit: "MB/s"}; nothing → "0 MB/s". */
export function splitSpeed(bytesPerSecond: number): { value: string; unit: string } {
  if (bytesPerSecond <= 0) return { value: '0', unit: 'MB/s' };
  const [value, unit] = formatBytes(bytesPerSecond).split(' ');
  return { value, unit: `${unit}/s` };
}

/** A size as a big number and its unit: 1_310_000_000_000 → {value: "1.3", unit: "TB"}. */
export function splitBytes(bytes: number): { value: string; unit: string } {
  const [value, unit] = formatBytes(bytes).split(' ');
  return { value, unit };
}

/** The card's heading and where it goes: "Frieren · S01E29–E38" → the title; a torrent nothing links stays unlinked. */
export interface TorrentLabel {
  text: string;
  href: string | null;
  /** Its library titles: none for an Inspect dry run or a torrent a WebDAV listing loaded. */
  linked: boolean;
}

export function torrentLabel(torrent: Pick<TorrentListItem, 'name' | 'titles'>): TorrentLabel {
  const [first] = torrent.titles;
  if (!first) return { text: torrent.name, href: null, linked: false };
  return {
    text: first.scope ? `${first.title} · ${first.scope}` : first.title,
    href: titleHref(first.media_type, first.tmdb_id),
    linked: true,
  };
}

/** "6 min", "45 s": how far ahead the buffer reaches. */
function aheadLabel(seconds: number): string {
  return seconds >= 60 ? `${Math.floor(seconds / 60)} min` : `${Math.max(0, Math.round(seconds))} s`;
}

/** What a session plays: "Severance S02E05", or the file WebDAV shows when the library doesn't know it. */
export function sessionSubject(session: StreamingSession): string {
  const { item } = session;
  if (!item) return session.file_name;
  return item.season_number !== null && item.episode_number !== null ? `${item.title} ${episodeCode(item.season_number, item.episode_number)}` : item.title;
}

/** Where it plays: "on Apple TV", else "in Infuse", else nothing (Infuse's own User-Agent rarely names a device). */
export function sessionWhere(session: StreamingSession): string | null {
  if (session.device) return `on ${session.device}`;
  if (session.client && session.client !== 'Unknown') return `in ${session.client}`;
  return null;
}

/** "18:42 of 58:10", "at 18:42", or nothing without a runtime. */
export function sessionPosition(session: StreamingSession): string | null {
  if (session.position_seconds === null) return null;
  return session.duration_seconds !== null
    ? `${formatClock(session.position_seconds)} of ${formatClock(session.duration_seconds)}`
    : `at ${formatClock(session.position_seconds)}`;
}

/** "buffered 6 min ahead"; nothing for HDRezka or without a runtime. */
export function sessionBuffer(session: StreamingSession): string | null {
  return session.buffered_ahead_seconds === null ? null : `buffered ${aheadLabel(session.buffered_ahead_seconds)} ahead`;
}

/**
 * The "Streaming now" tile (activity.js `summary`): "Severance S02E05 on Apple TV" over
 * "Trojan's Horse · 18:42 of 52:00 · buffered 33 min ahead", fields left out when unknown.
 * It links to the torrent's detail, or the title for HDRezka.
 */
export function nowTile(session: StreamingSession): { title: string; detail: string; href: string | null } {
  const where = sessionWhere(session);
  const detail = [session.item?.episode_name ?? null, sessionPosition(session), sessionBuffer(session)].filter(Boolean).join(' · ');
  const href =
    session.source === 'torrent' && session.info_hash
      ? `/activity/${session.info_hash}`
      : session.item
        ? titleHref(session.item.media_type, session.item.tmdb_id)
        : null;
  return { title: where ? `${sessionSubject(session)} ${where}` : sessionSubject(session), detail, href };
}

/** The streaming card's inner banner: the episode, then who reads it, where and how far ahead. */
export function sessionBanner(session: StreamingSession): { title: string; detail: string } {
  const { item } = session;
  const episode = item?.season_number != null && item.episode_number != null ? episodeCode(item.season_number, item.episode_number) : null;
  const title = item ? [episode, item.episode_name ?? (episode ? null : item.title)].filter(Boolean).join(' ') : session.file_name;
  const detail = [session.device, session.client && session.client !== 'Unknown' ? session.client : null, sessionPosition(session), sessionBuffer(session)]
    .filter(Boolean)
    .join(' · ');
  return { title, detail };
}

/** Active readers in Advanced: "Infuse · S02E05 · at 18:42 of 52:00 · 9.1 MB/s". */
export function readerLine(session: StreamingSession): string {
  const { item } = session;
  const episode = item?.season_number != null && item.episode_number != null ? episodeCode(item.season_number, item.episode_number) : null;
  const position = sessionPosition(session);
  return [
    session.client ?? 'Unknown player',
    episode ?? item?.title ?? session.file_name,
    position && (position.startsWith('at ') ? position : `at ${position}`),
    formatRate(session.rate_bps),
  ]
    .filter(Boolean)
    .join(' · ');
}

/** "21:42", the time the page last heard from momoshtrem. */
export function formatTimeOfDay(timestamp: number): string {
  return new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' }).format(new Date(timestamp));
}

/** Piece sizes are powers of two and read as clients show them: 16 MiB → "16 MB", 512 KiB → "512 KB". */
export function formatPieceSize(bytes: number): string {
  const mib = bytes / (1 << 20);
  if (mib >= 1) return `${Number.isInteger(mib) ? mib : mib.toFixed(1)} MB`;
  return `${Math.round(bytes / 1024)} KB`;
}

/** "Oct 9, 21:04", when the torrent was added. */
export function formatAdded(timestamp: string, now: Date = new Date()): string {
  const date = new Date(timestamp);
  const sameYear = date.getFullYear() === now.getFullYear();
  const day = new Intl.DateTimeFormat('en-US', sameYear ? { month: 'short', day: 'numeric' } : { month: 'short', day: 'numeric', year: 'numeric' }).format(date);
  return `${day}, ${formatTimeOfDay(date.getTime())}`;
}
