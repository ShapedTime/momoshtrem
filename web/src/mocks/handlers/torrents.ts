import { http, HttpResponse } from 'msw';
import type { TorrentListResponse } from '@/types/torrent';
import { LEGACY_TORRENTS, TORRENTS_SUMMARY } from '../fixtures/torrents';

/**
 * /api/torrents still answers with the legacy {torrents} shape: the web route
 * exists and the legacy pages read it. #7 switches it to {items: TorrentListItem[]}.
 */
export const torrentsHandlers = [
  http.get('*/api/torrents', () => HttpResponse.json<TorrentListResponse>({ torrents: LEGACY_TORRENTS })),
  http.get('*/api/torrents/summary', () => HttpResponse.json(TORRENTS_SUMMARY)),
];
