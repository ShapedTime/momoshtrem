'use client';

import { useEffect } from 'react';
import { ErrorView } from '@/components/system/ErrorView';

export default function RootError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error('Page error:', error);
  }, [error]);
  return (
    <main id="main" className="main--bare">
      <ErrorView digest={error.digest} onRetry={reset} />
    </main>
  );
}
