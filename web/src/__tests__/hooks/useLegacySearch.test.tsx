import { act, renderHook } from '@testing-library/react';
import { closeLegacySearch, openLegacySearch, useLegacySearchHost } from '@/hooks/useLegacySearch';
import type { TorrentSearchContext } from '@/types/jackett';

const frieren: TorrentSearchContext = { mediaType: 'episode', query: 'Frieren S01E28', title: 'Frieren', tmdbId: 209867, season: 1, episode: 28 };
const dune: TorrentSearchContext = { mediaType: 'movie', query: 'Dune: Part Two 2024', title: 'Dune: Part Two', tmdbId: 693134, year: 2024 };

afterEach(() => act(() => closeLegacySearch()));

describe('the legacy search store', () => {
  it('opens on the mounted host', () => {
    const { result } = renderHook(() => useLegacySearchHost());
    expect(result.current).toBeNull();
    act(() => openLegacySearch(dune));
    expect(result.current).toEqual(dune);
    act(() => closeLegacySearch());
    expect(result.current).toBeNull();
  });

  it('ignores a search opened with no host, so the next title doesn’t inherit it', () => {
    // An episode sheet deep-linked outside a show page asks for a search
    act(() => openLegacySearch(frieren));
    const { result } = renderHook(() => useLegacySearchHost());
    expect(result.current).toBeNull();
  });

  it('forgets an open search when its host goes away', () => {
    const first = renderHook(() => useLegacySearchHost());
    act(() => openLegacySearch(frieren));
    expect(first.result.current).toEqual(frieren);
    first.unmount();

    act(() => openLegacySearch(dune));
    const next = renderHook(() => useLegacySearchHost());
    expect(next.result.current).toBeNull();
  });
});
