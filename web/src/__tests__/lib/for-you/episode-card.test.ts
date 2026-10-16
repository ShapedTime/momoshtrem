import type { NewEpisodeItem, UpNextAvailability, UpNextItem } from '@/lib/api/contract';
import {
  airedLabel,
  airsLabel,
  calendarDay,
  episodeCardCode,
  episodeName,
  newEpisodeCard,
  upNextCard,
} from '@/lib/for-you/episode-card';
import { DEFAULT_TRAKT_SETTINGS, upNextItems } from '@/mocks/fixtures/trakt';
import { seedLibrary } from '@/mocks/fixtures/titles';

/** Friday 16 October 2026, 21:00 local. */
const NOW = new Date(2026, 9, 16, 21, 0);
const local = (day: number, hour = 20) => new Date(2026, 9, day, hour, 0).toISOString();

function upNext(availability: UpNextAvailability, overrides: Partial<UpNextItem> = {}): UpNextItem {
  return {
    show: { tmdb_id: 209867, title: 'Frieren', poster_path: null, backdrop_path: null, library_id: 1 },
    next_episode: { season: 1, episode: 29, absolute_number: 29, name: 'Let’s Go Somewhere We Can’t Reach', air_date: '2026-10-10', still_path: null, tmdb_id: null },
    availability,
    airs_at: null,
    progress: { completed: 28, aired: 30 },
    last_watched_at: local(15),
    source: 'progress',
    webdav_path: null,
    webdav_url: null,
    download: null,
    review_job_id: null,
    ...overrides,
  };
}

describe('episode labels', () => {
  it('reads S01E29, and adds the absolute number only where it isn’t the in-season one', () => {
    expect(episodeCardCode({ season: 1, episode: 29, absolute_number: 29 })).toBe('S01E29');
    expect(episodeCardCode({ season: 1, episode: 29, absolute_number: null })).toBe('S01E29');
    expect(episodeCardCode({ season: 22, episode: 39, absolute_number: 1124 })).toBe('S22E39 · #1124');
  });

  it('falls back to “Episode 29” without a name', () => {
    expect(episodeName({ episode: 29, name: null })).toBe('Episode 29');
    expect(episodeName({ episode: 29, name: '  ' })).toBe('Episode 29');
    expect(episodeName({ episode: 29, name: 'The Gift' })).toBe('The Gift');
  });
});

describe('days', () => {
  it('reads an air date at midnight UTC as that date, whatever the time zone', () => {
    const day = calendarDay('2026-10-15T00:00:00.000Z');
    expect([day.getFullYear(), day.getMonth(), day.getDate()]).toEqual([2026, 9, 15]);
    const timed = calendarDay(local(15, 23));
    expect(timed.getDate()).toBe(15);
  });

  it('says Today, Yesterday, the weekday within the week, then the date', () => {
    expect(airedLabel(local(16, 1), NOW)).toBe('Today');
    expect(airedLabel(local(15), NOW)).toBe('Yesterday');
    expect(airedLabel('2026-10-14T00:00:00.000Z', NOW)).toBe('Wed');
    expect(airedLabel(local(10), NOW)).toBe('Sat');
    expect(airedLabel(local(9), NOW)).toBe('Oct 9');
    expect(airedLabel(new Date(2025, 11, 30, 20).toISOString(), NOW)).toBe('Dec 30, 2025');
  });

  it('says when an episode airs: today, tomorrow, the weekday, then the date', () => {
    expect(airsLabel(local(16, 23), NOW)).toBe('Airs today');
    expect(airsLabel('2026-10-17T00:00:00.000Z', NOW)).toBe('Airs tomorrow');
    expect(airsLabel(local(18), NOW)).toBe('Airs Sun');
    expect(airsLabel(local(22), NOW)).toBe('Airs Thu');
    expect(airsLabel(local(23), NOW)).toBe('Airs Oct 23');
  });
});

