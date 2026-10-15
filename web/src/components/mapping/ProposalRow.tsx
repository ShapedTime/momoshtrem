'use client';

import {
  ArrowRight,
  ArrowRightLeft,
  Captions,
  Check,
  CircleAlert,
  CornerDownRight,
  EyeOff,
  Film,
  ListChecks,
  Plus,
  RotateCcw,
  Sparkle,
  TriangleAlert,
  User,
} from 'lucide-react';
import { Button, Chip, Confidence, FilePath } from '@/components/ui';
import type { Proposal } from '@/lib/api/contract';
import { canToggleAccept, ignoredLabel, isFullRow, maps, targetCode, targetName } from '@/lib/mapping/review';
import { formatBytes, plural } from '@/lib/title/format';
import { cx } from '@/lib/utils/cx';

/** What a row can ask the page to do. */
export interface RowActions {
  /** Desktop: select the row for the pane; phone: open the picker. `focusSearch` for Change. */
  select: (p: Proposal, focusSearch?: boolean) => void;
  accept: (p: Proposal) => void;
  unaccept: (p: Proposal) => void;
  ignore: (p: Proposal) => void;
  /** Undo an ignore, or Include an extra. */
  restore: (p: Proposal) => void;
}

export const rowId = (proposalId: number) => `proposal-${proposalId}`;

interface ProposalRowProps {
  proposal: Proposal;
  /** Path without the torrent's folder. */
  path: string;
  /** "#30", "S00E05": names the file in labels and notes. */
  short: string;
  /** The first other file that wants the same episode. */
  conflictWith: string | null;
  /** The job maps a movie: rows accept or ignore, there are no episodes to pick. */
  movieTitle: string | null;
  selected: boolean;
  flash: boolean;
  actions: RowActions;
}

function Target({ p, movieTitle }: { p: Proposal; movieTitle: string | null }) {
  if (p.decision === 'ignored') {
    return (
      <div className="mt-ep mt-ep--ignored">
        <EyeOff size={15} aria-hidden="true" />
        <span>{ignoredLabel(p)}</span>
        <span className="text-fg-3">· not in Infuse</span>
      </div>
    );
  }
  if (p.kind === 'movie') {
    return (
      <div className="mt-ep mt-ep--movie">
        <Film size={15} aria-hidden="true" />
        <span className="mt-name">{movieTitle}</span>
      </div>
    );
  }
  if (p.targets.length > 0) {
    return (
      <div className="mt-ep">
        <span className="mt-se t-num">{targetCode(p.targets)}</span>
        <span className="mt-name">{targetName(p.targets)}</span>
      </div>
    );
  }
  return (
    <div className="mt-ep mt-ep--none">
      <CircleAlert size={15} aria-hidden="true" />
      <span>No episode yet</span>
    </div>
  );
}

function Reason({ p }: { p: Proposal }) {
  if (p.source === 'manual') {
    return (
      <p className="mt-reason mt-reason--you">
        <User size={14} aria-hidden="true" />
        <span>You picked this episode</span>
      </p>
    );
  }
  if (!p.reason) return null;
  if (p.source === 'ai') {
    return (
      <p className="mt-reason mt-reason--ai">
        <Sparkle size={14} aria-hidden="true" />
        <span>
          <b>{p.hinted ? 'AI, with your hint:' : 'AI:'}</b> {p.reason}
        </span>
      </p>
    );
  }
  return (
    <p className="mt-reason">
      <ListChecks size={14} aria-hidden="true" />
      <span>{p.reason}</span>
    </p>
  );
}

function Meta({ p }: { p: Proposal }) {
  if (p.decision === 'ignored') return null;
  const confidence = p.source !== 'manual' && (p.kind !== 'extra' || p.targets.length > 0);
  if (!confidence && !p.updated && p.subtitle_count === 0) return null;
  return (
    <div className="mt-meta">
      {confidence && <Confidence level={p.confidence ?? 'none'} />}
      {p.updated && <Chip tone="accent">Updated</Chip>}
      {p.subtitle_count > 0 && (
        <span className="mt-subs">
          <Captions size={14} aria-hidden="true" />
          {plural(p.subtitle_count, 'subtitle')}
        </span>
      )}
    </div>
  );
}

function Notes({ p, conflictWith }: { p: Proposal; conflictWith: string | null }) {
  if (p.decision === 'ignored') return null;
  if (conflictWith) {
    return (
      <p className="mt-conflict">
        <TriangleAlert size={14} aria-hidden="true" />
        <span>
          Same episode as the <b>{conflictWith}</b> file. Keep one of them.
        </span>
      </p>
    );
  }
  if (p.replaces && p.targets[0]) {
    return (
      <p className="mt-note" title={p.replaces.release_name}>
        <ArrowRightLeft size={14} aria-hidden="true" />
        <span>Replaces the current {targetCode([p.targets[0]])} file</span>
      </p>
    );
  }
  return null;
}

