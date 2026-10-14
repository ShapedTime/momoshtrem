import type { Subtitle, SubtitleCandidate, SubtitleMatch } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';

/** Languages offered in the subtitle search, most asked-for first. */
export const SUBTITLE_LANGUAGES = [
  'en', 'az', 'ru', 'tr', 'de', 'fr', 'es', 'it', 'pt', 'nl', 'pl', 'sv', 'uk', 'ar', 'fa', 'he', 'hi', 'ja', 'ko', 'zh',
] as const;

/** The first language without a subtitle yet, so the search starts where you'd look next. */
export function defaultSubtitleLanguage(stored: Subtitle[]): string {
  const have = new Set(stored.map((subtitle) => subtitle.language_code));
  return SUBTITLE_LANGUAGES.find((code) => !have.has(code)) ?? SUBTITLE_LANGUAGES[0];
}

const MATCH_STATUS: Record<SubtitleMatch, StatusKey> = {
  exact_release: 'match_exact',
  same_source: 'match_same_source',
  different_timing: 'match_different_timing',
  unknown: 'match_unknown',
};

/** The status pill for a match grade (lib/status.ts words). */
export const matchStatus = (match: SubtitleMatch): StatusKey => MATCH_STATUS[match];

/** A candidate already stored: same language and release, downloaded from OpenSubtitles. */
export function isStored(candidate: SubtitleCandidate, stored: Subtitle[]): boolean {
  return stored.some(
    (subtitle) =>
      subtitle.source === 'opensubtitles' &&
      subtitle.language_code === candidate.language_code &&
      subtitle.release_name === candidate.release_name
  );
}
