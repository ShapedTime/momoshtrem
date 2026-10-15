'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import { useMappingActions } from '@/hooks/useMappingJob';
import type { AssignmentJobDetail, Proposal, ProposalChange, ProposalGroup as Group } from '@/lib/api/contract';
import { showEpisodesQuery } from '@/lib/api/endpoints';
import { type PickerEpisode, takenBy } from '@/lib/mapping/picker';
import {
  conflictMap,
  defaultGroupOpen,
  displayPath,
  fileName,
  firstBlocker,
  firstPending,
  footState,
  groupProposals,
  maps,
  movieUndo,
  restoredDecision,
  reviewCounts,
  shortName,
  titleHref,
  withDecisions,
} from '@/lib/mapping/review';
import { cx } from '@/lib/utils/cx';
import { EpisodePickerPane, EpisodePickerSheet, type PickerCatalog } from './EpisodePicker';
import { MappingFoot } from './MappingFoot';
import { MappingHeader, MappingMoreMenu, MappingSummary } from './MappingHeader';
import { ProposalGroup } from './ProposalGroup';
import { type RowActions, ProposalRow, rowId } from './ProposalRow';
import { RecheckPanel } from './RecheckPanel';

const FLASH_MS = 1200;
/** How long a focus move waits for its target to render. */
const FOCUS_WAIT_MS = 2000;
const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
const isDesktop = () => typeof window !== 'undefined' && window.matchMedia(DESKTOP_QUERY).matches;

/** After React has painted the change: the row to scroll to. */
const afterPaint = (run: () => void) => window.requestAnimationFrame(() => window.requestAnimationFrame(run));

type FocusTarget = { kind: 'row'; id: number; actions: string[] } | { kind: 'search' };

/**
 * The review (PLAN F4): groups by urgency with full and compact rows, the
 * episode picker (a pane on desktop, a sheet on phone and tablet), and the
 * sticky foot. Every change is a batch PATCH; the answer is the new state.
 */
