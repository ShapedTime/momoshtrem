/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { hdrezkaApi, subtitlesApi, titlesApi } from '@/lib/api/endpoints';
import type { Subtitle } from '@/lib/api/contract';
import { episodeId } from '@/mocks/fixtures/titles';

// The endpoint modules call relative /api/* URLs, as the browser does; resolve
// them against a base. Wrap the fetch MSW installs on listen(), not the original.
let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(resetMockState);
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const DUNE = 'https://rezka.ag/films/fiction/64829-dyuna-chast-vtoraya-2024.html';
const SEVERANCE = 'https://rezka.ag/series/thriller/47283-razdelenie-2022.html';
const ORIGINAL = 238;
const DUB = 56;

const summary = (items: Subtitle[]) => items.map((s) => `${s.language_code}:${s.source}:${s.match}:${s.release_name ?? ''}`);

describe('HDRezka subtitles in mock mode (#68)', () => {
  it('lists the original audio’s English and Russian subtitles with the stream, and only while it plays', async () => {
    await hdrezkaApi.assignMovie(1, { url: DUNE, translator_id: ORIGINAL, quality: '1080p' });
    const release = 'HDRezka · Оригинал (+субтитры) · 1080p';
    expect(summary((await subtitlesApi.list('movie', 1)).items)).toEqual([
      // Dune's download stays; with no release to compare, its match is unknown
      'ru:opensubtitles:unknown:Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
      `en:hdrezka:exact_release:${release}`,
      `ru:hdrezka:exact_release:${release}`,
    ]);

    // A dub brings none: the original's go
    await hdrezkaApi.assignMovie(1, { url: DUNE, translator_id: DUB, quality: '1080p' });
    expect((await subtitlesApi.list('movie', 1)).items.filter((s) => s.source === 'hdrezka')).toEqual([]);

    // Nothing assigned: none either
    await hdrezkaApi.assignMovie(1, { url: DUNE, translator_id: ORIGINAL, quality: '720p' });
    expect((await subtitlesApi.list('movie', 1)).items.filter((s) => s.source === 'hdrezka')).toHaveLength(2);
    await titlesApi.unassignMovie(1);
    expect((await subtitlesApi.list('movie', 1)).items.filter((s) => s.source === 'hdrezka')).toEqual([]);
  });

  it('deletes them until the stream is assigned again', async () => {
    await hdrezkaApi.assignMovie(1, { url: DUNE, translator_id: ORIGINAL, quality: '1080p' });
    const english = (await subtitlesApi.list('movie', 1)).items.find((s) => s.source === 'hdrezka' && s.language_code === 'en');
    await subtitlesApi.remove('movie', 1, english?.id ?? 0);
    expect((await subtitlesApi.list('movie', 1)).items.filter((s) => s.source === 'hdrezka').map((s) => s.language_code)).toEqual(['ru']);
    await hdrezkaApi.assignMovie(1, { url: DUNE, translator_id: ORIGINAL, quality: '1080p' });
    expect((await subtitlesApi.list('movie', 1)).items.filter((s) => s.source === 'hdrezka').map((s) => s.language_code)).toEqual(['en', 'ru']);
  });

  it('gives an episode playing from HDRezka its own', async () => {
    const id = episodeId(95396, 1, 1);
    await hdrezkaApi.assignShow(4, { url: SEVERANCE, translator_id: ORIGINAL, quality: '1080p', items: [{ episode_id: id, season: 1, episode: 1 }] });
    expect(summary((await subtitlesApi.list('episode', id)).items)).toEqual([
      'en:hdrezka:exact_release:HDRezka · Оригинал (+субтитры) · 1080p',
      'ru:hdrezka:exact_release:HDRezka · Оригинал (+субтитры) · 1080p',
    ]);
  });
});
