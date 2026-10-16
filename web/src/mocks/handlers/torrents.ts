import { http, HttpResponse } from 'msw';
import type { TorrentDetail, TorrentListItem, TorrentsPauseCount, TorrentsSummary, TorrentState } from '@/lib/api/contract';
import { TORRENTS, torrentDetail, torrentsSummary } from '../fixtures/torrents';
import { scenario } from '../scenario';
import { apiError, latency, noContent, notFound } from './util';

/**
 * Activity (PLAN §6 "Torrents and activity (B6)"). Pauses, resumes and deletes
 * last for the page (resetMockTorrents in tests). Scenarios: `torrents-offline`,
 * the list and the summary answer as the proxy does when the backend is gone;
 * `torrents-quiet`, only the idle torrents and nothing streaming.
 */
let torrents: TorrentListItem[] = structuredClone(TORRENTS);
/** What a paused torrent was before, so a resume puts it back. */
let beforePause = new Map<string, TorrentState>();

export function resetMockTorrents(): void {
  torrents = structuredClone(TORRENTS);
  beforePause = new Map();
}

const hex = /^[0-9a-f]{40}$/i;

/** The torrent with the path's hash: hex in either case, as the backend takes it. */
function find(hash: unknown): TorrentListItem | Response {
  if (typeof hash !== 'string' || !hex.test(hash)) {
    return apiError(400, 'bad_request', 'Not a BitTorrent info hash: send 40 hex characters or 32 base32 ones');
  }
  return torrents.find((t) => t.info_hash === hash.toLowerCase()) ?? notFound('Torrent not found');
}

function pause(t: TorrentListItem): void {
  if (t.state === 'paused') return;
  beforePause.set(t.info_hash, t.state);
  Object.assign(t, { state: 'paused', download_bps: 0, upload_bps: 0 });
}

/** Back to what it was; a torrent paused from the start comes back fetching (loaded) or idle. */
function resume(t: TorrentListItem): void {
  if (t.state !== 'paused') return;
  t.state = beforePause.get(t.info_hash) ?? (t.loaded && !t.complete ? 'downloading' : 'idle');
  beforePause.delete(t.info_hash);
}

const unreachable = () => apiError(502, 'backend_unreachable', 'momoshtrem isn’t answering.');
const listed = () => (scenario() === 'torrents-quiet' ? torrents.filter((t) => t.state === 'idle') : torrents);

export const torrentsHandlers = [
  http.get('*/api/torrents', () => {
    if (scenario() === 'torrents-offline') return unreachable();
    return HttpResponse.json({ items: listed() } satisfies { items: TorrentListItem[] });
  }),
  // The static routes go before the :hash ones.
  http.get('*/api/torrents/summary', () => {
    if (scenario() === 'torrents-offline') return unreachable();
    const summary = torrentsSummary(listed());
    if (scenario() !== 'torrents-quiet') return HttpResponse.json(summary);
    const still = summary.history.download_bps.map(() => 0);
    return HttpResponse.json<TorrentsSummary>({ ...summary, streaming: [], upload_bps: 0, history: { ...summary.history, download_bps: still, upload_bps: still } });
  }),
  http.post('*/api/torrents/pause-all', async () => {
    await latency();
    const targets = torrents.filter((t) => t.state === 'streaming' || t.state === 'downloading');
    targets.forEach(pause);
    return HttpResponse.json<TorrentsPauseCount>({ count: targets.length });
  }),
  http.post('*/api/torrents/resume-all', async () => {
    await latency();
    const targets = torrents.filter((t) => t.state === 'paused');
    targets.forEach(resume);
    return HttpResponse.json<TorrentsPauseCount>({ count: targets.length });
  }),
  /** The detail (#99): the item as the page left it, plus pieces, files, trackers and totals. */
  http.get('*/api/torrents/:hash', async ({ params }) => {
    await latency();
    const t = find(params.hash);
    if (t instanceof Response) return t;
    return HttpResponse.json<TorrentDetail>(torrentDetail(t));
  }),
  http.post('*/api/torrents/:hash/pause', async ({ params }) => {
    await latency();
    const t = find(params.hash);
    if (t instanceof Response) return t;
    pause(t);
    return HttpResponse.json<TorrentListItem>(t);
  }),
  http.post('*/api/torrents/:hash/resume', async ({ params }) => {
    await latency();
    const t = find(params.hash);
    if (t instanceof Response) return t;
    resume(t);
    return HttpResponse.json<TorrentListItem>(t);
  }),
  http.delete('*/api/torrents/:hash', async ({ params, request }) => {
    await latency();
    const deleteData = new URL(request.url).searchParams.get('delete_data');
    if (deleteData !== null && deleteData !== 'true' && deleteData !== 'false') {
      return apiError(400, 'bad_request', 'delete_data must be true or false');
    }
    const t = find(params.hash);
    if (t instanceof Response) return t;
    torrents = torrents.filter((other) => other !== t);
    beforePause.delete(t.info_hash);
    return noContent();
  }),
];
