/**
 * Links into Ask momoshtrem (#156): the top-bar pill, the palette's "Ask
 * momoshtrem: “q”", Search's "Not quite it?" card, Discover's empty state and
 * For You's Ask band all start an ask with `askHref(q)`.
 */

export const ASK_ROUTE = '/ask';
/** POST /recommendations/ask takes 1–500 characters. */
export const ASK_MESSAGE_MAX = 500;

/** `/ask`, or `/ask?q=…` to start a new conversation with that message (whitespace collapsed, at most 500 characters). */
export function askHref(q?: string | null): string {
  const message = (q ?? '').replace(/\s+/g, ' ').trim().slice(0, ASK_MESSAGE_MAX).trim();
  return message ? `${ASK_ROUTE}?q=${encodeURIComponent(message)}` : ASK_ROUTE;
}

/** A past conversation: `/ask/{thread_id}`. */
export function askThreadHref(threadId: number): string {
  return `${ASK_ROUTE}/${threadId}`;
}
