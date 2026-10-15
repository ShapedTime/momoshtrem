import { http, HttpResponse } from 'msw';
import type { DownloadSubtitleRequest, EpisodeStream, Subtitle, SubtitleItemType } from '@/lib/api/contract';
import { hdrezkaSubtitles, languageName, seedSubtitles, streamRelease, subtitleItemKey, subtitleSearch } from '../fixtures/subtitles';
import { findByLibraryId, findEpisode, itemRelease, libraryKey } from '../fixtures/titles';
import { mockLibrary } from './titles';
import { apiError, isRecord, jsonBody, latency, noContent, notFound } from './util';

/** This page's stored subtitles, keyed by `subtitleItemKey`. */
let stored = seedSubtitles();
/** The HDRezka stream whose subtitles each item has stored, as `streamKey` gives it. */
let recorded = new Map<string, string>();

/** Tests: back to the seeded subtitles. */
export function resetMockSubtitles(): void {
  stored = seedSubtitles();
  recorded = new Map();
}

/** The HDRezka stream an item plays from; null for a torrent file or none. */
function itemStream(type: SubtitleItemType, id: number): (EpisodeStream & { added_at?: string }) | null {
  const library = mockLibrary();
  if (type === 'movie') return findByLibraryId(library, 'movie', id)?.row.stream ?? null;
  const episode = findEpisode(id);
  return (episode && library.get(libraryKey('tv', episode.tmdbId))?.streamed.get(id)) || null;
}

/** A movie's stream assigned again is a new one (its added_at), as the backend refreshes its subtitles then. */
const streamKey = (stream: EpisodeStream & { added_at?: string }) => `${streamRelease(stream)}|${stream.added_at ?? ''}`;

/**
 * What the backend's HDRezka assign records (#68): the item's HDRezka
 * subtitles follow its stream. Another stream's replace them; with none
 * (a torrent, or nothing assigned) they go.
 */
function followStream(type: SubtitleItemType, id: number): EpisodeStream | null {
  const key = subtitleItemKey(type, id);
  const stream = itemStream(type, id);
  const current = stream ? streamKey(stream) : '';
  if ((recorded.get(key) ?? '') === current) return stream;
  const kept = (stored.get(key) ?? []).filter((s) => s.source !== 'hdrezka');
  const nextId = Math.max(0, ...[...stored.values()].flat().map((s) => s.id)) + 1;
  stored.set(key, stream ? [...kept, ...hdrezkaSubtitles(stream, nextId)] : kept);
  recorded.set(key, current);
  return stream;
}

const ITEMS = [
  ['movie', 'movies'],
  ['episode', 'episodes'],
] as const;

function isDownloadRequest(body: unknown): body is DownloadSubtitleRequest {
  return (
    isRecord(body) &&
    typeof body.file_id === 'number' &&
    typeof body.language_code === 'string' &&
    (body.release_name === undefined || typeof body.release_name === 'string')
  );
}

/**
 * Torrent and HDRezka subtitles go with their file; without a release
 * (an HDRezka stream, or no file), downloaded subtitles match nothing.
 */
function visible(type: SubtitleItemType, id: number, release: string | null): Subtitle[] {
  const stream = followStream(type, id);
  return (stored.get(subtitleItemKey(type, id)) ?? []).flatMap((subtitle): Subtitle[] => {
    if (subtitle.source === 'hdrezka') return stream ? [subtitle] : [];
    if (release !== null) return [subtitle];
    return subtitle.source === 'torrent' ? [] : [{ ...subtitle, match: 'unknown' }];
  });
}

export const subtitlesHandlers = ITEMS.flatMap(([type, path]) => [
  http.get(`*/api/${path}/:id/subtitles`, ({ params }) => {
    const id = Number(params.id);
    const release = itemRelease(mockLibrary(), type, id);
    if (release === undefined) return notFound(`No such ${type}`);
    const body: { items: Subtitle[] } = { items: visible(type, id, release) };
    return HttpResponse.json(body);
  }),
  http.get(`*/api/${path}/:id/subtitles/search`, async ({ params, request }) => {
    await latency();
    const id = Number(params.id);
    const release = itemRelease(mockLibrary(), type, id);
    if (release === undefined) return notFound(`No such ${type}`);
    const languages = (new URL(request.url).searchParams.get('languages') ?? 'en').split(',').filter(Boolean);
    return HttpResponse.json(subtitleSearch(type, id, languages, release));
  }),
  http.post(`*/api/${path}/:id/subtitles`, async ({ params, request }) => {
    await latency();
    const id = Number(params.id);
    const release = itemRelease(mockLibrary(), type, id);
    if (release === undefined) return notFound(`No such ${type}`);
    const body = await jsonBody(request);
    if (!isDownloadRequest(body)) return apiError(400, 'bad_request', 'Send {file_id, language_code, release_name?}.');
    const candidate = subtitleSearch(type, id, [body.language_code], release).items.find((c) => c.file_id === body.file_id);
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
    const id = Number(params.id);
    const release = itemRelease(mockLibrary(), type, id);
    if (release === undefined) return notFound(`No such ${type}`);
    const subtitle = visible(type, id, release).find((s) => s.id === Number(params.sid));
    if (!subtitle) return notFound('No such subtitle');
    if (subtitle.source === 'torrent') return apiError(409, 'conflict', 'This subtitle came with the torrent and stays with it.');
    const key = subtitleItemKey(type, id);
    stored.set(key, (stored.get(key) ?? []).filter((s) => s.id !== subtitle.id));
    return noContent();
  }),
]);
