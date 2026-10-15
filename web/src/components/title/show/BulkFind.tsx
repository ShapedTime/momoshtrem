import { Check, Layers, MonitorPlay, Search } from 'lucide-react';
import { Button } from '@/components/ui';

interface BulkFindProps {
  /** Aired episodes without a file in this season. */
  missing: number;
  /** Episodes still to air in this season. */
  upcoming: number;
  onFindSeason: () => void;
  onFindMissing: () => void;
  /** Opens HDRezka on this season; left out while HDRezka is off. */
  onStreamSeason?: () => void;
}

/** Season-wide finds above the list (torrents, and HDRezka when it's set up), or the reassurance that nothing is missing. */
export function BulkFind({ missing, upcoming, onFindSeason, onFindMissing, onStreamSeason }: BulkFindProps) {
  if (missing === 0) {
    return (
      <p className="t-small flex items-center gap-2 text-fg-3">
        <Check size={15} aria-hidden="true" />
        {upcoming > 0 ? 'Every aired episode this season is ready or watched' : 'Every episode this season is ready or watched'}
      </p>
    );
  }
  return (
    <div className="flex flex-wrap gap-2">
      <Button size="sm" onClick={onFindSeason}>
        <Layers size={15} aria-hidden="true" /> Find season pack
      </Button>
      <Button size="sm" onClick={onFindMissing}>
        <Search size={15} aria-hidden="true" /> Find missing episodes <span className="text-fg-3 tabular-nums">{missing}</span>
      </Button>
      {onStreamSeason && (
        <Button size="sm" onClick={onStreamSeason}>
          <MonitorPlay size={15} aria-hidden="true" /> Stream season from HDRezka
        </Button>
      )}
    </div>
  );
}
