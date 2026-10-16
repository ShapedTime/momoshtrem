import type { Download, DownloadList, DownloadReason, DownloadScope, TorrentHold, TorrentListItem, TorrentWanted } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';
import { formatBytes, formatNumber, plural } from '@/lib/title/format';
import { titleHref } from '@/lib/title/href';
import { keepExpiryLabel, keepStatePill, keepWantsBytes } from '@/lib/title/keep';
import { episodeCode, formatTimeOfDay } from './format';

/**
 * Background downloads on Activity (PLAN §6 "Downloads (#77)", B6 `wanted` and
 * `holds`, and "Storage and downloads: UI copy"): the cards' pills, progress
 * and ETA, the Downloading section's line, and the "Kept & smart" list. Pure,
 * so Jest covers them. What's on disk for a download comes from the piece
 * cache (`wanted.done_bytes`); `progress` stays anacrolix's, for the torrent.
 */

/** 0–100 of what its downloads want; floored, so it never reads 100% before it's done. */
export function wantedPct(wanted: Pick<TorrentWanted, 'bytes' | 'done_bytes' | 'state'>): number {
  if (wanted.state === 'done') return 100;
  return wanted.bytes > 0 ? Math.min(99, Math.floor((wanted.done_bytes / wanted.bytes) * 100)) : 0;
}

/** Its downloads still want bytes (queued, downloading, waiting, stalled, paused or the disk full). */
export const wantsMore = (torrent: Pick<TorrentListItem, 'wanted'>): boolean => torrent.wanted !== null && torrent.wanted.state !== 'done';

/**
 * Its downloads will carry on by themselves: they want bytes and aren't `paused`
 * (you paused the torrent, or background downloads are off). Such a torrent can
 * be paused, and keeps Activity from reading "quiet".
 */
export const waitsOnItsOwn = (torrent: Pick<TorrentListItem, 'wanted'>): boolean => wantsMore(torrent) && torrent.wanted?.state !== 'paused';

/**
 * "about 12 min left", "about 4 h 30 min left", "about 2 d 3 h left": an estimate, so past an
 * hour it moves in 10-minute steps and doesn't jitter or promise the minute (STYLE_GUIDE §9).
 * Only for wanted files with a speed (PLAN §6).
 */
export function formatEta(seconds: number): string {
  const minutes = Math.round(Math.max(0, seconds) / 60);
  if (minutes < 2) return 'about a minute left';
  if (minutes < 60) return `about ${minutes} min left`;
  const steps = Math.round(minutes / 10) * 10;
  const hours = Math.floor(steps / 60);
  if (hours < 24) return `about ${hours} h${steps % 60 ? ` ${steps % 60} min` : ''} left`;
  const wholeHours = Math.round(minutes / 60);
  const days = Math.floor(wholeHours / 24);
  return `about ${days} d${wholeHours % 24 ? ` ${wholeHours % 24} h` : ''} left`;
}

/** A StatusPill's props. */
export interface PillSpec {
  status: StatusKey;
  label?: string;
  /** The mini ring, for downloads. */
  pct?: number;
}

/**
 * The card's state pill (core.js `tcard`), first match:
 * - Streaming now; Paused;
 * - Downloading: of what its downloads want ("42%", or "Downloading 42%" large), else anacrolix's progress;
 * - idle with downloads: "On disk" once they're done, else why they wait ("Waits for the stream to end",
 *   "Waits until 01:00", "No peers · retrying", "Queued"…);
 * - Complete (only when it is), or Idle; also for an idle torrent whose downloads read `downloading`
 *   (the activity manager idled it a moment), as its health says.
 */
