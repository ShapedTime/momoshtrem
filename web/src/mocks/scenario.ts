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
 * - `hdrezka-partial`: assigning episodes fails for every third one (504 `timeout` per item).
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
  | 'hdrezka-partial';

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
