import { http, HttpResponse } from 'msw';
import type { AddTitleResult, EpisodeGroup, ShowSettingsPatch } from '@/lib/api/contract';
import { findTitle } from '../fixtures/catalog';
import {
  addToLibrary,
  buildSeason,
  buildTitle,
  findByLibraryId,
  findEpisode,
  libraryKey,
  type MockLibrary,
  seedLibrary,
} from '../fixtures/titles';
import { apiError, latency, noContent, notFound } from './util';

/** This page's library: adds, removes, unassigns and settings stick until a reload. */
let library: MockLibrary = seedLibrary();

export const mockLibrary = () => library;

/** Tests: back to the seeded library. */
export function resetMockLibrary(): void {
  library = seedLibrary();
}

const EPISODE_GROUPS: EpisodeGroup[] = [
  { id: '5eb730dd0d3f2c0020b4e3d4', name: 'Absolute order', type: 2, group_count: 1, episode_count: 38 },
  { id: '6501a1b2c3d4e5f6a7b8c9d0', name: 'Cours', type: 6, group_count: 3, episode_count: 38 },
];

const MEDIA = [
  ['movie', 'movies'],
  ['tv', 'shows'],
] as const;

export const titlesHandlers = [
  http.get('*/api/titles/:mediaType/:tmdbId', async ({ params }) => {
    await latency();
    const mediaType = params.mediaType === 'movie' || params.mediaType === 'tv' ? params.mediaType : null;
    const title = mediaType && buildTitle(mediaType, Number(params.tmdbId), library);
    return title ? HttpResponse.json(title) : notFound('No such title');
  }),
  http.get('*/api/titles/tv/:tmdbId/seasons/:season', async ({ params }) => {
    await latency();
    const tmdbId = Number(params.tmdbId);
    if (!findTitle('tv', tmdbId)) return notFound('No such show');
    if (!library.has(libraryKey('tv', tmdbId))) return apiError(404, 'not_in_library', 'Add the show to see its episodes.');
    const season = buildSeason(tmdbId, Number(params.season), library);
    return season ? HttpResponse.json(season) : notFound('No such season');
  }),
  ...MEDIA.flatMap(([mediaType, path]) => [
    http.post(`*/api/${path}`, async ({ request }) => {
      const { tmdb_id } = (await request.json()) as { tmdb_id: number };
      const entry = findTitle(mediaType, Number(tmdb_id));
      if (!entry) return notFound('No such title on TMDB');
      const existing = library.get(libraryKey(mediaType, entry.tmdb_id));
      const row = existing ?? addToLibrary(library, entry);
      const body: AddTitleResult = { id: row.library_id, tmdb_id: entry.tmdb_id, title: entry.title, year: entry.year };
      return HttpResponse.json(body, { status: existing ? 200 : 201 });
    }),
    http.delete(`*/api/${path}/:id`, ({ params }) => {
      const found = findByLibraryId(library, mediaType, Number(params.id));
      if (!found) return notFound();
      library.delete(found.key);
      return noContent();
    }),
    http.post(`*/api/${path}/:id/refresh`, async ({ params }) => {
      await latency();
      return findByLibraryId(library, mediaType, Number(params.id)) ? noContent() : notFound();
    }),
  ]),
  http.delete('*/api/movies/:id/assign', ({ params }) => {
    const found = findByLibraryId(library, 'movie', Number(params.id));
    if (!found) return notFound('No such movie');
    found.row.files = false;
    return noContent();
  }),
  http.delete('*/api/episodes/:id/assign', ({ params }) => {
    const id = Number(params.id);
    const episode = findEpisode(id);
    const row = episode && library.get(libraryKey('tv', episode.tmdbId));
    if (!row) return notFound('No such episode');
    row.unassigned.add(id);
    return noContent();
  }),
  http.patch('*/api/shows/:id', async ({ params, request }) => {
    const found = findByLibraryId(library, 'tv', Number(params.id));
    if (!found) return notFound('No such show');
    const settings = { ...found.row.settings, ...((await request.json()) as ShowSettingsPatch) };
    if (settings.episode_ordering === 'episode_group' && !EPISODE_GROUPS.some((group) => group.id === settings.episode_group_id)) {
      return apiError(422, 'validation_error', 'Pick an episode group for this ordering.');
    }
    found.row.settings = settings;
    return HttpResponse.json(buildTitle('tv', found.entry.tmdb_id, library));
  }),
  http.get('*/api/shows/:id/episode-groups', async () => {
    await latency();
    return HttpResponse.json<{ items: EpisodeGroup[] }>({ items: EPISODE_GROUPS });
  }),
];
