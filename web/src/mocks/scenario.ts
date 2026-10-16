/**
 * Mock scenarios for error and edge states. In the browser, set one with
 *   localStorage.setItem('msw:scenario', 'backend-down')
 * (Playwright: page.addInitScript). Handlers check it per request.
 *
 * - `backend-down`: /status says the backend can't be reached.
 * - `ai-off`: no ANTHROPIC_API_KEY. Mapping jobs have no AI (`ai_available: false`; re-check answers 503);
 *   /recommendations/home answers `configured: false` and `enabled: false` (no picks, no Ask entries), Refresh
 *   and Ask answer 503 `unavailable`, and the AI usage says `configured: false`. The AI settings still read and save.
 * - `ai-error`: a re-check ends with `ai_error` set and every proposal unchanged.
 * - `job-failed`: every open mapping job fails when it's next read.
 * - `job-hold:<step>`: working mapping jobs walk up to `<step>` (`queued`,
 *   `fetching_metadata`, `classifying` or `resolving`) and stay there, so each
 *   progress step can be asserted and screenshotted. A job that skips the step
 *   stops at its last one before it; `job-hold:resolving` holds re-checks too.
 * - `hdrezka-off`: HDRezka isn't set up (/status says so; /hdrezka/* answer 503 `unavailable`).
 * - `hdrezka-blocked`: HDRezka answers with its anti-bot page (503 `hdrezka_blocked`).
 * - `hdrezka-down`: HDRezka doesn't answer (502 `hdrezka_unavailable`).
 * - `hdrezka-premium`: every voice-over needs Premium; assigning answers 409 `premium_only`.
 * - `hdrezka-partial`: assigning episodes fails for every third one of a request (`timeout` per
 *   item), once: retrying those gets them through.
 * - `library-empty`: /library answers an empty library (the first-time state).
 * - `library-reviewed`: /library answers with nothing under mapping review.
 * - `library-error`: /library answers 500.
 * - `library-loading`: /library never answers (the loading state).
 * - `search-partial`: torrent search's last indexer times out until a Retry (`refresh=1`).
 * - `search-challenge`: RuTracker.org, 1337x and EZTV fail with Jackett's "Challenge detected but
 *   FlareSolverr is not configured", as they do on a Jackett without FlareSolverr (#106); Retry
 *   doesn't help.
 * - `search-none`: torrent search finds nothing; every indexer answers.
 * - `search-slow`: torrent search answers `complete: false` with pending indexers, and one more
 *   indexer answers each second (about three polls) until it's complete.
 * - `search-down`: torrent search answers 503 `unavailable` (Jackett isn't set up).
 * - `indexers-degraded`: /status lists a Jackett indexer timing out for 2 h and one failing for
 *   5 min, so Settings › Status says Degraded.
 * - `quality-invalid`: saving the quality profile answers 422 naming `avoid_terms`.
 * - `discover-error`: /api/discover answers 502 `tmdb_unavailable`.
 * - `discover-loading`: /api/discover never answers (the loading state).
 * - `title-search-error`: /api/title-search answers 502 `tmdb_unavailable`.
 * - `title-search-loading`: /api/title-search never answers (the loading state).
 * - `torrents-offline`: /torrents and /torrents/summary answer 502 `backend_unreachable`, as the proxy does
 *   when momoshtrem is gone (Activity keeps what it had and says so).
 * - `torrents-quiet`: /torrents lists only the idle torrents and the summary has nothing streaming (Activity's empty state).
 * - `trakt-off`: Trakt is set up but not connected: /me is empty (new episodes still list the library's). Connect works.
 * - `trakt-unconfigured`: no Trakt app or SECRET_KEY: `configured: false`, and connect answers 503 `unavailable`.
 * - `trakt-syncing`: connected, and the first import stays at its last step (`details`): history, ratings,
 *   watchlist and Up next are in.
 * - `trakt-sync-error`: connected, and the first import failed at the watchlist (history and ratings are in);
 *   Sync now picks it up from there.
 * - `trakt-reauth`: Trakt stopped accepting the tokens (`state: reauth`); the imported data is still there.
 * - `trakt-denied`: off, and the device code is denied on the second read after a connect (`connect_error: denied`).
 * - `first-time`: Trakt off and an empty library.
 * - `up-next-empty`: connected and synced, with nothing up next.
 * - `me-error`: every /me read answers 500.
 * - `trakt-pending`: off, and a device code from Connect is never approved, so the code and waiting
 *   screens hold (until Cancel or the code expires).
 * - `trakt-loading`: GET /integrations/trakt never answers (For You's loading state).
 * - `for-you-no-hero`: every /me read and /api/tmdb/trending fail and Noir has no picks yet, so For You has
 *   nothing to pick a hero from.
 * - `recs-none`: picks are on, but no run has finished yet: /recommendations/home has `run: null` and no
 *   shelves (For You keeps trending). Refresh starts the first run.
 * - `recs-first-run`: no picks yet, and the run after Trakt's first import (`first_sync`) holds at
 *   "Writing your shelves" (2 of 5). Trakt's last sync is that initial import.
 * - `recs-refreshing`: last night's picks, and a manual run holds at "Writing your shelves" (2 of 5).
 * - `recs-failed`: the last refresh failed (the earlier picks stay), and the next one fails at TMDB.
 * - `recs-off`: picks are switched off (`rec_enabled: false`): /home answers `configured: true` and
 *   `enabled: false` (no picks, but the Ask entries stay), and Refresh 409 `recs_disabled`. Ask still works;
 *   switching picks back on in Settings brings the shelves back.
 * - `ask-budget`: the month's $5 budget is spent: Ask and Refresh answer 429 `budget_exceeded` with `resets_at`
 *   (the 1st), and the usage says `budget_reached`.
 * - `ask-limit`: 30 asks today: Ask answers 429 `ask_limit` with `resets_at` (the next midnight).
 * - `ask-error`: an ask fails mid-stream after reading the history (`error`, code `upstream`); the thread keeps
 *   the failed turn.
 * - `ask-hold`: an ask stops at "Searching the web…" and never finishes, so Stop can be tested; stopping
 *   cancels the turn.
 * - `ask-working`: an ask holds like the design's working frame (screens/ask/working-*): the history and the web
 *   read, TMDB being checked, the first pick in; it never finishes.
 *
 * Without a scenario the recommendations mock is a state machine (fixtures/recommendations.ts): Refresh walks
 * a run through its steps, one per read of /home or the run, then /home serves the new picks; feedback hides
 * or marks titles at once; asks stream and grow the threads.
 */
