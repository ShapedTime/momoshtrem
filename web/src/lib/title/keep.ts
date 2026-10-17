import type { DownloadDuration, DownloadEstimate, DownloadReason, DownloadState, Keep, KeepDays, StorageKeptSettings, Timestamp } from '@/lib/api/contract';
import { formatTimeOfDay } from '@/lib/activity/format';
import { DOWNLOAD_STATE_STATUS, STATUS, type StatusKey } from '@/lib/status';
import { formatBytes, formatShortDate } from './format';

/**
 * Download & keep (PLAN §6 "Downloads (#77)" and "Storage and downloads: UI
 * copy"): the words for a download's state and how long it lasts, the Keep
 * sheet's choices and its size line, and which pill a row shows. Pure, so
 * title pages and Activity say the same thing.
 */

const DAY = 86_400_000;
const clampPct = (pct: number) => Math.min(100, Math.max(0, pct));

/** The backend sends `keep` once downloads land (#202, #204); before that it's missing, which reads as none. */
export const keepOf = (keep: Keep | null | undefined): Keep | null => keep ?? null;

/** "Downloading 42%" (floored, so it never reads 100% before it's on disk), "Waits until 01:00" (browser time), else the state's word. */
export function keepStateLabel(state: DownloadState, pct: number, resumesAt: Timestamp | null): string {
  if (state === 'downloading') return `Downloading ${Math.floor(clampPct(pct))}%`;
  if (state === 'waiting_for_window' && resumesAt) return `Waits until ${formatTimeOfDay(Date.parse(resumesAt))}`;
  return STATUS[DOWNLOAD_STATE_STATUS[state]].label;
}

/** The pill for a download's state: its status key and words. */
export const keepStatePill = (state: DownloadState, pct: number, resumesAt: Timestamp | null): { status: StatusKey; label: string } => ({
  status: DOWNLOAD_STATE_STATUS[state],
  label: keepStateLabel(state, pct, resumesAt),
});

/**
 * How long it lasts: "Kept until watched", "Kept until Nov 16", "Kept until
 * watched · by Nov 16", or "Kept" with neither. A smart pick reads "Smart pick ·
 * until watched · by Oct 31" (always until watched, 14 days at most).
 */
export function keepExpiryLabel(keep: Pick<Keep, 'reason' | 'until_watched' | 'expires_at'>, now: Date = new Date()): string {
  const date = keep.expires_at ? formatShortDate(keep.expires_at, now) : null;
  if (keep.reason === 'smart') return ['Smart pick', keep.until_watched && 'until watched', date && `by ${date}`].filter(Boolean).join(' · ');
  if (keep.until_watched) return date ? `Kept until watched · by ${date}` : 'Kept until watched';
  return date ? `Kept until ${date}` : 'Kept';
}

/** States in which a download still wants bytes, so its progress means something. */
const WANTING: ReadonlySet<DownloadState> = new Set([
  'queued',
  'downloading',
  'waiting_for_stream',
  'waiting_for_window',
  'paused',
  'stalled',
  'disk_full',
  'storage_full',
]);

/** Show a progress bar: it still wants bytes (`done`, `no_file` and `unavailable` have nothing to show). */
export const keepWantsBytes = (state: DownloadState): boolean => WANTING.has(state);

/** "Kept" or "Smart". */
export const keepReasonPill = (reason: DownloadReason): { status: StatusKey; label: string } => ({ status: reason, label: STATUS[reason].label });

/**
 * The one keep pill an episode row has room for, beside its state pill: what holds it ("Kept",
 * "Smart"), unless the file isn't really held: from HDRezka ("Can't keep"), over the kept cap, or
 * the disk is full. Nothing while it has no file (the row already says missing or upcoming).
 */
export function keepRowPill(keep: Keep | null | undefined): { status: StatusKey; label: string } | null {
  if (!keep || keep.state === 'no_file') return null;
  if (keep.state === 'unavailable') return { status: 'not_keepable', label: 'Can’t keep' };
  if (keep.state === 'storage_full' || keep.state === 'disk_full') return keepStatePill(keep.state, keep.pct, null);
  return keepReasonPill(keep.reason);
}

/** A kept download's duration can change; a smart pick has to be kept first. */
export const canChangeDuration = (keep: Pick<Keep, 'reason'>): boolean => keep.reason === 'kept';

// ---- The Keep sheet ---------------------------------------------------------------

export type KeepChoice = 'until_watched' | 'days_7' | 'days_30' | 'days_90' | 'until_removed';

export interface KeepOption {
  value: KeepChoice;
  label: string;
  /** "30 days at most", "Until Oct 24"; null for none. */
  detail: string | null;
}

