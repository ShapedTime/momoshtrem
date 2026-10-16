'use client';

import { useEffect, useState } from 'react';

/** The time, ticking every `ms`: a clock for countdowns and "4 min ago" (data polling uses TanStack's refetchInterval). */
export function useNow(ms: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), ms);
    return () => window.clearInterval(tick);
  }, [ms]);
  return now;
}
