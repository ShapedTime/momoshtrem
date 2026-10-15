import { http, HttpResponse } from 'msw';
import type { QualityProfile } from '@/lib/api/contract';
import { QUALITY_PROFILE, searchTorrents } from '../fixtures/search';
import { createInspectJob } from './assignment';
import { accepted, apiError, isRecord, jsonBody, latency } from './util';

let profile: QualityProfile = { ...QUALITY_PROFILE };

export const searchHandlers = [
  http.get('*/api/search/torrents', async ({ request }) => {
    await latency();
    const url = new URL(request.url);
    return HttpResponse.json(searchTorrents(Number(url.searchParams.get('page') ?? 1), Number(url.searchParams.get('page_size') ?? 25)));
  }),
  // A dry-run mapping job for the release (#6 builds the screen around it)
  http.post('*/api/search/torrents/inspect', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || (body.media_type !== 'movie' && body.media_type !== 'tv') || typeof body.tmdb_id !== 'number') {
      return apiError(400, 'bad_request', 'Send {release_id | magnet_uri, media_type, tmdb_id}.');
    }
    const release = searchTorrents(1, 100).items.find((item) => item.id === body.release_id);
    const magnet =
      typeof body.magnet_uri === 'string'
        ? body.magnet_uri
        : release?.magnet_uri && `${release.magnet_uri}&dn=${encodeURIComponent(release.raw_title)}`;
    const jobId = magnet ? createInspectJob(body.media_type, body.tmdb_id, magnet) : undefined;
    return jobId ? accepted({ job_id: jobId }) : apiError(400, 'bad_request', 'No such release, magnet or title.');
  }),
  http.get('*/api/settings/quality-profile', () => HttpResponse.json(profile)),
  http.put('*/api/settings/quality-profile', async ({ request }) => {
    profile = (await request.json()) as QualityProfile;
    return HttpResponse.json(profile);
  }),
];
