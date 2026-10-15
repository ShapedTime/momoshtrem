/**
 * Mock scenarios for error and edge states. In the browser, set one with
 *   localStorage.setItem('msw:scenario', 'backend-down')
 * (Playwright: page.addInitScript). Handlers check it per request.
 *
 * - `backend-down`: /status says the backend can't be reached.
 * - `ai-off`: mapping jobs have no AI (`ai_available: false`; re-check answers 503).
 * - `job-failed`: every open mapping job fails when it's next read.
 */
export type MockScenario = 'backend-down' | 'ai-off' | 'job-failed';

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
