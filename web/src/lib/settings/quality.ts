import type { QualityProfile } from '@/lib/api/contract';

/**
 * The quality profile's rules (PLAN §6 "Search"), shared by Settings › Quality
 * and the mock PUT: allowed values, the "Best match would prefer…" preview
 * (built here, there's no preview endpoint), the tag lists and validation.
 */

export type PreferredResolution = QualityProfile['preferred_resolution'];
export type MaxResolution = QualityProfile['max_resolution'];
export type QualityField = keyof QualityProfile;
export type TagListField = 'avoid_terms' | 'preferred_groups';

export const PREFERRED_RESOLUTIONS: PreferredResolution[] = ['720p', '1080p', '2160p'];
/** Null is "No limit". */
export const MAX_RESOLUTIONS: MaxResolution[] = ['1080p', '2160p', null];
export const GB_PER_HOUR = { min: 1, max: 20, step: 0.5 } as const;
/** Per list: at most `count` entries of 1–`length` characters. */
export const TAG_LIMITS = { count: 30, length: 30 } as const;

/** How the settings name each field, e.g. in "Check Maximum resolution". */
export const FIELD_LABELS: Record<QualityField, string> = {
  preferred_resolution: 'Preferred resolution',
  max_resolution: 'Maximum resolution',
  max_gb_per_hour: 'Max size per hour',
  prefer_x265: 'Prefer x265 / HEVC',
  prefer_hdr: 'Prefer HDR',
  prefer_dolby_vision: 'Prefer Dolby Vision',
  avoid_terms: 'Terms to avoid',
  preferred_groups: 'Preferred groups',
};

const LINES: Record<PreferredResolution, number> = { '720p': 720, '1080p': 1080, '2160p': 2160 };

export function isQualityField(value: unknown): value is QualityField {
  return typeof value === 'string' && value in FIELD_LABELS;
}

/** A maximum below the preferred resolution would hide every preferred release. */
export function maxAllowed(max: MaxResolution, preferred: PreferredResolution): boolean {
  return max === null || LINES[max] >= LINES[preferred];
}

/** Picking a preferred resolution above the maximum raises the maximum with it. */
export function withPreferredResolution(profile: QualityProfile, preferred: PreferredResolution): QualityProfile {
  const max = maxAllowed(profile.max_resolution, preferred)
    ? profile.max_resolution
    : (MAX_RESOLUTIONS.find((value) => maxAllowed(value, preferred)) ?? null);
  return { ...profile, preferred_resolution: preferred, max_resolution: max };
}

export function formatGbPerHour(value: number): string {
  return `${value} GB/h`;
}

/** The size help's example film: Dune: Part Two's runtime. */
const EXAMPLE_MINUTES = 166;

/** "8 GB/h ≈ 22 GB for a 2 h 46 m film". */
export function sizeExample(gbPerHour: number): string {
  const gb = (gbPerHour * EXAMPLE_MINUTES) / 60;
  const shown = gb < 10 ? Math.round(gb * 10) / 10 : Math.round(gb);
  return `${formatGbPerHour(gbPerHour)} ≈ ${shown} GB for a 2 h 46 m film`;
}

/** "FLUX or NTb +2": the first few, then how many more. */
function shortList(items: string[], shown: number, separator: string): string {
  const head = items.slice(0, shown).join(separator);
  return items.length > shown ? `${head} +${items.length - shown}` : head;
}

export interface QualityPreview {
  /** "2160p · DV · ≤ 8 GB/h". */
  main: string;
  /** "x265 when there’s a choice · from FLUX or NTb +2 · never CAM, TS, HDCAM +3"; empty when nothing applies. */
  sub: string;
}

/** What "Best match" would prefer under this profile (the sticky preview). */
export function qualityPreview(profile: QualityProfile): QualityPreview {
  const hdr = profile.prefer_dolby_vision ? 'DV' : profile.prefer_hdr ? 'HDR' : null;
  const main = [profile.preferred_resolution, hdr, `≤ ${formatGbPerHour(profile.max_gb_per_hour)}`].filter(Boolean).join(' · ');
  const sub = [
    profile.prefer_x265 ? 'x265 when there’s a choice' : null,
    profile.preferred_groups.length > 0 ? `from ${shortList(profile.preferred_groups, 2, ' or ')}` : null,
    profile.avoid_terms.length > 0 ? `never ${shortList(profile.avoid_terms, 3, ', ')}` : null,
  ]
    .filter(Boolean)
    .join(' · ');
  return { main, sub };
}

