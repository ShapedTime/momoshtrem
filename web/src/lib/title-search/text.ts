/**
 * Text matching for search and the palette (js/screens/search.js): folding
 * that keeps character positions (for highlights), the normalised form the
 * ranking compares, and the query's words. Pure, so Jest checks it.
 */

const COMBINING_MARKS = /[̀-ͯ]/g;

/** At most this many characters in a query (the route answers 400 beyond it). */
export const MAX_QUERY_LENGTH = 100;

/**
 * One character, lower-cased and without accents ("Ō" → "o"). Always one
 * UTF-16 unit, so a folded string's indices are the original's character
 * indices.
 */
export function foldChar(char: string): string {
  const folded = char.normalize('NFD').replace(COMBINING_MARKS, '').toLowerCase();
  return folded.length === 1 ? folded : char.toLowerCase().slice(0, 1) || ' ';
}

export const fold = (text: string): string => [...text].map(foldChar).join('');

/**
 * What ranking compares: folded, apostrophes dropped ("Journey’s" →
 * "journeys"), other punctuation as spaces, single spaces.
 */
export function normalize(text: string): string {
  return fold(text)
    .replace(/['’‘`]/g, '')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();
}

/** The query's words, normalised; none for a blank query. */
export function queryWords(query: string): string[] {
  const normalized = normalize(query);
  return normalized ? normalized.split(' ') : [];
}

/** Every word of the query appears somewhere in `text` ("sev" is in "Severance"). */
export function hasEveryWord(text: string, query: string): boolean {
  const words = queryWords(query);
  if (!words.length) return false;
  const haystack = normalize(text);
  return words.every((word) => haystack.includes(word));
}

/** A query as the route takes it: trimmed, single spaces, at most 100 characters; null when blank or too long. */
export function cleanQuery(raw: string | null | undefined): string | null {
  const query = (raw ?? '').replace(/\s+/g, ' ').trim();
  return query && query.length <= MAX_QUERY_LENGTH ? query : null;
}

export interface Segment {
  text: string;
  match: boolean;
}

/** Where `needle` first starts a word in `haystack`, else where it first appears; -1 when it doesn't. */
function findWord(haystack: string, needle: string): number {
  let index = haystack.indexOf(needle);
  const first = index;
  while (index > 0 && /[\p{L}\p{N}]/u.test(haystack[index - 1])) index = haystack.indexOf(needle, index + 1);
  return index >= 0 ? index : first;
}

/**
 * `text` split into the parts that match the query and the rest, for
 * `<mark>`: the whole query where it appears ("Sev|erance"), otherwise each
 * word where it first starts a word ("Dune|: |Prophecy" for "dune prophecy").
 */
export function highlight(text: string, query: string): Segment[] {
  const chars = [...text];
  const haystack = chars.map(foldChar).join('');
  const needle = fold(query).replace(/\s+/g, ' ').trim();
  if (!needle) return [{ text, match: false }];

  let ranges: Array<[number, number]> = [];
  const whole = findWord(haystack, needle);
  if (whole >= 0) ranges = [[whole, whole + needle.length]];
  else {
    for (const word of needle.split(' ')) {
      const at = findWord(haystack, word);
      if (at >= 0) ranges.push([at, at + word.length]);
    }
  }
  if (!ranges.length) return [{ text, match: false }];

  ranges.sort((a, b) => a[0] - b[0]);
  const merged: Array<[number, number]> = [];
  for (const [start, end] of ranges) {
    const last = merged[merged.length - 1];
    if (last && start <= last[1]) last[1] = Math.max(last[1], end);
    else merged.push([start, end]);
  }

  const segments: Segment[] = [];
  let cursor = 0;
  for (const [start, end] of merged) {
    if (start > cursor) segments.push({ text: chars.slice(cursor, start).join(''), match: false });
    segments.push({ text: chars.slice(start, end).join(''), match: true });
    cursor = end;
  }
  if (cursor < chars.length) segments.push({ text: chars.slice(cursor).join(''), match: false });
  return segments;
}
