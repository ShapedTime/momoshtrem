import type { EpisodeRange, ShowTitle } from '@/lib/api/contract';
import { defaultRangeIndex, earlierWatched, parseJump, rangeLabel, resolveJump, shouldVirtualize } from '@/lib/title/episodes';
import { buildSeason, buildTitle, episodeId, libraryKey, seedLibrary } from '@/mocks/fixtures/titles';

function show(tmdbId: number, library = seedLibrary()): ShowTitle {
  const title = buildTitle('tv', tmdbId, library);
  if (title?.media_type !== 'tv') throw new Error(`No fixture for show ${tmdbId}`);
  return title;
}
const season = (tmdbId: number, n: number, library = seedLibrary()) => {
  const built = buildSeason(tmdbId, n, library);
  if (!built) throw new Error(`No fixture for ${tmdbId} season ${n}`);
  return built;
};

describe('parseJump', () => {
  const onePiece = show(37854); // Specials, Wano Country (#892–#1047), Egghead (#1086–#1123), absolute ordering
  const frieren = show(209867); // Season 1 (#1–#38) and 6 specials, absolute ordering
  const doctorWho = show(57243); // aired ordering, four series of 13 and 8 specials
  const ctx = (s: ShowTitle, currentSeason: number) => ({ ordering: s.episode_ordering, seasons: s.seasons, currentSeason });

  it.each([
    ['an in-season number (aired ordering)', '7', ctx(doctorWho, 2), { ok: true, season: 2, episode: 7 }],
    ['the same number as an absolute one (absolute ordering)', '29', ctx(frieren, 1), { ok: true, season: 1, absolute: 29 }],
    ['an absolute number in this season', '1100', ctx(onePiece, 22), { ok: true, season: 22, absolute: 1100 }],
    ['an absolute number in another season', '1000', ctx(onePiece, 22), { ok: true, season: 21, absolute: 1000 }],
    ['a "#" absolute number', '#892', ctx(onePiece, 22), { ok: true, season: 21, absolute: 892 }],
    ['an absolute number from the Specials tab', '1100', ctx(onePiece, 0), { ok: true, season: 22, absolute: 1100 }],
    ['an in-season number when no season has it as an absolute', '15', ctx(onePiece, 22), { ok: true, season: 22, episode: 15 }],
    ['SxxEyy', 'S03E10', ctx(doctorWho, 1), { ok: true, season: 3, episode: 10 }],
    ['a lower-case code', 's22e01', ctx(onePiece, 21), { ok: true, season: 22, episode: 1 }],
    ['a lower-case code for a season it lacks', 's01e05', ctx(onePiece, 22), { ok: false, hint: 'There’s no season 1 here.' }],
    ['a special by number', '5', ctx(doctorWho, 0), { ok: true, season: 0, episode: 5 }],
    ['a special by code', 'S00E08', ctx(doctorWho, 3), { ok: true, season: 0, episode: 8 }],
    ['past the season with aired ordering: the absolute fallback', '20', ctx(doctorWho, 1), { ok: true, season: 2, absolute: 20 }],
  ])('%s', (_name, input, context, expected) => {
    expect(parseJump(input, context)).toEqual(expected);
  });

  // Absolute numbers needn't be contiguous: the season comes from its range, the row from the lookup
  const gappy = [
    { season_number: 1, name: 'Season 1', episodes_total: 12, absolute_from: 1, absolute_to: 12 },
    // 12 episodes over #20–#40: recaps TMDB numbers absolutely but lists elsewhere
    { season_number: 2, name: 'Season 2', episodes_total: 12, absolute_from: 20, absolute_to: 40 },
  ];
  const reordered = [
    // An episode group that runs Season 2 before Season 1
    { season_number: 1, name: 'Season 1', episodes_total: 10, absolute_from: 11, absolute_to: 20 },
    { season_number: 2, name: 'Season 2', episodes_total: 10, absolute_from: 1, absolute_to: 10 },
  ];
  it.each([
    ['a number inside a season’s range with a gap', '30', { ordering: 'absolute' as const, seasons: gappy, currentSeason: 1 }, { ok: true, season: 2, absolute: 30 }],
    ['a number in the gap before a range', '15', { ordering: 'absolute' as const, seasons: gappy, currentSeason: 2 }, { ok: false }],
    ['reordered seasons: by range, not position', '5', { ordering: 'absolute' as const, seasons: reordered, currentSeason: 1 }, { ok: true, season: 2, absolute: 5 }],
  ])('%s', (_name, input, context, expected) => {
    expect(parseJump(input, context)).toMatchObject(expected);
  });

  it.each([
    ['empty', '', /Type an episode number/],
    ['words', 'next one', /Type an episode number/],
    ['a season it doesn’t have', 'S09E01', /no season 9/],
    ['past the end of a season', 'S01E40', /Season 1 has 38 episodes/],
    ['a number nowhere in the show', '5000', /Season 1 has 38 episodes/],
    ['zero', '0', /Season 1 has 38 episodes/],
  ])('hints instead of throwing: %s', (_name, input, hint) => {
    const result = parseJump(input, ctx(frieren, 1));
    expect(result.ok).toBe(false);
    expect(!result.ok && result.hint).toMatch(hint);
  });
});

