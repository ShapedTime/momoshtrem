import type { IndexerHealth, SystemStatus } from '@/lib/api/contract';
import { aiMonthToDate } from './recommendations';
import { aiConfigured } from './settings';
import { traktStatus } from './trakt';

export function systemStatus(): SystemStatus {
  return {
    // CI stamps the backend with the full commit SHA of its image
    version: '9000f05e8c1b2d3a4f5e6d7c8b9a0f1e2d3c4b5a',
    uptime_seconds: 6 * 86_400,
    checked_at: new Date().toISOString(),
    db: { ok: true, size_bytes: 48_000_000 },
    webdav: { url: 'https://momoshtrem.duckdns.org:36911/', port: 36911, auth_enabled: true, username: 'teymur', active_readers: 1 },
    jackett: {
      configured: true,
      reachable: true,
      latency_ms: 210,
      indexers: [
        { name: 'Nyaa', status: 'ok', ms: 210, since: null },
        { name: 'TorrentLeech', status: 'ok', ms: 340, since: null },
        { name: 'The Pirate Bay', status: 'unknown', ms: null, since: null },
      ],
    },
    tmdb: { configured: true, reachable: true, latency_ms: 112 },
    // The page's Trakt account (fixtures/trakt.ts), so Settings and /status agree
    trakt: traktStatus(),
    // The page's AI spend (fixtures/recommendations.ts), so Settings and /status agree
    ai: { configured: aiConfigured(), mtd_cost_usd: aiMonthToDate() },
    opensubtitles: { configured: true },
    hdrezka: { configured: true, reachable: true, latency_ms: 340 },
  };
}

/** The `indexers-degraded` scenario: The Pirate Bay times out (for 2 h) and RuTracker fails (for 5 min). */
export function degradedIndexers(now = Date.now()): IndexerHealth[] {
  return [
    { name: 'Nyaa', status: 'ok', ms: 210, since: null },
    { name: 'TorrentLeech', status: 'ok', ms: 340, since: null },
    { name: 'The Pirate Bay', status: 'timeout', ms: 8000, since: new Date(now - 2 * 3_600_000).toISOString(), error: 'timed out' },
    { name: 'RuTracker', status: 'error', ms: 95, since: new Date(now - 5 * 60_000).toISOString(), error: 'jackett error: Login failed' },
  ];
}
