'use client';

import { type ReactNode, useEffect, useState } from 'react';

const MOCKING = process.env.NEXT_PUBLIC_API_MOCKING === 'enabled';

let workerReady: Promise<void> | null = null;

/** Start the MSW worker once per page load (StrictMode runs effects twice). */
function startWorker(): Promise<void> {
  workerReady ??= import('@/mocks/browser').then(({ worker }) =>
    worker
      .start({ onUnhandledRequest: 'bypass', quiet: true, serviceWorker: { url: '/mockServiceWorker.js' } })
      .then(() => undefined)
  );
  return workerReady;
}

/**
 * Mock mode (`NEXT_PUBLIC_API_MOCKING=enabled npm run dev`): the browser's
 * /api/* calls are answered by MSW handlers with fixtures for every PLAN §6
 * endpoint, so screens can be built before their backend lands. Children
 * wait until the worker is ready so no request slips past it. When mocking is
 * off this renders children directly and the MSW code isn't loaded.
 */
export function MockProvider({ children }: { children: ReactNode }) {
  if (!MOCKING) return <>{children}</>;
  return <MockGate>{children}</MockGate>;
}

function MockGate({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let cancelled = false;
    startWorker().then(() => {
      if (!cancelled) setReady(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return ready ? <>{children}</> : null;
}
