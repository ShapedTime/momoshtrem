/**
 * Links into Ask momoshtrem (#156): the top-bar pill, the palette's "Ask
 * momoshtrem: “q”", Search's "Not quite it?" card, Discover's empty state and
 * For You's Ask band all start an ask with `askHref(q)`.
 */

export const ASK_ROUTE = '/ask';
/** POST /recommendations/ask takes 1–500 characters: code points, as Go counts runes. */
export const ASK_MESSAGE_MAX = 500;

/** A message's length in code points (an emoji is one, not two UTF-16 units). */
export const askMessageLength = (message: string): number => Array.from(message).length;

/**
 * `/ask`, or `/ask?q=…` to start a new conversation with that message
 * (whitespace collapsed, at most 500 code points). Cutting by code point
 * never splits a surrogate pair, which `encodeURIComponent` would throw on.
 */
export function askHref(q?: string | null): string {
  const tidy = (q ?? '').replace(/\s+/g, ' ').trim();
  const message = Array.from(tidy).slice(0, ASK_MESSAGE_MAX).join('').trim();
  return message ? `${ASK_ROUTE}?q=${encodeURIComponent(message)}` : ASK_ROUTE;
}

/** A past conversation: `/ask/{thread_id}`. */
export function askThreadHref(threadId: number): string {
  return `${ASK_ROUTE}/${threadId}`;
}
