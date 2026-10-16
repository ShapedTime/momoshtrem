import type { DownloadWindow, KeepDays, StorageLimitSource, StorageSettings, StorageSettingsPatch } from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';

/**
 * Settings › Storage (PLAN §6 "Storage (#77)" and "Storage and downloads: UI
 * copy"): the draft the section edits and the partial PUT it sends, the size
 * inputs, the download windows, the disk bar and the words around them. Pure,
 * so Jest covers it; components/settings/StorageSection renders it, and the
 * settings nav reads its line here.
 *
 * Sizes are decimal, as `formatBytes` writes them everywhere else: the inputs
 * take gigabytes (1 GB = 10⁹ bytes).
 */

const GB = 1e9;

/** How the limit is chosen. `config` is config.yaml's, offered only while it's the source. */
export type LimitMode = 'auto' | 'custom' | 'config';

export const KEEP_DAYS: readonly KeepDays[] = [7, 30, 90];
export const EPISODES_AHEAD = [1, 2, 3, 4, 5] as const;
export const MAX_WINDOWS = 4;
/** A new window's times: overnight, when nobody streams. */
export const NEW_WINDOW: DownloadWindow = { start: '01:00', end: '07:00' };

const SOURCE_MODE: Record<StorageLimitSource, LimitMode> = { setting: 'custom', config: 'config', auto: 'auto' };

// ---- Sizes ----------------------------------------------------------------------

const SIZE_INPUT = new Intl.NumberFormat('en-US', { useGrouping: false, maximumFractionDigits: 1 });

/** A size as its input shows it, in GB: 300e9 → "300", 56_371_445_760 → "56.4". */
export function formatSizeInput(bytes: number): string {
  return SIZE_INPUT.format(bytes / GB);
}

const UNITS: Record<string, number> = { mb: 1e6, gb: GB, tb: 1e12, mib: 2 ** 20, gib: 2 ** 30, tib: 2 ** 40 };

/**
 * Read a size input: "150", "52.5", "52.5 GB", "1.2 TB", "500 MB" or "48 GiB",
 * in whole bytes. A bare number is gigabytes. Empty is null (the field's
 * default); anything else that doesn't read as a size is undefined.
 */
export function parseSize(text: string): number | null | undefined {
  const value = text.trim();
  if (value === '') return null;
  const match = /^(\d+(?:\.\d*)?|\.\d+)\s*([a-z]+)?$/i.exec(value);
  if (!match) return undefined;
  const unit = match[2] ? UNITS[match[2].toLowerCase()] : GB;
  if (unit === undefined) return undefined;
  const bytes = Math.round(Number(match[1]) * unit);
  return Number.isSafeInteger(bytes) ? bytes : undefined;
}

/** A size in GB to a tenth, rounded the way given: 189.67e9 down is "189.6". */
const tenths = (bytes: number, round: (value: number) => number) => SIZE_INPUT.format(round(bytes / (GB / 10)) / 10);

/**
 * The limit's bounds as the input's help says them, in its unit and precision:
 * the minimum rounded up and the maximum down, so typing what it says always fits.
 */
export function limitBounds(settings: Pick<StorageSettings, 'min_bytes' | 'max_bytes'>): { min: string; max: string } {
  return { min: `${tenths(settings.min_bytes, Math.ceil)} GB`, max: `${tenths(settings.max_bytes, Math.floor)} GB` };
}

/**
 * The custom input's first text when the limit isn't custom yet: the limit that
 * applies now, held inside the bounds as the help says them (a limit clamped at
 * the maximum would otherwise round up past it).
 */
function limitPrefill(settings: Pick<StorageSettings, 'limit_effective_bytes' | 'min_bytes' | 'max_bytes'>): string {
  const text = formatSizeInput(settings.limit_effective_bytes);
  const bytes = parseSize(text) ?? 0;
  if (bytes > settings.max_bytes) return tenths(settings.max_bytes, Math.floor);
  if (bytes < settings.min_bytes) return tenths(settings.min_bytes, Math.ceil);
  return text;
}

// ---- Download windows ---------------------------------------------------------------

const HHMM = /^([01]\d|2[0-3]):[0-5]\d$/;

export interface WindowProblem {
  /** The window it's about, from 0. */
  index: number;
  message: string;
}

