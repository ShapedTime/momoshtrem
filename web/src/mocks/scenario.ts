/**
 * Mock scenarios for error and edge states. In the browser, set one with
 *   localStorage.setItem('msw:scenario', 'backend-down')
 * (Playwright: page.addInitScript). Handlers check it per request.
 */
export type MockScenario = 'backend-down';

export function scenario(): MockScenario | null {
  if (typeof window === 'undefined') return null;
  return window.localStorage.getItem('msw:scenario') as MockScenario | null;
}
