import type { HeroPick, MediaType, UpNextItem, WatchlistItem } from '@/lib/api/contract';
import { episodeCardCode } from './episode-card';

/**
 * The For You hero's picks until #9's "Tonight's pick" exists (decided on
 * #8): at most three, stable for the day. First a ready Up next show, then
 * ready watchlist titles, then trending. Within each group the day decides
 * the order (a hash of the local date and the title), so a reload never
 * reshuffles them and tomorrow brings others. #9 hands its `hero_picks` to
 * the same hero as `source: 'recommendation'`.
 */

export const MAX_HERO_PICKS = 3;
/** Trending picks come from the top of TMDB's list, not its long tail. */
const TRENDING_POOL = 10;

export type HeroPickSource = 'up_next' | 'watchlist' | 'trending' | 'recommendation';

/** A hero pick and where it came from: the kicker's icon and the primary action follow it. */
export interface ForYouPick extends HeroPick {
  source: HeroPickSource;
}

export const PICK_KICKER: Record<Exclude<HeroPickSource, 'recommendation'>, string> = {
  up_next: 'Ready in Infuse',
  watchlist: 'From your watchlist',
  trending: 'Trending this week',
};

/** A trending title: just what a pick needs. */
export interface TrendingRef {
  media_type: MediaType;
  tmdb_id: number;
}

/** "2026-10-16", the viewer's own calendar day. */
export function localDateKey(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** FNV-1a: a cheap, stable 32-bit hash, so the day's order is the same on every device. */
function hash(text: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

const keyOf = (pick: { media_type: MediaType; tmdb_id: number }) => `${pick.media_type}:${pick.tmdb_id}`;

/** The day's order for these titles. */
function byDay<T extends { media_type: MediaType; tmdb_id: number }>(items: T[], day: string): T[] {
  return [...items].sort((a, b) => hash(`${day}|${keyOf(a)}`) - hash(`${day}|${keyOf(b)}`) || keyOf(a).localeCompare(keyOf(b)));
}

function upNextReason(item: UpNextItem): string {
  const episode = item.next_episode;
  const name = episode.name?.trim();
  return `Up next: ${episodeCardCode(episode)}${name ? ` · ${name}` : ''}. It’s ready to play in Infuse.`;
}

export interface HeroPickInput {
  upNext: UpNextItem[];
  watchlist: WatchlistItem[];
  trending: TrendingRef[];
  /** Today (a Date) or its `localDateKey`. */
  date: Date | string;
}

/** At most three picks for the hero, in order: one ready Up next show, ready watchlist titles, then trending. */
export function heroPicks({ upNext, watchlist, trending, date }: HeroPickInput): ForYouPick[] {
  const day = typeof date === 'string' ? date : localDateKey(date);
  const picks: ForYouPick[] = [];
  const taken = new Set<string>();
  const add = (pick: ForYouPick) => {
    if (picks.length >= MAX_HERO_PICKS || taken.has(keyOf(pick))) return;
    taken.add(keyOf(pick));
    picks.push(pick);
  };

  const readyShows = upNext.filter((item) => item.availability === 'ready').map((item) => ({ ...item, media_type: 'tv' as const, tmdb_id: item.show.tmdb_id }));
  const [show] = byDay(readyShows, day);
  if (show) add({ media_type: 'tv', tmdb_id: show.tmdb_id, kicker: PICK_KICKER.up_next, reason: upNextReason(show), source: 'up_next' });

  for (const item of byDay(watchlist.filter((w) => w.library?.availability_stage === 'ready'), day)) {
    add({ media_type: item.media_type, tmdb_id: item.tmdb_id, kicker: PICK_KICKER.watchlist, reason: 'On your Trakt watchlist, and it’s ready in Infuse.', source: 'watchlist' });
  }

  for (const item of byDay(trending.slice(0, TRENDING_POOL), day)) {
    add({ media_type: item.media_type, tmdb_id: item.tmdb_id, kicker: PICK_KICKER.trending, reason: '', source: 'trending' });
  }
  return picks;
}
