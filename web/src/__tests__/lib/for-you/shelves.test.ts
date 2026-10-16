import type { Shelf, ShelfKind } from '@/lib/api/contract';
import { askSuggestions, layoutShelves, refreshingShelfSlug } from '@/lib/for-you/shelves';

/** Noir's shelves on For You (#155): their order and the Ask band. */

const shelf = (slug: string, kind: ShelfKind = 'other'): Shelf => ({ slug, kind, title: slug, subtitle: null, items: [] });

describe('layoutShelves', () => {
  it('the first shelf leads, the rest follow in the run’s order, Coming soon goes last', () => {
    const layout = layoutShelves([shelf('because', 'because'), shelf('new', 'new'), shelf('soon', 'coming_soon'), shelf('anime', 'anime'), shelf('gems', 'gems')]);
    expect(layout.lead?.slug).toBe('because');
    expect(layout.rest.map((s) => s.slug)).toEqual(['new', 'anime', 'gems']);
    expect(layout.comingSoon.map((s) => s.slug)).toEqual(['soon']);
  });

  it('Coming soon never leads', () => {
    const layout = layoutShelves([shelf('soon', 'coming_soon'), shelf('gems', 'gems')]);
    expect(layout.lead?.slug).toBe('gems');
    expect(layout.rest).toEqual([]);
    expect(layout.comingSoon.map((s) => s.slug)).toEqual(['soon']);
  });

  it('no shelves, nothing to lay out', () => {
    expect(layoutShelves([])).toEqual({ lead: null, rest: [], comingSoon: [] });
    expect(layoutShelves([shelf('soon', 'coming_soon')]).lead).toBeNull();
  });

  it('refreshing: the first shelf after the Ask band shows skeletons, else the lead', () => {
    expect(refreshingShelfSlug(layoutShelves([shelf('a'), shelf('b'), shelf('c')]))).toBe('b');
    expect(refreshingShelfSlug(layoutShelves([shelf('a'), shelf('soon', 'coming_soon')]))).toBe('a');
    expect(refreshingShelfSlug(layoutShelves([]))).toBeNull();
  });
});

describe('askSuggestions', () => {
  it('three, the first leaning on a show you’re watching', () => {
    expect(askSuggestions('The Studio')).toEqual(['something like The Studio but lighter', 'a 90-minute thriller for tonight', 'anime with a gentle, cosy vibe']);
    expect(askSuggestions(null)[0]).toBe('something light and funny for tonight');
    expect(askSuggestions('  ')).toHaveLength(3);
  });
});