/** The server's rules (PLAN §6): at most 4, each two different HH:MM times. A window may cross midnight. */
export function windowsProblem(windows: readonly DownloadWindow[]): WindowProblem | null {
  if (windows.length > MAX_WINDOWS) return { index: MAX_WINDOWS, message: `Keep it to ${MAX_WINDOWS} windows or fewer.` };
  for (const [index, window] of windows.entries()) {
    if (!HHMM.test(window.start) || !HHMM.test(window.end)) {
      return { index, message: `Window ${index + 1} needs a start and an end time, as HH:MM.` };
    }
    if (window.start === window.end) {
      return { index, message: `Window ${index + 1} starts when it ends. Pick two different times.` };
    }
  }
  return null;
}

/**
 * Where focus goes once window `index` of `count` is removed (its button goes
 * with it): the next window's Remove, else the previous one's (both by their
 * index in the shorter list), else Add.
 */
export function focusAfterRemove(index: number, count: number): number | 'add' {
  if (index < count - 1) return index;
  return index > 0 ? index - 1 : 'add';
}

/** "23:00"–"06:00" ends the next day. */
export const crossesMidnight = (window: DownloadWindow): boolean => HHMM.test(window.start) && HHMM.test(window.end) && window.end < window.start;

const sameWindows = (a: readonly DownloadWindow[], b: readonly DownloadWindow[]) =>
  a.length === b.length && a.every((window, i) => window.start === b[i].start && window.end === b[i].end);

// ---- The draft and the partial PUT ------------------------------------------------------

/** What the section edits: the settings, with sizes as typed (GB). */
export interface StorageDraft {
  limitMode: LimitMode;
  /** The custom limit's text. */
  limit: string;
  /** Empty: half the limit. */
  keptMax: string;
  untilWatched: boolean;
  days: KeepDays | null;
  smartEnabled: boolean;
  episodesAhead: number;
  /** Empty: 15% of the limit. */
  smartMax: string;
  backgroundEnabled: boolean;
  pauseWhileStreaming: boolean;
  windows: DownloadWindow[];
}

/** The fields a 422 can name (PLAN §6 "Storage (#77)"), and how the section names them. */
export const FIELD_LABELS = {
  limit_bytes: 'Storage limit',
  'kept.max_bytes': 'Space for kept downloads',
  'kept.default_until_watched': 'Keep until watched',
  'kept.default_days': 'Keep for at most',
  'smart.enabled': 'Smart downloads',
  'smart.episodes_ahead': 'Episodes ahead',
  'smart.max_bytes': 'Space for smart downloads',
  'background.enabled': 'Background downloads',
  'background.pause_while_streaming': 'Pause while streaming',
  'background.windows': 'Download windows',
} as const;

export type StorageField = keyof typeof FIELD_LABELS;

export function isStorageField(value: unknown): value is StorageField {
  return typeof value === 'string' && Object.hasOwn(FIELD_LABELS, value);
}

const sizeText = (bytes: number | null) => (bytes === null ? '' : formatSizeInput(bytes));

export function draftOf(settings: StorageSettings): StorageDraft {
  return {
    limitMode: SOURCE_MODE[settings.source],
    // Custom starts from the stored limit, else from the one that applies now
    limit: settings.source === 'setting' && settings.limit_bytes !== null ? formatSizeInput(settings.limit_bytes) : limitPrefill(settings),
    keptMax: sizeText(settings.kept.max_bytes),
    untilWatched: settings.kept.default_until_watched,
    days: settings.kept.default_days,
    smartEnabled: settings.smart.enabled,
    episodesAhead: settings.smart.episodes_ahead,
    smartMax: sizeText(settings.smart.max_bytes),
    backgroundEnabled: settings.background.enabled,
    pauseWhileStreaming: settings.background.pause_while_streaming,
    windows: settings.background.windows.map((window) => ({ ...window })),
  };
}

/** What's on screen: the saved settings with this page's edits on top (untouched fields follow the server). */
export function withEdits(saved: StorageSettings, edits: Partial<StorageDraft>): StorageDraft {
  return { ...draftOf(saved), ...edits };
}

