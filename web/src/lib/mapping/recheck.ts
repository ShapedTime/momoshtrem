import type { Proposal, ReclassifyRequest } from '@/lib/api/contract';
import { plural } from '@/lib/title/format';
import { conflictMap } from './review';

/**
 * "Re-check with AI" (PLAN §6 "Re-check", F4; #5): which files a re-check
 * may change, and its words. The user's own decisions always survive it.
 */

export type RecheckScope = ReclassifyRequest['scope'];

/** The hint field's limit: the backend refuses longer hints (400). */
export const HINT_MAX = 500;

/**
 * Files each scope sends, as the backend picks them: episode files the user
 * hasn't decided (extras and subtitles keep the rules' reading). `unresolved`
 * is the pending ones (unmatched included) and those in a conflict; `all` is every one.
 */
export function recheckCounts(proposals: Proposal[]): Record<RecheckScope, number> {
  const conflicts = conflictMap(proposals);
  let unresolved = 0;
  let all = 0;
  for (const p of proposals) {
    if (p.user_decided || p.kind !== 'episode') continue;
    all++;
    if (p.decision === 'pending' || conflicts.has(p.id)) unresolved++;
  }
  return { unresolved, all };
}

/** The scope the panel opens on: the unresolved files, unless there are none. */
export const defaultScope = (counts: Record<RecheckScope, number>): RecheckScope => (counts.unresolved > 0 ? 'unresolved' : 'all');

/** Proposals the latest re-check changed (`updated` resets when a re-check starts). */
export const updatedCount = (proposals: Proposal[]): number => proposals.filter((p) => p.updated).length;

/** "Re-checking 3 files with your hint…"; "Re-checking the files…" before the count is known. */
export function workingTitle(files: number | null, hinted: boolean): string {
  const what = files === null ? 'the files' : plural(files, 'file');
  return `Re-checking ${what}${hinted ? ' with your hint' : ''}…`;
}

/** The second line: the hint quoted, or what Noir compares. */
export const workingDetail = (hint: string): string => (hint ? `“${hint}”` : 'Comparing names, air dates and runtimes');

/** The steps under the working line: Noir has read the names and is comparing. */
export const WORKING_STEPS: ReadonlyArray<{ label: string; state: 'done' | 'active' | 'pending' }> = [
  { label: 'Read file names', state: 'done' },
  { label: 'Comparing with TMDB air dates', state: 'active' },
  { label: 'Updating suggestions', state: 'pending' },
];

/** After the run: how many suggestions moved, or that none did. */
export function doneMessage(changed: number): { lead: string; tail: string } | null {
  if (changed === 0) return null;
  return { lead: `Updated ${plural(changed, 'suggestion')}, marked `, tail: ' below. Accept the ones that look right.' };
}

/** When nothing changed: the suggestions stand. */
export const NOTHING_CHANGED = 'Noir looked again and kept every suggestion. Pick the episodes yourself where they’re wrong.';

/** The lead before the job's `ai_error`, which says what happened ("…, so nothing changed."). The first run can set it too. */
export const AI_ERROR_LEAD = 'Noir couldn’t finish.';

/** 503: AI isn't available for this job (no key, turned off, or rules only). */
export const AI_UNAVAILABLE = 'AI isn’t available right now, so Noir can’t take another look. You can still pick the episodes yourself.';
