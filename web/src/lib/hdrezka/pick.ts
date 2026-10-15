import type {
  HDRezkaCandidate,
  HDRezkaCategory,
  HDRezkaQuality,
  HDRezkaTitle,
  HDRezkaTranslator,
  MediaType,
} from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';

/**
 * Picking an HDRezka stream (#67): which hit is the best match and why,
 * which voice-overs and qualities can play without an account, and what
 * to pick first. Pure, so the sheet stays thin.
 */

/** The fields of a TMDB title a match is judged against. */
export interface MatchTarget {
  media_type: MediaType;
  title: string;
  original_title: string | null;
  year: number | null;
}

/** The hit to suggest: the first exact one, else the first likely one. A weak hit never is. */
export function bestCandidate(items: HDRezkaCandidate[]): HDRezkaCandidate | null {
  return items.find((item) => item.match === 'exact') ?? items.find((item) => item.match === 'likely') ?? null;
}

const CATEGORY_LABEL: Record<HDRezkaCategory, string> = {
  films: 'Film',
  series: 'Series',
  cartoons: 'Cartoon',
  animation: 'Anime',
};

export const categoryLabel = (category: HDRezkaCategory): string => CATEGORY_LABEL[category] ?? category;

/** Case, punctuation and spacing aside: "Dune: Part Two" = "dune part two". */
const normalize = (text: string) =>
  text
    .toLocaleLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();

const names = (title: string, original: string | null) => [title, original].filter((name): name is string => Boolean(name)).map(normalize);

function sameTitle(candidate: HDRezkaCandidate, target: MatchTarget): boolean {
  const wanted = names(target.title, target.original_title);
  return names(candidate.title, candidate.original_title).some((name) => wanted.includes(name));
}

const rating = (value: number) => `Rated ${value.toFixed(1)}`;

/** Year, kind and rating: what every card says about a hit. */
export function candidateFacts(candidate: HDRezkaCandidate): string[] {
  return [candidate.year !== null ? String(candidate.year) : null, categoryLabel(candidate.category), candidate.rating !== null ? rating(candidate.rating) : null].filter(
    (fact): fact is string => fact !== null
  );
}

/** Up to 4 short reasons for the best match: "Exact title · 2014 · Film · Rated 8.6". */
export function matchReasons(candidate: HDRezkaCandidate, target: MatchTarget): string[] {
  const reasons: string[] = [sameTitle(candidate, target) ? 'Exact title' : 'Similar title'];
  if (candidate.year !== null) {
    const off = target.year !== null && Math.abs(candidate.year - target.year) > 1;
    reasons.push(off ? `${candidate.year}, not ${target.year}` : String(candidate.year));
  }
  reasons.push(categoryLabel(candidate.category));
  if (candidate.rating !== null) reasons.push(rating(candidate.rating));
  return reasons.slice(0, 4);
}

/** What a weak hit says: the title or the year didn't line up. */
export const weakMatchNote = (target: MatchTarget) =>
  `Weak match: check it’s the right ${target.media_type === 'movie' ? 'movie' : 'show'}`;

/* ---- Voice-overs ------------------------------------------------------------ */

/** One translator id can come in several cuts; its flags tell them apart. */
export const translatorKey = (t: HDRezkaTranslator) =>
  `${t.id}${t.camrip ? '-c' : ''}${t.ads ? '-a' : ''}${t.director ? '-d' : ''}`;

/** A `Chip` tone. */
export interface TranslatorChip {
  label: string;
  tone: 'accent' | 'warn' | 'default';
}

/** Premium, CAMRip, Ads and Director's cut, in that order. */
export function translatorChips(t: HDRezkaTranslator): TranslatorChip[] {
  const chips: Array<TranslatorChip | false> = [
    t.premium && { label: 'Premium', tone: 'accent' },
    t.camrip && { label: 'CAMRip', tone: 'warn' },
    t.ads && { label: 'Ads', tone: 'warn' },
    t.director && { label: 'Director’s cut', tone: 'default' },
  ];
  return chips.filter((chip): chip is TranslatorChip => chip !== false);
}

/** Why a voice-over can't be picked, or null when it can. */
export const translatorBlocked = (t: HDRezkaTranslator): string | null =>
  t.premium ? 'Needs an HDRezka Premium account, which momoshtrem doesn’t use' : null;

/**
 * The voice-over to start on: the one kept from earlier, else the one the
 * answer is for (a movie's current stream, or the page's default), else the
 * page's default, else the first that plays without Premium.
 */
export function defaultTranslator(title: HDRezkaTitle, keep: string | null = null): HDRezkaTranslator | null {
  const usable = title.translators.filter((t) => !t.premium);
  return (
    usable.find((t) => translatorKey(t) === keep) ??
    usable.find((t) => t.id === title.translator_id) ??
    usable.find((t) => t.default) ??
    usable[0] ??
    null
  );
}

/* ---- Qualities ---------------------------------------------------------------- */

/** Plays without an account, as a progressive MP4. */
export const qualityUsable = (q: HDRezkaQuality) => q.available && !q.premium;

/** The quality to start on: the one kept from earlier when it's still offered, else the best usable one. */
export function defaultQuality(qualities: HDRezkaQuality[], keep: string | null = null): HDRezkaQuality | null {
  const usable = qualities.filter(qualityUsable);
  const kept = usable.find((q) => q.label === keep);
  if (kept) return kept;
  return usable.reduce<HDRezkaQuality | null>((best, q) => (best === null || q.resolution >= best.resolution ? q : best), null);
}

/** Under a quality's label: its size, or why it can't be picked. */
export function qualityNote(q: HDRezkaQuality): string {
  if (q.premium) return 'Premium';
  if (!q.available) return 'Not offered';
  return q.size_bytes !== null ? formatBytes(q.size_bytes) : 'Size unknown';
}

/** "Subtitles: English, Русский" for the voice-over's stream. */
export function subtitlesLine(subtitles: Array<{ language_name: string }>): string {
  return subtitles.length ? `Subtitles: ${subtitles.map((s) => s.language_name).join(', ')}` : 'No subtitles with this voice-over';
}
