'use client';

import { createContext, useContext, useEffect } from 'react';

export interface ShellContextValue {
  openPalette: () => void;
  /** Hero pages draw the top bar transparent over their artwork until you scroll. */
  setOverArt: (overArt: boolean) => void;
  /** Task pages with their own back link and sticky foot hide the phone bar and tab bar. */
  setTaskPage: (taskPage: boolean) => void;
}

export const ShellContext = createContext<ShellContextValue | null>(null);

export function useShell(): ShellContextValue {
  const shell = useContext(ShellContext);
  if (!shell) throw new Error('useShell must be used inside the app shell');
  return shell;
}

/**
 * Call from a page with a full-bleed hero (HANDOFF §4.2): the main area
 * slides under the top bar, which starts transparent, and the shell's phone
 * bar steps aside for the page's own. Pass `false` while the hero isn't shown
 * (an error or not-found view).
 */
export function useOverArt(enabled = true) {
  const { setOverArt } = useShell();
  useEffect(() => {
    if (!enabled) return;
    setOverArt(true);
    return () => setOverArt(false);
  }, [setOverArt, enabled]);
}

/**
 * Call from a focused task page (the mapping review; the prototype's
 * `noTabbar`): below 1024 px the shell's phone bar and tab bar step aside, so
 * the page's own back link leads and its sticky foot sits at the bottom of the
 * screen. The desktop top bar stays.
 */
export function useTaskPage(enabled = true) {
  const { setTaskPage } = useShell();
  useEffect(() => {
    if (!enabled) return;
    setTaskPage(true);
    return () => setTaskPage(false);
  }, [setTaskPage, enabled]);
}