/** The sheet reads the defaults from /settings/storage's `kept`; until they load (or if they can't), the contract's. */
export type KeepDefaults = Pick<StorageKeptSettings, 'default_until_watched' | 'default_days'>;
export const CONTRACT_KEEP_DEFAULTS: KeepDefaults = { default_until_watched: true, default_days: 30 };

const DAYS: readonly KeepDays[] = [7, 30, 90];
const daysChoice = (days: KeepDays): KeepChoice => `days_${days}`;

/** "Until I watch it" lasts the default maximum when the default is until watched, else 30 days. */
function untilWatchedMaxDays(defaults: KeepDefaults): KeepDays | null {
  return defaults.default_until_watched ? defaults.default_days : 30;
}

/** The choice the sheet starts on: `kept.default_*` ("until watched, at most 30 days" out of the box). */
export function defaultKeepChoice(defaults: KeepDefaults = CONTRACT_KEEP_DEFAULTS): KeepChoice {
  if (defaults.default_until_watched) return 'until_watched';
  return defaults.default_days === null ? 'until_removed' : daysChoice(defaults.default_days);
}

/** The sheet's choices, in the spec's order: until watched, 7, 30 and 90 days, until removed. */
export function keepOptions(defaults: KeepDefaults = CONTRACT_KEEP_DEFAULTS, now: Date = new Date()): KeepOption[] {
  const max = untilWatchedMaxDays(defaults);
  return [
    { value: 'until_watched', label: 'Until I watch it', detail: max === null ? 'No time limit' : `${max} days at most` },
    ...DAYS.map((days) => ({
      value: daysChoice(days),
      label: `${days} days`,
      detail: `Until ${formatShortDate(new Date(now.getTime() + days * DAY).toISOString(), now)}`,
    })),
    { value: 'until_removed', label: 'Until I remove it', detail: 'No time limit' },
  ];
}

const isoIn = (now: Date, days: number) => new Date(now.getTime() + days * DAY).toISOString();

/** A choice as a duration from now: what PATCH sends to change one. */
export function keepDuration(choice: KeepChoice, defaults: KeepDefaults = CONTRACT_KEEP_DEFAULTS, now: Date = new Date()): Required<DownloadDuration> {
  switch (choice) {
    case 'until_watched': {
      const max = untilWatchedMaxDays(defaults);
      return { until_watched: true, expires_at: max === null ? null : isoIn(now, max) };
    }
    case 'until_removed':
      return { until_watched: false, expires_at: null };
    default:
      return { until_watched: false, expires_at: isoIn(now, Number(choice.slice('days_'.length))) };
  }
}

/** What POST sends: nothing for the default, so the server applies `kept.default_*` from its own clock; else the choice. */
export function keepCreateDuration(choice: KeepChoice, defaults: KeepDefaults | null, now: Date = new Date()): DownloadDuration {
  return choice === defaultKeepChoice(defaults ?? CONTRACT_KEEP_DEFAULTS) ? {} : keepDuration(choice, defaults ?? CONTRACT_KEEP_DEFAULTS, now);
}

export const NOT_DOWNLOADABLE_COPY = 'This plays from HDRezka, so it can’t be kept on disk. Pick a torrent for it to keep it.';

/** The 409's first sentence; the sheet adds "Free some kept downloads or raise the limit in Settings › Storage." with the link. */
export const storageFullLead = (needed: number, available: number) =>
  `Not enough space: needs ${formatBytes(needed)}, ${formatBytes(Math.max(0, available))} left.`;
export const STORAGE_FULL_HINT = 'Free some kept downloads or raise the limit in';
export const STORAGE_SETTINGS_HREF = '/settings?section=storage';

/** The Keep sheet's size line from GET /downloads/estimate, and whether the Keep can go ahead. */
export type EstimateCopy =
  | { kind: 'fits'; text: string }
  | { kind: 'full'; needed: number; available: number }
  | { kind: 'not_downloadable' };

export function estimateCopy(estimate: DownloadEstimate): EstimateCopy {
  const { wanted_bytes: wanted, needed_bytes: needed, available_bytes: available, downloadable } = estimate;
  if (!downloadable) return { kind: 'not_downloadable' };
  if (needed > available) return { kind: 'full', needed, available };
  const left = `${formatBytes(Math.max(0, available))} left for kept downloads`;
  if (wanted === 0) return { kind: 'fits', text: `Nothing to download yet: it starts once there’s a file · ${left}` };
  if (needed === 0) return { kind: 'fits', text: `Needs no more space: another keep has these files · ${left}` };
  return { kind: 'fits', text: `Needs ${formatBytes(needed)} · ${left}` };
}
