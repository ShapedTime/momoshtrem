import type { TraktIntegration, TraktSync, TraktSyncStep } from '@/lib/api/contract';
import { formatNumber, plural } from '@/lib/title/format';

/**
 * Which For You the page shows (#135), first match:
 * - `loading`: the integration (or, without Trakt, the library) isn't known yet;
 * - `first_time`: no Trakt and an empty library: "Evening. I'm Noir." and three steps;
 * - `trakt_off`: no Trakt: a trending hero, "Make this page yours" and trending rails;
 * - `first_sync`: the first import after connecting is running, or stopped with an error;
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
}

export function forYouState({ trakt, libraryTotal }: ForYouInput): ForYouState {
  if (trakt === undefined) return 'loading';
  if (trakt === null || !traktLinked(trakt)) {
    if (libraryTotal === undefined) return 'loading';
    return libraryTotal === 0 ? 'first_time' : 'trakt_off';
  }
  if (trakt.sync.kind === 'initial' && trakt.sync.status !== 'idle') return 'first_sync';
  return 'default';
}

// ---- The first sync's steps -------------------------------------------------------

export const SYNC_STEP_ORDER: readonly TraktSyncStep[] = ['history', 'ratings', 'watchlist', 'progress', 'details'];

type Progress = TraktSync['progress'];

interface StepCopy {
  running: string;
  done: (progress: Progress) => string;
  /** The count beside a finished step. */
  detail?: (progress: Progress) => string | null;
  /** "stopped at …" */
  noun: string;
}

const STEP_COPY: Record<TraktSyncStep, StepCopy> = {
  history: {
    running: 'Importing your history',
    done: () => 'Imported your history',
    detail: (p) => (p.plays > 0 ? plural(p.plays, 'play') : null),
    noun: 'your history',
  },
  ratings: {
    running: 'Importing your ratings',
    done: () => 'Imported your ratings',
    detail: (p) => (p.ratings > 0 ? plural(p.ratings, 'rating') : null),
    noun: 'your ratings',
  },
  watchlist: {
    running: 'Importing your watchlist',
    done: () => 'Imported your watchlist',
    detail: (p) => (p.watchlist > 0 ? plural(p.watchlist, 'title') : null),
    noun: 'your watchlist',
  },
  progress: {
    running: 'Finding Up next for your shows',
    done: (p) => (p.shows > 0 ? `Found Up next for ${plural(p.shows, 'show')}` : 'Checked your shows for Up next'),
    noun: 'Up next',
  },
  details: {
    running: 'Fetching artwork and episode names',
    done: () => 'Fetched artwork and episode names',
    noun: 'the artwork',
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
  const current = sync.step === null ? -1 : SYNC_STEP_ORDER.indexOf(sync.step);
  const failed = sync.status === 'error';
  return SYNC_STEP_ORDER.map((step, index) => {
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

/** The loader's line under "First sync with Trakt". */
export function firstSyncDetail(sync: TraktSync): string {
  const imported = importedSoFar(sync.progress);
  return imported ? `Imported ${imported} so far. Up next fills in as it goes.` : 'Reading your history from Trakt. Up next fills in as it goes.';
}

/** "The first sync stopped at your watchlist". */
export function firstSyncStoppedTitle(sync: TraktSync): string {
  return sync.step ? `The first sync stopped at ${STEP_COPY[sync.step].noun}` : 'The first sync stopped';
}
