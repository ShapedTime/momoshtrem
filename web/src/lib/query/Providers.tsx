'use client';

import { type ReactNode, useState } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { MockProvider } from '@/components/dev/MockProvider';
import { ToastProvider } from '@/components/ui';
import { makeQueryClient } from './client';

/** Client-side app providers: mock mode (dev), TanStack Query and toasts. */
export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(makeQueryClient);
  return (
    <MockProvider>
      <QueryClientProvider client={queryClient}>
        <ToastProvider>{children}</ToastProvider>
      </QueryClientProvider>
    </MockProvider>
  );
}
