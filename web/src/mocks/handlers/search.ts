import { http, HttpResponse } from 'msw';
import type { ResolveResponse } from '@/lib/api/contract';
import { searchTorrents } from '../fixtures/search';
import { createInspectJob } from './assignment';
import { accepted, apiError, isRecord, jsonBody, latency, notFound } from './util';

const findRelease = (id: unknown) => searchTorrents(1, 100).items.find((item) => item.id === id);

export const searchHandlers = [
  http.get('*/api/search/torrents', async ({ request }) => {
    await latency();
    const url = new URL(request.url);
    return HttpResponse.json(searchTorrents(Number(url.searchParams.get('page') ?? 1), Number(url.searchParams.get('page_size') ?? 25)));
  }),
  // The magnet of a release an earlier search found
  http.post('*/api/search/torrents/resolve', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || typeof body.release_id !== 'string') return apiError(400, 'bad_request', 'Send {release_id}.');
    const release = findRelease(body.release_id);
    if (!release?.magnet_uri || !release.info_hash) return notFound('This result expired: search again.');
    const answer: ResolveResponse = { magnet_uri: release.magnet_uri, info_hash: release.info_hash };
    return HttpResponse.json(answer);
  }),
  // A dry-run mapping job for the release (#6 builds the screen around it)
  http.post('*/api/search/torrents/inspect', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || (body.media_type !== 'movie' && body.media_type !== 'tv') || typeof body.tmdb_id !== 'number') {
      return apiError(400, 'bad_request', 'Send {media_type, tmdb_id, release_id | magnet_uri}.');
    }
    const release = findRelease(body.release_id);
    const magnet =
      typeof body.magnet_uri === 'string'
        ? body.magnet_uri
        : release?.magnet_uri && `${release.magnet_uri}&dn=${encodeURIComponent(release.raw_title)}`;
    const jobId = magnet ? createInspectJob(body.media_type, body.tmdb_id, magnet) : undefined;
    return jobId ? accepted({ job_id: jobId }) : apiError(400, 'bad_request', 'No such release, magnet or title.');
  }),
];