/** The settings nav's line: "2160p preferred · ≤ 8 GB/h". */
export function qualitySummary(profile: QualityProfile): string {
  return `${profile.preferred_resolution} preferred · ≤ ${formatGbPerHour(profile.max_gb_per_hour)}`;
}

export type AddTagResult = { ok: true; list: string[] } | { ok: false; error: string };

/**
 * Add what was typed to a tag list: trimmed, 1–30 characters, at most 30
 * entries, and never twice (case-insensitive, as the server matches).
 */
export function addTag(list: string[], typed: string, noun: 'terms' | 'groups'): AddTagResult {
  const value = typed.trim();
  if (!value) return { ok: true, list };
  const existing = list.find((item) => item.toLowerCase() === value.toLowerCase());
  if (existing) return { ok: false, error: `${existing} is already on the list.` };
  if (value.length > TAG_LIMITS.length) return { ok: false, error: `Keep it to ${TAG_LIMITS.length} characters or fewer.` };
  if (list.length >= TAG_LIMITS.count) return { ok: false, error: `That’s ${TAG_LIMITS.count} ${noun} already. Remove one to add another.` };
  return { ok: true, list: [...list, value] };
}

export interface QualityFieldError {
  field: QualityField;
  message: string;
}

function tagListError(field: TagListField, list: unknown): QualityFieldError | null {
  if (!Array.isArray(list)) return { field, message: 'Send a list of words.' };
  if (list.length > TAG_LIMITS.count) return { field, message: `At most ${TAG_LIMITS.count} entries.` };
  const bad = list.find((item) => typeof item !== 'string' || item.trim().length === 0 || item.trim().length > TAG_LIMITS.length);
  if (bad !== undefined) return { field, message: `Each entry needs 1 to ${TAG_LIMITS.length} characters.` };
  return null;
}

/** The contract's PUT validation: the first bad field, or null. */
export function validateQualityProfile(profile: QualityProfile): QualityFieldError | null {
  if (!PREFERRED_RESOLUTIONS.includes(profile.preferred_resolution)) {
    return { field: 'preferred_resolution', message: 'Pick 720p, 1080p or 2160p.' };
  }
  if (!MAX_RESOLUTIONS.includes(profile.max_resolution)) {
    return { field: 'max_resolution', message: 'Pick 1080p, 2160p or No limit.' };
  }
  if (!maxAllowed(profile.max_resolution, profile.preferred_resolution)) {
    return { field: 'max_resolution', message: 'The maximum can’t be below the preferred resolution.' };
  }
  const gb = profile.max_gb_per_hour;
  if (typeof gb !== 'number' || gb < GB_PER_HOUR.min || gb > GB_PER_HOUR.max || !Number.isInteger(gb / GB_PER_HOUR.step)) {
    return { field: 'max_gb_per_hour', message: `Pick ${GB_PER_HOUR.min} to ${GB_PER_HOUR.max} GB/h, in steps of ${GB_PER_HOUR.step}.` };
  }
  for (const field of ['prefer_x265', 'prefer_hdr', 'prefer_dolby_vision'] as const) {
    if (typeof profile[field] !== 'boolean') return { field, message: 'Turn it on or off.' };
  }
  return tagListError('avoid_terms', profile.avoid_terms) ?? tagListError('preferred_groups', profile.preferred_groups);
}

/** Whether two profiles are the same settings (the save bar shows while they differ). */
export function sameProfile(a: QualityProfile, b: QualityProfile): boolean {
  return (Object.keys(FIELD_LABELS) as QualityField[]).every((field) => {
    const left = a[field];
    const right = b[field];
    if (Array.isArray(left) && Array.isArray(right)) return left.length === right.length && left.every((item, i) => item === right[i]);
    return left === right;
  });
}
