/**
 * Attributes a release name carries beyond the contract's resolution and
 * source, for the chips on the current-torrent card and the "Ready in
 * Infuse" line. Scene names separate tokens with dots, so dots count as
 * word boundaries.
 */

export interface ReleaseTag {
  label: string;
  /** Worth highlighting: resolution, Dolby Vision. */
  hi: boolean;
  kind: 'hdr' | 'audio' | 'codec' | 'edition' | 'group';
}

const TAGS: Array<{ pattern: RegExp; label: string; kind: ReleaseTag['kind']; hi?: boolean }> = [
  { pattern: /\b(DV|DoVi|Dolby[ .]?Vision)\b/i, label: 'Dolby Vision', kind: 'hdr', hi: true },
  { pattern: /\bHDR10(\+|Plus)/i, label: 'HDR10+', kind: 'hdr' },
  { pattern: /\bHDR(10)?\b/i, label: 'HDR', kind: 'hdr' },
  { pattern: /\bAtmos\b/i, label: 'Atmos', kind: 'audio' },
  { pattern: /\bTrueHD\b/i, label: 'TrueHD', kind: 'audio' },
  { pattern: /\bDTS-HD\b/i, label: 'DTS-HD', kind: 'audio' },
  { pattern: /\b(H\.?265|x265|HEVC)\b/i, label: 'H.265', kind: 'codec' },
  { pattern: /\b(H\.?264|x264|AVC)\b/i, label: 'H.264', kind: 'codec' },
  { pattern: /\bREMUX\b/i, label: 'Remux', kind: 'edition' },
  { pattern: /\bIMAX\b/i, label: 'IMAX', kind: 'edition' },
];

/** Hyphenated source tokens that end some names without a group ("…1080p.WEB-DL"). */
const NOT_GROUPS = new Set(['DL', 'RIP', 'HD', 'MA']);

/** "…H.265-FLUX" or "…H.265-FLUX.mkv" → "FLUX"; "[SubsPlease] …" → "SubsPlease". */
export function releaseGroup(name: string): string | null {
  const bracketed = /^\[([^\]]+)\]/.exec(name);
  if (bracketed) return bracketed[1];
  const suffixed = /-([A-Za-z0-9]+)(\.[a-z0-9]{2,4})?$/.exec(name);
  return suffixed && !NOT_GROUPS.has(suffixed[1].toUpperCase()) ? suffixed[1] : null;
}

/** HDR, audio, codec and edition tags in a fixed order, then the group. "HDR" is dropped once HDR10+ matched. */
export function releaseTags(name: string): ReleaseTag[] {
  const tags: ReleaseTag[] = [];
  for (const { pattern, label, kind, hi = false } of TAGS) {
    if (!pattern.test(name)) continue;
    if (label === 'HDR' && tags.some((tag) => tag.label === 'HDR10+')) continue;
    tags.push({ label, kind, hi });
  }
  const group = releaseGroup(name);
  if (group) tags.push({ label: group, kind: 'group', hi: false });
  return tags;
}

/** The attributes worth a word on the "Ready in Infuse" line: Dolby Vision and Atmos-class audio. */
export function releaseHighlights(name: string): string[] {
  const tags = releaseTags(name);
  const hdr = tags.find((tag) => tag.kind === 'hdr' && tag.hi);
  const audio = tags.find((tag) => tag.kind === 'audio');
  return [hdr?.label, audio?.label].filter((label): label is string => Boolean(label));
}