export function torrentPill(torrent: Pick<TorrentListItem, 'state' | 'progress' | 'complete' | 'wanted'>, size: 'md' | 'lg' = 'md'): PillSpec {
  const { wanted } = torrent;
  switch (torrent.state) {
    case 'streaming':
      return { status: 'streaming', label: 'Streaming now' };
    case 'paused':
      return { status: 'paused' };
    case 'downloading': {
      const pct = wanted ? wantedPct(wanted) : torrent.progress !== null ? Math.floor(torrent.progress * 100) : null;
      if (pct === null) return { status: 'downloading' };
      return { status: 'downloading', label: size === 'lg' ? `Downloading ${pct}%` : `${pct}%`, pct };
    }
    default:
      if (wanted?.state === 'done') return { status: 'on_disk' };
      if (wanted && wanted.state !== 'downloading') return keepStatePill(wanted.state, wantedPct(wanted), wanted.resumes_at);
      return torrent.complete ? { status: 'complete', label: size === 'lg' ? 'Complete · ready in Infuse' : undefined } : { status: 'idle' };
  }
}

/**
 * A torrent while some of its downloads wait out a Stop keeping's Undo: their
 * holds go, and once none is left nothing wants it any more (`wanted` null), as
 * the server will say once the DELETE lands.
 */
export function withoutHolds<T extends Pick<TorrentListItem, 'holds' | 'wanted'>>(torrent: T, released: (downloadId: number) => boolean): T {
  if (!torrent.holds.some((hold) => released(hold.download_id))) return torrent;
  const holds = torrent.holds.filter((hold) => !released(hold.download_id));
  return { ...torrent, holds, wanted: holds.length > 0 ? torrent.wanted : null };
}

/** The Kept and Smart pills: each reason once, kept first (`holds` is kept first, then by id). */
export function holdReasons(holds: readonly Pick<TorrentHold, 'reason'>[]): DownloadReason[] {
  const reasons = new Set(holds.map((hold) => hold.reason));
  return (['kept', 'smart'] as const).filter((reason) => reasons.has(reason));
}

/** What its downloads have on disk while they still want more: the bar, and "26.5 GB of 63.1 GB on disk · about 4 h 25 min left". */
export interface WantedProgress {
  pct: number;
  detail: string;
}

export function wantedProgress(torrent: Pick<TorrentListItem, 'wanted'>): WantedProgress | null {
  const { wanted } = torrent;
  if (!wanted || wanted.state === 'done' || wanted.bytes <= 0) return null;
  const onDisk = `${formatBytes(wanted.done_bytes)} of ${formatBytes(wanted.bytes)} on disk`;
  return { pct: wantedPct(wanted), detail: wanted.eta_seconds !== null ? `${onDisk} · ${formatEta(wanted.eta_seconds)}` : onDisk };
}

const forReasons = (reasons: readonly DownloadReason[]) =>
  reasons.length === 2 ? 'kept and smart downloads' : reasons[0] === 'smart' ? 'smart downloads' : 'kept downloads';

/**
 * The Downloading section's line: what its downloads still want, not the whole torrents
 * ("36.6 GB left for kept downloads"); nothing when none of them wants anything.
 */
export function downloadingLine(items: readonly Pick<TorrentListItem, 'wanted' | 'holds'>[]): string | undefined {
  const wanting = items.filter(wantsMore);
  const left = wanting.reduce((sum, item) => sum + Math.max(0, (item.wanted?.bytes ?? 0) - (item.wanted?.done_bytes ?? 0)), 0);
  if (left <= 0) return undefined;
  return `${formatBytes(left)} left for ${forReasons(holdReasons(wanting.flatMap((item) => item.holds)))}`;
}

/** The detail page's hold rows: what each download is on. */
const SCOPE_LABEL: Record<DownloadScope, string> = { movie: 'The movie', show: 'The whole show', season: 'A season', episode: 'An episode' };
export const holdScopeLabel = (scope: DownloadScope) => SCOPE_LABEL[scope];

// ---- The "Kept & smart" list (GET /downloads) -----------------------------------

/** It still wants bytes (keepWantsBytes): it shows a bar, and a paused banner is about it. */
export const downloadPending = (download: Pick<Download, 'state'>) => keepWantsBytes(download.state);

/** Hidden while its Stop keeping waits out the Undo. */
export const downloadRemovalKey = (id: number) => `download:${id}`;

