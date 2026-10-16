import type { RecItem } from '@/lib/api/contract';

/**
 * What a recommended title's card says (`RecPosterCard`): For You's shelves
 * (#155) and anything else that shows a `RecItem` (#156).
 */

const MONTH_DAY = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
/** Further out than this, "Jan 16" could be read as the wrong year: the year says it. */
const MONTH_DAY_WITHIN_DAYS = 183;
const DAY_MS = 24 * 60 * 60 * 1000;

/** "2026-01-16" as that local calendar day (not UTC midnight, which is the day before west of Greenwich). */
function localDate(date: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (!match) return null;
  const [, y, m, d] = match;
  return new Date(Number(y), Number(m) - 1, Number(d));
}

/**
 * A Coming soon poster's label: the run's `badge` ("Jan 16", "Summer",
 * "2026"), else from `release_date` while it's still ahead: "Jan 16" within
 * about six months, the year after that. Null once it's out (a show's
 * `release_date` is its first air date, so a new season has no date to go by).
 */
export function comingSoonBadge(item: Pick<RecItem, 'badge' | 'release_date'>, today: Date): string | null {
  if (item.badge?.trim()) return item.badge.trim();
  const release = item.release_date ? localDate(item.release_date) : null;
  if (!release) return null;
  const start = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const days = Math.round((release.getTime() - start.getTime()) / DAY_MS);
  if (days < 0) return null;
  return days <= MONTH_DAY_WITHIN_DAYS ? MONTH_DAY.format(release) : String(release.getFullYear());
}

/**
 * What the card's link says after the title ("Silo, 2023 · Series"): the
 * reason replaces the visible subtitle, so screen readers still hear the
 * year and kind, and on Coming soon the date ("Coming Jan 16").
 */
export function recCardSubtitle(item: Pick<RecItem, 'year' | 'media_type'>, badge: string | null = null): string {
  const kind = item.media_type === 'tv' ? 'Series' : 'Movie';
  return [badge ? `Coming ${badge}` : item.year, kind].filter(Boolean).join(' · ');
}
