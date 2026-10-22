/** @jest-environment node */
import { leavingPage, loadPage, resetPage } from '@/lib/members/page';
import { currentUrl, switchMember } from '@/lib/members/switch';

/** Switching who's watching (#239): cookie, then the cache, then a full navigation. */

const steps: string[] = [];

function stubPage(url: string) {
  const { pathname, search, hash, href } = new URL(url, 'http://localhost');
  const doc = {};
  Object.defineProperty(doc, 'cookie', { set: (value: string) => steps.push(`cookie ${value.split(';')[0]}`), configurable: true });
  Object.assign(globalThis, {
    document: doc,
    window: {
      location: {
        href,
        pathname,
        search,
        hash,
        assign: (to: string) => steps.push(`go ${to}`),
        reload: () => steps.push('reload'),
      },
    },
  });
}

afterEach(() => {
  steps.length = 0;
  resetPage();
  delete (globalThis as { document?: unknown }).document;
  delete (globalThis as { window?: unknown }).window;
});

describe('switchMember', () => {
  it('marks the page as leaving, writes the cookie, drops the cache, then goes to For You', () => {
    stubPage('/tv/209867?season=2#e5');
    switchMember(2, () => steps.push('clear'));
    expect(steps).toEqual(['cookie momoshtrem_member=2', 'clear', 'go /']);
    expect(leavingPage()).toBe(true);
  });

  it('the picker’s target is this page: a reload, since assigning it with a #… would only scroll', () => {
    stubPage('/activity#act-dl-h');
    switchMember(1, () => steps.push('clear'), currentUrl());
    expect(steps).toEqual(['cookie momoshtrem_member=1', 'clear', 'reload']);
  });
});

describe('loadPage', () => {
  it.each([
    ['/tv/209867?season=2', '/tv/209867?season=2', 'reload'],
    ['/activity#act-dl-h', '/activity#act-dl-h', 'reload'],
    ['/activity#act-dl-h', '/activity', 'reload'],
    ['/?sheet=infuse-setup', '/', 'go /'],
    ['/library', '/', 'go /'],
  ])('on %s, loading %s → %s', (here, to, want) => {
    stubPage(here);
    loadPage(to);
    expect(steps).toEqual([want]);
  });
});
