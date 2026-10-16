import type { EpisodeRef, NewEpisodeItem, Timestamp, UpNextAvailability, UpNextItem } from '@/lib/api/contract';
import { STATUS, type StatusKey } from '@/lib/status';
import { episodeCode } from '@/lib/title/episodes';
import { titleHref } from '@/lib/title/href';

/**
 * The landscape episode cards on For You (Up next, New episodes; HANDOFF §3
 * EpisodeCard): the code over the still, the pill and the one action each
 * availability gets (js/core.js `ui.landCard`). Pure, so the cards and the
 * tests agree on every word.
 */

export interface EpisodeCardPill {
  status: StatusKey;
  label: string;
  /** Downloading: the ring's percentage. */
  pct?: number;
}

/** What the card's button does; the component turns each intent into a sheet, a link or a mutation. */
export type EpisodeCardAction =
  /** The infuse-setup sheet with the episode's file. */
  | { intent: 'play'; label: string; ariaLabel: string; path: string | null; url: string | null }
  /** Activity's detail for the torrent fetching it. */
  | { intent: 'details'; label: string; ariaLabel: string; href: string }
  /** The mapping review that wants the episode. */
  | { intent: 'review'; label: string; ariaLabel: string; href: string }
  /** The torrent search sheet on this episode (`?sheet=torrent&show=<tmdb>&se=S01E29`). */
  | { intent: 'find'; label: string; ariaLabel: string; params: Record<string, string> }
  /** Add the show; the card then flips to Find. */
  | { intent: 'add'; label: string; ariaLabel: string; tmdbId: number; show: string };

export interface EpisodeCardModel {
  key: string;
  /** The show's page, on the episode's season. */
  href: string;
  show: string;
  /** "S01E29", or "S22E39 · #1124" with an absolute number. */
  code: string;
  name: string;
  pill: EpisodeCardPill;
  action: EpisodeCardAction | null;
  /** New episodes: "Today", "Yesterday", "Fri" or "Oct 2", on glass over the still. */
  when: string | null;
  /** While downloading: the bar along the still's bottom edge. */
  downloadPct: number | null;
  /** TMDB's still, else the show's backdrop; null shows the placeholder. */
  image: { path: string; kind: 'still' | 'backdrop' } | null;
}

/** "S01E29", or "S22E39 · #1124" when the absolute number isn't the in-season one. */
export function episodeCardCode(episode: Pick<EpisodeRef, 'season' | 'episode' | 'absolute_number'>): string {
  const code = episodeCode(episode.season, episode.episode);
  const absolute = episode.absolute_number;
  return absolute !== null && absolute !== episode.episode ? `${code} · #${absolute}` : code;
}

export const episodeName = (episode: Pick<EpisodeRef, 'episode' | 'name'>) => episode.name?.trim() || `Episode ${episode.episode}`;

// ---- Days -------------------------------------------------------------------------

const DAY_MS = 86_400_000;
const WEEKDAY = new Intl.DateTimeFormat('en-US', { weekday: 'short' });
const DAY_MONTH = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const DAY_MONTH_YEAR = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' });

/**
 * The local calendar day a timestamp falls on. An air date without a time
 * comes as midnight UTC (PLAN §6 "Trakt (B5)"), so that one is read as the
 * date itself: west of UTC it would otherwise land on the day before.
 */
export function calendarDay(timestamp: Timestamp): Date {
  const date = new Date(timestamp);
  if (/T00:00(:00(\.0+)?)?(Z|[+-]00:?00)$/.test(timestamp)) {
    return new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate());
  }
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

/** Whole days from today to that day (negative in the past). */
export function daysFromToday(timestamp: Timestamp, now: Date): number {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.round((calendarDay(timestamp).getTime() - today.getTime()) / DAY_MS);
}

function shortDay(day: Date, now: Date): string {
  return (day.getFullYear() === now.getFullYear() ? DAY_MONTH : DAY_MONTH_YEAR).format(day);
}

/** When a new episode aired: "Today", "Yesterday", "Fri" within the week, else "Oct 2". */
export function airedLabel(airedAt: Timestamp, now: Date): string {
  const days = daysFromToday(airedAt, now);
  if (days >= 0) return 'Today';
  if (days === -1) return 'Yesterday';
  if (days > -7) return WEEKDAY.format(calendarDay(airedAt));
  return shortDay(calendarDay(airedAt), now);
}