export const SIZE_ERRORS = {
  limit: 'Enter a size in GB, like 150 or 52.5, or choose Auto.',
  limitRange: (bounds: { min: string; max: string }) => `Enter a size from ${bounds.min} to ${bounds.max}.`,
  kept: 'Enter a size in GB, like 100 or 37.5, or leave it empty for half the limit.',
  smart: 'Enter a size in GB, like 20 or 7.5, or leave it empty for 15% of the limit.',
} as const;

/** The fields with a problem Save can't send, by field. */
export type DraftErrors = Partial<Record<StorageField, string>>;

export interface StorageChanges {
  /** Only the edited fields that differ from what was loaded. */
  patch: StorageSettingsPatch;
  /** Inputs that don't read: nothing is saved until they do. */
  errors: DraftErrors;
  /** The window the windows' problem is about. */
  windowIndex: number | null;
}

/**
 * The limit's part of the PUT. `limit_bytes` goes only when the limit control
 * changed: GET's null may be config.yaml's limit, which gives way as soon as a
 * PUT sends `limit_bytes`, so it's never echoed back. A custom size still
 * showing the saved one as the input formats it is unchanged.
 */
function limitChange(edits: Partial<StorageDraft>, saved: StorageSettings): { bytes?: number | null; error?: string } {
  const savedMode = SOURCE_MODE[saved.source];
  const mode = edits.limitMode ?? savedMode;
  if (mode === 'config') return {};
  if (mode === 'auto') return savedMode === 'auto' ? {} : { bytes: null };
  const text = edits.limit ?? draftOf(saved).limit;
  if (savedMode === 'custom' && saved.limit_bytes !== null && text === formatSizeInput(saved.limit_bytes)) return {};
  const bytes = parseSize(text);
  if (bytes === undefined || bytes === null) return { error: SIZE_ERRORS.limit };
  if (bytes < saved.min_bytes || bytes > saved.max_bytes) return { error: SIZE_ERRORS.limitRange(limitBounds(saved)) };
  return bytes === saved.limit_bytes && savedMode === 'custom' ? {} : { bytes };
}

/** A cap's part: empty is the default share (null). A cap still showing the saved value as the input formats it is unchanged. */
function capChange(text: string | undefined, saved: number | null, error: string): { bytes?: number | null; error?: string } {
  if (text === undefined || text === sizeText(saved)) return {};
  const bytes = parseSize(text);
  if (bytes === undefined) return { error };
  return bytes === saved ? {} : { bytes };
}

/**
 * What Save would send: a partial PUT of the fields this page edited and that
 * differ from the server's (nested objects merge on the server, `windows` goes
 * whole). Untouched fields are never sent, so a change made elsewhere meanwhile
 * stays. Inputs that don't read stay here with a reason.
 */
export function storageChanges(edits: Partial<StorageDraft>, saved: StorageSettings): StorageChanges {
  const patch: StorageSettingsPatch = {};
  const errors: DraftErrors = {};
  let windowIndex: number | null = null;

  const limit = limitChange(edits, saved);
  if (limit.error) errors.limit_bytes = limit.error;
  else if (limit.bytes !== undefined) patch.limit_bytes = limit.bytes;

  const kept: NonNullable<StorageSettingsPatch['kept']> = {};
  const keptMax = capChange(edits.keptMax, saved.kept.max_bytes, SIZE_ERRORS.kept);
  if (keptMax.error) errors['kept.max_bytes'] = keptMax.error;
  else if (keptMax.bytes !== undefined) kept.max_bytes = keptMax.bytes;
  if (edits.untilWatched !== undefined && edits.untilWatched !== saved.kept.default_until_watched) kept.default_until_watched = edits.untilWatched;
  if (edits.days !== undefined && edits.days !== saved.kept.default_days) kept.default_days = edits.days;
  if (Object.keys(kept).length > 0) patch.kept = kept;

  const smart: NonNullable<StorageSettingsPatch['smart']> = {};
  if (edits.smartEnabled !== undefined && edits.smartEnabled !== saved.smart.enabled) smart.enabled = edits.smartEnabled;
  if (edits.episodesAhead !== undefined && edits.episodesAhead !== saved.smart.episodes_ahead) smart.episodes_ahead = edits.episodesAhead;
  const smartMax = capChange(edits.smartMax, saved.smart.max_bytes, SIZE_ERRORS.smart);
  if (smartMax.error) errors['smart.max_bytes'] = smartMax.error;
  else if (smartMax.bytes !== undefined) smart.max_bytes = smartMax.bytes;
  if (Object.keys(smart).length > 0) patch.smart = smart;

  const background: NonNullable<StorageSettingsPatch['background']> = {};
  if (edits.backgroundEnabled !== undefined && edits.backgroundEnabled !== saved.background.enabled) background.enabled = edits.backgroundEnabled;
  if (edits.pauseWhileStreaming !== undefined && edits.pauseWhileStreaming !== saved.background.pause_while_streaming) {
    background.pause_while_streaming = edits.pauseWhileStreaming;
  }
  if (edits.windows !== undefined && !sameWindows(edits.windows, saved.background.windows)) {
    const problem = windowsProblem(edits.windows);
    if (problem) {
      errors['background.windows'] = problem.message;
      windowIndex = problem.index;
    } else {
      background.windows = edits.windows.map(({ start, end }) => ({ start, end }));
    }
  }
  if (Object.keys(background).length > 0) patch.background = background;

  return { patch, errors, windowIndex };
}

