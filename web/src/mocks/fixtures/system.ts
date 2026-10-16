import type { SystemStatus } from '@/lib/api/contract';
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
    trakt: { configured: true, connected: false },
    ai: { configured: true, mtd_cost_usd: 3.12 },
    opensubtitles: { configured: true },
    hdrezka: { configured: true, reachable: true, latency_ms: 340 },
  };
}
