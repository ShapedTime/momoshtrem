'use client';

import { useQuery } from '@tanstack/react-query';
import { recommendationsHomeQuery } from '@/lib/api/endpoints';

/**
 * Whether Ask momoshtrem's entry points show (PLAN §6): only with an
 * Anthropic key, `/recommendations/home`'s `configured`. Picks switched off
 * (`enabled: false`) don't hide them. False while loading and on an error,
 * so an entry never flashes in and out. This observer never polls: For You
 * polls /home while a run is active, not every page with a top bar.
 */
export function useAskAvailable(): boolean {
  const configured = useQuery({ ...recommendationsHomeQuery(), refetchInterval: false, select: (home) => home.configured });
  return configured.data === true;
}
