import { http, HttpResponse } from 'msw';
import type { AddTitleResult, ClassifierMode, EpisodeGroup, EpisodeOrdering, ShowSettingsPatch } from '@/lib/api/contract';
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
import { apiError, isRecord, jsonBody, latency, noContent, notFound } from './util';

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

const ORDERINGS: EpisodeOrdering[] = ['aired', 'absolute', 'episode_group'];
const CLASSIFIER_MODES: ClassifierMode[] = ['auto', 'rules_only', 'ai_always'];

const isAddRequest = (body: unknown): body is { tmdb_id: number } => isRecord(body) && typeof body.tmdb_id === 'number';

function isSettingsPatch(body: unknown): body is ShowSettingsPatch {
  return (
    isRecord(body) &&
    (body.episode_ordering === undefined || ORDERINGS.some((ordering) => ordering === body.episode_ordering)) &&
    (body.episode_group_id === undefined || body.episode_group_id === null || typeof body.episode_group_id === 'string') &&
    (body.include_specials === undefined || typeof body.include_specials === 'boolean') &&
    (body.classifier_mode === undefined || CLASSIFIER_MODES.some((mode) => mode === body.classifier_mode))
  );
}

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
      const body = await jsonBody(request);
      if (!isAddRequest(body)) return apiError(400, 'bad_request', 'Send {tmdb_id}.');
      // As the legacy backend: an id TMDB doesn't know is a bad request.
      const entry = findTitle(mediaType, body.tmdb_id);
      if (!entry) return apiError(400, 'bad_request', 'TMDB has no such title.');
      const existing = library.get(libraryKey(mediaType, entry.tmdb_id));
      const row = existing ?? addToLibrary(library, entry);
      const added: AddTitleResult = { id: row.library_id, tmdb_id: entry.tmdb_id, title: entry.title, year: entry.year };
      return HttpResponse.json(added, { status: existing ? 200 : 201 });
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
    found.row.assigned = undefined;
    return noContent();
  }),
  http.delete('*/api/episodes/:id/assign', ({ params }) => {
    const id = Number(params.id);
    const episode = findEpisode(id);
    const row = episode && library.get(libraryKey('tv', episode.tmdbId));
    if (!row) return notFound('No such episode');
    row.unassigned.add(id);
    row.filed.delete(id);
    return noContent();
  }),
  http.patch('*/api/shows/:id', async ({ params, request }) => {
    const found = findByLibraryId(library, 'tv', Number(params.id));
    if (!found) return notFound('No such show');
    const patch = await jsonBody(request);
    if (!isSettingsPatch(patch)) return apiError(422, 'validation_error', 'Unknown ordering, classifier mode or field type.');
    const current = found.row.settings;
    const settings = {
      episode_ordering: patch.episode_ordering ?? current.episode_ordering,
      episode_group_id: patch.episode_group_id === undefined ? current.episode_group_id : patch.episode_group_id,
      include_specials: patch.include_specials ?? current.include_specials,
      classifier_mode: patch.classifier_mode ?? current.classifier_mode,
    };
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
