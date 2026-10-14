import { QueryClient } from '@tanstack/react-query';
import { isApiError } from '@/lib/api/client';

/**
 * Query defaults for the app:
 * - data is fresh for 30 s, then refetched on mount/focus;
 * - polling (`refetchInterval`) pauses while the tab is hidden;
 * - 4xx answers aren't retried (they won't change), 5xx and network errors
 *   twice with backoff; mutations never retry.
 */
export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: true,
        refetchIntervalInBackground: false,
        retry: (failureCount, error) => {
          if (isApiError(error) && error.status >= 400 && error.status < 500) return false;
          return failureCount < 2;
        },
      },
      mutations: { retry: false },
    },
  });
}
