import { type Ref, useId } from 'react';
import { ArrowDown, ArrowUp, Check, Clock, HardDrive, Info, Magnet, Plus, ScanSearch, Server, Sparkle, TriangleAlert } from 'lucide-react';
import { Button, Chip, ReleaseName } from '@/components/ui';
import type { Release } from '@/lib/api/contract';
import { prefersGroup, releaseChips, releaseFacts } from '@/lib/search/release';
import { cx } from '@/lib/utils/cx';

/** The best match's "why": at most four of its reasons. */
const MAX_REASONS = 4;

interface ReleaseCardProps {
  release: Release;
  /** The best match: amber outline, its score and reasons, and the list's only primary button. */
  best: boolean;
  /** Add's accessible name, saying what it does ("Add to library & assign"). */
  addLabel: string;
  adding: boolean;
  added: boolean;
  copying: boolean;
  onAdd: () => void;
  onCopyMagnet: () => void;
  /** Inspect (#93), between Add and the magnet: a dry run of the mapping in a nested sheet. */
  onInspect?: () => void;
  /** Its place in the list (`data-index`): Load more focuses the first new card. */
  index?: number;
  ref?: Ref<HTMLElement>;
}

/**
 * One release (HANDOFF §5.2): the full name (wrapping at separators, never
 * truncated), attribute chips, facts, then notes and warnings. Actions sit in
 * one row on phone, Add at its natural width and the magnet pushed right, and
 * in a 132 px column on desktop.
 */
export function ReleaseCard({ release, best, addLabel, adding, added, copying, onAdd, onCopyMagnet, onInspect, index, ref }: ReleaseCardProps) {
  const nameId = useId();
  const chips = releaseChips(release, { preferredGroup: prefersGroup(release) });
  const facts = releaseFacts(release);
  const reasons = best ? release.score_reasons.slice(0, MAX_REASONS) : [];
  return (
    <article
      ref={ref}
      data-index={index}
      tabIndex={-1}
      aria-label={release.raw_title}
      className={cx('release ts-release outline-none', best && 'is-best', release.seeders === 0 && 'is-dim', added && 'is-added')}
    >
      <div className="r-main flex min-w-0 flex-col gap-3">
        {best && (
          <p className="best-head">
            <Sparkle size={15} aria-hidden="true" /> Best match <span className="font-medium text-fg-3">· score {release.score}</span>
          </p>
        )}
        <div id={nameId}>
          <ReleaseName name={release.raw_title} />
        </div>
        {chips.length > 0 && (
          <p className="chips">
            {chips.map((chip, i) => (
              <Chip key={`${i}-${chip.label}`} tone={chip.tone}>
                {chip.label}
              </Chip>
            ))}
          </p>
        )}
        <p className="facts">
          <span>
            <HardDrive size={14} aria-hidden="true" />
            {facts.size}
          </span>
          <span className={cx('seeds', facts.seeders.health === 'low' && 'low', facts.seeders.health === 'none' && 'none')}>
            <ArrowUp size={14} aria-hidden="true" />
            {facts.seeders.label}
          </span>
          <span>
            <ArrowDown size={14} aria-hidden="true" />
            {facts.leechers}
            <span className="sr-only"> leechers</span>
          </span>
          {facts.age && (
            <span>
              <Clock size={14} aria-hidden="true" />
              <time dateTime={facts.age.dateTime} title={facts.age.title}>
                {facts.age.label}
              </time>
              <span className="sr-only"> old</span>
            </span>
          )}
          <span>
            <Server size={14} aria-hidden="true" />
            {facts.indexer}
          </span>
        </p>
        {reasons.length > 0 && (
          <p className="why">
            {reasons.map((reason) => (
              <span key={reason}>{reason}</span>
            ))}
          </p>
        )}
        {release.notes.map((note) => (
          <p key={note} className="t-small flex items-start gap-2 text-fg-2">
            <Info size={15} aria-hidden="true" className="mt-0.5 shrink-0 text-fg-3" />
            <span>{note}</span>
          </p>
        ))}
        {release.warnings.map((warning) => (
          <p key={warning} className="warnline">
            <TriangleAlert size={15} aria-hidden="true" className="shrink-0" />
            <span>{warning}</span>
          </p>
        ))}
      </div>
      <div className="actions">
        {added ? (
          <span className="btn btn-ready" role="status">
            <Check size={18} aria-hidden="true" /> Added
          </span>
        ) : (
          <Button variant={best ? 'primary' : 'secondary'} loading={adding} onClick={onAdd} aria-label={addLabel} aria-describedby={nameId}>
            <Plus size={18} aria-hidden="true" /> Add
          </Button>
        )}
        {onInspect && (
          <Button variant="secondary" onClick={onInspect} aria-haspopup="dialog" aria-describedby={nameId}>
            <ScanSearch size={18} aria-hidden="true" /> Inspect
          </Button>
        )}
        <span className="spacer" />
        <Button
          variant="ghost"
          iconOnly
          className="ts-magnet"
          loading={copying}
          onClick={onCopyMagnet}
          aria-label="Copy magnet link"
          aria-describedby={nameId}
        >
          <Magnet size={18} aria-hidden="true" />
          <span className="lbl" aria-hidden="true">
            Magnet
          </span>
        </Button>
      </div>
    </article>
  );
}
