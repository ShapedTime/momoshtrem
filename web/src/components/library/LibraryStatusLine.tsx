import { CheckCheck } from 'lucide-react';
import type { LibraryItem } from '@/lib/api/contract';
import { statusLine } from '@/lib/library/display';
import { STATUS } from '@/lib/status';
import { formatNumber } from '@/lib/title/format';

/** "28/38 ready", "Downloading 42%", "Ready", "Needs a torrent": icon plus words, never colour alone. */
export function LibraryStatusLine({ item }: { item: LibraryItem }) {
  const line = statusLine(item);
  switch (line.kind) {
    case 'needs_torrent': {
      const Icon = STATUS.needs_torrent.icon;
      return (
        <span className="ls ls--missing">
          <Icon size={13} aria-hidden="true" />
          {STATUS.needs_torrent.label}
        </span>
      );
    }
    case 'episodes': {
      const Icon = line.complete ? CheckCheck : STATUS.partial.icon;
      return (
        <span className={line.complete ? 'ls ls--ready' : 'ls ls--partial'}>
          <Icon size={13} aria-hidden="true" />
          <span>
            <b>{formatNumber(line.ready)}</b>/{formatNumber(line.total)} ready
          </span>
        </span>
      );
    }
    case 'downloading': {
      const Icon = STATUS.downloading.icon;
      return (
        <span className="ls ls--dl">
          <Icon size={13} aria-hidden="true" />
          {STATUS.downloading.label} {line.pct}%
        </span>
      );
    }
    case 'ready': {
      const Icon = STATUS.ready.icon;
      return (
        <span className="ls ls--ready">
          <Icon size={12} fill="currentColor" aria-hidden="true" />
          {STATUS.ready.label}
        </span>
      );
    }
  }
}
