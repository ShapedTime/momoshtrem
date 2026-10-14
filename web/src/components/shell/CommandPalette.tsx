'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { Command } from 'cmdk';
import { useQueryClient } from '@tanstack/react-query';
import { CornerDownLeft, LogOut, Search, Settings, Tv, type LucideIcon } from 'lucide-react';
import { Kbd } from '@/components/ui';
import { useRestoreFocus } from '@/hooks/useRestoreFocus';
import { NAV } from '@/lib/nav';
import { useSheet } from '@/lib/sheets/useSheet';

interface PaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialQuery?: string;
}

function Item({ icon: Icon, label, hint, onSelect, value }: { icon: LucideIcon; label: string; hint?: string; onSelect: () => void; value?: string }) {
  return (
    <Command.Item className="p-item" value={value ?? label} onSelect={onSelect}>
      <span className="ic" aria-hidden="true">
        <Icon size={16} />
      </span>
      <span className="grow">
        <span className="font-medium text-fg-1">{label}</span>
        {hint && <span>{hint}</span>}
      </span>
      <CornerDownLeft size={16} className="enter" aria-hidden="true" />
    </Command.Item>
  );
}

/**
 * ⌘K / Ctrl K / "/" (HANDOFF §4.5): jump anywhere and run actions. Title
 * and library results arrive with Discover & search (#7); "Ask momoshtrem"
 * with #9.
 */
export function CommandPalette({ open, onOpenChange, initialQuery = '' }: PaletteProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const sheets = useSheet();
  const focus = useRestoreFocus();
  const [query, setQuery] = useState(initialQuery);

  const run = (action: () => void) => {
    onOpenChange(false);
    setQuery('');
    action();
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim z-(--z-palette)" />
        <Dialog.Content className="palette" aria-describedby={undefined} {...focus}>
          <Dialog.Title className="sr-only">Search or jump to</Dialog.Title>
          <Command label="Search or jump to" loop>
            <div className="p-input">
              <Search size={20} className="text-fg-3" aria-hidden="true" />
              <Command.Input value={query} onValueChange={setQuery} placeholder="Search or jump to…" autoFocus />
              <Dialog.Close className="btn btn-ghost btn-sm lg:hidden">Cancel</Dialog.Close>
            </div>
            <Command.List className="p-body">
              <Command.Empty className="px-3 py-6 text-center text-fg-3">Nothing matches “{query}”.</Command.Empty>
              <Command.Group heading="Go to">
                {NAV.map((item) => (
                  <Item key={item.id} icon={item.icon} label={item.label} onSelect={() => run(() => router.push(item.href))} />
                ))}
                <Item icon={Settings} label="Settings" onSelect={() => run(() => router.push('/settings'))} />
              </Command.Group>
              <Command.Group heading="Actions">
                <Item icon={Tv} label="How to play in Infuse" hint="WebDAV address and setup" onSelect={() => run(() => sheets.open('infuse-setup'))} />
                <Item
                  icon={LogOut}
                  label="Log out"
                  onSelect={() =>
                    run(async () => {
                      await fetch('/api/auth', { method: 'DELETE', credentials: 'same-origin' }).catch(() => undefined);
                      queryClient.clear();
                      window.location.assign('/login');
                    })
                  }
                />
              </Command.Group>
              {query.trim() && (
                <Command.Group heading="Search">
                  <Item
                    icon={Search}
                    value={`search-all ${query}`}
                    label={`Search all results for “${query.trim()}”`}
                    onSelect={() => run(() => router.push(`/search?q=${encodeURIComponent(query.trim())}`))}
                  />
                </Command.Group>
              )}
            </Command.List>
            <div className="p-foot" aria-hidden="true">
              <span>
                <Kbd>↑</Kbd>
                <Kbd>↓</Kbd> Navigate
              </span>
              <span>
                <Kbd>↵</Kbd> Open
              </span>
              <span>
                <Kbd>esc</Kbd> Close
              </span>
              <span className="ml-auto">
                <Kbd>⌘K</Kbd> anywhere
              </span>
            </div>
          </Command>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
