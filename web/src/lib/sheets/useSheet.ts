'use client';

import { useCallback } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

/** The sheet opened by this tab (vs. arriving on a deep link), so closing can go Back. */
let pushedSheet: string | null = null;

/**
 * URL-driven sheets (HANDOFF §4.1): `?sheet=<name>` opens a registered sheet,
 * so Back closes it and every sheet state is deep-linkable. Extra params
 * (e.g. `se=S01E29`) ride along.
 */
export function useSheet() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const current = searchParams.get('sheet');

  const open = useCallback(
    (name: string, params: Record<string, string> = {}) => {
      const next = new URLSearchParams(searchParams.toString());
      next.set('sheet', name);
      for (const [key, value] of Object.entries(params)) next.set(key, value);
      pushedSheet = name;
      router.push(`${pathname}?${next}`, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  const close = useCallback(
    (extraParams: string[] = []) => {
      if (pushedSheet && pushedSheet === current) {
        pushedSheet = null;
        router.back();
        return;
      }
      const next = new URLSearchParams(searchParams.toString());
      next.delete('sheet');
      for (const key of extraParams) next.delete(key);
      const query = next.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [current, pathname, router, searchParams]
  );

  return { current, open, close, params: searchParams };
}
