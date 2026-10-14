import type { Timestamp } from './common';

/** GET /status (G2): what's configured and reachable. Never contains secrets. */
export interface ProbeStatus {
  configured: boolean;
  reachable: boolean;
  latency_ms?: number;
  error?: string;
}

export interface SystemStatus {
  version: string;
  uptime_seconds: number;
  /** When the probes ran (cached 30 s; ?refresh=1 re-checks). */
  checked_at: Timestamp;
  db: { ok: boolean; size_bytes: number | null; error?: string };
  webdav: { url: string; port: number; auth_enabled: boolean; username?: string; active_readers?: number };
  jackett: ProbeStatus;
  tmdb: ProbeStatus;
  trakt: { configured: boolean; connected: boolean };
  ai: { configured: boolean; mtd_cost_usd: number | null };
  opensubtitles: { configured: boolean };
}