describe('resolveJump', () => {
  const episodes = [
    { episode_number: 1, absolute_number: 20 },
    { episode_number: 2, absolute_number: 21 },
    // #22–#29 aren't in this season
    { episode_number: 3, absolute_number: 30 },
    { episode_number: 4, absolute_number: 32 },
  ];

  it('finds an absolute number by lookup, gaps and all', () => {
    expect(resolveJump({ season: 2, absolute: 30 }, episodes)).toBe(3);
    expect(resolveJump({ season: 2, absolute: 32 }, episodes)).toBe(4);
    expect(resolveJump({ season: 2, absolute: 25 }, episodes)).toBeNull();
  });

  it('finds reordered absolute numbers', () => {
    expect(resolveJump({ season: 1, absolute: 12 }, [{ episode_number: 1, absolute_number: 14 }, { episode_number: 2, absolute_number: 12 }])).toBe(2);
  });

  it('keeps in-season numbers that exist', () => {
    expect(resolveJump({ season: 2, episode: 4 }, episodes)).toBe(4);
    expect(resolveJump({ season: 2, episode: 9 }, episodes)).toBeNull();
  });

  it('lands One Piece #1100 on S22E15', () => {
    expect(resolveJump({ season: 22, absolute: 1100 }, season(37854, 22).episodes)).toBe(15);
  });
});

describe('range chips', () => {
  const block = (from: number, to: number, missing = 0): EpisodeRange => ({ label: `${from}–${to}`, kind: 'block', from, to, absolute_from: null, absolute_to: null, missing });

  it('labels arcs with absolute numbers where the rows show them, blocks by their label', () => {
    const egghead: EpisodeRange = { label: 'Egghead', kind: 'arc', from: 1, to: 30, absolute_from: 1086, absolute_to: 1115, missing: 0 };
    expect(rangeLabel(egghead, 'absolute')).toBe('Egghead · 1086–1115');
    expect(rangeLabel(egghead, 'episode_group')).toBe('Egghead · 1086–1115');
    expect(rangeLabel(egghead, 'aired')).toBe('Egghead · 1–30');
    expect(rangeLabel({ ...egghead, absolute_from: null, absolute_to: null }, 'absolute')).toBe('Egghead · 1–30');
    expect(rangeLabel(block(21, 28), 'absolute')).toBe('21–28');
  });

  it('presses the chip holding Trakt’s next episode, else the frontier', () => {
    const frieren = show(209867);
    const s1 = season(209867, 1);
    expect(s1.ranges.map((r) => r.label)).toEqual(['1–10', '11–20', '21–28', '29–38']);
    expect(defaultRangeIndex(s1.ranges, frieren, 1)).toBe(2); // S01E27
    expect(defaultRangeIndex(s1.ranges, { ...frieren, watch_state: null }, 1)).toBe(3); // the frontier, E29
    expect(defaultRangeIndex(season(37854, 21).ranges, show(37854), 21)).toBe(0); // One Piece's rewatch: Wano: Act 1
  });

  it('presses none in a season without the next-up row', () => {
    const s22 = season(37854, 22);
    expect(defaultRangeIndex(s22.ranges, show(37854), 22)).toBeNull();
    expect(defaultRangeIndex(season(209867, 1).ranges, show(209867), 0)).toBeNull();
  });

  it('follows the frontier into an arc', () => {
    const library = seedLibrary();
    library.get(libraryKey('tv', 37854))?.unassigned.add(episodeId(37854, 22, 36)); // #1121; E37–E38 haven't aired
    const s22 = season(37854, 22, library);
    const onePiece = { ...show(37854, library), watch_state: null }; // without Trakt's rewatch at Wano
    expect(defaultRangeIndex(s22.ranges, onePiece, 22)).toBe(1); // Elbaph (31–38)
  });
});

describe('earlierWatched', () => {
  it('collapses a leading watched run of 10 or more in a long season', () => {
    expect(earlierWatched(season(209867, 1).episodes)).toEqual({ count: 26, from: 1, to: 26 });
    expect(earlierWatched(season(37854, 22).episodes)).toEqual({ count: 25, from: 1, to: 25 });
  });

  it('stops before the next-up row, so a rewatch’s next episode stays in view', () => {
    const s1 = season(209867, 1).episodes;
    expect(earlierWatched(s1, episodeId(209867, 1, 13))).toEqual({ count: 12, from: 1, to: 12 });
    expect(earlierWatched(s1, episodeId(209867, 1, 8))).toBeNull(); // a run of 7
    expect(earlierWatched(s1, episodeId(209867, 1, 27))).toEqual({ count: 26, from: 1, to: 26 });
  });

  it('leaves short seasons, short runs and fully watched seasons alone', () => {
    expect(earlierWatched(season(95396, 2).episodes)).toBeNull(); // 10 episodes
    expect(earlierWatched(season(37854, 21).episodes)).toBeNull(); // E1–E6 have no file, then all watched
    const s1 = season(209867, 1).episodes;
    expect(earlierWatched(s1.map((e, i) => (i === 9 ? { ...e, state: 'ready' as const } : e)))).toBeNull(); // run of 9
  });
});

describe('shouldVirtualize', () => {
  it('virtualises seasons over 60 rows only', () => {
    expect(shouldVirtualize(60)).toBe(false);
    expect(shouldVirtualize(61)).toBe(true);
    expect(shouldVirtualize(season(37854, 21).episodes.length)).toBe(true);
    expect(shouldVirtualize(season(37854, 22).episodes.length)).toBe(false);
  });
});
