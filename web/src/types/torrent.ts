/**
 * Torrent status and management types.
 * Maps to momoshtrem /api/torrents API responses.
 */

import type { TorrentListItem } from '@/lib/api/contract/torrents';

/**
 * Torrent status from momoshtrem API.
 * Matches the TorrentResponse structure from the backend.
 */
export interface TorrentStatus {
  info_hash: string;
  name: string;
  total_size: number;
  downloaded: number;
  progress: number; // 0.0 to 1.0
  seeders: number;
  leechers: number;
  download_speed: number; // bytes/sec
  upload_speed: number; // bytes/sec
  is_paused: boolean;
}

/**
 * Response from GET /api/torrents
 */
export interface TorrentListResponse {
  torrents: TorrentStatus[];
}

/**
 * The legacy status of one of the backend's GET /api/torrents items (B6), so
 * the legacy /torrents page and the Activity badge keep working until #102.
 * What only the loaded client knows is 0 for a torrent that isn't loaded, and
 * `is_paused` means "network off", as it did: paused or idle.
 */
export function legacyTorrentStatus(item: TorrentListItem): TorrentStatus {
  const seeders = item.seeders ?? 0;
  return {
    info_hash: item.info_hash,
    name: item.name,
    total_size: item.size_bytes ?? 0,
    downloaded: item.bytes_completed ?? 0,
    progress: item.progress ?? 0,
    seeders,
    leechers: Math.max(0, (item.peers ?? 0) - seeders),
    download_speed: item.download_bps,
    upload_speed: item.upload_bps,
    is_paused: item.state === 'paused' || item.state === 'idle',
  };
}

/**
 * Display status derived from TorrentStatus for UI rendering.
 */
export type TorrentDisplayStatus = 'downloading' | 'seeding' | 'paused' | 'none';

/**
 * Derive display status from a TorrentStatus object.
 */
export function getTorrentDisplayStatus(status: TorrentStatus | null | undefined): TorrentDisplayStatus {
  if (!status) return 'none';
  if (status.is_paused) return 'paused';
  if (status.progress >= 1) return 'seeding';
  return 'downloading';
}

/**
 * Format bytes to human readable string.
 * @param bytes - Size in bytes
 * @param decimals - Decimal places (default 1)
 * @returns Formatted string (e.g., "1.5 GB")
 */
export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`;
}

/**
 * Format bytes per second to speed string.
 * @param bytesPerSec - Speed in bytes/second
 * @returns Formatted string (e.g., "5.2 MB/s")
 */
export function formatSpeed(bytesPerSec: number): string {
  return `${formatBytes(bytesPerSec)}/s`;
}

/**
 * Format progress as percentage string.
 * @param progress - Progress 0-1
 * @returns Formatted percentage (e.g., "85.5%")
 */
export function formatProgress(progress: number): string {
  return `${(progress * 100).toFixed(1)}%`;
}
