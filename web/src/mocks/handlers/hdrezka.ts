import { http, HttpResponse } from 'msw';
import type {
  AssignMovieStreamRequest,
  AssignShowStreamRequest,
  AssignShowStreamResult,
  HDRezkaSearchResult,
  HDRezkaTranslator,
  StreamEpisodeTarget,
} from '@/lib/api/contract';
import { buildHDRezkaTitle, buildStreams, findSeed, HDREZKA_URL, searchHDRezka, seedTranslators, type TitleSeed, titlePath } from '../fixtures/hdrezka';
import { assignEpisodeStream, assignMovieStream, findByLibraryId, findEpisode } from '../fixtures/titles';
import { scenario } from '../scenario';
import { mockLibrary } from './titles';
import { apiError, isRecord, jsonBody, latency, noContent, notFound } from './util';

/** HDRezka (PLAN §6 "HDRezka (#63)"); scenarios in ../scenario.ts. */

const MAX_EPISODES = 30;

/** The backend's answers when HDRezka is off or failing: never 401 or 403. */
function outage(): Response | null {
  switch (scenario()) {
    case 'hdrezka-off':
      return apiError(503, 'unavailable', 'HDRezka is off. Set HDREZKA_ENABLED=true on the server to use it.');
    case 'hdrezka-blocked':
      return apiError(503, 'hdrezka_blocked', 'HDRezka answered with its anti-bot page instead of the title.');
    case 'hdrezka-down':
      return apiError(502, 'hdrezka_unavailable', 'HDRezka didn’t answer (connection refused).');
    default:
      return null;
  }
}

const everyPremium = () => scenario() === 'hdrezka-premium';

const PREMIUM_ONLY = 'This needs an HDRezka Premium account, which momoshtrem doesn’t use.';

const positive = (value: string | null) => {
  const n = Number(value);
  return value !== null && Number.isInteger(n) && n > 0 ? n : null;
};

function isStreamRequest(body: unknown): body is AssignMovieStreamRequest {
  return isRecord(body) && typeof body.url === 'string' && typeof body.translator_id === 'number' && typeof body.quality === 'string';
}

const isTarget = (item: unknown): item is StreamEpisodeTarget =>
  isRecord(item) && typeof item.episode_id === 'number' && typeof item.season === 'number' && typeof item.episode === 'number';

function isShowRequest(body: unknown): body is AssignShowStreamRequest {
  return isStreamRequest(body) && isRecord(body) && Array.isArray(body.items) && body.items.every(isTarget);
}

type Resolved = { seed: TitleSeed; translator: HDRezkaTranslator; quality: string; size_bytes: number };

/** Checks a stream pick as the backend does before it writes: the title, the voice-over, then the quality. */
function resolvePick(body: AssignMovieStreamRequest): Resolved | Response {
  const path = titlePath(body.url);
  if (!path) return apiError(422, 'validation_error', 'That isn’t an HDRezka title page.');
  const seed = findSeed(path);
  if (!seed) return notFound('No title at that address on HDRezka.');
  const translator = seedTranslators(seed, everyPremium()).find((t) => t.id === body.translator_id);
  if (!translator) return apiError(422, 'validation_error', 'This title has no such voice-over.');
  if (translator.premium) return apiError(409, 'premium_only', PREMIUM_ONLY);
  const quality = buildStreams(seed, translator).qualities.find((q) => q.label === body.quality);
  if (!quality || !quality.available) return apiError(422, 'validation_error', `${body.quality} isn’t offered with this voice-over.`);
  if (quality.premium) return apiError(409, 'premium_only', PREMIUM_ONLY);
  return { seed, translator, quality: quality.label, size_bytes: quality.size_bytes ?? 0 };
}

