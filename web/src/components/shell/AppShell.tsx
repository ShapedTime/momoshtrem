'use client';

import { type ReactNode, Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { ConnectionBanner } from '@/components/system/ConnectionBanner';
import { CommandPalette } from './CommandPalette';
import { PageTransition } from './PageTransition';
import { PhoneBar } from './PhoneBar';
import { SheetHost } from './SheetHost';
import { ShellContext } from './ShellContext';
import { TabBar } from './TabBar';
import { TopBar } from './TopBar';
import { useActiveTorrentCount } from './useActiveTorrentCount';

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
}

/**
 * The app frame (HANDOFF §2–§4): glass top bar on desktop, phone bar + tab
 * bar on phone and tablet, ⌘K palette, URL-driven sheets and page
 * transitions around every protected page.
 */
export function AppShell({ username, children }: { username: string; children: ReactNode }) {
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [overArt, setOverArt] = useState(false);
  const activeCount = useActiveTorrentCount();

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const shell = useMemo(() => ({ openPalette, setOverArt }), [openPalette]);

  // ⌘K / Ctrl K anywhere; "/" when not typing
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setPaletteOpen((open) => !open);
      } else if (event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey && !isTyping(event.target)) {
        event.preventDefault();
        setPaletteOpen(true);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  return (
    <ShellContext.Provider value={shell}>
      <a href="#main" className="skip">
        Skip to content
      </a>
      <TopBar username={username} overArt={overArt} activeCount={activeCount} />
      <PhoneBar username={username} />
      <main id="main" tabIndex={-1} className="main outline-none">
        <ConnectionBanner />
        <PageTransition>{children}</PageTransition>
      </main>
      <TabBar activeCount={activeCount} />
      <Suspense fallback={null}>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
        <SheetHost />
      </Suspense>
    </ShellContext.Provider>
  );
}