describe('availability → pill and action', () => {
  it('ready: Ready to watch, and Play in Infuse with the episode’s file', () => {
    const card = upNextCard(upNext('ready', { webdav_path: '/TV Shows/Frieren/S01E29.mkv', webdav_url: 'https://m.local/TV%20Shows/Frieren/S01E29.mkv' }), NOW);
    expect(card.pill).toEqual({ status: 'ready', label: 'Ready to watch' });
    expect(card.action).toEqual({
      intent: 'play',
      label: 'Play in Infuse',
      ariaLabel: 'Play Frieren S01E29 in Infuse',
      path: '/TV Shows/Frieren/S01E29.mkv',
      url: 'https://m.local/TV%20Shows/Frieren/S01E29.mkv',
    });
    expect(card.downloadPct).toBeNull();
  });

  it('downloading: the percentage, a bar on the still, and Details on Activity', () => {
    const card = upNextCard(upNext('downloading', { download: { pct: 41.6, info_hash: 'abc' } }), NOW);
    expect(card.pill).toEqual({ status: 'downloading', label: 'Downloading 42%', pct: 42 });
    expect(card.action).toMatchObject({ intent: 'details', label: 'Details', href: '/activity/abc' });
    expect(card.downloadPct).toBe(42);
  });

  it('needs review: Review opens the mapping job', () => {
    const card = upNextCard(upNext('needs_review', { review_job_id: 102 }), NOW);
    expect(card.pill).toEqual({ status: 'needs_review', label: 'Needs review' });
    expect(card.action).toMatchObject({ intent: 'review', label: 'Review', href: '/mapping/102' });
  });

  it('needs a torrent: Find opens the torrent sheet on the episode', () => {
    const card = upNextCard(upNext('needs_torrent'), NOW);
    expect(card.pill).toEqual({ status: 'needs_torrent', label: 'Needs a torrent' });
    expect(card.action).toEqual({ intent: 'find', label: 'Find', ariaLabel: 'Find a torrent for Frieren S01E29', params: { show: '209867', se: 'S01E29' } });
  });

  it('not in the library: Add the show', () => {
    const card = upNextCard(upNext('not_in_library', { show: { tmdb_id: 66573, title: 'The Good Place', poster_path: null, backdrop_path: null, library_id: null } }), NOW);
    expect(card.pill).toEqual({ status: 'not_in_library', label: 'Not in library' });
    expect(card.action).toEqual({ intent: 'add', label: 'Add', ariaLabel: 'Add The Good Place to your library', tmdbId: 66573, show: 'The Good Place' });
  });

  it('airs on: no action, and the pill says when', () => {
    expect(upNextCard(upNext('airs_on', { airs_at: '2026-10-18T00:00:00.000Z' }), NOW)).toMatchObject({ pill: { status: 'airs_on', label: 'Airs Sun' }, action: null });
    expect(upNextCard(upNext('airs_on'), NOW).pill).toEqual({ status: 'upcoming', label: 'Upcoming' });
  });

  it('links to the show on the episode’s season, with the still or else the backdrop', () => {
    const card = upNextCard(upNext('ready', { show: { tmdb_id: 209867, title: 'Frieren', poster_path: null, backdrop_path: '/b.jpg', library_id: 1 } }), NOW);
    expect(card).toMatchObject({ href: '/tv/209867?season=1', show: 'Frieren', code: 'S01E29', image: { path: '/b.jpg', kind: 'backdrop' } });
    const still = upNextCard(upNext('ready', { next_episode: { ...upNext('ready').next_episode, still_path: '/s.jpg' } }), NOW);
    expect(still.image).toEqual({ path: '/s.jpg', kind: 'still' });
  });

  it('gives every card of the mock account its action', () => {
    const cards = upNextItems(seedLibrary(), DEFAULT_TRAKT_SETTINGS).map((item) => upNextCard(item, NOW));
    expect(cards.map((card) => [card.show, card.code, card.action?.label ?? null])).toEqual([
      ['The Studio', 'S01E10', 'Details'],
      ['Frieren: Beyond Journey’s End', 'S01E27', 'Play in Infuse'],
      ['One Piece', 'S21E01 · #892', 'Review'],
      ['The Good Place', 'S01E06', 'Add'],
      ['Doctor Who', expect.stringMatching(/^S03E04/), 'Find'],
      ['Bluey', 'S01E01', 'Play in Infuse'],
      ['Severance', expect.stringMatching(/^S02E10/), null],
    ]);
  });
});

describe('new episodes', () => {
  it('put when it aired on the still', () => {
    const item: NewEpisodeItem = {
      show: { tmdb_id: 37854, title: 'One Piece', poster_path: null, backdrop_path: null, library_id: 2 },
      episode: { season: 22, episode: 38, absolute_number: 1123, name: 'The Bonds Between Giants', air_date: '2026-10-16', still_path: null, tmdb_id: null },
      aired_at: '2026-10-16T00:00:00.000Z',
      availability: 'needs_torrent',
      webdav_path: null,
      webdav_url: null,
      download: null,
      review_job_id: null,
    };
    expect(newEpisodeCard(item, NOW)).toMatchObject({
      code: 'S22E38 · #1123',
      when: 'Today',
      pill: { label: 'Needs a torrent' },
      action: { intent: 'find', params: { show: '37854', se: 'S22E38' } },
    });
  });
});
