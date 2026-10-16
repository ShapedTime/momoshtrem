import { http, HttpResponse } from 'msw';
import type { IndexerReport, ResolveResponse } from '@/lib/api/contract';
import { findMockRelease, searchFixture } from '../fixtures/search';
import { matchesQuery, parseSearchParams, planKey, searchAnswer } from '../fixtures/search-rules';
import { buildTitle } from '../fixtures/titles';
import { scenario } from '../scenario';
import { createInspectJob } from './assignment';
import { getMockQualityProfile } from './settings';
import { mockLibrary } from './titles';
import { accepted, apiError, isRecord, jsonBody, latency, notFound } from './util';

/**
 * Torrent search against the context-aware fixtures (fixtures/search.ts),
 * read like the backend reads its cache (fixtures/search-rules.ts). Per plan,
 * the page remembers when its search started (search-slow's clock) and
 * whether a Retry (`refresh=1`) asked again, which brings timed-out indexers
 * back. Scenarios: search-partial, search-none, search-slow, search-down.
 */
interface PlanState {
  started: number;
  retried: boolean;
}

let plans = new Map<string, PlanState>();

/** Tests: forget every search's progress and retries. */
export function resetMockSearch(): void {
  plans = new Map();
}

/** search-slow: one more indexer answers every second. */
const SLOW_STEP_MS = 1000;
const TIMEOUT_MS = 8000;

type IndexerState = Omit<IndexerReport, 'count'>;

export const searchHandlers = [
  http.get('*/api/search/torrents', async ({ request }) => {
    await latency();
    const mode = scenario();
    if (mode === 'search-down') return apiError(503, 'unavailable', 'Jackett isn’t configured on the server: set JACKETT_URL and JACKETT_API_KEY.');
    const parsed = parseSearchParams(new URL(request.url).searchParams);
    if ('error' in parsed) return apiError(400, 'bad_request', parsed.error);
    const { params } = parsed;
    const fixture = searchFixture(params.media_type, params.tmdb_id, { season: params.season, episode: params.episode, scope: parsed.scope });
    if (!fixture) return notFound('TMDB has no such title.');

    const now = Date.now();
    const key = planKey(params);
    const previous = plans.get(key);
    const plan = !previous || parsed.refresh ? { started: now, retried: Boolean(previous?.retried) || parsed.refresh } : previous;
    plans.set(key, plan);

    const flaky = plan.retried ? undefined : mode === 'search-partial' ? fixture.indexers.at(-1) : fixture.flaky;
    const answering = mode === 'search-slow' ? Math.floor((now - plan.started) / SLOW_STEP_MS) : Infinity;
    const indexers = fixture.indexers.map((name, i): IndexerState => {
      if (i >= answering) return { name, status: 'pending', ms: now - plan.started };
      if (name === flaky) return { name, status: 'timeout', ms: TIMEOUT_MS, error: 'No answer in 8 s' };
      return { name, status: 'ok', ms: 180 + i * 140 };
    });
    const answered = new Set(indexers.filter((indexer) => indexer.status === 'ok').map((indexer) => indexer.name));
    const found =
      mode === 'search-none'
        ? []
        : fixture.releases.filter((entry) => answered.has(entry.release.indexer) && (!params.q || matchesQuery(entry.release, params.q)));
    const title = buildTitle(params.media_type, params.tmdb_id, mockLibrary());

    return HttpResponse.json(
      searchAnswer({
        parsed,
        found,
        // The page's live profile, as Settings › Quality (#94) saved it
        profile: getMockQualityProfile(),
        queries: params.q ? [{ q: params.q, label: null }] : fixture.queries,
        indexers,
        complete: indexers.every((indexer) => indexer.status !== 'pending'),
        missingRun: title?.media_type === 'tv' ? title.library.missing : null,
        cachedAt: new Date(plan.started).toISOString(),
      })
    );
  }),
  // The magnet of a release an earlier search found; one without a magnet "fetches" its .torrent
  http.post('*/api/search/torrents/resolve', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || typeof body.release_id !== 'string') return apiError(400, 'bad_request', 'Send {release_id}.');
    const entry = findMockRelease(body.release_id);
    const { magnet_uri, info_hash } = entry?.release ?? {};
    const answer: ResolveResponse | undefined = entry?.resolves ?? (magnet_uri && info_hash ? { magnet_uri, info_hash } : undefined);
    if (!answer) return notFound('This result expired: search again.');
    return HttpResponse.json(answer);
  }),
  // A dry-run mapping job for the release (#93 builds Inspect around it)
  http.post('*/api/search/torrents/inspect', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || (body.media_type !== 'movie' && body.media_type !== 'tv') || typeof body.tmdb_id !== 'number') {
      return apiError(400, 'bad_request', 'Send {media_type, tmdb_id, release_id | magnet_uri}.');
    }
    const entry = findMockRelease(body.release_id);
    const magnet = typeof body.magnet_uri === 'string' ? body.magnet_uri : (entry?.release.magnet_uri ?? entry?.resolves?.magnet_uri);
    const jobId = magnet ? createInspectJob(body.media_type, body.tmdb_id, magnet) : undefined;
    return jobId ? accepted({ job_id: jobId }) : apiError(400, 'bad_request', 'No such release, magnet or title.');
  }),
];
