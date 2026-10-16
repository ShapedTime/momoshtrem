/**
 * Mock scenarios for error and edge states. In the browser, set one with
 *   localStorage.setItem('msw:scenario', 'backend-down')
 * (Playwright: page.addInitScript). Handlers check it per request.
 *
 * - `backend-down`: /status says the backend can't be reached.
 * - `ai-off`: mapping jobs have no AI (`ai_available: false`; re-check answers 503).
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
  | 'me-error';

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
