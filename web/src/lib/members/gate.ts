import { isApiError } from '@/lib/api/client';
import type { Member } from '@/lib/api/contract';

/**
 * Who's watching? (#239): what the protected layout shows, from the cookie the
 * server read and GET /members. Pure, so Jest covers the whole table;
 * components/members/MembersGate renders it.
 */

/** GET /members: `absent` is a 404 (a backend before household members), `error` any other failure. */
export type MembersStatus = 'pending' | 'ready' | 'absent' | 'error';

export type GateState =
  /** No cookie, and the list is on its way: render nothing (no personal query may start). */
  | { kind: 'wait' }
  /**
   * The app. With a valid cookie it renders at once (optimistically while the list loads, is absent or
   * failed), so the normal case never waits; `member` is null until the list confirms it, and without members.
   */
  | { kind: 'app'; member: Member | null }
  /** Two or more members and no valid cookie: the picker. */
  | { kind: 'pick'; members: Member[] }
  /** Exactly one member and no valid cookie: choose them. `stale`: a cookie named someone else, so the page reloads. */
  | { kind: 'auto'; member: Member; stale: boolean }
  /** The cookie names nobody and there's a choice to make: forget it and reload (the picker follows). */
  | { kind: 'stale' }
  /** No cookie, and the list failed (not a 404): the picker's error state. */
  | { kind: 'error' };

/** The list query's status: data wins (a failed refetch keeps it), then a 404, then any other error. */
export function membersStatus(query: { data?: unknown; error: unknown }): MembersStatus {
  if (query.data !== undefined) return 'ready';
  if (query.error) return isApiError(query.error) && query.error.status === 404 ? 'absent' : 'error';
  return 'pending';
}

export function gateState(cookieId: number | null, status: MembersStatus, items: readonly Member[] = []): GateState {
  if (status === 'absent') return { kind: 'app', member: null };
  if (status !== 'ready') {
    if (cookieId !== null) return { kind: 'app', member: null };
    return status === 'error' ? { kind: 'error' } : { kind: 'wait' };
  }
  // The contract says never empty; a backend that answers none acts as one without members
  if (items.length === 0) return { kind: 'app', member: null };
  const member = cookieId === null ? undefined : items.find((m) => m.id === cookieId);
  if (member) return { kind: 'app', member };
  const stale = cookieId !== null;
  if (items.length === 1) return { kind: 'auto', member: items[0], stale };
  return stale ? { kind: 'stale' } : { kind: 'pick', members: [...items] };
}