/** What it's on, in one name: "Interstellar", "Severance · Season 2", "Severance · S02E03". */
export function downloadName(download: Pick<Download, 'scope' | 'item'>): string {
  const { item } = download;
  if (download.scope === 'season' && item.season_number !== null) return `${item.title} · ${item.season_number === 0 ? 'Specials' : `Season ${item.season_number}`}`;
  if (download.scope === 'episode' && item.season_number !== null && item.episode_number !== null) {
    return `${item.title} · ${episodeCode(item.season_number, item.episode_number)}`;
  }
  return item.title;
}

/**
 * Stop keeping, from the list (a 10 s Undo, then DELETE /downloads/{id}), in the
 * title pages' words (#206): a smart pick is dismissed, and isn't picked again.
 */
export function stopKeepingCopy(download: Pick<Download, 'reason' | 'scope' | 'item'>): { action: string; title: string; body: string; failed: string } {
  const name = downloadName(download);
  const body = 'What’s downloaded stays as cache until it’s cleared to make room.';
  return download.reason === 'smart'
    ? { action: 'Dismiss', title: `Dismissed the smart pick for ${name}`, body: `It won’t be downloaded ahead again. ${body}`, failed: `Couldn’t dismiss the smart pick for ${name}` }
    : { action: 'Stop keeping', title: `Stopped keeping ${name}`, body, failed: `Couldn’t stop keeping ${name}` };
}

/** 0–100 of what it wants; floored, 100 only when done. */
export function downloadPct(download: Pick<Download, 'state' | 'wanted_bytes' | 'done_bytes'>): number {
  return wantedPct({ state: download.state === 'done' ? 'done' : 'queued', bytes: download.wanted_bytes, done_bytes: download.done_bytes });
}

/** What it's on: the title, and the season or episode under it. */
export function downloadSubject(download: Pick<Download, 'scope' | 'item'>): { title: string; scope: string | null } {
  const { item } = download;
  switch (download.scope) {
    case 'movie':
      return { title: item.title, scope: null };
    case 'show':
      return { title: item.title, scope: 'Whole show' };
    case 'season':
      return { title: item.title, scope: item.season_number === 0 ? 'Specials' : `Season ${item.season_number ?? ''}`.trim() };
    case 'episode': {
      const code = item.season_number !== null && item.episode_number !== null ? episodeCode(item.season_number, item.episode_number) : null;
      return { title: item.title, scope: [code, item.episode_name].filter(Boolean).join(' · ') || null };
    }
  }
}

/** The files a show or season covers, when that says more than one file: "49 episodes · 2 play from HDRezka". */
function filesLine(download: Pick<Download, 'scope' | 'files'>): string | null {
  const { total, unavailable, over_cap: overCap } = download.files;
  const parts: string[] = [];
  if ((download.scope === 'show' || download.scope === 'season') && total > 0) parts.push(plural(total, 'episode'));
  if (unavailable > 0 && total > 1) parts.push(`${formatNumber(unavailable)} play${unavailable === 1 ? 's' : ''} from HDRezka`);
  if (overCap > 0) parts.push(`${plural(overCap, 'episode')} over the kept space`);
  return parts.length > 0 ? parts.join(' · ') : null;
}

/** "Kept until watched · by Nov 16" (keepExpiryLabel); a smart pick also says where it came from: "Smart pick · until watched · by Oct 31 · from Up next". */
export function downloadExpiry(download: Pick<Download, 'reason' | 'smart_source' | 'until_watched' | 'expires_at'>, now: Date = new Date()): string {
  const expiry = keepExpiryLabel(download, now);
  if (download.reason === 'kept' || !download.smart_source) return expiry;
  return `${expiry} · ${download.smart_source === 'streaming' ? 'after what’s streaming' : 'from Up next'}`;
}

