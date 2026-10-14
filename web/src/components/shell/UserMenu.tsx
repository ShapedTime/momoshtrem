'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Command, LogOut, Settings, Tv } from 'lucide-react';
import { CatAvatar, MarkMono } from '@/components/brand';
import { Button, Kbd, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, Sheet } from '@/components/ui';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import { useShell } from './ShellContext';

/** Sign out: end the session, drop every cached query, then a full navigation. */
async function signOut(clearCache: () => void) {
  await fetch('/api/auth', { method: 'DELETE', credentials: 'same-origin' }).catch(() => undefined);
  clearCache();
  window.location.assign('/login');
}

function MenuHeader({ username }: { username: string }) {
  return (
    <div className="flex items-center gap-3 px-3 py-2">
      <CatAvatar size={36} />
      <div className="flex min-w-0 flex-col">
        <span className="truncate font-semibold text-fg-1">{username}</span>
        <span className="t-small text-fg-3">Signed in</span>
      </div>
    </div>
  );
}

/**
 * The cat menu (HANDOFF §3 Menu): Settings, Infuse setup, Command palette,
 * Log out. A popover on desktop; a sheet on phone.
 */
export function UserMenu({ username, className }: { username: string; className?: string }) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const queryClient = useQueryClient();
  const { openPalette } = useShell();
  const [sheetOpen, setSheetOpen] = useState(false);
  const clearCache = () => queryClient.clear();

  const triggerProps = {
    type: 'button' as const,
    className: `avatar-btn ${className ?? ''}`,
    'aria-label': 'Account menu: settings, log out',
  };
  const mark = <MarkMono size={20} fill="var(--text-2)" />;

  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>
          <button {...triggerProps}>{mark}</button>
        </MenuTrigger>
        <MenuContent aria-label="Account">
          <MenuHeader username={username} />
          <MenuSeparator />
          <MenuItem href="/settings" icon={<Settings size={18} aria-hidden="true" />}>
            Settings
          </MenuItem>
          <MenuItem href="/settings?section=playback" icon={<Tv size={18} aria-hidden="true" />}>
            Infuse setup
          </MenuItem>
          <MenuItem icon={<Command size={18} aria-hidden="true" />} hint="⌘K" onSelect={() => openPalette()}>
            Command palette
          </MenuItem>
          <MenuSeparator />
          <MenuItem danger icon={<LogOut size={18} aria-hidden="true" />} onSelect={() => signOut(clearCache)}>
            Log out
          </MenuItem>
        </MenuContent>
      </Menu>
    );
  }

  return (
    <>
      <button {...triggerProps} aria-haspopup="dialog" onClick={() => setSheetOpen(true)}>
        {mark}
      </button>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="Account" size="narrow">
        <MenuHeader username={username} />
        <nav aria-label="Account" className="mt-3 flex flex-col gap-1">
          <Link href="/settings" className="list-row rounded-md" onClick={() => setSheetOpen(false)}>
            <Settings size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Settings</span>
          </Link>
          <Link href="/settings?section=playback" className="list-row rounded-md" onClick={() => setSheetOpen(false)}>
            <Tv size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Infuse setup</span>
          </Link>
          <button
            type="button"
            className="list-row rounded-md text-left"
            onClick={() => {
              setSheetOpen(false);
              openPalette();
            }}
          >
            <Command size={20} className="text-fg-3" aria-hidden="true" />
            <span className="grow font-medium">Command palette</span>
            <Kbd>⌘K</Kbd>
          </button>
        </nav>
        <Button variant="danger-quiet" block className="mt-4" onClick={() => signOut(clearCache)}>
          <LogOut size={18} aria-hidden="true" /> Log out
        </Button>
      </Sheet>
    </>
  );
}
