import { ApiError } from '@/lib/api/client';
import type { Member } from '@/lib/api/contract';
import { gateState, type GateState, type MembersStatus, membersStatus } from '@/lib/members/gate';

/** Who's watching? (#239): what the protected layout shows, for every cookie and list. */

const member = (id: number, name: string): Member => ({
  id,
  name,
  color: 'teal',
  created_at: '2026-01-01T00:00:00Z',
  trakt: { state: 'off', username: null },
});
const TEYMUR = member(1, 'Teymur');
const SAM = member(2, 'Sam');
const TWO = [TEYMUR, SAM];
const ONE = [TEYMUR];

describe('gateState', () => {
  it.each<[string, number | null, MembersStatus, Member[] | undefined, GateState]>([
    // A valid cookie renders the app at once, whatever the list is doing
    ['cookie, list pending', 1, 'pending', undefined, { kind: 'app', member: null }],
    ['cookie, backend without members', 1, 'absent', undefined, { kind: 'app', member: null }],
    ['cookie, list failed', 2, 'error', undefined, { kind: 'app', member: null }],
    ['cookie, list confirms it', 2, 'ready', TWO, { kind: 'app', member: SAM }],
    ['cookie, the only member', 1, 'ready', ONE, { kind: 'app', member: TEYMUR }],
    // No cookie: nothing personal mounts until someone is chosen
    ['no cookie, list pending', null, 'pending', undefined, { kind: 'wait' }],
    ['no cookie, two members', null, 'ready', TWO, { kind: 'pick', members: TWO }],
    ['no cookie, one member', null, 'ready', ONE, { kind: 'auto', member: TEYMUR, stale: false }],
    ['no cookie, list failed', null, 'error', undefined, { kind: 'error' }],
    ['no cookie, backend without members', null, 'absent', undefined, { kind: 'app', member: null }],
    // A cookie that names nobody
    ['stale cookie, two members', 9, 'ready', TWO, { kind: 'stale' }],
    ['stale cookie, one member', 9, 'ready', ONE, { kind: 'auto', member: TEYMUR, stale: true }],
    // The contract says never empty: an empty list acts as no members
    ['cookie, empty list', 1, 'ready', [], { kind: 'app', member: null }],
    ['no cookie, empty list', null, 'ready', [], { kind: 'app', member: null }],
  ])('%s', (_name, cookie, status, items, want) => {
    expect(gateState(cookie, status, items)).toEqual(want);
  });
});

describe('membersStatus', () => {
  it('data wins, even after a failed refetch', () => {
    expect(membersStatus({ data: { items: ONE }, error: new ApiError(502, 'backend_unreachable', 'x') })).toBe('ready');
  });

  it('a 404 is a backend before household members; anything else is an error', () => {
    expect(membersStatus({ data: undefined, error: new ApiError(404, 'not_found', 'No such API route.') })).toBe('absent');
    expect(membersStatus({ data: undefined, error: new ApiError(502, 'backend_unreachable', 'x') })).toBe('error');
    expect(membersStatus({ data: undefined, error: new ApiError(500, 'internal_error', 'x') })).toBe('error');
    expect(membersStatus({ data: undefined, error: new TypeError('Failed to fetch') })).toBe('error');
  });

  it('pending until either', () => {
    expect(membersStatus({ data: undefined, error: null })).toBe('pending');
  });
});
