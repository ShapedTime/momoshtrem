import { Captions, Globe, Link2, type LucideIcon, MonitorPlay, Search, Server, Sparkles, Tv } from 'lucide-react';
import type { IndexerHealth, ProbeStatus, SystemStatus } from '@/lib/api/contract';
import type { StatusKey } from '@/lib/status';
import { formatBytes } from '@/types/torrent';

export type ServiceHealth = 'ok' | 'warning' | 'error' | 'off';

/** One Jackett indexer under its row. */
export interface IndexerRow {
  name: string;
  state: 'ok' | 'failing' | 'unknown';
  /** ok: "210 ms"; failing: "Timing out for 2 h" or "Failing for 5 min: {error}"; unknown: "Not searched yet". */
  text: string;
}

export interface ServiceRow {
  id: string;
  name: string;
  icon: LucideIcon;
  detail: string;
  health: ServiceHealth;
  pill: { status: StatusKey; label?: string };
  /** Jackett's indexers, when the backend reports them (#6). */
  indexers?: IndexerRow[];
  /** What the summary banner says when this row is the only warning. */
  note?: string;
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

/** "5 min", "2 h", "6 days". */
function duration(seconds: number): string {
  const days = Math.floor(seconds / 86_400);
  if (days >= 1) return `${days} ${days === 1 ? 'day' : 'days'}`;
  const hours = Math.floor(seconds / 3600);
  if (hours >= 1) return `${hours} h`;
  return `${Math.max(1, Math.floor(seconds / 60))} min`;
}

const uptime = (seconds: number) => `up ${duration(seconds)}`;

/**
 * An indexer's line. A failing streak's age is measured from the check
 * itself, so the browser's clock doesn't matter.
 */
function indexerRow(indexer: IndexerHealth, checkedAt: string): IndexerRow {
  const { name } = indexer;
  const age = indexer.since ? (Date.parse(checkedAt) - Date.parse(indexer.since)) / 1000 : NaN;
  const streak = Number.isFinite(age) ? ` for ${duration(age)}` : '';
  switch (indexer.status) {
    case 'ok':
      return { name, state: 'ok', text: indexer.ms !== null ? `${indexer.ms} ms` : 'OK' };
    case 'timeout':
      return { name, state: 'failing', text: `Timing out${streak}` };
    case 'error':
      return { name, state: 'failing', text: `Failing${streak}${indexer.error ? `: ${indexer.error}` : ''}` };
    default:
      return { name, state: 'unknown', text: 'Not searched yet' };
  }
}

/** The banner's words for failing indexers: "The Pirate Bay is timing out. Search still works with the others." */
function indexersNote(failing: IndexerHealth[], total: number): string {
  const verb = failing.every((i) => i.status === 'timeout') ? 'timing out' : failing.every((i) => i.status === 'error') ? 'failing' : 'not answering';
  const who = failing.length === 1 ? `${failing[0].name} is ${verb}.` : `${failing.length} indexers are ${verb}.`;
  return failing.length < total ? `${who} Search still works with the others.` : `${who} Torrent search finds nothing until one answers.`;
}

/** Jackett answers: "N of M indexers OK", Degraded while any indexer times out or fails. */
function withIndexers(row: ServiceRow, indexers: IndexerHealth[], checkedAt: string): ServiceRow {
  if (indexers.length === 0) {
    return {
      ...row,
      detail: 'Answering, but no indexers are set up',
      health: 'warning',
      pill: { status: 'warning', label: 'No indexers' },
      note: 'Jackett has no indexers, so torrent search finds nothing. Add one in Jackett.',
    };
  }
  const ok = indexers.filter((i) => i.status === 'ok').length;
  const failing = indexers.filter((i) => i.status === 'timeout' || i.status === 'error');
  const detailed = { ...row, detail: `${ok} of ${indexers.length} indexers OK`, indexers: indexers.map((i) => indexerRow(i, checkedAt)) };
  if (failing.length === 0) return detailed;
  return { ...detailed, health: 'warning', pill: { status: 'warning', label: 'Degraded' }, note: indexersNote(failing, indexers.length) };
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

  const probe = (id: string, name: string, icon: LucideIcon, value: ProbeStatus, okDetail: string, offDetail: string): ServiceRow => {
    if (!value.configured) return { id, name, icon, detail: offDetail, health: 'off', pill: { status: 'not_configured' } };
    if (!value.reachable) {
      return { id, name, icon, detail: value.error ?? 'Not answering', health: 'error', pill: { status: 'unreachable' } };
    }
    return { id, name, icon, detail: `${okDetail}${latency(value.latency_ms)}`, health: 'ok', pill: { status: 'ok' } };
  };
  const jackett = probe('jackett', 'Jackett', Search, status.jackett, 'Answering', 'Not set up: torrent search needs JACKETT_URL and JACKETT_API_KEY');
  // A backend older than #6 doesn't list indexers
  const indexers = status.jackett.indexers;
  rows.push(jackett.health === 'ok' && indexers ? withIndexers(jackett, indexers, status.checked_at) : jackett);
  // Opt-in (#63); a backend older than #66 doesn't report it
  if (status.hdrezka) {
    rows.push(probe('hdrezka', 'HDRezka', MonitorPlay, status.hdrezka, 'Answering', 'Off: set HDREZKA_ENABLED=true to stream from HDRezka'));
  }
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
      body:
        warnings.length === 1 && warnings[0].note
          ? warnings[0].note
          : `${warnings.map((row) => row.name).join(', ')} ${warnings.length === 1 ? 'needs' : 'need'} attention.`,
      short: warnings.length === 1 ? '1 warning' : `${warnings.length} warnings`,
      attention: true,
    };
  }
  return { tone: 'ready', title: 'All good', body: 'Every service momoshtrem depends on answered.', short: 'All good', attention: false };
}
