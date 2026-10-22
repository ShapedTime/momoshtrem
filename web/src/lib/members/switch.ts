import { useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { writeMemberCookie } from './cookie';

let leaving = false;

/**
 * Switch who's watching (#239): the cookie names them, every cached query is
 * dropped, then a full navigation. The reload also resets the module-level
 * stores (feedback per title, Ask's turn count) and closes an open Ask
 * stream; the backend cancels that turn as the member who started it,
 * because that request carried their header. Query keys stay unprefixed.
 *
 * The picker passes the current URL, so a deep link survives; the account
 * menu goes to `/`, which also drops any `?sheet=`.
 */
export function switchMember(id: number, clearCache: () => void, to = '/'): void {
  leaving = true;
  writeMemberCookie(id);
  clearCache();
  page.assign(to);
}

/**
 * This page switched person and is on its way out: the cookie it just wrote
 * isn't another tab's doing, so the gate mustn't reload over the navigation.
 */
export const switchingMember = (): boolean => leaving;

/** The full navigations who's watching needs, in one object so tests can watch them (jsdom can't navigate). */
export const page = {
  assign: (to: string) => window.location.assign(to),
  reload: () => window.location.reload(),
};

/** This page's URL, for the picker: where the person was going. */
export function currentUrl(): string {
  const { pathname, search, hash } = window.location;
  return `${pathname}${search}${hash}`;
}

/** `switchMember` with this app's cache, like `useSignOut`. */
export function useSwitchMember(): (id: number, to?: string) => void {
  const queryClient = useQueryClient();
  return useCallback((id: number, to?: string) => switchMember(id, () => queryClient.clear(), to), [queryClient]);
}

/** Tests: a fresh page. */
export function resetSwitching(): void {
  leaving = false;
}
