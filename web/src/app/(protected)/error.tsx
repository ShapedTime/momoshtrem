'use client';

import { useEffect } from 'react';
import { ErrorView } from '@/components/system/ErrorView';

export default function ProtectedError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error('Page error:', error);
  }, [error]);
  return <ErrorView digest={error.digest} onRetry={reset} />;
}
