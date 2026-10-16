import type { ChipTone } from '@/components/ui';
import type { Release, ReleaseAttributes, Timestamp } from '@/lib/api/contract';
import { formatBytes, formatDate, formatNumber } from '@/lib/title/format';

/** What a release card shows besides its name (HANDOFF §5.2): attribute chips and facts. */

export interface ReleaseChip {
  label: string;
  tone: ChipTone;
}

/** HDR formats get the highlighted chip, like 2160p and Atmos. */
const HI_HDR = new Set(['DV', 'HDR10+', 'HDR10', 'HDR']);

const LANGUAGE_WORD: Record<string, string> = { multi: 'Multi-audio', dual: 'Dual audio' };

/** "DD+ 5.1": the channels go with the first sound format that isn't Atmos (which rides on one). */
function audioChips({ audio, channels }: Pick<ReleaseAttributes, 'audio' | 'channels'>): ReleaseChip[] {
  const withChannels = audio.findIndex((format) => format !== 'Atmos');
  return audio.map((format, i) => ({
    label: i === withChannels && channels ? `${format} ${channels}` : format,
    tone: format === 'Atmos' ? 'hi' : 'default',
  }));
}

/**
 * The parsed attributes as chips, in reading order: resolution, source, HDR,
 * audio, codec, bit depth, group, pack, languages, subtitles, edition, repack.
 * `hi` for 2160p, HDR and Atmos; `err` for CAM; `pack` for a pack;
 * `accent` for a group the quality profile prefers. Unknowns are left out.
 */
export function releaseChips(release: Pick<Release, 'attrs'>, { preferredGroup = false }: { preferredGroup?: boolean } = {}): ReleaseChip[] {
  const { attrs } = release;
  const chips: ReleaseChip[] = [];
  if (attrs.resolution !== 'unknown') chips.push({ label: attrs.resolution, tone: attrs.resolution === '2160p' ? 'hi' : 'default' });
  if (attrs.source !== 'unknown') chips.push({ label: attrs.source, tone: attrs.source === 'CAM' ? 'err' : 'default' });
  for (const hdr of attrs.hdr) chips.push({ label: hdr, tone: HI_HDR.has(hdr) ? 'hi' : 'default' });
  chips.push(...audioChips(attrs));
  if (attrs.codec !== 'unknown' && attrs.codec !== 'other') chips.push({ label: attrs.codec, tone: 'default' });
  if (attrs.bit_depth) chips.push({ label: `${attrs.bit_depth}-bit`, tone: 'default' });
  if (attrs.group) chips.push({ label: attrs.group, tone: preferredGroup ? 'accent' : 'default' });
  // A single episode needs no pack chip: the name says which
  if (attrs.pack && attrs.pack.type !== 'episode') chips.push({ label: attrs.pack.label, tone: 'pack' });
  const languages = attrs.languages.map((code) => LANGUAGE_WORD[code] ?? code.toUpperCase());
  if (languages.length > 0) chips.push({ label: languages.join(' · '), tone: 'default' });
  if (attrs.subs === 'multi') chips.push({ label: 'Multi-sub', tone: 'default' });
  if (attrs.subs === 'hardcoded') chips.push({ label: 'Hardsub', tone: 'default' });
  if (attrs.edition) chips.push({ label: attrs.edition, tone: 'default' });
  if (attrs.repack) chips.push({ label: 'REPACK', tone: 'default' });
  return chips;
}

/** A score reason naming a preferred group ("…from NTb, a group you prefer") marks that group's chip. */
export function prefersGroup(release: Pick<Release, 'attrs' | 'score_reasons'>): boolean {
  const { group } = release.attrs;
  return group !== null && release.score_reasons.some((reason) => reason.includes(group) && /prefer/i.test(reason));
}

/** Seeders in status colour: green from 20, amber below, red at none. */
export type SeedHealth = 'ok' | 'low' | 'none';

export const seedHealth = (seeders: number): SeedHealth => (seeders === 0 ? 'none' : seeders < 20 ? 'low' : 'ok');

const DAY = 86_400_000;

/** "12 min", "5 h", "3 d", "2 wk", "9 mo", "1 yr": a release's age at a glance. */
export function relativeAge(timestamp: Timestamp, now: Date = new Date()): string {
  const ms = Math.max(0, now.getTime() - new Date(timestamp).getTime());
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 60) return `${Math.max(1, minutes)} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h`;
  const days = Math.floor(ms / DAY);
  if (days < 7) return `${days} d`;
  if (days < 30) return `${Math.floor(days / 7)} wk`;
  if (days < 365) return `${Math.max(1, Math.floor(days / 30.44))} mo`;
  return `${Math.floor(days / 365.25)} yr`;
}

export interface ReleaseFacts {
  size: string;
  seeders: { label: string; health: SeedHealth };
  leechers: string;
  /** Null when the indexer gives no date. */
  age: { label: string; title: string; dateTime: Timestamp } | null;
  indexer: string;
}

/** Size, seeders, leechers, age and indexer, formatted. */
export function releaseFacts(release: Pick<Release, 'size_bytes' | 'seeders' | 'leechers' | 'published_at' | 'indexer'>, now?: Date): ReleaseFacts {
  return {
    size: formatBytes(release.size_bytes),
    seeders: { label: `${formatNumber(release.seeders)} ${release.seeders === 1 ? 'seeder' : 'seeders'}`, health: seedHealth(release.seeders) },
    leechers: formatNumber(release.leechers),
    age: release.published_at
      ? { label: relativeAge(release.published_at, now), title: formatDate(release.published_at), dateTime: release.published_at }
      : null,
    indexer: release.indexer,
  };
}

/** The magnet Add and Copy magnet use: the release's, or the one resolve fetched (`needs_torrent_fetch`). */
export const magnetOf = (release: Pick<Release, 'id' | 'magnet_uri'>, resolved: Readonly<Record<string, string>>): string | null =>
  release.magnet_uri ?? resolved[release.id] ?? null;
