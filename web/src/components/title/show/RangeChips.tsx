import type { EpisodeRange } from '@/lib/api/contract';
import { rangeLabel } from '@/lib/title/episodes';
import { cx } from '@/lib/utils/cx';

interface RangeChipsProps {
  ranges: EpisodeRange[];
  /** The pressed chip: the frontier's, then wherever you jumped. */
  active: number | null;
  onSelect: (range: EpisodeRange, index: number) => void;
}

/** Story arcs or blocks of ~10 in a long season; each jumps to its first episode and says how many are missing. */
export function RangeChips({ ranges, active, onSelect }: RangeChipsProps) {
  if (!ranges.length) return null;
  return (
    <div className="ep-range" role="group" aria-label="Episode ranges">
      {ranges.map((range, index) => (
        <button
          key={`${range.from}-${range.to}`}
          type="button"
          className={cx('fchip', range.missing === 0 && index !== active && 'text-fg-2')}
          aria-pressed={index === active}
          onClick={() => onSelect(range, index)}
        >
          {rangeLabel(range)}
          {range.missing > 0 && <span className="n">{range.missing} missing</span>}
        </button>
      ))}
    </div>
  );
}
