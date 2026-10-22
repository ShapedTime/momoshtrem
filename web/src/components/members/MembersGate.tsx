'use client';

import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Loader, LoadingRegion } from '@/components/ui';
import type { Member } from '@/lib/api/contract';
import { membersQuery } from '@/lib/api/endpoints';
import { clearMemberCookie, writeMemberCookie } from '@/lib/members/cookie';
import { gateState, type MembersStatus, membersStatus } from '@/lib/members/gate';
import { leavePage, leavingPage, memberChangedUnderPage, page, pageRestored, setPageMember } from '@/lib/members/page';
import { WhoIsWatching } from './WhoIsWatching';

export interface MembersContextValue {
  status: MembersStatus;
  /** Everyone, by id; empty until the list loads, and without household members. */
  members: Member[];
  /** Who's watching; null until the list confirms them, and without household members. */
  current: Member | null;
  /** Everyone else: the account menu's "Switch to …" rows. */
  others: Member[];
}

const EMPTY: MembersContextValue = { status: 'pending', members: [], current: null, others: [] };

const MembersContext = createContext<MembersContextValue>(EMPTY);

/** The household as the gate sees it. Outside a gate (/dev/ui, Jest) it's empty, as without members. */
export function useMembers(): MembersContextValue {
  return useContext(MembersContext);
}

/**
 * Another tab switched person (the cookie is shared), it happened between the
 * server's render and now, or back/forward cache restored a page rendered for
 * someone else: load the page again as whoever the cookie names now. Until
 * then `apiFetch` sends nothing (lib/members/page.ts).
 */
function useReloadWhenCookieChanges(renderedFor: number | null) {
  useEffect(() => {
    const check = () => {
      // This page's own switch wrote the cookie and is navigating away: never reload over it
      if (!leavingPage() && memberChangedUnderPage()) page.reload();
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return;
      // Restored from the back/forward cache: the switch it started is over, and the cookie may be someone else's
      pageRestored();
      check();
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible') check();
    };
    check();
    window.addEventListener('pageshow', onPageShow);
    window.addEventListener('focus', check);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      window.removeEventListener('pageshow', onPageShow);
      window.removeEventListener('focus', check);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [renderedFor]);
}

/** No cookie while the list loads: nothing for 150 ms, then Noir. */
function Waiting() {
  return (
    <main id="main" className="who">
      <LoadingRegion label="Loading who’s watching" className="who-body grow justify-center">
        <Loader title="Loading who’s watching…" />
      </LoadingRegion>
    </main>
  );
}

/**
 * Who's watching? (#239), around the app shell in the protected layout. The
 * server read the `momoshtrem_member` cookie (`initialMemberId`), so the first
 * render matches the server's (#148): with a cookie the app renders at once,
 * without one nothing personal mounts (the shell's own queries included)
 * until someone is chosen. One member is chosen automatically; a cookie that
 * names nobody, or changed in another tab, reloads the page. A backend
 * without household members (404) gets the app as before.
 */
export function MembersGate({ initialMemberId, children }: { initialMemberId: number | null; children: ReactNode }) {
  const query = useQuery(membersQuery());
  const [cookieId, setCookieId] = useState(initialMemberId);
  const status = membersStatus(query);
  const state = gateState(cookieId, status, query.data?.items);
  // Who this page renders for, before any child's first request (client only; the same value on every render)
  if (typeof window !== 'undefined') setPageMember(cookieId);

  // Effects, not render-phase writes: the cookie is in place before any child's query starts
  const autoId = state.kind === 'auto' ? state.member.id : null;
  const reload = (state.kind === 'auto' && state.stale) || state.kind === 'stale';
  useEffect(() => {
    if (reload) {
      // The page was rendered for someone nobody is: load it again as the only member, or for the picker
      leavePage();
      if (autoId !== null) writeMemberCookie(autoId);
      else clearMemberCookie();
      page.reload();
    } else if (autoId !== null) {
      setPageMember(autoId);
      writeMemberCookie(autoId);
      // The cookie (an external system) is written now; only then may the children mount and send it
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reveals the app once the cookie is in place
      setCookieId(autoId);
    }
  }, [autoId, reload]);

  useReloadWhenCookieChanges(cookieId);

  const current = state.kind === 'app' ? state.member : null;
  const value = useMemo<MembersContextValue>(() => {
    const members = query.data?.items ?? [];
    return { status, members, current, others: current ? members.filter((m) => m.id !== current.id) : [] };
  }, [status, query.data, current]);

  switch (state.kind) {
    case 'app':
      return <MembersContext.Provider value={value}>{children}</MembersContext.Provider>;
    case 'pick':
      return <WhoIsWatching members={state.members} />;
    case 'error':
      return <WhoIsWatching failed={{ onRetry: () => void query.refetch(), retrying: query.isFetching }} />;
    case 'wait':
      return <Waiting />;
    default:
      // Choosing the only member, or reloading without a stale cookie: nothing to show for a moment
      return null;
  }
}
