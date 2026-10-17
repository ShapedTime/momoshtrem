import type { StreamingSession, TorrentDetail, TorrentListItem, TorrentWanted } from '@/lib/api/contract';
import { formatSpeed, formatNumber, plural } from '@/lib/title/format';
import { keepStateLabel } from '@/lib/title/keep';
import { formatEta, wantedPct } from './downloads';
import { formatTimeOfDay } from './format';

/**
 * Torrent health in words (activity.js `health`), built on the client from
 * what the backend knows. Torrents stream on demand, so there's no finish
 * time to promise, except for the files downloads want (#77): those have an
 * ETA while they download with a speed (`wanted.eta_seconds`).
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

export type HealthKind = 'healthy' | 'slow' | 'paused' | 'waiting' | 'idle' | 'complete';

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

/** Waiting downloads don't stop Infuse: pieces stream on demand. */
const STREAMS_ANYWAY = 'Infuse still plays it: pieces stream on demand.';

/** "42% of what you keep is on disk": the piece cache's, not anacrolix's. */
const keptOnDisk = (wanted: TorrentWanted) => `${wantedPct(wanted)}% of what you keep is on disk`;

/**
 * An idle torrent whose downloads still want bytes: why they wait (PLAN §6
 * "Downloads (#77)" states). Null for `downloading` (the activity manager
 * idled it a moment), which reads as Idle.
 */
function waitingHealth(wanted: TorrentWanted): Health | null {
  const title = keepStateLabel(wanted.state, wantedPct(wanted), wanted.resumes_at);
  const onDisk = `${keptOnDisk(wanted)}.`;
  switch (wanted.state) {
    case 'waiting_for_stream':
      return {
        kind: 'waiting',
        title,
        // resumes_at is set once the last stream ended: the minute's grace in case it picks up again
        body: sentence(
          wanted.resumes_at
            ? `The stream just ended. Background downloads wait a minute in case it picks up again, and resume at ${formatTimeOfDay(Date.parse(wanted.resumes_at))}.`
            : 'Something else is streaming, so background downloads wait and Infuse gets the bandwidth. They pick up a minute after it ends.',
          onDisk
        ),
      };
    case 'waiting_for_window':
      return { kind: 'waiting', title, body: sentence('It’s outside your download windows, so it waits for the next one.', onDisk, STREAMS_ANYWAY) };
    case 'queued':
      return { kind: 'waiting', title, body: sentence('Two downloads run at a time, so this one starts when a slot frees up.', onDisk, STREAMS_ANYWAY) };
    case 'stalled':
      return {
        kind: 'slow',
        title,
        body: sentence(
          'Nothing arrived for 10 minutes, so its slot went to the next download.',
          wanted.resumes_at ? `It tries again at ${formatTimeOfDay(Date.parse(wanted.resumes_at))}.` : 'It tries again soon.',
          onDisk
        ),
      };
    case 'disk_full':
      return {
        kind: 'slow',
        title,
        body: sentence('The disk is nearly full (momoshtrem keeps 5 GB of it free), so nothing more downloads. Free some space, or stop keeping something.', onDisk),
      };
    case 'paused':
      // The torrent isn't paused (that's its own branch): background downloads are off
      return { kind: 'paused', title, body: sentence('Background downloads are off in Settings › Storage, so its downloads wait.', onDisk, STREAMS_ANYWAY) };
    default:
      return null;
  }
}

/**
 * The detail page's health panel: Healthy, Slow but moving (or Waiting for
 * peers), Paused, Idle, Complete or On disk, or why its downloads wait (Queued,
 * Waits for the stream to end, Waits until 01:00, No peers · retrying, Disk
 * full), with what it means for Infuse.
 */
export function healthFor(torrent: TorrentDetail | TorrentListItem, session: StreamingSession | null = null): Health {
  const pct = pctOnDisk(torrent);
  const { wanted } = torrent;
  const wantsMore = wanted !== null && wanted.state !== 'done';

  if (torrent.state === 'paused') {
    const plays = torrent.complete
      ? 'Everything is on disk, so Infuse still plays it.'
      : pct !== null
        ? `What’s on disk (${pct}%) still plays in Infuse; the rest waits until you resume.`
        : 'Infuse plays only what was already on disk; the rest waits until you resume.';
    return {
      kind: 'paused',
      title: 'Paused',
      body: sentence('You paused it: nothing downloads, and playing it in Infuse won’t wake it.', plays, wantsMore && `Its downloads wait too: ${keptOnDisk(wanted)}.`),
    };
  }

  if (torrent.state === 'idle') {
    if (wanted?.state === 'done') {
      return {
        kind: 'complete',
        title: 'On disk',
        body: torrent.complete
          ? 'Everything is on disk. Infuse can play it any time; nothing downloads until it opens a file.'
          : 'Everything you keep of it is on disk, so Infuse plays those files without waiting for the network. The rest fetches when Infuse plays it.',
      };
    }
    const waiting = wanted ? waitingHealth(wanted) : null;
    if (waiting) return waiting;
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
  // What its downloads still want, fetched in the background: how far along, and the ETA while there's a speed
  const kept = wantsMore
    ? `It downloads what you keep in the background: ${wantedPct(wanted)}% is on disk${wanted.eta_seconds !== null ? `, ${formatEta(wanted.eta_seconds)}` : ''}.`
    : null;

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
        kept,
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
            ahead !== null ? `Pieces arrive ahead of where Infuse is reading, about ${formatNumber(ahead)} min ahead of playback.` : 'Pieces arrive around where Infuse is reading.',
            kept
          ),
    };
  }

  return {
    kind: 'healthy',
    title: 'Healthy',
    body: sentence(
      swarm ? `${swarm[0].toUpperCase()}${swarm.slice(1)}${speed ? `, at ${speed}` : ''}.` : null,
      kept ?? 'It fetches pieces around what Infuse plays, so you can start watching now.',
      kept && 'You can start watching now.'
    ),
  };
}
