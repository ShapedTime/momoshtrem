/** @jest-environment node */
import { safeRedirect } from '@/lib/auth/redirect';

describe('safeRedirect', () => {
  it.each([
    ['/library', '/library'],
    ['/library?type=tv&sort=added', '/library?type=tv&sort=added'],
    ['/tv/123#season-2', '/tv/123#season-2'],
    ['/settings?section=playback', '/settings?section=playback'],
  ])('keeps same-origin path %s', (input, expected) => {
    expect(safeRedirect(input)).toBe(expected);
  });

  it.each([
    ['//evil.com'],
    ['//evil.com/library'],
    ['/\\evil.com'],
    ['https://evil.com'],
    ['javascript:alert(1)'],
    ['evil.com'],
    ['/\tjavascript:alert(1)'],
    ['/login'],
    ['/login?redirect=/library'],
    ['/api/torrents'],
    [''],
    [undefined],
    [42],
  ])('falls back to / for %p', (input) => {
    expect(safeRedirect(input)).toBe('/');
  });

  it('keeps encoded characters that stay on this origin', () => {
    expect(safeRedirect('/search?q=Frieren%3A%20Beyond')).toBe('/search?q=Frieren%3A%20Beyond');
  });

  it('uses a custom fallback', () => {
    expect(safeRedirect('//evil.com', '/library')).toBe('/library');
  });
});
