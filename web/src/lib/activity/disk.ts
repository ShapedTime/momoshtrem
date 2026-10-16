import type { TorrentsSummary } from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';

type Disk = NonNullable<TorrentsSummary['disk']>;

/**
 * The summary strip's disk tile (B6 `disk`, #77): the bar is the piece cache's
 * fill split into kept, smart and streaming cache, then the rest of the disk in
 * use, then free, with a marker at the cap the cache applies now
 * (`cache_limit_bytes`). Every share is a percentage of the disk.
 */
export interface DiskBar {
  segments: { kind: 'kept' | 'smart' | 'cache' | 'other'; bytes: number; width: string }[];
  /** Where the cache limit sits along the bar. */
  limitAt: string;
  /** Read out for the bar: what each part holds, and the limit. */
  label: string;
  /** "62.4 GB of 750 GB cache · 1 TB disk" */
  cacheLine: string;
  /** "48.8 GB kept" and "1.9 GB smart", when downloads pin anything. */
  pinned: { kept: string; smart: string } | null;
}

const percent = (bytes: number, total: number) => `${total > 0 ? Math.min(100, Math.max(0, (bytes / total) * 100)).toFixed(1) : '0'}%`;

export function diskBar(disk: Disk): DiskBar {
  const kept = Math.max(0, disk.kept_bytes);
  const smart = Math.max(0, disk.smart_bytes);
  const cache = Math.max(0, disk.torrents_bytes - kept - smart);
  const other = Math.max(0, disk.total_bytes - disk.free_bytes - disk.torrents_bytes);
  const parts = [
    { kind: 'kept' as const, bytes: kept },
    { kind: 'smart' as const, bytes: smart },
    { kind: 'cache' as const, bytes: cache },
    { kind: 'other' as const, bytes: other },
  ];
  const pinned = kept > 0 || smart > 0;
  const read = [
    pinned && `${formatBytes(kept)} kept`,
    pinned && `${formatBytes(smart)} smart`,
    `${formatBytes(cache)} ${pinned ? 'streaming cache' : 'in torrents'}`,
    `${formatBytes(other)} other`,
    `${formatBytes(disk.free_bytes)} free of ${formatBytes(disk.total_bytes)}`,
  ]
    .filter(Boolean)
    .join(', ');
  return {
    // Kept and smart only once something is pinned, so a backend without downloads draws today's bar
    segments: parts.filter((part) => part.kind === 'cache' || part.kind === 'other' || part.bytes > 0).map((part) => ({ ...part, width: percent(part.bytes, disk.total_bytes) })),
    limitAt: percent(disk.cache_limit_bytes, disk.total_bytes),
    label: `Disk: ${read}; the cache may grow to ${formatBytes(disk.cache_limit_bytes)}`,
    cacheLine: `${formatBytes(disk.torrents_bytes)} of ${formatBytes(disk.cache_limit_bytes)} cache · ${formatBytes(disk.total_bytes)} disk`,
    pinned: pinned ? { kept: `${formatBytes(kept)} kept`, smart: `${formatBytes(smart)} smart` } : null,
  };
}
