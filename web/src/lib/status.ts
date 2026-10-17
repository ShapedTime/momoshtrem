import {
  ArrowDown,
  Ban,
  Calendar,
  Check,
  CircleDashed,
  CircleAlert,
  Clock,
  EyeOff,
  HardDrive,
  Hourglass,
  Layers,
  type LucideIcon,
  Minus,
  Pause,
  Pin,
  Play,
  Plus,
  Search,
  Sparkle,
  Sparkles,
  TriangleAlert,
  Tv,
  WifiOff,
} from 'lucide-react';
import type { DownloadState } from '@/lib/api/contract';

/**
 * The one status vocabulary. Every surface (cards, rows, detail panels,
 * activity, settings) takes its wording and icon from here, so the grid
 * never says "In Library" while the detail page says "Ready to Stream".
 * Mirrors STATUS in docs/redesign/design-cinematic/js/core.js.
 */

/** Visual tone; maps to a `.pill--{tone}` class and the --st-* colours. */
export type StatusTone =
  | 'ready'
  | 'downloading'
  | 'missing'
  | 'review'
  | 'watched'
  | 'upcoming'
  | 'warning'
  | 'error'
  | 'neutral'
  | 'accent';

export interface StatusDefinition {
  tone: StatusTone;
  label: string;
  icon: LucideIcon;
}

export const STATUS = {
  ready: { tone: 'ready', label: 'Ready', icon: Play },
  ready_infuse: { tone: 'ready', label: 'Ready in Infuse', icon: Play },
  downloading: { tone: 'downloading', label: 'Downloading', icon: ArrowDown },
  partial: { tone: 'downloading', label: 'Partial', icon: Layers },
  needs_torrent: { tone: 'missing', label: 'Needs a torrent', icon: Search },
  missing: { tone: 'missing', label: 'Missing', icon: Minus },
  not_in_library: { tone: 'neutral', label: 'Not in library', icon: Plus },
  airs_on: { tone: 'upcoming', label: 'Airs', icon: Calendar },
  upcoming: { tone: 'upcoming', label: 'Upcoming', icon: Calendar },
  needs_review: { tone: 'review', label: 'Needs review', icon: CircleAlert },
  watched: { tone: 'watched', label: 'Watched', icon: Check },
  streaming: { tone: 'accent', label: 'Streaming', icon: Tv },
  paused: { tone: 'neutral', label: 'Paused', icon: Pause },
  idle: { tone: 'neutral', label: 'Idle', icon: Clock },
  ok: { tone: 'ready', label: 'OK', icon: Check },
  unreachable: { tone: 'error', label: 'Unreachable', icon: WifiOff },
  not_configured: { tone: 'neutral', label: 'Not set up', icon: CircleDashed },
  warning: { tone: 'warning', label: 'Warning', icon: TriangleAlert },
  error: { tone: 'error', label: 'Error', icon: TriangleAlert },
  // Torrents
  healthy: { tone: 'ready', label: 'Healthy', icon: Check },
  no_seeders: { tone: 'warning', label: 'No seeders', icon: TriangleAlert },
  added: { tone: 'ready', label: 'Added', icon: Check },
  // Activity (#102): "Complete" only when every piece is on disk; trackers anacrolix doesn't report on
  complete: { tone: 'ready', label: 'Complete', icon: Check },
  few_peers: { tone: 'warning', label: 'Few peers', icon: TriangleAlert },
  working: { tone: 'ready', label: 'Working', icon: Check },
  status_unknown: { tone: 'neutral', label: 'Status unknown', icon: CircleDashed },
  // How well a subtitle fits the release (B7)
  match_exact: { tone: 'ready', label: 'Exact release', icon: Check },
  match_same_source: { tone: 'neutral', label: 'Same source', icon: Check },
  match_different_timing: { tone: 'warning', label: 'Different cut timing', icon: TriangleAlert },
  match_unknown: { tone: 'neutral', label: 'Match unknown', icon: CircleDashed },
  // A file in a mapping job (#5): where it goes, or why it stays out of Infuse
  matched: { tone: 'ready', label: 'Matched', icon: Check },
  special: { tone: 'upcoming', label: 'Special', icon: Sparkle },
  unmatched: { tone: 'warning', label: 'Unmatched', icon: CircleAlert },
  ignored: { tone: 'missing', label: 'Ignored', icon: EyeOff },
  // Downloads (#77): a download's reason and state (PLAN §6 "Storage and downloads: UI copy"); `downloading` and `paused` above
  kept: { tone: 'watched', label: 'Kept', icon: Pin },
  smart: { tone: 'upcoming', label: 'Smart', icon: Sparkles },
  queued: { tone: 'neutral', label: 'Queued', icon: Hourglass },
  waiting_for_stream: { tone: 'neutral', label: 'Waits for the stream to end', icon: Tv },
  waiting_for_window: { tone: 'neutral', label: 'Waits for its window', icon: Clock },
  stalled: { tone: 'warning', label: 'No peers · retrying', icon: TriangleAlert },
  disk_full: { tone: 'error', label: 'Disk full', icon: HardDrive },
  storage_full: { tone: 'warning', label: 'Kept space is full', icon: HardDrive },
  on_disk: { tone: 'ready', label: 'On disk', icon: HardDrive },
  no_file: { tone: 'missing', label: 'No file yet', icon: CircleDashed },
  not_keepable: { tone: 'neutral', label: 'Can’t keep this file', icon: Ban },
} as const satisfies Record<string, StatusDefinition>;

export type StatusKey = keyof typeof STATUS;

export function getStatus(key: StatusKey): StatusDefinition {
  return STATUS[key];
}

export function isStatusKey(value: string): value is StatusKey {
  return Object.prototype.hasOwnProperty.call(STATUS, value);
}

/** A download's state (PLAN §6 "Downloads (#77)") as a pill. `waiting_for_window` reads "Waits until 01:00" with `resumes_at` (lib/title/keep.ts `keepStateLabel`). */
export const DOWNLOAD_STATE_STATUS = {
  queued: 'queued',
  downloading: 'downloading',
  waiting_for_stream: 'waiting_for_stream',
  waiting_for_window: 'waiting_for_window',
  paused: 'paused',
  stalled: 'stalled',
  disk_full: 'disk_full',
  storage_full: 'storage_full',
  done: 'on_disk',
  no_file: 'no_file',
  unavailable: 'not_keepable',
} as const satisfies Record<DownloadState, StatusKey>;
