import { http, HttpResponse } from 'msw';
import type { DownloadSubtitleRequest, Subtitle } from '@/lib/api/contract';
import { languageName, seedSubtitles, subtitleItemKey, subtitleSearch } from '../fixtures/subtitles';
import { apiError, latency, noContent, notFound } from './util';

/** This page's stored subtitles, keyed by `subtitleItemKey`. */
let stored = seedSubtitles();

/** Tests: back to the seeded subtitles. */
export function resetMockSubtitles(): void {
  stored = seedSubtitles();
}

const ITEMS = [
  ['movie', 'movies'],
  ['episode', 'episodes'],
] as const;

export const subtitlesHandlers = ITEMS.flatMap(([type, path]) => [
  http.get(`*/api/${path}/:id/subtitles`, ({ params }) =>
    HttpResponse.json<{ items: Subtitle[] }>({ items: stored.get(subtitleItemKey(type, Number(params.id))) ?? [] })
  ),
  http.get(`*/api/${path}/:id/subtitles/search`, async ({ params, request }) => {
    await latency();
    const languages = (new URL(request.url).searchParams.get('languages') ?? 'en').split(',').filter(Boolean);
    return HttpResponse.json(subtitleSearch(type, Number(params.id), languages));
  }),
  http.post(`*/api/${path}/:id/subtitles`, async ({ params, request }) => {
    await latency();
    const id = Number(params.id);
    const body = (await request.json()) as DownloadSubtitleRequest;
    const candidate = subtitleSearch(type, id, [body.language_code]).items.find((c) => c.file_id === body.file_id);
    const key = subtitleItemKey(type, id);
    const all = [...stored.values()].flat();
    const subtitle: Subtitle = {
      id: Math.max(0, ...all.map((s) => s.id)) + 1,
      language_code: body.language_code,
      language_name: languageName(body.language_code),
      format: 'srt',
      source: 'opensubtitles',
      release_name: body.release_name ?? candidate?.release_name ?? null,
      match: candidate?.match ?? 'unknown',
      created_at: new Date().toISOString(),
    };
    // One downloaded subtitle per language; torrent ones stay.
    const kept = (stored.get(key) ?? []).filter((s) => s.source !== 'opensubtitles' || s.language_code !== body.language_code);
    stored.set(key, [...kept, subtitle]);
    return HttpResponse.json(subtitle, { status: 201 });
  }),
  http.delete(`*/api/${path}/:id/subtitles/:sid`, ({ params }) => {
    const key = subtitleItemKey(type, Number(params.id));
    const list = stored.get(key) ?? [];
    const subtitle = list.find((s) => s.id === Number(params.sid));
    if (!subtitle) return notFound('No such subtitle');
    if (subtitle.source === 'torrent') return apiError(409, 'conflict', 'This subtitle came with the torrent and stays with it.');
    stored.set(key, list.filter((s) => s !== subtitle));
    return noContent();
  }),
]);
