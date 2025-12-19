'use client';

import { useEffect } from 'react';
import Link from 'next/link';
import { Button, AlertCircleIcon } from '@/components/ui';

interface ErrorProps {
  error: Error & { digest?: string };
  reset: () => void;
}

export default function GlobalError({ error, reset }: ErrorProps) {
  useEffect(() => {
    // Log error to console for debugging
    console.error('Global error:', error);
  }, [error]);

  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <AlertCircleIcon size={48} className="mx-auto mb-4 text-red-400" />
        <h2 className="text-xl font-semibold text-white mb-2">
          Something went wrong
        </h2>
        <p className="text-text-secondary mb-6">
          An unexpected error occurred. Please try again.
        </p>
        <div className="flex gap-3 justify-center flex-wrap">
          <Button variant="primary" onClick={reset}>
            Try Again
          </Button>
          <Link href="/">
            <Button variant="secondary">
              Go Home
            </Button>
          </Link>
        </div>
      </div>
    </div>
  );
}
