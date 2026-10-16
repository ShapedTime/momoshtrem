import type { RecItem, Shelf } from '@/lib/api/contract';

/**
 * Noir's shelves on For You (#155; home.js): where each goes, what a card
 * says, and the Ask band's suggestions.
 */

export interface ShelfLayout {
  /** The first shelf, right after New episodes; the Ask band follows it. */
  lead: Shelf | null;
  /** The rest, in the run's order, before the watchlist. */
  rest: Shelf[];
  /** `coming_soon` shelves go last, after the watchlist. */
  comingSoon: Shelf[];
}

/**
 * The design's order: Up next, New episodes, the first shelf, the Ask band,
 * the other shelves, the watchlist, then Coming soon. The run's order stands
 * otherwise; a Coming soon shelf never leads.
 */
export function layoutShelves(shelves: readonly Shelf[]): ShelfLayout {
  const comingSoon = shelves.filter((shelf) => shelf.kind === 'coming_soon');
  const [lead = null, ...rest] = shelves.filter((shelf) => shelf.kind !== 'coming_soon');
  return { lead, rest, comingSoon };
}

/** While a run refreshes the picks, one shelf shows skeletons ("Refreshing…"): the first after the Ask band, else the lead. */
export function refreshingShelfSlug({ lead, rest }: ShelfLayout): string | null {
  return rest[0]?.slug ?? lead?.slug ?? null;
}

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

/**
 * The Ask band's three suggestion chips (home.js). The first leans on a show
 * you're watching, when there is one ("something like The Studio but
 * lighter"); the others are evergreen.
 */
export function askSuggestions(watching?: string | null): string[] {
  const show = watching?.trim();
  return [show ? `something like ${show} but lighter` : 'something light and funny for tonight', 'a 90-minute thriller for tonight', 'anime with a gentle, cosy vibe'];
}