export const HOLD_STEPS = ['queued', 'fetching_metadata', 'classifying', 'resolving'] as const;
export type HoldStep = (typeof HOLD_STEPS)[number];

export type MockScenario =
  | 'backend-down'
  | 'ai-off'
  | 'ai-error'
  | 'job-failed'
  | `job-hold:${HoldStep}`
  | 'hdrezka-off'
  | 'hdrezka-blocked'
  | 'hdrezka-down'
  | 'hdrezka-premium'
  | 'hdrezka-partial'
  | 'library-empty'
  | 'library-reviewed'
  | 'library-error'
  | 'library-loading'
  | 'search-partial'
  | 'search-challenge'
  | 'search-none'
  | 'search-slow'
  | 'search-down'
  | 'indexers-degraded'
  | 'quality-invalid'
  | 'discover-error'
  | 'discover-loading'
  | 'title-search-error'
  | 'title-search-loading'
  | 'torrents-offline'
  | 'torrents-quiet'
  | 'trakt-off'
  | 'trakt-unconfigured'
  | 'trakt-syncing'
  | 'trakt-sync-error'
  | 'trakt-reauth'
  | 'trakt-denied'
  | 'first-time'
  | 'up-next-empty'
  | 'me-error'
  | 'trakt-pending'
  | 'trakt-loading'
  | 'for-you-no-hero'
  | 'recs-none'
  | 'recs-first-run'
  | 'recs-refreshing'
  | 'recs-failed'
  | 'recs-off'
  | 'ask-budget'
  | 'ask-limit'
  | 'ask-error'
  | 'ask-hold'
  | 'ask-working';

let override: MockScenario | null = null;

/** Tests in Node, where there's no localStorage: set the scenario (null clears it). */
export function setMockScenario(value: MockScenario | null): void {
  override = value;
}

export function scenario(): MockScenario | null {
  if (override) return override;
  if (typeof window === 'undefined') return null;
  return window.localStorage.getItem('msw:scenario') as MockScenario | null;
}

/** The step `job-hold:<step>` keeps working jobs at, or null. */
export function heldStep(): HoldStep | null {
  const step = scenario()?.replace(/^job-hold:/, '');
  return HOLD_STEPS.find((known) => known === step) ?? null;
}
