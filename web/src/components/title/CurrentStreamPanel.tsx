import { ExternalLink, Search, SlidersHorizontal } from 'lucide-react';
import { Button, buttonClassName, Chip } from '@/components/ui';
import type { TitleStream } from '@/lib/api/contract';
import { formatBytes, formatShortDate } from '@/lib/title/format';

interface CurrentStreamPanelProps {
  stream: TitleStream;
  /** Reopens the HDRezka sheet on this title; left out while HDRezka is off. */
  onChange?: () => void;
  /** Opens torrent search. */
  onReplaceWithTorrent: () => void;
}

/**
 * What Infuse plays from when a ready movie streams from HDRezka (#63), in
 * the Current torrent card's place: the title on HDRezka, the voice-over and
 * quality, size and date, and the ways to change it.
 */
export function CurrentStreamPanel({ stream, onChange, onReplaceWithTorrent }: CurrentStreamPanelProps) {
  return (
    <section className="panel panel-pad flex flex-col gap-3" aria-labelledby="stream-h">
      <h2 className="t-title-3" id="stream-h">
        Current stream
      </h2>
      <p className="hd-name" lang="ru">
        {stream.title}
      </p>
      <div className="chips">
        <Chip tone="accent">HDRezka</Chip>
        <Chip>
          <span lang="ru">{stream.translator}</span>
        </Chip>
        <Chip tone="hi">{stream.quality}</Chip>
      </div>
      <dl className="facts-grid">
        <div>
          <dt className="t-micro text-fg-3">Size</dt>
          <dd>{formatBytes(stream.size_bytes)}</dd>
        </div>
        <div>
          <dt className="t-micro text-fg-3">Added</dt>
          <dd>{formatShortDate(stream.added_at)}</dd>
        </div>
      </dl>
      <div className="flex flex-wrap gap-2">
        {onChange && (
          <Button size="sm" onClick={onChange}>
            <SlidersHorizontal size={15} aria-hidden="true" /> Change voice-over or quality
          </Button>
        )}
        <a href={stream.url} target="_blank" rel="noopener noreferrer" className={buttonClassName({ variant: 'ghost', size: 'sm' })}>
          <ExternalLink size={15} aria-hidden="true" /> Open on HDRezka
        </a>
        <Button variant="ghost" size="sm" onClick={onReplaceWithTorrent}>
          <Search size={15} aria-hidden="true" /> Replace with a torrent
        </Button>
      </div>
    </section>
  );
}
