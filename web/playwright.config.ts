import { defineConfig } from '@playwright/test';

const PORT = 3100;

/**
 * E2E against a dedicated dev server (own port and build dir, so it can run
 * next to `npm run dev`) with throwaway credentials. Two projects match the
 * design's check widths: a 390 px touch phone and a 1440 px desktop.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  expect: { timeout: 10_000 },
  use: {
    baseURL: `http://localhost:${PORT}`,
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'mobile',
      use: { browserName: 'chromium', viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 },
    },
    {
      name: 'desktop',
      use: { browserName: 'chromium', viewport: { width: 1440, height: 900 } },
    },
  ],
  webServer: {
    command: `npx next dev -p ${PORT}`,
    url: `http://localhost:${PORT}/login`,
    reuseExistingServer: false,
    timeout: 180_000,
    env: {
      E2E: '1',
      NEXT_DIST_DIR: '.next-e2e',
      APP_USERNAME: 'e2e',
      APP_PASSWORD: 'e2e-password',
      SESSION_SECRET: 'e2e-session-secret-that-is-at-least-32-chars',
      TMDB_API_KEY: 'e2e-dummy-key',
      // Tests sign in many times from one address; the limiter has unit tests
      AUTH_MAX_ATTEMPTS: '1000',
      AUTH_GLOBAL_MAX_FAILURES: '100000',
    },
  },
});
