'use client';

import './globals.css';
import { ErrorView } from '@/components/system/ErrorView';

/** Replaces the root layout when it fails, so it brings its own <html> and styles. */
export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <html lang="en" className="dark">
      <body className="min-h-screen">
        <main id="main" className="main--bare">
          <ErrorView title="momoshtrem couldn’t start this page" digest={error.digest} onRetry={reset} />
        </main>
      </body>
    </html>
  );
}
