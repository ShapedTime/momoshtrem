import {
  ArrowDown,
  Calendar,
  Check,
  CircleDashed,
  CircleAlert,
  Clock,
  Layers,
  type LucideIcon,
  Minus,
  Pause,
  Play,
  Plus,
  Search,
  TriangleAlert,
  Tv,
  WifiOff,
} from 'lucide-react';

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
  // How well a subtitle fits the release (B7)
  match_exact: { tone: 'ready', label: 'Exact release', icon: Check },
  match_same_source: { tone: 'neutral', label: 'Same source', icon: Check },
  match_different_timing: { tone: 'warning', label: 'Different cut timing', icon: TriangleAlert },
  match_unknown: { tone: 'neutral', label: 'Match unknown', icon: CircleDashed },
} as const satisfies Record<string, StatusDefinition>;

export type StatusKey = keyof typeof STATUS;

export function getStatus(key: StatusKey): StatusDefinition {
  return STATUS[key];
}

export function isStatusKey(value: string): value is StatusKey {
  return Object.prototype.hasOwnProperty.call(STATUS, value);
}
