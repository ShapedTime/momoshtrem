'use client';

import { useCallback, useEffect, useRef } from 'react';
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

  // Closed some other way (Back): a later deep link to the same sheet must not go Back
  const previous = useRef(current);
  useEffect(() => {
    if (previous.current && !current) pushedSheet = null;
    previous.current = current;
  }, [current]);

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

  /**
   * Another sheet on the same params (a sibling tab: `torrent` ⇄ `hdrezka`).
   * It replaces the history entry, so Back still closes the sheet.
   */
  const swap = useCallback(
    (name: string) => {
      const next = new URLSearchParams(searchParams.toString());
      next.set('sheet', name);
      if (pushedSheet !== null && pushedSheet === current) pushedSheet = name;
      router.replace(`${pathname}?${next}`, { scroll: false });
    },
    [current, pathname, router, searchParams]
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

  return { current, open, swap, close, params: searchParams };
}