export const hdrezkaHandlers = [
  http.get('*/api/hdrezka/search', async ({ request }) => {
    await latency();
    const failed = outage();
    if (failed) return failed;
    const params = new URL(request.url).searchParams;
    const mediaType = params.get('media_type');
    const tmdbId = positive(params.get('tmdb_id'));
    if ((mediaType !== 'movie' && mediaType !== 'tv') || tmdbId === null) {
      return apiError(422, 'validation_error', 'Send media_type (movie or tv) and tmdb_id.');
    }
    const q = params.get('q')?.trim() || null;
    const body: HDRezkaSearchResult = { items: searchHDRezka(mediaType, tmdbId, q), query: q ?? '' };
    if (!q) {
      // The backend searches by the TMDB title (its original title first)
      const first = body.items[0];
      body.query = first ? (first.original_title ?? first.title) : '';
    }
    return HttpResponse.json(body);
  }),
  http.get('*/api/hdrezka/titles', async ({ request }) => {
    await latency();
    const failed = outage();
    if (failed) return failed;
    const params = new URL(request.url).searchParams;
    const path = titlePath(params.get('url') ?? '');
    if (!path) return apiError(422, 'validation_error', 'That isn’t an HDRezka title page.');
    const seed = findSeed(path);
    if (!seed) return notFound('No title at that address on HDRezka.');
    return HttpResponse.json(
      buildHDRezkaTitle(seed, mockLibrary(), {
        translatorId: positive(params.get('translator_id')),
        showId: positive(params.get('show_id')),
        movieId: positive(params.get('movie_id')),
        everyPremium: everyPremium(),
      })
    );
  }),
  http.get('*/api/hdrezka/streams', async ({ request }) => {
    await latency();
    const failed = outage();
    if (failed) return failed;
    const params = new URL(request.url).searchParams;
    const path = titlePath(params.get('url') ?? '');
    if (!path) return apiError(422, 'validation_error', 'That isn’t an HDRezka title page.');
    const seed = findSeed(path);
    if (!seed) return notFound('No title at that address on HDRezka.');
    const translator = seedTranslators(seed, everyPremium()).find((t) => t.id === positive(params.get('translator_id')));
    if (!translator) return apiError(422, 'validation_error', 'This title has no such voice-over.');
    if (translator.premium) return apiError(409, 'premium_only', PREMIUM_ONLY);
    return HttpResponse.json(buildStreams(seed, translator));
  }),
  http.post('*/api/movies/:id/assign-hdrezka', async ({ params, request }) => {
    await latency();
    const failed = outage();
    if (failed) return failed;
    const body = await jsonBody(request);
    if (!isStreamRequest(body)) return apiError(422, 'validation_error', 'Send {url, translator_id, quality}.');
    const found = findByLibraryId(mockLibrary(), 'movie', Number(params.id));
    if (!found) return notFound('No such movie');
    const pick = resolvePick(body);
    if (pick instanceof Response) return pick;
    assignMovieStream(found.row, {
      title: pick.seed.title,
      url: HDREZKA_URL + pick.seed.path,
      translator: pick.translator.name,
      quality: pick.quality,
      size_bytes: pick.size_bytes,
    });
    return noContent();
  }),
  http.post('*/api/shows/:id/assign-hdrezka', async ({ params, request }) => {
    await latency();
    const failed = outage();
    if (failed) return failed;
    const body = await jsonBody(request);
    if (!isShowRequest(body)) return apiError(422, 'validation_error', 'Send {url, translator_id, quality, items: [{episode_id, season, episode}]}.');
    if (body.items.length < 1 || body.items.length > MAX_EPISODES) {
      return apiError(422, 'validation_error', `Send 1–${MAX_EPISODES} episodes a request.`);
    }
    const found = findByLibraryId(mockLibrary(), 'tv', Number(params.id));
    if (!found) return notFound('No such show');
    const pick = resolvePick(body);
    if (pick instanceof Response) return pick;
    const result: AssignShowStreamResult = { assigned: [], failed: [] };
    body.items.forEach((item, i) => {
      if (findEpisode(item.episode_id)?.tmdbId !== found.entry.tmdb_id) {
        result.failed.push({ episode_id: item.episode_id, code: 'not_found', message: 'That episode isn’t in this show.' });
      } else if (scenario() === 'hdrezka-partial' && i % 3 === 2) {
        result.failed.push({ episode_id: item.episode_id, code: 'timeout', message: 'HDRezka took too long to answer for this episode.' });
      } else {
        assignEpisodeStream(found.row, item.episode_id, { translator: pick.translator.name, quality: pick.quality });
        result.assigned.push({ episode_id: item.episode_id, quality: pick.quality, size_bytes: pick.size_bytes });
      }
    });
    return HttpResponse.json(result);
  }),
];
