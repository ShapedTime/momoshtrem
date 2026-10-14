import type { MovieTitle, ShowTitle, Title } from '@/lib/api/contract';
import { primaryActionFor } from '@/lib/title/stage-copy';
import { addToLibrary, buildTitle, episodeId, libraryKey, type MockLibrary, seedLibrary } from '@/mocks/fixtures/titles';
import { findTitle } from '@/mocks/fixtures/catalog';

function title(mediaType: 'movie' | 'tv', tmdbId: number, library: MockLibrary = seedLibrary()): Title {
  const built = buildTitle(mediaType, tmdbId, library);
  if (!built) throw new Error(`No fixture for ${mediaType} ${tmdbId}`);
  return built;
}
const movie = (tmdbId: number) => title('movie', tmdbId) as MovieTitle;
const show = (tmdbId: number, library?: MockLibrary) => title('tv', tmdbId, library) as ShowTitle;

/** The seeded library with some episodes' files unassigned. */
function without(tmdbId: number, ...episodes: Array<[number, number]>): MockLibrary {
  const library = seedLibrary();
  for (const [season, episode] of episodes) library.get(libraryKey('tv', tmdbId))?.unassigned.add(episodeId(tmdbId, season, episode));
  return library;
}

function withDownload(base: MovieTitle, download: Partial<NonNullable<MovieTitle['library']['download']>>): MovieTitle {
  const current = base.library.download ?? { pct: 0, speed_bps: 0, eta_seconds: null, seeders: 0 };
  return { ...base, library: { ...base.library, download: { ...current, ...download } } };
}

const summary = (t: Title) => {
  const copy = primaryActionFor(t);
  return { step: copy.step, title: copy.title, primary: copy.primary.label, dock: copy.dock.button.label };
};

describe('primaryActionFor: movies', () => {
  it.each([
    ['none (Past Lives)', 666277, { step: 0, title: 'Not in your library', primary: 'Add to library', dock: 'Add to library' }],
    ['needs_torrent (One Battle After Another)', 1054867, { step: 1, title: 'In your library · no torrent yet', primary: 'Find a torrent', dock: 'Find a torrent' }],
    ['downloading (Interstellar)', 157336, { step: 2, title: 'Downloading', primary: 'View download', dock: 'View download' }],
    ['ready (Dune: Part Two)', 693134, { step: 3, title: 'Ready in Infuse', primary: 'How to play in Infuse', dock: 'Play in Infuse' }],
  ])('%s', (_name, tmdbId, expected) => {
    expect(summary(movie(tmdbId))).toEqual(expected);
  });

  it('says adding is free before anything is in the library', () => {
    expect(primaryActionFor(movie(666277)).hint).toBe('Add it, then pick a torrent. Adding is free: nothing downloads until you pick a torrent.');
  });

  it('promises streaming, not a finish time, while downloading', () => {
    const copy = primaryActionFor(movie(157336));
    expect(copy).toMatchObject({
      pct: 42,
      facts: ['9 seeders'],
      lead: 'You can start watching in Infuse now;',
      hint: 'it streams while it downloads.',
      primary: { intent: 'view_download', href: '/torrents', variant: 'secondary' },
      secondary: { label: 'How to play in Infuse', variant: 'ghost' },
    });
    expect(JSON.stringify(copy)).not.toMatch(/left|ETA|min/);
  });

  it.each([
    ['speed and seeders when known', { speed_bps: 2_300_000, seeders: 9 }, ['2.3 MB/s', '9 seeders']],
    ['no speed while it is 0', { speed_bps: 0, seeders: 1 }, ['1 seeder']],
    ['neither when both are 0', { speed_bps: 0, seeders: 0 }, []],
  ])('downloading facts: %s', (_name, download, facts) => {
    expect(primaryActionFor(withDownload(movie(157336), download)).facts).toEqual(facts);
  });

  it('shows the release and a copyable WebDAV path once ready', () => {
    const dune = movie(693134);
    expect(primaryActionFor(dune)).toMatchObject({
      facts: ['2160p', '18.4 GB', 'Dolby Vision', 'Atmos'],
      path: { display: dune.library.webdav_path, copy: dune.library.webdav_url },
      primary: { intent: 'how_to_play', variant: 'primary' },
      secondary: null,
      dock: { button: { label: 'Play in Infuse', intent: 'how_to_play' }, copyPath: true },
    });
  });
});

describe('primaryActionFor: shows', () => {
  it('counts what adding a show brings', () => {
    expect(primaryActionFor(show(66573))).toMatchObject({
      title: 'Not in your library',
      hint: '53 episodes across 4 seasons. Adding is free: nothing downloads until you pick a torrent.',
      primary: { label: 'Add to library', intent: 'add' },
    });
  });

  it('says nothing is assigned yet for a show just added', () => {
    const library = seedLibrary();
    const entry = findTitle('tv', 66573);
    if (entry) addToLibrary(library, entry);
    expect(summary(show(66573, library))).toEqual({
      step: 1,
      title: 'In your library · no episodes yet',
      primary: 'Find a torrent',
      dock: 'Find a torrent',
    });
  });

  it.each([
    ['a run in absolute numbers (Frieren)', show(209867), 'Episodes 29–38 have no torrent', 'Find episodes 29–38'],
    ['one absolute episode (One Piece #1121)', show(37854, without(37854, [22, 36])), 'Episode 1121 has no torrent', 'Find episode 1121'],
    ['one aired episode of a multi-season show (Severance)', show(95396, without(95396, [2, 9])), 'S02E09 has no torrent', 'Find S02E09'],
  ])('needs a torrent: %s', (_name, t, panelTitle, button) => {
    const copy = primaryActionFor(t);
    expect([copy.stage, copy.title, copy.primary.label, copy.dock.button.label]).toEqual(['needs_torrent', panelTitle, button, button]);
  });

  it('uses absolute numbers past the first season', () => {
    const onePiece = show(37854);
    const missing = { season_number: 22, from: 37, to: 38, absolute_from: 1122, absolute_to: 1123, count: 2 };
    const copy = primaryActionFor({ ...onePiece, library: { ...onePiece.library, availability_stage: 'needs_torrent', missing } });
    expect([copy.title, copy.primary.label, copy.hint]).toEqual([
      'Episodes 1122–1123 have no torrent',
      'Find episodes 1122–1123',
      'A season pack usually covers them all in one go.',
    ]);
  });

  it('uses in-season numbers for aired ordering', () => {
    const onePiece = show(37854);
    const missing = { season_number: 22, from: 37, to: 38, absolute_from: 1122, absolute_to: 1123, count: 2 };
    const aired: ShowTitle = { ...onePiece, episode_ordering: 'aired', library: { ...onePiece.library, availability_stage: 'needs_torrent', missing } };
    expect(primaryActionFor(aired).title).toBe('S22E37–E38 have no torrent');
  });

  it('streams episodes while they download', () => {
    expect(primaryActionFor(show(247767))).toMatchObject({ title: 'Downloading', hint: 'episodes stream while they download.' });
  });

  it('counts ready episodes and offers the show folder', () => {
    const doctorWho = show(57243);
    const copy = primaryActionFor(doctorWho);
    expect(copy.title).toBe('Ready in Infuse');
    expect(copy.facts).toEqual([`${doctorWho.library.episodes_ready} of ${doctorWho.library.episodes_total} episodes ready`]);
    expect(copy.path?.display).toBe('/TV Shows/Doctor Who (2005)/');
  });
});
