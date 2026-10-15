import { Captions, Globe, Link2, type LucideIcon, Search, Server, Sparkles, Tv } from 'lucide-react';
import type { SystemStatus } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';
import { formatBytes } from '@/types/torrent';

export type ServiceHealth = 'ok' | 'warning' | 'error' | 'off';

export interface ServiceRow {
  id: string;
  name: string;
  icon: LucideIcon;
  detail: string;
  health: ServiceHealth;
  pill: { status: StatusKey; label?: string };
}

/**
 * How a version reads in Settings. CI stamps images with the full commit SHA,
 * shown short like git does; a release number reads "v1.2.3"; anything else
 * (a local "dev" build) as is.
 */
export function versionLabel(version: string): string {
  if (/^[0-9a-f]{40}$/.test(version)) return version.slice(0, 7);
  return /^\d/.test(version) ? `v${version}` : version;
}

function uptime(seconds: number): string {
  const days = Math.floor(seconds / 86_400);
  if (days >= 1) return `up ${days} ${days === 1 ? 'day' : 'days'}`;
  const hours = Math.floor(seconds / 3600);
  if (hours >= 1) return `up ${hours} h`;
  return `up ${Math.max(1, Math.floor(seconds / 60))} min`;
}

const latency = (ms?: number) => (ms !== undefined ? ` · ${ms} ms` : '');

/** Turn GET /status into the System status rows (screens/settings/status). */
export function serviceRows(status: SystemStatus): ServiceRow[] {
  const rows: ServiceRow[] = [];

  rows.push({
    id: 'backend',
    name: 'momoshtrem backend',
    icon: Server,
    detail: [versionLabel(status.version), uptime(status.uptime_seconds), status.db.size_bytes !== null ? `database ${formatBytes(status.db.size_bytes)}` : null]
      .filter(Boolean)
      .join(' · '),
    health: status.db.ok ? 'ok' : 'error',
    pill: status.db.ok ? { status: 'ok' } : { status: 'error', label: 'Database down' },
  });

  const readers = status.webdav.active_readers;
  rows.push({
    id: 'webdav',
    name: 'WebDAV',
    icon: Tv,
    detail: `:${status.webdav.port}${readers !== undefined ? ` · ${readers} ${readers === 1 ? 'client' : 'clients'} now` : ''}${status.webdav.auth_enabled ? '' : ' · no password'}`,
    health: 'ok',
    pill: { status: 'ok' },
  });

  const probe = (id: string, name: string, icon: LucideIcon, value: SystemStatus['jackett'], okDetail: string, offDetail: string): ServiceRow => {
    if (!value.configured) return { id, name, icon, detail: offDetail, health: 'off', pill: { status: 'not_configured' } };
    if (!value.reachable) {
      return { id, name, icon, detail: value.error ?? 'Not answering', health: 'error', pill: { status: 'unreachable' } };
    }
    return { id, name, icon, detail: `${okDetail}${latency(value.latency_ms)}`, health: 'ok', pill: { status: 'ok' } };
  };
  rows.push(probe('jackett', 'Jackett', Search, status.jackett, 'Answering', 'Not set up: torrent search needs JACKETT_URL and JACKETT_API_KEY'));
  rows.push(probe('tmdb', 'TMDB', Globe, status.tmdb, 'API v3', 'Not set up: titles and artwork need TMDB_API_KEY'));

  rows.push({
    id: 'trakt',
    name: 'Trakt',
    icon: Link2,
    detail: !status.trakt.configured ? 'No Trakt app keys yet' : status.trakt.connected ? 'Connected' : 'Not connected',
    health: status.trakt.connected ? 'ok' : 'off',
    pill: status.trakt.connected ? { status: 'ok' } : { status: 'not_configured', label: status.trakt.configured ? 'Not connected' : undefined },
  });

  const cost = status.ai.mtd_cost_usd;
  rows.push({
    id: 'ai',
    name: 'AI',
    icon: Sparkles,
    detail: status.ai.configured
      ? `Anthropic key configured${cost !== null ? ` · $${cost.toFixed(2)} this month` : ''}`
      : 'No Anthropic key: AI features are off',
    health: status.ai.configured ? 'ok' : 'off',
    pill: status.ai.configured ? { status: 'ok' } : { status: 'not_configured' },
  });

  rows.push({
    id: 'opensubtitles',
    name: 'OpenSubtitles',
    icon: Captions,
    detail: status.opensubtitles.configured ? 'Key configured' : 'No API key: subtitle search is off',
    health: status.opensubtitles.configured ? 'ok' : 'off',
    pill: status.opensubtitles.configured ? { status: 'ok' } : { status: 'not_configured' },
  });

  return rows;
}

export interface StatusSummary {
  tone: 'ready' | 'warning' | 'error';
  title: string;
  body: string;
  /** Short form for the settings nav. */
  short: string;
  attention: boolean;
}

export function summarize(rows: ServiceRow[]): StatusSummary {
  const errors = rows.filter((row) => row.health === 'error');
  const warnings = rows.filter((row) => row.health === 'warning');
  if (errors.length > 0) {
    return {
      tone: 'error',
      title: errors.length === 1 ? `${errors[0].name} isn’t working` : `${errors.length} services aren’t working`,
      body: `${errors.map((row) => row.name).join(', ')} ${errors.length === 1 ? 'needs' : 'need'} attention. Everything else answered.`,
      short: errors.length === 1 ? '1 problem' : `${errors.length} problems`,
      attention: true,
    };
  }
  if (warnings.length > 0) {
    return {
      tone: 'warning',
      title: 'Mostly fine',
      body: `${warnings.map((row) => row.name).join(', ')} ${warnings.length === 1 ? 'needs' : 'need'} attention.`,
      short: warnings.length === 1 ? '1 warning' : `${warnings.length} warnings`,
      attention: true,
    };
  }
  return { tone: 'ready', title: 'All good', body: 'Every service momoshtrem depends on answered.', short: 'All good', attention: false };
}
