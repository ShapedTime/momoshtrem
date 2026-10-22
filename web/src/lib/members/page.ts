import { readMemberCookie } from './cookie';

/**
 * This page and who it was rendered for (#239). The cookie is shared by every
 * tab, so a tab rendered for one person must notice when another tab switched
 * to someone else: `MembersGate` publishes the member it rendered for, and
 * `apiFetch`, the gate's checks and recent searches compare against it. Client
 * only: nothing here runs on the server.
 */

/** The full navigations who's watching needs, in one object so tests can watch them (jsdom can't navigate). */
export const page = {
  assign: (to: string) => window.location.assign(to),
  reload: () => window.location.reload(),
};

/** This page's URL: where the person was going. */
export function currentUrl(): string {
  const { pathname, search, hash } = window.location;
  return `${pathname}${search}${hash}`;
}

/**
 * A full navigation to `to`. The current page itself (whatever its hash)
 * reloads: assigning the same URL with a `#…` only scrolls.
 */
export function loadPage(to: string): void {
  const target = new URL(to, window.location.href);
  const here = new URL(window.location.href);
  target.hash = '';
  here.hash = '';
  if (target.href === here.href) page.reload();
  else page.assign(to);
}

/** The member the page was rendered for: null without one, undefined until the gate says (and outside it). */
let rendered: number | null | undefined;
/** This page switched person (or forgot one) and is navigating away. */
let leaving = false;
/** Told when a leave is called off (`stayOnPage`): the gate, which holds the app on screen while the page leaves (#247). */
const stayListeners = new Set<() => void>();

/** Run `listener` whenever `stayOnPage()` calls a leave off. Answers the unsubscribe. */
export function onStayOnPage(listener: () => void): () => void {
  stayListeners.add(listener);
  return () => {
    stayListeners.delete(listener);
  };
}

export function setPageMember(id: number | null): void {
  rendered = id;
}

export const pageMember = (): number | null | undefined => rendered;

/** The cookie names someone other than the member this page was rendered for. */
export function memberChangedUnderPage(): boolean {
  return rendered !== undefined && readMemberCookie() !== rendered;
}

/** Before this page writes the cookie and navigates away: that change is its own, not another tab's. */
export function leavePage(): void {
  leaving = true;
}

export const leavingPage = (): boolean => leaving;

/**
 * A leave that didn't happen (#247: removing yourself failed): this page stays as it is, and a later cookie
 * change or `unknown_member` is someone else's doing again, and the gate goes back to what the list says.
 */
export function stayOnPage(): void {
  if (!leaving) return;
  leaving = false;
  for (const listener of stayListeners) listener();
}

/** A page restored from the back/forward cache is live again: whatever the cookie says now is someone else's doing. */
export function pageRestored(): void {
  leaving = false;
}

/** Tests: a fresh page. */
export function resetPage(): void {
  rendered = undefined;
  leaving = false;
}
