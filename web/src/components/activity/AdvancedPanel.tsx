'use client';

import { useState } from 'react';
import { ChevronDown, Copy, Tv } from 'lucide-react';
import { Button, copyText, StatusPill, useToast } from '@/components/ui';
import type { TorrentDetail, TorrentTracker } from '@/lib/api/contract';
import { formatAdded, formatPieceSize, readerLine } from '@/lib/activity/format';
import { piecesHave } from '@/lib/activity/pieces';
import { formatBytes, formatNumber, plural } from '@/lib/title/format';
import { PieceMap } from './PieceMap';

function TrackerPill({ tracker }: { tracker: TorrentTracker }) {
  if (tracker.status === 'working') return <StatusPill status="working" />;
  if (tracker.status === 'error') return <StatusPill status="error" />;
  // anacrolix keeps its announce results to itself
  return <StatusPill status="status_unknown" />;
}

function PieceSection({ detail }: { detail: TorrentDetail }) {
  const { pieces, pieces_total: total, piece_length: pieceLength } = detail;
  const have = pieces ? (detail.pieces_have ?? piecesHave(pieces)) : null;
  const caption = [
    `1 square = 1 piece${pieceLength ? ` (${formatPieceSize(pieceLength)})` : ''}`,
    have !== null && total !== null ? `${formatNumber(have)} of ${formatNumber(total)}` : null,
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <section className="flex flex-col gap-3" aria-labelledby="pm-h">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id="pm-h">Piece map</h3>
        {pieces && <span className="t-small text-fg-3 tabular-nums">{caption}</span>}
      </div>
      {pieces && total ? (
        <>
          <PieceMap runs={pieces} total={total} pieceLength={pieceLength} files={detail.files} label={`Piece map: ${formatNumber(have ?? 0)} of ${formatNumber(total)} pieces downloaded`} />
          <ul className="pm-legend">
            <li>
              <i className="k-have" aria-hidden="true" />
              Have
            </li>
            <li>
              <i className="k-dl" aria-hidden="true" />
              Downloading
            </li>
            <li>
              <i className="k-prio" aria-hidden="true" />
              Prioritised for streaming
            </li>
            <li>
              <i className="k-miss" aria-hidden="true" />
              Missing
            </li>
          </ul>
        </>
      ) : (
        <p className="t-small text-fg-3">Load the torrent to see its pieces: it loads when Infuse opens a file.</p>
      )}
    </section>
  );
}

/**
 * Advanced (activity.js `td-adv`): collapsed by default, open with
 * `?advanced=1`. The piece map, active readers, the info hash with Copy,
 * trackers, and ratio, piece size, added and connected.
 */
export function AdvancedPanel({ detail, defaultOpen }: { detail: TorrentDetail; defaultOpen: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  const { toast } = useToast();

  const copyHash = async () => {
    const ok = await copyText(detail.info_hash);
    toast(ok ? { tone: 'ready', icon: Copy, title: 'Info hash copied' } : { tone: 'error', title: 'Couldn’t copy', body: 'Select the text and copy it by hand.' });
  };

  return (
    <details className="panel td-adv" open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>
        <span className="flex flex-col">
          <span className="t-title-3">Advanced</span>
          <span className="t-small text-fg-3">Piece map, readers, trackers, info hash</span>
        </span>
        <ChevronDown size={20} className="chev" aria-hidden="true" />
      </summary>
      {open && (
        <div className="td-adv-body">
          <PieceSection detail={detail} />

          <section className="flex flex-col gap-2" aria-labelledby="rd-h">
            <h3 id="rd-h">Active readers</h3>
            {detail.streaming.length > 0 ? (
              <ul className="flex flex-col gap-2">
                {detail.streaming.map((session) => (
                  <li key={session.id} className="td-reader">
                    <span className="ic" aria-hidden="true">
                      <Tv size={18} />
                    </span>
                    <div className="grow">
                      <strong>{readerLine(session)}</strong>
                      <span className="t-small text-fg-3">{[session.device, session.file_name].filter(Boolean).join(' · ')}</span>
                    </div>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="t-small text-fg-3">No one is reading this torrent right now. Readers appear when Infuse opens a file.</p>
            )}
          </section>

          <section className="flex flex-col gap-2" aria-labelledby="ih-h">
            <h3 id="ih-h">Info hash</h3>
            <div className="copy-field">
              <code className="path">{detail.info_hash}</code>
              <Button size="sm" onClick={copyHash} aria-label="Copy info hash">
                <Copy size={15} aria-hidden="true" /> Copy
              </Button>
            </div>
          </section>

          <section className="flex flex-col gap-2" aria-labelledby="tk-h">
            <h3 id="tk-h">
              Trackers <span className="font-medium text-fg-3 tabular-nums">{formatNumber(detail.trackers.length)}</span>
            </h3>
            {detail.trackers.length > 0 ? (
              <ul className="td-trackers">
                {detail.trackers.map((tracker) => (
                  <li key={tracker.url}>
                    <span className="path">{tracker.url}</span>
                    <TrackerPill tracker={tracker} />
                    {tracker.error && <span className="t-small text-fg-3">{tracker.error}</span>}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="t-small text-fg-3">No trackers: it finds peers through DHT.</p>
            )}
          </section>

          <dl className="td-nerd">
            <div>
              <dt>Ratio</dt>
              <dd>
                {detail.ratio !== null ? detail.ratio.toFixed(2) : '—'}
                {detail.uploaded_bytes !== null && <span className="text-fg-3"> ({formatBytes(detail.uploaded_bytes)} up)</span>}
              </dd>
            </div>
            <div>
              <dt>Piece size</dt>
              <dd>{detail.piece_length !== null ? formatPieceSize(detail.piece_length) : '—'}</dd>
            </div>
            <div>
              <dt>Added</dt>
              <dd>{formatAdded(detail.added_at)}</dd>
            </div>
            <div>
              <dt>Connected</dt>
              <dd>{detail.swarm && detail.swarm.connected_peers > 0 ? plural(detail.swarm.connected_peers, 'peer') : 'None'}</dd>
            </div>
          </dl>
        </div>
      )}
    </details>
  );
}
