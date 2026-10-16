import { ASK_ROUTE } from '@/lib/recommendations/ask-href';

/**
 * The thread in an Ask URL: null on /ask, the id on /ask/{id}. Anything
 * else under /ask/ reads as 0, a thread that never exists, so the page says
 * the conversation isn't there.
 */
export function askRouteThread(pathname: string): number | null {
  if (pathname === ASK_ROUTE || pathname === `${ASK_ROUTE}/`) return null;
  const match = /^\/ask\/(\d{1,15})\/?$/.exec(pathname);
  return match ? Number(match[1]) : 0;
}

/**
 * "New conversation" from the history sheet, which lives outside the Ask
 * screen: on /ask itself a link changes nothing, so the screen also
 * listens for this.
 */
export const ASK_NEW_EVENT = 'momoshtrem:ask-new';
