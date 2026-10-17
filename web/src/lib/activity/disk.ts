import type { TorrentsSummary } from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';

type Disk = NonNullable<TorrentsSummary['disk']>;

export type DiskPart = 'kept' | 'smart' | 'cache' | 'other';

/**
 * The summary strip's disk tile (B6 `disk`, #77): the bar is the piece cache's
 * fill split into kept, smart and streaming cache, then the rest of the disk in
 * use, then free (the track), with a marker at the cap the cache applies now
 * (`cache_limit_bytes`). Every share is a percentage of the disk. The colours
 * are Settings › Storage's (#205): kept and smart in the accent, the cache cool,
 * other files grey.
 */
export interface DiskBar {
  segments: { kind: DiskPart; bytes: number; width: string }[];
  /** Where the cache limit sits along the bar. */
  limitAt: string;
  /** Read out for the bar: what each part holds, and the limit. */
  label: string;
  /** "62.4 GB of 750 GB limit": the cache's fill against the marker. */
  limitLine: string;
  /** The swatches under the bar: "48.8 GB kept", "1.9 GB smart" (once pinned) and "11.7 GB cache". */
  legend: { kind: DiskPart; text: string }[];
}

const percent = (bytes: number, total: number) => `${total > 0 ? Math.min(100, Math.max(0, (bytes / total) * 100)).toFixed(1) : '0'}%`;

export function diskBar(disk: Disk): DiskBar {
  const kept = Math.max(0, disk.kept_bytes);
  const smart = Math.max(0, disk.smart_bytes);
  const cache = Math.max(0, disk.torrents_bytes - kept - smart);
  const other = Math.max(0, disk.total_bytes - disk.free_bytes - disk.torrents_bytes);
  const pinned = kept > 0 || smart > 0;
  // Kept and smart only once something is pinned, so a backend without downloads draws today's bar
  const parts = [
    { kind: 'kept' as const, bytes: kept, text: `${formatBytes(kept)} kept` },
    { kind: 'smart' as const, bytes: smart, text: `${formatBytes(smart)} smart` },
    { kind: 'cache' as const, bytes: cache, text: `${formatBytes(cache)} cache` },
    { kind: 'other' as const, bytes: other, text: `${formatBytes(other)} other` },
  ].filter((part) => part.kind === 'cache' || part.kind === 'other' || part.bytes > 0);
  const read = [
    ...parts.map((part) => (part.kind === 'cache' ? `${formatBytes(cache)} ${pinned ? 'streaming cache' : 'in torrents'}` : part.text)),
    `${formatBytes(disk.free_bytes)} free of ${formatBytes(disk.total_bytes)}`,
  ].join(', ');
  return {
    segments: parts.map(({ kind, bytes }) => ({ kind, bytes, width: percent(bytes, disk.total_bytes) })),
    limitAt: percent(disk.cache_limit_bytes, disk.total_bytes),
    label: `Disk: ${read}; the cache may grow to ${formatBytes(disk.cache_limit_bytes)}`,
    limitLine: `${formatBytes(disk.torrents_bytes)} of ${formatBytes(disk.cache_limit_bytes)} limit`,
    legend: parts.filter((part) => part.kind !== 'other').map(({ kind, text }) => ({ kind, text })),
  };
}