/** One row of the "Kept & smart" list. */
export interface DownloadRow {
  title: string;
  scope: string | null;
  href: string;
  pill: PillSpec;
  /** The bar, while it still wants bytes it hasn't got. */
  pct: number | null;
  /** "26.5 GB of 63.1 GB · about 4 h 25 min left", "21.7 GB", or null with nothing assigned. */
  size: string | null;
  files: string | null;
  /** A stalled download's next try: "Tries again at 21:45". */
  retry: string | null;
  expiry: string;
}

export function downloadRow(download: Download, now: Date = new Date()): DownloadRow {
  const { title, scope } = downloadSubject(download);
  const pct = downloadPct(download);
  const pending = downloadPending(download) && download.wanted_bytes > 0;
  const size =
    download.wanted_bytes <= 0
      ? null
      : download.state === 'done'
        ? formatBytes(download.wanted_bytes)
        : [`${formatBytes(download.done_bytes)} of ${formatBytes(download.wanted_bytes)}`, download.eta_seconds !== null ? formatEta(download.eta_seconds) : null]
            .filter(Boolean)
            .join(' · ');
  return {
    title,
    scope,
    href: titleHref(download.item.media_type, download.item.tmdb_id),
    pill: { ...keepStatePill(download.state, pct, download.resumes_at), ...(download.state === 'downloading' ? { pct } : {}) },
    pct: pending && pct < 100 ? pct : null,
    size,
    files: filesLine(download),
    retry: download.state === 'stalled' && download.resumes_at ? `Tries again at ${formatTimeOfDay(Date.parse(download.resumes_at))}` : null,
    expiry: downloadExpiry(download, now),
  };
}

/** GET /downloads as the page shows it: the rows waiting out a Stop keeping's Undo left out. */
export type ShownList = Omit<DownloadList, 'items'> & { items: readonly Download[] };

/** The list's line: "48.8 GB of 375 GB kept · 1.9 GB of 112 GB smart", on disk against the caps. */
export function downloadsUsageLine(list: Pick<ShownList, 'items' | 'usage'>): string {
  const { usage } = list;
  const kept = `${formatBytes(usage.kept_bytes)} of ${formatBytes(usage.kept_max_bytes)} kept`;
  const hasSmart = usage.smart_bytes > 0 || list.items.some((download) => download.reason === 'smart');
  return hasSmart ? `${kept} · ${formatBytes(usage.smart_bytes)} of ${formatBytes(usage.smart_max_bytes)} smart` : kept;
}

/** Why background downloads wait now, when some download still wants bytes. */
export interface PausedBanner {
  title: string;
  body: string;
  /** Changed in Settings › Storage: the banner links there. */
  settings: boolean;
}

/**
 * Only while a download the reason holds still wants bytes: background downloads
 * off hold every one (`paused`), but the stream and the window don't hold one
 * whose torrent you paused.
 */
export function pausedBanner(list: Pick<ShownList, 'items' | 'paused_reason' | 'resumes_at'>): PausedBanner | null {
  const reason = list.paused_reason;
  if (!reason || !list.items.some((download) => downloadPending(download) && (reason === 'disabled' || download.state !== 'paused'))) return null;
  const at = list.resumes_at ? formatTimeOfDay(Date.parse(list.resumes_at)) : null;
  switch (reason) {
    case 'streaming':
      // resumes_at is set once the last stream ended: the minute's grace before they carry on
      return at
        ? { title: `Downloads resume at ${at}`, body: 'They wait a minute after a stream ends, in case it picks up again.', settings: false }
        : { title: 'Downloads wait for the stream to end', body: 'So Infuse gets the bandwidth. They pick up again a minute after it ends.', settings: false };
    case 'window':
      return {
        title: at ? `Downloads wait until ${at}` : 'Downloads wait for their window',
        body: `${at ? 'That’s when your next download window opens.' : 'Nothing downloads outside your download windows.'} What’s on disk still plays in Infuse.`,
        settings: true,
      };
    case 'disabled':
      return {
        title: 'Background downloads are off',
        body: 'Kept and smart downloads wait until you turn them back on. What’s on disk still plays in Infuse.',
        settings: true,
      };
  }
}
