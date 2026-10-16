import type { Shelf, ShelfKind } from '@/lib/api/contract';
import { askSuggestions, comingSoonBadge, layoutShelves, recCardSubtitle, refreshingShelfSlug } from '@/lib/for-you/shelves';

/** Noir's shelves on For You (#155): their order, Coming soon's labels, the cards' names and the Ask band. */

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

describe('comingSoonBadge', () => {
  const today = new Date(2026, 9, 16); // Oct 16, 2026

  it('the run’s badge wins', () => {
    expect(comingSoonBadge({ badge: 'Summer', release_date: '2026-11-01' }, today)).toBe('Summer');
    expect(comingSoonBadge({ badge: ' 2026 ', release_date: null }, today)).toBe('2026');
  });

  it('else the release date while it’s ahead: the day within about six months, the year after that', () => {
    expect(comingSoonBadge({ badge: null, release_date: '2026-10-16' }, today)).toBe('Oct 16');
    expect(comingSoonBadge({ badge: null, release_date: '2027-01-16' }, today)).toBe('Jan 16');
    expect(comingSoonBadge({ badge: '', release_date: '2027-04-16' }, today)).toBe('Apr 16');
    expect(comingSoonBadge({ badge: null, release_date: '2027-06-30' }, today)).toBe('2027');
  });

  it('nothing once it’s out, or without a date', () => {
    expect(comingSoonBadge({ badge: null, release_date: '2022-02-17' }, today)).toBeNull();
    expect(comingSoonBadge({ badge: null, release_date: '2026-10-15' }, today)).toBeNull();
    expect(comingSoonBadge({ badge: null, release_date: null }, today)).toBeNull();
    expect(comingSoonBadge({ badge: null, release_date: 'soon' }, today)).toBeNull();
  });
});

describe('recCardSubtitle', () => {
  it('the year and kind, or the coming date', () => {
    expect(recCardSubtitle({ year: 2023, media_type: 'tv' })).toBe('2023 · Series');
    expect(recCardSubtitle({ year: 2016, media_type: 'movie' })).toBe('2016 · Movie');
    expect(recCardSubtitle({ year: null, media_type: 'tv' })).toBe('Series');
    expect(recCardSubtitle({ year: 2022, media_type: 'tv' }, 'Jan 16')).toBe('Coming Jan 16 · Series');
  });
});

describe('askSuggestions', () => {
  it('three, the first leaning on a show you’re watching', () => {
    expect(askSuggestions('The Studio')).toEqual(['something like The Studio but lighter', 'a 90-minute thriller for tonight', 'anime with a gentle, cosy vibe']);
    expect(askSuggestions(null)[0]).toBe('something light and funny for tonight');
    expect(askSuggestions('  ')).toHaveLength(3);
  });
});