/** Something differs from what was loaded (an input that doesn't read counts). */
export const hasChanges = ({ patch, errors }: StorageChanges): boolean => Object.keys(patch).length > 0 || Object.keys(errors).length > 0;

/** The first field with a problem, in the page's order. */
export function firstError(errors: DraftErrors): StorageField | null {
  return (Object.keys(FIELD_LABELS) as StorageField[]).find((field) => errors[field] !== undefined) ?? null;
}

// ---- Errors from the server ---------------------------------------------------------------

const ROOM = 'Kept and smart downloads together get the limit less the streaming reserve.';

/**
 * A 422's message for the field it names, in words (the server's are for
 * developers): the ranges that depend on the disk say what has to fit. A
 * limit outside the bounds (the disk shrank since they were read) gets them;
 * otherwise a limit fails because it squeezes the caps. The other fields
 * can't fail from this page; they keep the server's words.
 */
export function fieldErrorCopy(
  field: StorageField,
  message: string,
  limit?: { sent: number | null | undefined; settings: Pick<StorageSettings, 'min_bytes' | 'max_bytes'> }
): string {
  switch (field) {
    case 'limit_bytes':
      if (limit && typeof limit.sent === 'number' && (limit.sent < limit.settings.min_bytes || limit.sent > limit.settings.max_bytes)) {
        return SIZE_ERRORS.limitRange(limitBounds(limit.settings));
      }
      return 'This limit leaves too little for the space you set for kept or smart downloads. Lower that too, or choose a larger limit.';
    case 'kept.max_bytes':
      return `${ROOM} Enter less here, or lower the space for smart downloads.`;
    case 'smart.max_bytes':
      return `${ROOM} Enter less here, or leave it empty for 15% of the limit.`;
    case 'background.windows':
      return 'Each window needs a start and an end at two different times, and there can be 4 at most.';
    default:
      return message;
  }
}

/** PUT's 409 `limit_below_committed` from a lower limit: the change would leave kept downloads less than they need. */
export function belowCommittedCopy(committedKeptBytes: number): string {
  return `Kept downloads need ${formatBytes(committedKeptBytes)}. Stop keeping some, or set a limit that leaves room for them.`;
}

/** The same 409 when the PUT lowered the kept cap itself: it shows on that field. */
export function belowCommittedCapCopy(committedKeptBytes: number): string {
  return `Kept downloads need ${formatBytes(committedKeptBytes)}. Enter at least that here, or stop keeping some.`;
}

/** What a refused PUT carries (`ApiError`). */
export interface SaveError {
  status: number;
  code: string;
  field?: string;
  message: string;
  details: Readonly<Record<string, unknown>>;
}

/**
 * The field a refused PUT is about, and its words: a 422's field, or the kept
 * cap for a 409 `limit_below_committed` when the PUT sent one (the kept cap
 * moves only with the limit and `kept.max_bytes`). Null: the problem belongs
 * in the save bar (a 409 from the limit, anything else).
 */
