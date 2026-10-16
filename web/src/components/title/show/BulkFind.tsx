import { Check, HardDriveDownload, Layers, MonitorPlay, Pin, Search } from 'lucide-react';
import { Button } from '@/components/ui';
import type { Keep } from '@/lib/api/contract';
import { keepExpiryLabel } from '@/lib/title/keep';

/** The season's keep (#206): its own download, or the show's, which holds every season. */
export interface SeasonKeep {
  keep: Keep | null;
  /** Whose download `keep` is; a new Keep is the season's. */
  scope: 'season' | 'show';
  /** Opens the Keep sheet on that scope. */
  onKeep: () => void;
}

interface BulkFindProps {
  /** Aired episodes without a file in this season. */
  missing: number;
  /** Episodes still to air in this season. */
  upcoming: number;
  onFindSeason: () => void;
  onFindMissing: () => void;
  /** Opens HDRezka on this season; left out while HDRezka is off. */
  onStreamSeason?: () => void;
  /** Download & keep the season, or change its keep; left out while the show isn't in the library. */
  keep?: SeasonKeep;
}

/** "Download & keep season", or what keeps it: "Season kept until watched · by Nov 16" ("Show kept …" for the show's). */
function KeepSeasonButton({ keep, scope, onKeep }: SeasonKeep) {
  if (!keep) {
    return (
      <Button size="sm" onClick={onKeep} aria-haspopup="dialog">
        <HardDriveDownload size={15} aria-hidden="true" /> Download &amp; keep season
      </Button>
    );
  }
  const whose = scope === 'show' ? 'Show' : 'Season';
  return (
    <Button size="sm" onClick={onKeep} aria-haspopup="dialog">
      <Pin size={15} aria-hidden="true" /> {keepExpiryLabel(keep).replace(/^Kept/, `${whose} kept`)}
      <span className="sr-only">{scope === 'show' ? ': change or stop keeping the show' : ': change or stop keeping this season'}</span>
    </Button>
  );
}

/**
 * Season-wide actions above the list: finds (torrents, and HDRezka when it's
 * set up) or the reassurance that nothing is missing, and Download & keep (#206).
 */
export function BulkFind({ missing, upcoming, onFindSeason, onFindMissing, onStreamSeason, keep }: BulkFindProps) {
  const keepButton = keep && <KeepSeasonButton {...keep} />;
  if (missing === 0) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <p className="t-small flex items-center gap-2 text-fg-3">
          <Check size={15} aria-hidden="true" />
          {upcoming > 0 ? 'Every aired episode this season is ready or watched' : 'Every episode this season is ready or watched'}
        </p>
        {keepButton}
      </div>
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
      {keepButton}
    </div>
  );
}
