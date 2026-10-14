'use client';

import { createContext, useContext, useEffect } from 'react';

export interface ShellContextValue {
  openPalette: (query?: string) => void;
  /** Hero pages draw the top bar transparent over their artwork until you scroll. */
  setOverArt: (overArt: boolean) => void;
}

export const ShellContext = createContext<ShellContextValue | null>(null);

export function useShell(): ShellContextValue {
  const shell = useContext(ShellContext);
  if (!shell) throw new Error('useShell must be used inside the app shell');
  return shell;
}

/** Call from a page with a full-bleed hero (HANDOFF §4.2). */
export function useOverArt() {
  const { setOverArt } = useShell();
  useEffect(() => {
    setOverArt(true);
    return () => setOverArt(false);
  }, [setOverArt]);
}