export function fieldProblem(
  error: SaveError,
  sent: StorageSettingsPatch | undefined,
  settings?: Pick<StorageSettings, 'min_bytes' | 'max_bytes'>
): { field: StorageField; text: string } | null {
  if (error.status === 422 && isStorageField(error.field)) {
    const limit = settings ? { sent: sent?.limit_bytes, settings } : undefined;
    return { field: error.field, text: fieldErrorCopy(error.field, error.message, limit) };
  }
  const committed = error.details.committed_kept_bytes;
  if (error.status === 409 && error.code === 'limit_below_committed' && typeof committed === 'number' && sent?.kept?.max_bytes !== undefined) {
    return { field: 'kept.max_bytes', text: belowCommittedCapCopy(committed) };
  }
  return null;
}

// ---- The limit's words ----------------------------------------------------------------------

/** "Auto (75% of the disk) · 750 GB". */
export const autoLine = (settings: Pick<StorageSettings, 'auto_bytes'>): string => `Auto (75% of the disk) · ${formatBytes(settings.auto_bytes)}`;

/** "From config.yaml · 56.4 GB": the limit config.yaml's `global_cache_size` gives, as it applies (clamped). */
export const configLine = (settings: Pick<StorageSettings, 'limit_effective_bytes'>): string => `From config.yaml · ${formatBytes(settings.limit_effective_bytes)}`;

/** The limit row's help, for the mode on screen. */
export function limitHelp(mode: LimitMode, settings: Pick<StorageSettings, 'auto_bytes'>): string {
  switch (mode) {
    case 'auto':
      return `${autoLine(settings)}. It follows the disk when it grows.`;
    case 'custom':
      return 'Custom: the cache stays within the size below.';
    default:
      return `Choose Auto (75% of the disk · ${formatBytes(settings.auto_bytes)}) or a custom size.`;
  }
}

/** The custom input's help: its bounds. */
export const customLimitHelp = (settings: Pick<StorageSettings, 'min_bytes' | 'max_bytes'>): string => {
  const { min, max } = limitBounds(settings);
  return `From ${min} to ${max} (90% of the disk).`;
};

// ---- Downloads ------------------------------------------------------------------------------

/** The kept cap's help: what empty means, and what applies and is needed now. */
export function keptCapHelp(settings: Pick<StorageSettings, 'kept' | 'committed_kept_bytes'>): string {
  const need = settings.committed_kept_bytes > 0 ? `; kept downloads need ${formatBytes(settings.committed_kept_bytes)}` : '';
  return `Empty: half the limit. Now ${formatBytes(settings.kept.effective_max_bytes)}${need}.`;
}

/** The smart cap's help. */
export function smartCapHelp(settings: Pick<StorageSettings, 'smart' | 'usage'>): string {
  const held = settings.usage.smart_bytes > 0 ? `; smart downloads hold ${formatBytes(settings.usage.smart_bytes)}` : '';
  return `Empty: 15% of the limit. Now ${formatBytes(settings.smart.effective_max_bytes)}${held}.`;
}

/** The Downloads card's line: the reserve the caps leave. */
export const reserveLine = (settings: Pick<StorageSettings, 'reserve_bytes'>): string =>
  `Kept and smart downloads share the limit, and always leave ${formatBytes(settings.reserve_bytes)} of it for streaming.`;

export const DAYS_LABELS: Record<KeepDays | 'none', string> = { 7: '7 days', 30: '30 days', 90: '90 days', none: 'No maximum' };

/** What a new Keep defaults to (the Keep sheet starts from it): "New keeps last until you watch them, 30 days at most." */
export function keepDefaultsLine(untilWatched: boolean, days: KeepDays | null): string {
  if (untilWatched) return days === null ? 'New keeps last until you watch them.' : `New keeps last until you watch them, ${days} days at most.`;
  return days === null ? 'New keeps stay until you remove them.' : `New keeps last ${days} days.`;
}

export const untilWatchedHelp = (on: boolean): string =>
  on ? 'A kept title is released once you’ve watched it.' : 'Kept titles stay until their time runs out, or you remove them.';

export const smartHelp = (on: boolean): string =>
  on
    ? 'Fetches the next episodes of the shows you’re watching, from Trakt’s Up next and what’s streaming.'
    : 'Off: only what you keep downloads in the background.';

// ---- Background ------------------------------------------------------------------------------

