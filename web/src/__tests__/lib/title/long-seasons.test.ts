import type { EpisodeRange, ShowTitle } from '@/lib/api/contract';
import { defaultRangeIndex, earlierWatched, parseJump, rangeLabel, shouldVirtualize } from '@/lib/title/episodes';
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
    ['an in-season number', '29', ctx(frieren, 1), { ok: true, season: 1, episode: 29 }],
    ['an absolute number in this season', '1100', ctx(onePiece, 22), { ok: true, season: 22, episode: 15 }],
    ['an absolute number in another season', '1000', ctx(onePiece, 22), { ok: true, season: 21, episode: 109 }],
    ['a "#" absolute number', '#892', ctx(onePiece, 22), { ok: true, season: 21, episode: 1 }],
    ['an in-season number when no season has it as an absolute', '15', ctx(onePiece, 22), { ok: true, season: 22, episode: 15 }],
    ['SxxEyy', 'S03E10', ctx(doctorWho, 1), { ok: true, season: 3, episode: 10 }],
    ['a lower-case code for a season it lacks', 's01e05', ctx(onePiece, 22), { ok: false }],
    ['a lower-case code', 's22e01', ctx(onePiece, 21), { ok: true, season: 22, episode: 1 }],
    ['a special by number', '5', ctx(doctorWho, 0), { ok: true, season: 0, episode: 5 }],
    ['a special by code', 'S00E08', ctx(doctorWho, 3), { ok: true, season: 0, episode: 8 }],
    ['past the season with aired ordering: the absolute fallback', '20', ctx(doctorWho, 1), { ok: true, season: 2, episode: 7 }],
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

describe('range chips', () => {
  const block = (from: number, to: number, missing = 0): EpisodeRange => ({ label: `${from}–${to}`, kind: 'block', from, to, absolute_from: null, absolute_to: null, missing });

  it('labels arcs with absolute numbers when known, blocks by their label', () => {
    expect(rangeLabel({ label: 'Egghead', kind: 'arc', from: 1, to: 30, absolute_from: 1086, absolute_to: 1115, missing: 0 })).toBe('Egghead · 1086–1115');
    expect(rangeLabel({ label: 'Act 1', kind: 'arc', from: 1, to: 25, absolute_from: null, absolute_to: null, missing: 0 })).toBe('Act 1 · 1–25');
    expect(rangeLabel(block(21, 28))).toBe('21–28');
  });

  it('presses the chip holding the frontier', () => {
    const frieren = show(209867);
    const s1 = season(209867, 1);
    expect(s1.ranges.map((r) => r.label)).toEqual(['1–10', '11–20', '21–28', '29–38']);
    expect(defaultRangeIndex(s1.ranges, frieren, 1, s1.episodes)).toBe(3);
  });

  it('presses none in a season without the frontier or next-up', () => {
    const onePiece = show(37854);
    const s22 = season(37854, 22);
    expect(defaultRangeIndex(s22.ranges, onePiece, 22, s22.episodes)).toBeNull();
  });

  it('follows the frontier into an arc', () => {
    const library = seedLibrary();
    library.get(libraryKey('tv', 37854))?.unassigned.add(episodeId(37854, 22, 36)); // #1121; E37–E38 haven't aired
    const s22 = season(37854, 22, library);
    expect(defaultRangeIndex(s22.ranges, show(37854, library), 22, s22.episodes)).toBe(1); // Elbaph (31–38)
  });
});

describe('earlierWatched', () => {
  it('collapses a leading watched run of 10 or more in a long season', () => {
    expect(earlierWatched(season(209867, 1).episodes)).toEqual({ count: 25, from: 1, to: 25 });
    expect(earlierWatched(season(37854, 22).episodes)).toEqual({ count: 25, from: 1, to: 25 });
  });

  it('leaves short seasons, short runs and fully watched seasons alone', () => {
    expect(earlierWatched(season(95396, 2).episodes)).toBeNull(); // 10 episodes
    expect(earlierWatched(season(37854, 21).episodes)).toBeNull(); // all 156 watched
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
