import { comingSoonBadge, recCardSubtitle } from '@/lib/recommendations/cards';

/** What a recommended title's card says (#155): Coming soon's label and the link's name. */

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