export function ReviewScreen({ detail }: { detail: AssignmentJobDetail }) {
  const { job, proposals } = detail;
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const actions = useMappingActions(job);
  const movie = job.item.media_type === 'movie';
  const showTitle = job.item.title;
  const catalogQuery = useQuery({ ...showEpisodesQuery(job.item.id ?? 0), enabled: !movie && job.item.id !== null });

  const counts = useMemo(() => reviewCounts(proposals), [proposals]);
  const conflicts = useMemo(() => conflictMap(proposals), [proposals]);
  const groups = useMemo(() => groupProposals(proposals), [proposals]);
  const foot = footState(counts, { movie });

  const [selectedId, setSelectedId] = useState<number | null>(() => (isDesktop() && !movie ? (firstPending(proposals)?.id ?? null) : null));
  const [opened, setOpened] = useState<Partial<Record<Group, boolean>>>({});
  const [flash, setFlash] = useState<number | null>(null);
  const [sheet, setSheet] = useState<{ id: number; open: boolean; key: number } | null>(null);
  const [hideTaken, setHideTaken] = useState(false);
  const [applyError, setApplyError] = useState<{ message: string; edits: number } | null>(null);
  const [edits, setEdits] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);

  const selected = proposals.find((p) => p.id === selectedId) ?? null;
  const sheetProposal = sheet ? (proposals.find((p) => p.id === sheet.id) ?? null) : null;
  const pickerFor = desktop ? selected : sheetProposal;
  const taken = useMemo(() => takenBy(proposals, pickerFor?.id ?? null, showTitle), [proposals, pickerFor?.id, showTitle]);
  const isOpen = (group: Group) => opened[group] ?? defaultGroupOpen(group, desktop);
  // Movie cuts share the title: their file names tell them apart
  const short = useCallback((p: Proposal) => (movie ? fileName(p.file_path) : shortName(p.file_path, showTitle)), [movie, showTitle]);
  const path = useCallback((p: Proposal) => displayPath(p.file_path, job.torrent.name), [job.torrent.name]);

  useEffect(() => {
    if (flash === null) return;
    const timer = window.setTimeout(() => setFlash(null), FLASH_MS);
    return () => window.clearTimeout(timer);
  }, [flash]);
  const flashRow = useCallback((id: number) => {
    // Restart the animation when the same row flashes twice
    setFlash(null);
    window.requestAnimationFrame(() => setFlash(id));
  }, []);

  const change = useCallback(
    (items: ProposalChange[]) => {
      setEdits((n) => n + 1);
      return actions.change(items);
    },
    [actions]
  );

  const catalog: PickerCatalog =
    catalogQuery.isPending ? { status: 'pending' }
    : catalogQuery.isError ? { status: 'error', message: catalogQuery.error.message, retry: () => void catalogQuery.refetch() }
    : { status: 'success', seasons: catalogQuery.data.items };

  /**
   * Where focus goes once a change has rendered (the button you pressed may be
   * gone): a row's action, the first that exists, or the pane's search. Kept
   * briefly, so a change that never renders (a failed PATCH) doesn't move focus later.
   */
  const pendingFocus = useRef<{ target: FocusTarget; scroll: boolean; until: number } | null>(null);
  const focusAfterRender = (target: FocusTarget, scroll = false) => {
    pendingFocus.current = { target, scroll, until: Date.now() + FOCUS_WAIT_MS };
  };
  useLayoutEffect(() => {
    const pending = pendingFocus.current;
    if (!pending) return;
    if (Date.now() > pending.until) {
      pendingFocus.current = null;
      return;
    }
    const { target } = pending;
    const row = target.kind === 'row' ? document.getElementById(rowId(target.id)) : null;
    const element =
      target.kind === 'search'
        ? searchRef.current
        : target.actions.map((action) => row?.querySelector<HTMLElement>(`[data-action="${action}"]`)).find((found) => found);
    if (!element) return;
    pendingFocus.current = null;
    element.focus({ preventScroll: true });
    if (pending.scroll) row?.scrollIntoView({ block: 'center', behavior: prefersReducedMotion() ? 'auto' : 'smooth' });
  });

  const rowActions: RowActions = {
    select: (p, focusSearch = false) => {
      if (movie) return;
      setSelectedId(p.id);
      if (desktop) {
        if (focusSearch) focusAfterRender({ kind: 'search' });
      } else setSheet((current) => ({ id: p.id, open: true, key: (current?.key ?? 0) + 1 }));
    },
    accept: (p) => {
      void change([{ id: p.id, decision: 'accepted' }]);
      flashRow(p.id);
      if (desktop && !movie) {
        const next = firstPending(withDecisions(proposals, [{ id: p.id, decision: 'accepted' }]), p.id);
        if (next) setSelectedId(next.id);
      }
    },
    // A movie pick ignored the other files: they all come back, so you choose again
    unaccept: (p) => void change(movie && p.kind === 'movie' ? movieUndo(proposals) : [{ id: p.id, decision: 'pending' }]),
    ignore: (p) => {
      if (sheet?.id === p.id) setSheet({ ...sheet, open: false });
      focusAfterRender({ kind: 'row', id: p.id, actions: ['restore'] });
      void change([{ id: p.id, decision: 'ignored' }]);
      flashRow(p.id);
    },
    restore: (p) => {
      // An extra with no episode becomes pending, and you pick one
      const decision = restoredDecision(p);
      if (p.group === 'extras' && !maps(p)) rowActions.select(p);
      else focusAfterRender({ kind: 'row', id: p.id, actions: ['accept', 'change', 'ignore'] });
      void change([{ id: p.id, decision }]);
      flashRow(p.id);
    },
  };

  /** Accept all: focus moves to the first of them, now "Accepted" (the button you pressed goes away). */
  const acceptAll = (pending: Proposal[]) => {
    if (pending.length === 0) return;
    focusAfterRender({ kind: 'row', id: pending[0].id, actions: ['accept'] });
    void change(pending.map((p) => ({ id: p.id, decision: 'accepted' as const })));
  };

  const pick = async (p: Proposal, episode: PickerEpisode) => {
    const same = p.targets.length === 1 && p.targets[0].episode_id === episode.id;
    if (sheet?.open) setSheet({ ...sheet, open: false });
    const answer = await change([same ? { id: p.id, decision: 'accepted' } : { id: p.id, episode_ids: [episode.id], decision: 'accepted' }]);
    if (!answer) return;
    flashRow(p.id);
    if (!desktop) afterPaint(() => document.getElementById(rowId(p.id))?.scrollIntoView({ block: 'center' }));
  };

  const apply = () => {
    if (foot.blocked) {
      const blocker = firstBlocker(proposals);
      if (!blocker) return;
      const { proposal } = blocker;
      setOpened((current) => ({ ...current, [proposal.group]: true }));
      if (desktop && !movie) setSelectedId(proposal.id);
      flashRow(proposal.id);
      const first = blocker.reason === 'conflict' && !movie ? 'change' : 'accept';
      focusAfterRender({ kind: 'row', id: proposal.id, actions: [first, 'change', 'accept', 'ignore'] }, true);
      return;
    }
    setApplyError(null);
    const at = edits;
    void actions.apply((message) => setApplyError({ message, edits: at }));
  };

  return (
    <>
      <MappingHeader job={job} summary={<MappingSummary counts={counts} />} more={<MappingMoreMenu onDiscard={() => void actions.discard()} />} />
      <div className={cx('map-layout page', desktop && !movie && 'has-pane')}>
        <div className="map-main">
          {job.ai_available && <RecheckPanel detail={detail} />}
          <div className="map-groups">
            {groups.map(({ group, proposals: rows }) => (
              <ProposalGroup
                key={group}
                group={group}
                proposals={rows}
                open={isOpen(group)}
                onToggle={() => setOpened((current) => ({ ...current, [group]: !isOpen(group) }))}
                onAcceptAll={acceptAll}
                movie={movie}
              >
                {rows.map((p) => {
                  const others = conflicts.get(p.id);
                  return (
                    <ProposalRow
                      key={p.id}
                      proposal={p}
                      path={path(p)}
                      short={short(p)}
                      conflictWith={others?.[0] ? short(others[0]) : null}
                      movieTitle={movie ? showTitle : null}
                      selected={desktop && p.id === selectedId}
                      flash={p.id === flash}
                      actions={rowActions}
                    />
                  );
                })}
              </ProposalGroup>
            ))}
          </div>
        </div>
        {desktop && !movie && (
          <EpisodePickerPane
            key={selected?.id ?? 'none'}
            catalog={catalog}
            proposal={selected}
            path={selected ? path(selected) : null}
            taken={taken}
            hideTaken={hideTaken}
            onHideTaken={setHideTaken}
            onPick={(episode) => selected && void pick(selected, episode)}
            searchRef={searchRef}
          />
        )}
      </div>
      <MappingFoot
        state={foot}
        error={applyError && applyError.edits === edits ? applyError.message : null}
        cancelHref={titleHref(job)}
        applying={actions.applying}
        onApply={apply}
      />
      {!desktop && sheet && sheetProposal && (
        <EpisodePickerSheet
          key={sheet.key}
          open={sheet.open}
          onOpenChange={(open) => setSheet({ ...sheet, open })}
          catalog={catalog}
          proposal={sheetProposal}
          path={path(sheetProposal)}
          taken={taken}
          hideTaken={hideTaken}
          onHideTaken={setHideTaken}
          onPick={(episode) => void pick(sheetProposal, episode)}
          onIgnore={() => rowActions.ignore(sheetProposal)}
        />
      )}
    </>
  );
}
