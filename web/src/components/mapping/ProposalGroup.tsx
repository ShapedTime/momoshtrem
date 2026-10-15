'use client';

import { useId, type ReactNode } from 'react';
import { Check, CheckCheck, ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui';
import type { Proposal, ProposalGroup as Group } from '@/lib/api/contract';
import { extrasPeek, GROUPS, MOVIE_GROUPS, pendingInGroup, seasonRangeSummary } from '@/lib/mapping/review';
import { cx } from '@/lib/utils/cx';

interface ProposalGroupProps {
  group: Group;
  proposals: Proposal[];
  open: boolean;
  onToggle: () => void;
  /** Accepts every pending row with an episode (Needs review, when more than one waits). */
  onAcceptAll: (proposals: Proposal[]) => void;
  movie: boolean;
  children: ReactNode;
}

function Peek({ group, proposals, open }: { group: Group; proposals: Proposal[]; open: boolean }) {
  // Needs review always says how many wait; another group only when a re-check disputed a file in it
  const pending = pendingInGroup(proposals).length;
  if (pending > 0) return <span className="mg-peek mg-peek--review">{pending} to decide</span>;
  if (group === 'needs_review') {
    return (
      <span className="mg-peek mg-peek--ok">
        <Check size={14} aria-hidden="true" />
        All decided
      </span>
    );
  }
  if (open) return null;
  const peek = group === 'matched' ? seasonRangeSummary(proposals) : group === 'extras' ? extrasPeek(proposals) : '';
  return peek ? <span className="mg-peek">{peek}</span> : null;
}

/**
 * A collapsible group of rows (an accessible disclosure): tone dot, title,
 * count and a peek ("2 to decide", "S01 E29–E34, E36–E38") on the toggle.
 */
export function ProposalGroup({ group, proposals, open, onToggle, onAcceptAll, movie, children }: ProposalGroupProps) {
  const id = useId();
  const copy = GROUPS[group];
  const description = (movie && MOVIE_GROUPS[group]?.description) || copy.description;
  // A movie job keeps one file: there's nothing to accept in bulk
  const acceptable = group === 'needs_review' && !movie ? pendingInGroup(proposals) : [];
  return (
    <section className={cx('mgroup', `mgroup--${copy.tone}`, open && 'is-open')} aria-labelledby={`${id}-title`}>
      <div className="mgroup-head">
        <button type="button" className="mgroup-toggle" aria-expanded={open} aria-controls={`${id}-body`} onClick={onToggle}>
          <span className="mg-dot" aria-hidden="true" />
          <span className="mg-title" id={`${id}-title`}>
            {copy.title}
          </span>
          <span className="mg-n t-num">
            {proposals.length}
            <span className="sr-only"> {proposals.length === 1 ? 'file' : 'files'}</span>
          </span>
          <Peek group={group} proposals={proposals} open={open} />
          <ChevronDown size={18} className="mg-chev" aria-hidden="true" />
        </button>
      </div>
      <div className="mgroup-body" id={`${id}-body`} hidden={!open}>
        <div className="mgroup-desc">
          <p>{description}</p>
          {acceptable.length > 1 && (
            <Button variant="ghost" size="sm" onClick={() => onAcceptAll(acceptable)}>
              <CheckCheck size={16} aria-hidden="true" />
              Accept all {acceptable.length}
            </Button>
          )}
        </div>
        <ul className="mlist" aria-label={copy.title}>
          {children}
        </ul>
      </div>
    </section>
  );
}