export const backgroundHelp = (on: boolean): string =>
  on ? 'Kept and smart downloads fetch in the background.' : 'Off: kept and smart downloads wait. Streaming still works.';

export const PAUSE_HELP = 'While anything streams, other downloads wait until a minute after it ends.';

export const windowsHelp = (count: number): string =>
  count === 0
    ? 'Any time. Add a window to download only then, in the server’s time zone.'
    : 'Downloads run only inside these times, in the server’s time zone. A window can cross midnight.';

// ---- The disk bar -----------------------------------------------------------------------------

export type DiskPart = 'kept' | 'smart' | 'cache' | 'other' | 'free';

export const DISK_LABELS: Record<DiskPart, string> = { kept: 'Kept', smart: 'Smart', cache: 'Cache', other: 'Other files', free: 'Free' };

export interface DiskSegment {
  part: DiskPart;
  label: string;
  bytes: number;
  /** Width on the bar, 0–100, a share of the disk. Free is the track itself. */
  pct: number;
}

export interface DiskBar {
  /** Kept · Smart · Cache · Other files · Free, always all five in this order. */
  segments: DiskSegment[];
  /** What the piece cache holds: kept, smart and cache. */
  filled: number;
  /** The limit's marker, 0–100. */
  limitPct: number;
  /** The bar's accessible name. */
  label: string;
}

const pctOf = (bytes: number, total: number) => (total > 0 ? Math.min(100, Math.max(0, (bytes / total) * 100)) : 0);

/**
 * The disk as the bar draws it: the piece cache's kept, smart and cache bytes,
 * everything else on the disk ("Other files": what isn't free or the cache),
 * and what's free, with a marker at the limit. Zero kept and smart (before any
 * download) are drawn as nothing and still listed.
 */
export function diskBar(settings: Pick<StorageSettings, 'disk' | 'usage' | 'limit_effective_bytes'>): DiskBar {
  const { total_bytes: total, free_bytes: free } = settings.disk;
  const { kept_bytes: kept, smart_bytes: smart, cache_bytes: cache } = settings.usage;
  const filled = kept + smart + cache;
  const sizes: Record<DiskPart, number> = { kept, smart, cache, other: Math.max(0, total - free - filled), free };
  const segments = (Object.keys(DISK_LABELS) as DiskPart[]).map((part) => ({ part, label: DISK_LABELS[part], bytes: sizes[part], pct: pctOf(sizes[part], total) }));
  const parts = segments.map((segment) => `${segment.label} ${formatBytes(segment.bytes)}`).join(', ');
  return {
    segments,
    filled,
    limitPct: pctOf(settings.limit_effective_bytes, total),
    label: `Disk of ${formatBytes(total)}: ${parts}. Limit ${formatBytes(settings.limit_effective_bytes)}.`,
  };
}

/** The floor's note: what it does while it holds the cache, else that it's there. */
export function floorNote(settings: Pick<StorageSettings, 'floored' | 'effective_bytes' | 'floor_free_bytes'>): { text: string; warn: boolean } {
  const floor = formatBytes(settings.floor_free_bytes);
  return settings.floored
    ? { text: `Holding the cache at ${formatBytes(settings.effective_bytes)} to keep ${floor} of the disk free.`, warn: true }
    : { text: `The cache always leaves ${floor} of the disk free.`, warn: false };
}

/** While the cache holds more than it may (a lower limit, or the floor), it's trimmed in steps: say so. */
export function trimNote(settings: Pick<StorageSettings, 'usage' | 'effective_bytes'>): string | null {
  const { kept_bytes: kept, smart_bytes: smart, cache_bytes: cache } = settings.usage;
  return kept + smart + cache > settings.effective_bytes ? `Trimming the cache down to ${formatBytes(settings.effective_bytes)}.` : null;
}

// ---- The settings nav ------------------------------------------------------------------------

const SOURCE_WORD: Record<StorageLimitSource, string> = { auto: 'auto', setting: 'custom', config: 'config.yaml' };

/** The settings nav's line under Storage: "750 GB · auto". */
export function storageNavSummary(settings: StorageSettings | undefined): { text: string } {
  return settings ? { text: `${formatBytes(settings.limit_effective_bytes)} · ${SOURCE_WORD[settings.source]}` } : { text: '' };
}
