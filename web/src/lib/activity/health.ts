import type { StreamingSession, TorrentDetail, TorrentListItem } from '@/lib/api/contract';
import { formatSpeed, formatNumber, plural } from '@/lib/title/format';

/**
 * Torrent health in words (activity.js `health`), built on the client from
 * what the backend knows. Torrents stream on demand, so there's no finish
 * time to promise: never an ETA.
 */

/** Fewer connected seeders than this while fetching is "Few peers". */
export const FEW_SEEDERS = 5;

/** Fetching pieces: downloading, or streaming a file that isn't all on disk yet. */
const fetching = (torrent: Pick<TorrentListItem, 'state' | 'complete'>) =>
  torrent.state === 'downloading' || (torrent.state === 'streaming' && torrent.complete !== true);

/** Few seeders while it fetches (seeders are only known while it's loaded). */
export function isWeak(torrent: Pick<TorrentListItem, 'state' | 'complete' | 'seeders'>): boolean {
  return fetching(torrent) && torrent.seeders !== null && torrent.seeders < FEW_SEEDERS;
}

export type HealthKind = 'healthy' | 'slow' | 'paused' | 'idle' | 'complete';

export interface Health {
  kind: HealthKind;
  title: string;
  body: string;
}

/** How much is on disk, when some of it is (null when unknown, none or all). */
const pctOnDisk = (torrent: Pick<TorrentListItem, 'progress'>) =>
  torrent.progress !== null && torrent.progress > 0 && torrent.progress < 1 ? Math.round(torrent.progress * 100) : null;

const sentence = (...parts: (string | null | false)[]) => parts.filter(Boolean).join(' ');

/** "23 seeders and 48 peers connected" */
function swarmLine(torrent: Pick<TorrentListItem, 'seeders' | 'peers'>): string | null {
  if (torrent.seeders === null) return null;
  const seeders = plural(torrent.seeders, 'seeder');
  return torrent.peers !== null && torrent.peers > torrent.seeders ? `${seeders} and ${plural(torrent.peers, 'peer')} connected` : `${seeders} connected`;
}

/**
 * The detail page's health panel: Healthy, Slow but moving (or Waiting for
 * peers), Paused, Idle or Complete, with what it means for Infuse.
 */
export function healthFor(torrent: TorrentDetail | TorrentListItem, session: StreamingSession | null = null): Health {
  const pct = pctOnDisk(torrent);

  if (torrent.state === 'paused') {
    const plays = torrent.complete
      ? 'Everything is on disk, so Infuse still plays it.'
      : pct !== null
        ? `What’s on disk (${pct}%) still plays in Infuse; the rest waits until you resume.`
        : 'Infuse plays only what was already on disk; the rest waits until you resume.';
    return { kind: 'paused', title: 'Paused', body: sentence('You paused it: nothing downloads, and playing it in Infuse won’t wake it.', plays) };
  }

  if (torrent.state === 'idle') {
    if (torrent.complete) {
      return {
        kind: 'complete',
        title: 'Complete',
        body: 'Everything is on disk. Infuse can play it any time; nothing downloads until it opens a file.',
      };
    }
    return {
      kind: 'idle',
      title: 'Idle',
      body: torrent.loaded
        ? `Nothing is downloading right now. It fetches pieces when Infuse plays a file${pct !== null ? `; ${pct}% is on disk` : ''}.`
        : 'Nothing is downloading. It fetches pieces when Infuse plays a file, and loads the moment you press play.',
    };
  }

  const swarm = swarmLine(torrent);
  const speed = torrent.download_bps > 0 ? formatSpeed(torrent.download_bps) : null;

  if (isWeak(torrent)) {
    if (torrent.seeders === 0 && torrent.download_bps === 0) {
      return {
        kind: 'slow',
        title: 'Waiting for peers',
        body: 'No seeders are connected right now, so nothing arrives. It keeps looking; until then Infuse plays only what’s already on disk.',
      };
    }
    return {
      kind: 'slow',
      title: 'Slow but moving',
      body: sentence(
        `Only ${swarm}${speed ? `, at ${speed}` : ''}.`,
        torrent.state === 'streaming' ? 'Pieces still arrive, just slowly, so Infuse may stop to buffer.' : 'Pieces still arrive, just slowly, so Infuse may stop to buffer when you play it.',
        'Nothing for you to do.'
      ),
    };
  }

  if (torrent.state === 'streaming') {
    const ahead = session?.buffered_ahead_seconds != null && session.buffered_ahead_seconds >= 60 ? Math.floor(session.buffered_ahead_seconds / 60) : null;
    return {
      kind: 'healthy',
      title: 'Healthy',
      body: torrent.complete
        ? 'Everything is on disk, so playback never waits for the network.'
        : sentence(
            swarm ? `${swarm[0].toUpperCase()}${swarm.slice(1)}.` : null,
            ahead !== null ? `Pieces arrive ahead of where Infuse is reading, about ${formatNumber(ahead)} min ahead of playback.` : 'Pieces arrive around where Infuse is reading.'
          ),
    };
  }

  return {
    kind: 'healthy',
    title: 'Healthy',
    body: sentence(
      swarm ? `${swarm[0].toUpperCase()}${swarm.slice(1)}${speed ? `, at ${speed}` : ''}.` : null,
      'It fetches pieces around what Infuse plays, so you can start watching now.'
    ),
  };
}
