import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { clearMemberCookie, writeMemberCookie } from './cookie';
import { leavePage, loadPage } from './page';

export { currentUrl } from './page';

/**
 * Switch who's watching (#239): the cookie names them, every cached query is
 * dropped, then a full navigation (a reload when `to` is this page). The
 * reload also resets the module-level stores (feedback per title, Ask's turn
 * count) and closes an open Ask stream; the backend cancels that turn as the
 * member who started it, because that request carried their header. Query
 * keys stay unprefixed. Until the page goes, `apiFetch` sends nothing more.
 *
 * The picker passes the current URL, so a deep link survives; the account
 * menu goes to `/`, which also drops any `?sheet=`.
 */
export function switchMember(id: number, clearCache: () => void, to = '/'): void {
  leavePage();
  writeMemberCookie(id);
  clearCache();
  loadPage(to);
}

/**
 * Removing yourself (#247): nobody is chosen any more. As a switch does, the
 * page is marked leaving, then the cookie goes, every cached query is
 * dropped, and a full navigation to `/`, where "Who's watching?" follows (or
 * the only person left, chosen automatically).
 */
export function leaveHousehold(clearCache: () => void): void {
  leavePage();
  clearMemberCookie();
  clearCache();
  loadPage('/');
}

/** `switchMember` with this app's cache, like `useSignOut`. */
export function useSwitchMember(): (id: number, to?: string) => void {
  const queryClient = useQueryClient();
  return useCallback((id: number, to?: string) => switchMember(id, () => queryClient.clear(), to), [queryClient]);
}
