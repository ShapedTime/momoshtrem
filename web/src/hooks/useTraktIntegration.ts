'use client';

import { useEffect, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { libraryKeys, titleKeys, traktIntegrationQuery, traktKeys } from '@/lib/api/endpoints';

/**
 * The Trakt integration (PLAN §6 "Trakt (B5)"), polled every 3 s while a
 * device code waits or a sync runs. When a sync stops (idle, or an error
 * after a partial import), what it imported shows: /me, the title pages
 * (watch state) and the library refetch.
 */
export function useTraktIntegration() {
  const queryClient = useQueryClient();
  const query = useQuery(traktIntegrationQuery());
  const status = query.data?.sync.status;
  const previous = useRef(status);

  useEffect(() => {
    const was = previous.current;
    previous.current = status;
    if (was !== 'syncing' || status === 'syncing' || status === undefined) return;
    void queryClient.invalidateQueries({ queryKey: traktKeys.me });
    void queryClient.invalidateQueries({ queryKey: titleKeys.all });
    void queryClient.invalidateQueries({ queryKey: libraryKeys.all });
  }, [queryClient, status]);

  return query;
}
