import type { SystemStatus } from '@/lib/api/contract';
export function systemStatus(): SystemStatus {
  return {
    version: '0.9.2',
    uptime_seconds: 6 * 86_400,
    checked_at: new Date().toISOString(),
    db: { ok: true, size_bytes: 48_000_000 },
    webdav: { url: 'https://momoshtrem.duckdns.org:36911/', port: 36911, auth_enabled: true, username: 'teymur', active_readers: 1 },
    jackett: { configured: true, reachable: true, latency_ms: 210 },
    tmdb: { configured: true, reachable: true, latency_ms: 112 },
    trakt: { configured: true, connected: false },
    ai: { configured: true, mtd_cost_usd: 3.12 },
    opensubtitles: { configured: true },
  };
}