function Actions({ p, short, movie, actions }: { p: Proposal; short: string; movie: boolean; actions: RowActions }) {
  if (p.decision === 'ignored') {
    const include = p.group === 'extras';
    return (
      <Button
        variant="ghost"
        size="sm"
        data-action="restore"
        aria-label={include ? `Include ${short}` : `Undo: map ${short} again`}
        onClick={() => actions.restore(p)}
      >
        {include ? <Plus size={16} aria-hidden="true" /> : <RotateCcw size={16} aria-hidden="true" />}
        {include ? 'Include' : 'Undo'}
      </Button>
    );
  }

  if (!isFullRow(p)) {
    return (
      <>
        {!movie && (
          <Button variant="ghost" size="sm" iconOnly data-action="change" aria-label={`Change episode for ${short}`} onClick={() => actions.select(p, true)}>
            <ArrowRightLeft size={17} aria-hidden="true" />
          </Button>
        )}
        <Button variant="ghost" size="sm" iconOnly data-action="ignore" aria-label={`Ignore ${short}`} onClick={() => actions.ignore(p)}>
          <EyeOff size={17} aria-hidden="true" />
        </Button>
      </>
    );
  }

  const accepted = p.decision === 'accepted';
  const target = p.targets.length > 0;
  return (
    <>
      {!movie && (
        <Button
          variant={target ? 'ghost' : 'secondary'}
          size="sm"
          data-action="change"
          aria-label={`${target ? 'Change episode' : 'Pick an episode'} for ${short}`}
          onClick={() => actions.select(p, true)}
        >
          <ArrowRightLeft size={16} aria-hidden="true" />
          {target ? 'Change' : 'Pick episode'}
        </Button>
      )}
      <Button variant="ghost" size="sm" data-action="ignore" aria-label={`Ignore ${short}`} onClick={() => actions.ignore(p)}>
        <EyeOff size={16} aria-hidden="true" />
        Ignore
      </Button>
      {canToggleAccept(p) &&
        (accepted ? (
          <Button variant="ready" size="sm" data-action="accept" aria-pressed="true" aria-label={`Accepted: ${short}. Tap to undo`} onClick={() => actions.unaccept(p)}>
            <Check size={16} aria-hidden="true" />
            Accepted
          </Button>
        ) : (
          <Button variant="secondary" size="sm" className="mt-accept" data-action="accept" aria-label={`Accept ${short}`} onClick={() => actions.accept(p)}>
            <Check size={16} aria-hidden="true" />
            Accept
          </Button>
        ))}
    </>
  );
}

/**
 * One file → its episode (PLAN §6 Proposal): the path, where it goes with
 * confidence and why (rules, AI or you), notes (a conflict, the file it
 * replaces) and its actions. Full rows for decisions, compact ones to spot-check.
 */
export function ProposalRow({ proposal: p, path, short, conflictWith, movieTitle, selected, flash, actions }: ProposalRowProps) {
  const full = isFullRow(p);
  const pending = p.decision === 'pending';
  const ignored = p.decision === 'ignored';
  const movie = movieTitle !== null;
  return (
    <li
      id={rowId(p.id)}
      className={cx(
        'mrow',
        full ? 'mrow--full' : 'mrow--compact',
        p.group === 'needs_review' && pending && 'is-review',
        p.group === 'needs_review' && !pending && !ignored && 'is-resolved',
        p.group === 'unmatched' && pending && !maps(p) && 'is-unmatched',
        ignored && 'is-ignored',
        conflictWith && 'is-conflict',
        selected && 'is-selected',
        flash && 'is-flash'
      )}
    >
      <div className="mrow-file">
        {movie ? (
          <span className="mrow-name">
            <FilePath path={path} />
          </span>
        ) : (
          <button type="button" className="mrow-name" aria-label={`${path}: choose its episode`} onClick={() => actions.select(p)}>
            <FilePath path={path} />
          </button>
        )}
        <span className="mrow-size t-num">{formatBytes(p.file_size)}</span>
      </div>
      <span className="mrow-arrow" aria-hidden="true">
        <CornerDownRight size={16} className="a-stack" />
        <ArrowRight size={16} className="a-wide" />
      </span>
      <div className="mrow-target">
        <Target p={p} movieTitle={movieTitle} />
        <Meta p={p} />
        <Reason p={p} />
        <Notes p={p} conflictWith={conflictWith} />
      </div>
      <div className="mrow-actions">
        <Actions p={p} short={short} movie={movie} actions={actions} />
      </div>
    </li>
  );
}
