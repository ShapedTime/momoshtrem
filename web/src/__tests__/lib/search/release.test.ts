import type { Release, ReleaseAttributes } from '@/lib/api/contract';
import { magnetOf, prefersGroup, relativeAge, releaseChips, releaseFacts, seedHealth } from '@/lib/search/release';

const ATTRS: ReleaseAttributes = {
  resolution: '1080p',
  source: 'WEB-DL',
  codec: 'x264',
  bit_depth: null,
  hdr: [],
  audio: [],
  channels: null,
  languages: [],
  subs: null,
  group: null,
  edition: null,
  repack: false,
  pack: null,
};

const chips = (attrs: Partial<ReleaseAttributes>, preferredGroup = false) =>
  releaseChips({ attrs: { ...ATTRS, ...attrs } }, { preferredGroup }).map(({ label, tone }) => `${label}:${tone}`);

describe('releaseChips', () => {
  it('reads resolution, source, HDR, audio, codec, bit depth and group in order, highlighting 2160p, HDR and Atmos', () => {
    expect(
      chips({ resolution: '2160p', codec: 'x265', bit_depth: 10, hdr: ['DV', 'HDR10'], audio: ['DD+', 'Atmos'], channels: '5.1', group: 'FLUX' })
    ).toEqual(['2160p:hi', 'WEB-DL:default', 'DV:hi', 'HDR10:hi', 'DD+ 5.1:default', 'Atmos:hi', 'x265:default', '10-bit:default', 'FLUX:default']);
  });

  it('CAM is an error, a pack gets the pack tone, a preferred group the accent', () => {
    expect(chips({ resolution: 'unknown', source: 'CAM' })).toEqual(['CAM:err', 'x264:default']);
    expect(chips({ group: 'SubsPlease', pack: { type: 'multi_episode', label: 'Episodes 29–38', seasons: [], episodes: { from: 29, to: 38 } } }, true)).toEqual([
      '1080p:default',
      'WEB-DL:default',
      'x264:default',
      'SubsPlease:accent',
      'Episodes 29–38:pack',
    ]);
  });

  it('leaves out unknowns and a single episode’s pack, and names languages, subtitles, edition and repack', () => {
    expect(
      chips({
        source: 'unknown',
        codec: 'unknown',
        languages: ['dual'],
        subs: 'multi',
        edition: 'Extended',
        repack: true,
        pack: { type: 'episode', label: 'S01E29', seasons: [1], episodes: { from: 29, to: 29 } },
      })
    ).toEqual(['1080p:default', 'Dual audio:default', 'Multi-sub:default', 'Extended:default', 'REPACK:default']);
    expect(chips({ languages: ['multi'] })).toContain('Multi-audio:default');
    expect(chips({ languages: ['en', 'fr'] })).toContain('EN · FR:default');
    expect(chips({ subs: 'hardcoded' })).toContain('Hardsub:default');
  });

  it('puts the channels on the first sound format that isn’t Atmos', () => {
    expect(chips({ audio: ['Atmos', 'TrueHD'], channels: '7.1' })).toEqual(['1080p:default', 'WEB-DL:default', 'Atmos:hi', 'TrueHD 7.1:default', 'x264:default']);
  });
});

describe('prefersGroup', () => {
  it('is true when a reason names the group as preferred', () => {
    const attrs = { ...ATTRS, group: 'NTb' };
    expect(prefersGroup({ attrs, score_reasons: ['1080p WEB-DL from NTb, a group you prefer'] })).toBe(true);
    expect(prefersGroup({ attrs, score_reasons: ['1080p WEB-DL from NTb'] })).toBe(false);
    expect(prefersGroup({ attrs: ATTRS, score_reasons: ['a group you prefer'] })).toBe(false);
  });
});

describe('facts', () => {
  const now = new Date('2026-10-14T00:00:00Z');

  it('seeders: green from 20, amber below, red at none', () => {
    expect([seedHealth(312), seedHealth(20), seedHealth(19), seedHealth(1), seedHealth(0)]).toEqual(['ok', 'ok', 'low', 'low', 'none']);
  });

  it('relativeAge: minutes, hours, days, weeks, months, years', () => {
    const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();
    const DAY = 86_400_000;
    expect(relativeAge(ago(30_000), now)).toBe('1 min');
    expect(relativeAge(ago(42 * 60_000), now)).toBe('42 min');
    expect(relativeAge(ago(5 * 3_600_000), now)).toBe('5 h');
    expect(relativeAge(ago(3 * DAY), now)).toBe('3 d');
    expect(relativeAge(ago(15 * DAY), now)).toBe('2 wk');
    expect(relativeAge(ago(270 * DAY), now)).toBe('8 mo');
    expect(relativeAge(ago(400 * DAY), now)).toBe('1 yr');
    expect(relativeAge(new Date(now.getTime() + DAY).toISOString(), now)).toBe('1 min');
  });

  it('formats size, seeders, leechers, age and indexer', () => {
    const facts = releaseFacts({ size_bytes: 13_200_000_000, seeders: 1, leechers: 1204, published_at: '2026-01-16T12:00:00Z', indexer: 'Nyaa' }, now);
    expect(facts).toEqual({
      size: '13.2 GB',
      seeders: { label: '1 seeder', health: 'low' },
      leechers: '1,204',
      age: { label: '8 mo', title: 'Jan 16, 2026', dateTime: '2026-01-16T12:00:00Z' },
      indexer: 'Nyaa',
    });
    expect(releaseFacts({ size_bytes: 410_000_000, seeders: 0, leechers: 0, published_at: null, indexer: 'EZTV' }, now)).toMatchObject({
      size: '410 MB',
      seeders: { label: '0 seeders', health: 'none' },
      age: null,
    });
  });
});

describe('magnetOf', () => {
  it('uses the release’s magnet, else what resolve fetched', () => {
    const withMagnet = { id: 'a', magnet_uri: 'magnet:?xt=urn:btih:a' } satisfies Pick<Release, 'id' | 'magnet_uri'>;
    const without = { id: 'b', magnet_uri: null };
    expect(magnetOf(withMagnet, {})).toBe('magnet:?xt=urn:btih:a');
    expect(magnetOf(without, {})).toBeNull();
    expect(magnetOf(without, { b: 'magnet:?xt=urn:btih:b' })).toBe('magnet:?xt=urn:btih:b');
  });
});
