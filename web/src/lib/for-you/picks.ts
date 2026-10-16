import type { RunProgress, RunSummary } from '@/lib/api/contract';
import { stepLabel } from '@/lib/recommendations/steps';

/**
 * The runs behind For You's picks (#155): the refreshing banner's line, the
 * note when a refresh failed, and the picks footer.
 */

/**
 * The refreshing banner's line (home.js: "Reading your history · searching
 * what's new · 2 of 5 shelves done"): the step under way with its tally, then
 * the shelves written so far. The writing step's own tally ("2 of 5 shelves")
 * is the shelf count, so it isn't said twice.
 */
export function refreshSummary(run: Pick<RunProgress, 'status' | 'steps' | 'shelves_done' | 'shelves_target'>): string {
  const current = run.steps.find((step) => step.state === 'active') ?? run.steps.at(-1);
  if (run.status === 'queued' || !current) return 'Starting in a moment';
  const shelves = run.shelves_target > 0 ? `${run.shelves_done} of ${run.shelves_target} shelves done` : null;
  const detail = current.step === 'write' ? null : current.detail;
  return [stepLabel(current.step, 'active', 'run'), detail, shelves].filter(Boolean).join(' · ');
}

/**
 * The newest run, when it failed after the picks on screen were written: the
 * old shelves stay, with a quiet note. `/home` carries only succeeded runs,
 * so this reads `/runs?limit=1`.
 */
export function failedRefresh(newest: RunSummary | undefined, picks: RunSummary | null): RunSummary | null {
  if (!newest || newest.status !== 'failed') return null;
  return picks === null || newest.id > picks.id ? newest : null;
}

const pad = (n: number) => String(n).padStart(2, '0');
const MONTH_DAY = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const dayStart = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();

/**
 * "Picks written by Noir at 03:00 from your Trakt history." When the run
 * started (local time, 24-hour as the design has it); "yesterday at 03:00" or
 * "on Oct 12 at 03:00" when it wasn't today.
 */
export function picksFootLine(run: Pick<RunSummary, 'started_at' | 'finished_at'>, now: Date): string {
  const at = run.started_at ?? run.finished_at;
  if (!at) return 'Picks written by Noir from your Trakt history.';
  const date = new Date(at);
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  const days = Math.round((dayStart(now) - dayStart(date)) / 86_400_000);
  const when = days <= 0 ? `at ${time}` : days === 1 ? `yesterday at ${time}` : `on ${MONTH_DAY.format(date)} at ${time}`;
  return `Picks written by Noir ${when} from your Trakt history.`;
}