/** When an upcoming episode airs: "Airs today", "Airs tomorrow", "Airs Sun" within the week, else "Airs Oct 21". */
export function airsLabel(airsAt: Timestamp, now: Date): string {
  const days = daysFromToday(airsAt, now);
  if (days <= 0) return 'Airs today';
  if (days === 1) return 'Airs tomorrow';
  if (days < 7) return `Airs ${WEEKDAY.format(calendarDay(airsAt))}`;
  return `Airs ${shortDay(calendarDay(airsAt), now)}`;
}

// ---- Availability → pill and action -----------------------------------------------

type CardItem = Pick<UpNextItem, 'show' | 'webdav_path' | 'webdav_url' | 'download' | 'review_job_id'> & {
  availability: UpNextAvailability;
};

export function cardPill(availability: UpNextAvailability, item: Pick<CardItem, 'download'>, airsAt: Timestamp | null, now: Date): EpisodeCardPill {
  switch (availability) {
    case 'ready':
      return { status: 'ready', label: 'Ready to watch' };
    case 'downloading': {
      const pct = Math.round(item.download?.pct ?? 0);
      return { status: 'downloading', label: `${STATUS.downloading.label} ${pct}%`, pct };
    }
    case 'needs_review':
    case 'needs_torrent':
    case 'not_in_library':
      return { status: availability, label: STATUS[availability].label };
    case 'airs_on':
      return airsAt ? { status: 'airs_on', label: airsLabel(airsAt, now) } : { status: 'upcoming', label: STATUS.upcoming.label };
  }
}

export function cardAction(item: CardItem, episode: Pick<EpisodeRef, 'season' | 'episode'>): EpisodeCardAction | null {
  const code = episodeCode(episode.season, episode.episode);
  const what = `${item.show.title} ${code}`;
  switch (item.availability) {
    case 'ready':
      return { intent: 'play', label: 'Play in Infuse', ariaLabel: `Play ${what} in Infuse`, path: item.webdav_path, url: item.webdav_url };
    case 'downloading': {
      const hash = item.download?.info_hash;
      return { intent: 'details', label: 'Details', ariaLabel: `Download details for ${what}`, href: hash ? `/activity/${hash}` : '/activity' };
    }
    case 'needs_review':
      return {
        intent: 'review',
        label: 'Review',
        ariaLabel: `Review the files for ${what}`,
        href: item.review_job_id !== null ? `/mapping/${item.review_job_id}` : titleHref('tv', item.show.tmdb_id),
      };
    case 'needs_torrent':
      return { intent: 'find', label: 'Find', ariaLabel: `Find a torrent for ${what}`, params: { show: String(item.show.tmdb_id), se: code } };
    case 'not_in_library':
      return { intent: 'add', label: 'Add', ariaLabel: `Add ${item.show.title} to your library`, tmdbId: item.show.tmdb_id, show: item.show.title };
    case 'airs_on':
      return null;
  }
}

function card(item: CardItem, episode: EpisodeRef, airsAt: Timestamp | null, when: string | null, now: Date): EpisodeCardModel {
  return {
    key: `${item.show.tmdb_id}:${episode.season}:${episode.episode}`,
    href: `${titleHref('tv', item.show.tmdb_id)}?season=${episode.season}`,
    show: item.show.title,
    code: episodeCardCode(episode),
    name: episodeName(episode),
    pill: cardPill(item.availability, item, airsAt, now),
    action: cardAction(item, episode),
    when,
    downloadPct: item.availability === 'downloading' ? Math.round(item.download?.pct ?? 0) : null,
    image: episode.still_path
      ? { path: episode.still_path, kind: 'still' }
      : item.show.backdrop_path
        ? { path: item.show.backdrop_path, kind: 'backdrop' }
        : null,
  };
}

/** An Up next card: the show's next episode and what it takes to play it. */
export function upNextCard(item: UpNextItem, now: Date): EpisodeCardModel {
  return card(item, item.next_episode, item.airs_at, null, now);
}

/** A New episodes card, with when it aired on the still. */
export function newEpisodeCard(item: NewEpisodeItem, now: Date): EpisodeCardModel {
  return card(item, item.episode, null, airedLabel(item.aired_at, now), now);
}
