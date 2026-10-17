import type { TorrentListItem } from '@/lib/api/contract';
import { formatBytes } from '@/lib/title/format';
import { torrentLabel } from './format';

/** The pending-removal key that hides a torrent from Activity while its Undo runs. */
export const torrentRemovalKey = (infoHash: string) => `torrent:${infoHash.toLowerCase()}`;

/**
 * The "Remove this torrent?" dialog (activity.js `act-remove`): what Infuse
 * loses, that the titles stay in the library, and that a Keep on it stays
 * (#77, B6 DELETE: with nothing else assigned it's `no_file` until something is).
 */
export function removeTorrentCopy(torrent: Pick<TorrentListItem, 'name' | 'titles' | 'size_bytes' | 'complete' | 'holds'>): {
  title: string;
  body: string;
  checkbox: string | null;
} {
  const label = torrentLabel(torrent);
  const kinds = new Set(torrent.titles.map((title) => title.media_type));
  const many = torrent.titles.length > 1;
  let stays: string;
  if (!label.linked) stays = 'Nothing in your library uses it, so nothing else changes.';
  else if (kinds.size > 1) stays = 'The titles stay in your library and show “Needs a torrent”.';
  else if (kinds.has('tv')) stays = `The ${many ? 'shows stay' : 'show stays'} in your library and those episodes show “Needs a torrent”.`;
  else stays = `The ${many ? 'movies stay' : 'movie stays'} in your library and ${many ? 'show' : 'shows'} “Needs a torrent”.`;

  const keeps = torrent.holds.filter((hold) => hold.reason === 'kept').length;
  const kept =
    keeps === 0
      ? null
      : keeps === 1
        ? 'The Keep stays; with no other file it shows “No file yet” until it has one again.'
        : 'The keeps stay; with no other file they show “No file yet” until they have one again.';

  const plays = label.linked ? `Infuse will no longer be able to play ${label.text}.` : 'Infuse will no longer be able to play its files.';
  // Torrents stream on demand: unless it's complete, only part of it is on disk
  const size = torrent.size_bytes === null ? null : torrent.complete ? formatBytes(torrent.size_bytes) : `up to ${formatBytes(torrent.size_bytes)}`;
  return {
    title: 'Remove this torrent?',
    body: [plays, stays, kept].filter(Boolean).join(' '),
    checkbox: size ? `Also delete downloaded data (${size})` : null,
  };
}
