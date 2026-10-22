import type { Member } from '@/lib/api/contract';
import { readMemberCookie } from '@/lib/members/cookie';
import { type MockScenario, scenario } from '../scenario';
import { iso } from './catalog';

/**
 * The household (PLAN §6 "Household members"; #239): Teymur (1, teal, Trakt
 * `teymur`) and Sam (2, rose, Trakt `sam`). The list is kept in localStorage
 * (`msw:members`), because switching reloads the page and MSW state lives per
 * page; in Node it's kept in memory. Who's asking is the `momoshtrem_member`
 * cookie (or `setMockMember` in Node), as the proxy's header would be: a
 * member nobody has, or none, is the lowest id (Go's no-header rule).
 *
 * Trakt and picks state per member lives in fixtures/trakt.ts and
 * fixtures/recommendations.ts (per page, as before), keyed by `mockMemberId()`.
 */

export type MockMember = Omit<Member, 'trakt'>;

interface MockHousehold {
  /** The scenario it was seeded for: another one starts over. */
  scenario: MockScenario | null;
  items: MockMember[];
  nextId: number;
}

const STORAGE_KEY = 'msw:members';

const SEEDS: MockMember[] = [
  { id: 1, name: 'Teymur', color: 'teal', created_at: iso(400) },
  { id: 2, name: 'Sam', color: 'rose', created_at: iso(120) },
];

/** `members-full`: six people, every colour taken. */
const FULL: MockMember[] = [
  ...SEEDS,
  { id: 3, name: 'Alex', color: 'periwinkle', created_at: iso(60) },
  { id: 4, name: 'Mina', color: 'lime', created_at: iso(30) },
  { id: 5, name: 'Jo', color: 'orchid', created_at: iso(20) },
  { id: 6, name: 'Riley', color: 'sand', created_at: iso(10) },
];

function seeded(from: MockScenario | null): MockHousehold {
  const items = from === 'members-one' ? SEEDS.slice(0, 1) : from === 'members-full' ? FULL : SEEDS;
  return { scenario: from, items: items.map((m) => ({ ...m })), nextId: Math.max(...items.map((m) => m.id)) + 1 };
}

let memory: MockHousehold | undefined;
let memberOverride: number | null = null;

const isHousehold = (value: unknown): value is MockHousehold =>
  typeof value === 'object' && value !== null && Array.isArray((value as MockHousehold).items) && typeof (value as MockHousehold).nextId === 'number';

function load(): MockHousehold | undefined {
  if (typeof window === 'undefined') return memory;
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? 'null');
    return isHousehold(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

function save(household: MockHousehold): void {
  memory = household;
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(household));
  } catch {
    // Not kept past this page
  }
}

function household(): MockHousehold {
  const from = scenario();
  const stored = load();
  if (stored && stored.scenario === from) return stored;
  const fresh = seeded(from);
  save(fresh);
  return fresh;
}

/** Everyone, by id. */
export const mockMembers = (): MockMember[] => household().items;

/** Writes go through here so the list survives the reload a switch does. Answers what `change` does. */
export function updateMockMembers<T>(change: (household: { items: MockMember[]; nextId: number }) => T): T {
  const current = household();
  const result = change(current);
  save(current);
  return result;
}

/** `members-absent`: a backend before household members, which ignores the header and has one Trakt account. */
const absent = () => scenario() === 'members-absent';

/** The member the request names, or null. */
function named(): number | null {
  if (memberOverride !== null) return memberOverride;
  return typeof document === 'undefined' ? null : readMemberCookie(document.cookie);
}

/** Who the request is for: the cookie's member, else the lowest id. */
export function mockMemberId(): number {
  const items = mockMembers();
  const id = named();
  if (!absent() && id !== null && items.some((m) => m.id === id)) return id;
  return Math.min(...items.map((m) => m.id));
}

/** The request names a member nobody has (personal routes answer 400 `unknown_member`). */
export function mockMemberStale(): boolean {
  const id = named();
  return !absent() && id !== null && !mockMembers().some((m) => m.id === id);
}

export function mockMemberName(id: number): string | undefined {
  return mockMembers().find((m) => m.id === id)?.name;
}

/** Tests in Node, where there's no cookie: who's asking (null: nobody named). */
export function setMockMember(id: number | null): void {
  memberOverride = id;
}

/** Tests: back to the seeds. */
export function resetMockMembers(): void {
  memory = undefined;
  memberOverride = null;
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Nothing kept
  }
}
