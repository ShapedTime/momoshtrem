/** @jest-environment node */
import { currentUrl, switchMember } from '@/lib/members/switch';

/** Switching who's watching (#239): cookie, then the cache, then a full navigation. */

const steps: string[] = [];

beforeEach(() => {
  const doc = {};
  Object.defineProperty(doc, 'cookie', { set: (value: string) => steps.push(`cookie ${value.split(';')[0]}`), configurable: true });
  Object.assign(globalThis, {
    document: doc,
    window: { location: { pathname: '/tv/209867', search: '?season=2', hash: '#e5', assign: (to: string) => steps.push(`go ${to}`) } },
  });
});

afterEach(() => {
  steps.length = 0;
  delete (globalThis as { document?: unknown }).document;
  delete (globalThis as { window?: unknown }).window;
});

describe('switchMember', () => {
  it('writes the cookie, drops the cache, then goes to For You', () => {
    switchMember(2, () => steps.push('clear'));
    expect(steps).toEqual(['cookie momoshtrem_member=2', 'clear', 'go /']);
  });

  it('goes where it’s told: the picker keeps the URL', () => {
    switchMember(1, () => steps.push('clear'), currentUrl());
    expect(steps).toEqual(['cookie momoshtrem_member=1', 'clear', 'go /tv/209867?season=2#e5']);
  });
});
