import { isApiError } from '@/lib/api/client';
import type { TitleStream } from '@/lib/api/contract';

/** The HDRezka sheet's words for what went wrong, what's empty and what's replaced (#67). */

export interface HDRezkaProblem {
  title: string;
  /** The backend's message, which names the cause. */
  body: string;
  /** Trying again can help (not when HDRezka is off or the page doesn't exist). */
  retry: boolean;
}

/** An error from /hdrezka/* or assign-hdrezka, by its contract code. */
export function hdrezkaProblem(error: unknown): HDRezkaProblem {
  const message = error instanceof Error ? error.message : '';
  const code = isApiError(error) ? error.code : null;
  switch (code) {
    case 'hdrezka_blocked':
      return { title: 'HDRezka is showing its bot check', body: message || 'It usually lets requests through again after a few minutes.', retry: true };
    case 'hdrezka_unavailable':
    case 'bad_gateway':
      return { title: 'HDRezka isn’t answering', body: message || 'The site may be down. Torrents still work.', retry: true };
    case 'timeout':
      return { title: 'HDRezka took too long', body: message || 'It’s often slow for a moment. Try again.', retry: true };
    case 'unavailable':
      return { title: 'HDRezka is off', body: message || 'Set HDREZKA_ENABLED=true on the server to use it.', retry: false };
    case 'premium_only':
      return { title: 'This needs HDRezka Premium', body: message || 'momoshtrem streams without an account. Pick another voice-over or quality.', retry: false };
    case 'not_found':
      return { title: 'HDRezka has no such page', body: message || 'Check the link, or search by name instead.', retry: false };
    default:
      return { title: 'Couldn’t reach HDRezka', body: message || 'Something went wrong on the way. Try again.', retry: true };
  }
}

/**
 * A pasted title page, on any mirror, with or without the scheme; null for
 * anything else (a search). The backend keeps only the path.
 */
export function hdrezkaLink(text: string): string | null {
  const trimmed = text.trim();
  const match = /^(https?:\/\/)?([a-z0-9.-]+\.[a-z]{2,})?(\/(films|series|cartoons|animation)\/[^\s?#]+\.html)(?:[?#]\S*)?$/i.exec(trimmed);
  if (!match) return null;
  const [, scheme, host, path] = match;
  if (!host) return path;
  return `${scheme ?? 'https://'}${host}${path}`;
}

/**
 * Shorter queries to suggest when nothing matched, best first: without the
 * year, the part before a colon, then without the last word.
 */
export function shorterQuery(query: string): string | null {
  const trimmed = query.trim();
  const withoutYear = trimmed.replace(/\s*\(?(19|20)\d\d\)?$/, '').trim();
  if (withoutYear && withoutYear !== trimmed) return withoutYear;
  const beforeColon = trimmed.split(/\s*[:–—]\s*|\s+-\s+/)[0].trim();
  if (beforeColon && beforeColon !== trimmed) return beforeColon;
  const words = trimmed.split(/\s+/);
  return words.length > 1 ? words.slice(0, -1).join(' ') : null;
}

/** What the confirm dialog says before a stream replaces the movie's current file. */
export function replaceFileConfirm(
  name: string,
  current: { torrent: boolean; stream: Pick<TitleStream, 'translator' | 'quality'> | null },
  next: { translator: string; quality: string }
): { title: string; description: string; confirmLabel: string } {
  if (current.stream) {
    return {
      title: `Switch ${name} to ${next.translator} in ${next.quality}?`,
      description: `Infuse plays it from now on, instead of ${current.stream.translator} in ${current.stream.quality}.`,
      confirmLabel: 'Use HDRezka',
    };
  }
  return {
    title: `Stream ${name} from HDRezka?`,
    description:
      'Infuse plays the HDRezka stream from now on, instead of the torrent. It may show up as a new file (it ends in .mp4), so Infuse may forget where you stopped.',
    confirmLabel: 'Use HDRezka',
  };
}
