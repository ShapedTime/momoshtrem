import type { Shelf } from '@/lib/api/contract';

/**
 * Noir's shelves on For You (#155; home.js): where each goes and the Ask
 * band's suggestions. What a card says lives in lib/recommendations/cards.ts.
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

/**
 * The Ask band's three suggestion chips (home.js). The first leans on a show
 * you're watching, when there is one ("something like The Studio but
 * lighter"); the others are evergreen.
 */
export function askSuggestions(watching?: string | null): string[] {
  const show = watching?.trim();
  return [show ? `something like ${show} but lighter` : 'something light and funny for tonight', 'a 90-minute thriller for tonight', 'anime with a gentle, cosy vibe'];
}
