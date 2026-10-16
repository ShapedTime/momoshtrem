import type { Timestamp } from './common';

/** GET /status (G2): what's configured and reachable. Never contains secrets. */
export interface ProbeStatus {
  configured: boolean;
  reachable: boolean;
  latency_ms?: number;
  error?: string;
}

/** One Jackett indexer's latest search outcome since startup; `unknown` before any search (#6). */
export interface IndexerHealth {
  name: string;
  status: 'ok' | 'timeout' | 'error' | 'unknown';
  ms: number | null;
  /** When the current timeout or error streak began ("Timing out for 2 h"). */
  since: Timestamp | null;
  error?: string;
}

export interface JackettStatus extends ProbeStatus {
  /** Omitted when Jackett isn't configured or reachable, and by a backend older than #6. */
  indexers?: IndexerHealth[];
}

export interface SystemStatus {
  version: string;
  uptime_seconds: number;
  /** When the probes ran (cached 30 s; ?refresh=1 re-checks). */
  checked_at: Timestamp;
  db: { ok: boolean; size_bytes: number | null; error?: string };
  webdav: { url: string; port: number; auth_enabled: boolean; username?: string; active_readers?: number };
  jackett: JackettStatus;
  tmdb: ProbeStatus;
  trakt: { configured: boolean; connected: boolean };
  ai: { configured: boolean; mtd_cost_usd: number | null };
  opensubtitles: { configured: boolean };
  /** HDRezka as a second source (#63). Missing from a backend older than #66: read it as off. */
  hdrezka?: ProbeStatus;
  /** Added by the web app's /api/status route. */
  web?: { version: string };
}

/** Body of the web's 503 when the backend can't be reached. */
export interface BackendUnreachableBody {
  error: { code: 'backend_unreachable'; message: string };
  web: { version: string };
  /** Whether WebDAV still answers (it runs in the same process, so usually not). */
  webdav: { reachable: boolean };
}
