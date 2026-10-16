import type { RecommendationsHome, RunProgress, TraktIntegration, TraktSync, TraktSyncStep } from '@/lib/api/contract';
import { IMPORT_STEPS } from '@/lib/settings/trakt';
import { formatNumber, plural } from '@/lib/title/format';

/**
 * Which For You the page shows (#135), first match:
 * - `loading`: the integration (or, without Trakt, the library) isn't known yet;
 * - `first_time`: no Trakt and an empty library: "Evening. I'm Noir." and three steps;
 * - `trakt_off`: no Trakt: a trending hero, "Make this page yours" and trending rails;
 * - `first_sync`: the first import after connecting is running, or stopped with an error, or
 *   Noir is writing the first shelves after it (#155);
 * - `default`: the hero, Up next, New episodes and the watchlist.
 */
export type ForYouState = 'loading' | 'first_time' | 'trakt_off' | 'first_sync' | 'default';

/**
 * An account is linked, so /me answers from what it imported: connected,
 * reauth (the data stays), or a new code pending from reauth.
 */
export const traktLinked = (trakt: Pick<TraktIntegration, 'state' | 'username'>) =>
  trakt.state === 'connected' || trakt.state === 'reauth' || (trakt.state === 'pending' && trakt.username !== null);

export interface ForYouInput {
  /** Undefined while loading; null when it failed (the page carries on without Trakt). */
  trakt: TraktIntegration | null | undefined;
  /** The library's `summary.total`: undefined while loading, null when it failed. */
  libraryTotal: number | null | undefined;
  /** Noir's first run is writing the first shelves (`firstPicksRun`), or For You is still asking /home whether it is. */
  writingFirstPicks?: boolean;
  /** /recommendations/home hasn't answered yet. */
  picksPending?: boolean;
}

export function forYouState({ trakt, libraryTotal, writingFirstPicks = false, picksPending = false }: ForYouInput): ForYouState {
  if (trakt === undefined) return 'loading';
  if (trakt === null || !traktLinked(trakt)) {
    if (libraryTotal === undefined) return 'loading';
    return libraryTotal === 0 ? 'first_time' : 'trakt_off';
  }
  if (trakt.sync.kind === 'initial' && trakt.sync.status !== 'idle') return 'first_sync';
  if (writingFirstPicks) return 'first_sync';
  // Right after the first import, Noir's first run may be writing: wait for /home rather than flash the personal page
  if (trakt.sync.kind === 'initial' && picksPending) return 'loading';
  return 'default';
}

/**
 * The run queued when Trakt's first import ends (`first_sync`), while there
 * are no picks yet: For You stays on the first sync with "Writing your
 * shelves" until it's done. Null otherwise (a refresh over old picks shows
 * the refreshing banner instead).
 */
export function firstPicksRun(home: RecommendationsHome | undefined): RunProgress | null {
  const run = home?.active_run;
  return home?.enabled && home.run === null && run?.kind === 'first_sync' ? run : null;
}

// ---- The first sync's steps -------------------------------------------------------
// For You says each step as a sentence ("Found Up next for 6 shows"); Settings › Trakt
// keeps its compact labels and owns the shared copy (lib/settings/trakt.ts).

type Progress = TraktSync['progress'];

interface StepCopy {
  running: string;
  done: (progress: Progress) => string;
  /** The count beside a finished step. */
  detail?: (progress: Progress) => string | null;
}

const STEP_COPY: Record<TraktSyncStep, StepCopy> = {
  history: {
    running: 'Importing your history',
    done: () => 'Imported your history',
    detail: (p) => (p.plays > 0 ? plural(p.plays, 'play') : null),
  },
  ratings: {
    running: 'Importing your ratings',
    done: () => 'Imported your ratings',
    detail: (p) => (p.ratings > 0 ? plural(p.ratings, 'rating') : null),
  },
  watchlist: {
    running: 'Importing your watchlist',
    done: () => 'Imported your watchlist',
    detail: (p) => (p.watchlist > 0 ? plural(p.watchlist, 'title') : null),
  },
  progress: {
    running: 'Finding Up next for your shows',
    done: (p) => (p.shows > 0 ? `Found Up next for ${plural(p.shows, 'show')}` : 'Checked your shows for Up next'),
  },
  details: {
    running: 'Fetching artwork and episode names',
    done: () => 'Fetched artwork and episode names',
  },
};

export interface SyncStepView {
  step: TraktSyncStep;
  label: string;
  detail: string | null;
  state: 'done' | 'active' | 'pending';
}

/**
 * The import's steps for the Steps list: done before the running one, the
 * running one active (with plays counted while it reads the history), the
 * rest pending. After an error the failed step says "Stopped here".
 */
export function firstSyncSteps(sync: TraktSync): SyncStepView[] {
  const current = sync.step === null ? -1 : IMPORT_STEPS.indexOf(sync.step);
  const failed = sync.status === 'error';
  return IMPORT_STEPS.map((step, index) => {
    const copy = STEP_COPY[step];
    if (sync.status === 'idle' || index < current) {
      return { step, label: copy.done(sync.progress), detail: copy.detail?.(sync.progress) ?? null, state: 'done' };
    }
    if (index === current) {
      const reading = step === 'history' && sync.progress.plays_total > 0
        ? `${formatNumber(sync.progress.plays)} of ${plural(sync.progress.plays_total, 'play')}`
        : null;
      return { step, label: copy.running, detail: failed ? 'Stopped here' : reading, state: failed ? 'pending' : 'active' };
    }
    return { step, label: copy.running, detail: null, state: 'pending' };
  });
}

/** "2,184 plays, 312 ratings and 41 watchlist items", or null before anything's in. */
export function importedSoFar(progress: Progress): string | null {
  const parts = [
    progress.plays > 0 ? plural(progress.plays, 'play') : null,
    progress.ratings > 0 ? plural(progress.ratings, 'rating') : null,
    progress.watchlist > 0 ? plural(progress.watchlist, 'watchlist item') : null,
  ].filter((part): part is string => part !== null);
  if (parts.length === 0) return null;
  return parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}`;
}

/** The loader's line under "First sync with Trakt"; once Noir writes the shelves, "Building your picks…". */
export function firstSyncDetail(sync: TraktSync, writing = false): string {
  const imported = importedSoFar(sync.progress);
  if (writing) return imported ? `Imported ${imported}. Building your picks…` : 'Building your picks…';
  return imported ? `Imported ${imported} so far. Up next fills in as it goes.` : 'Reading your history from Trakt. Up next fills in as it goes.';
}

/** The step after the import while Noir's first run writes (home.js: "Writing your shelves · 2 of 5"). */
export function writingShelvesStep(run: Pick<RunProgress, 'shelves_done' | 'shelves_target'>): { label: string; detail: string | null; state: 'active' } {
  return {
    label: 'Writing your shelves',
    detail: run.shelves_target > 0 ? `${run.shelves_done} of ${run.shelves_target}` : null,
    state: 'active',
  };
}
