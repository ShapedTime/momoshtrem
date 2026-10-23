'use client';

import Link from 'next/link';
import { useState } from 'react';
import { Command, LogOut, Settings, Tv, Users } from 'lucide-react';
import { CatAvatar, MarkMono } from '@/components/brand';
import { useMembers } from '@/components/members/MembersGate';
import { Button, Kbd, MemberAvatar, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, Sheet, Spinner } from '@/components/ui';
import { useHydrated } from '@/hooks/useHydrated';
import { DESKTOP_QUERY, useMediaQuery } from '@/hooks/useMediaQuery';
import type { Member } from '@/lib/api/contract';
import { memberTraktLine } from '@/lib/members/display';
import { useSwitchMember } from '@/lib/members/switch';
import { useShell } from './ShellContext';
import { useSignOut } from './signOut';

/** Who's signed in: the member and their Trakt, or (without household members) the app. */
export function AccountMenuHeader({ member }: { member: Member | null }) {
  return (
    <div className="flex items-center gap-3 px-3 py-2">
      {member ? <MemberAvatar name={member.name} color={member.color} size="md" /> : <CatAvatar size={36} />}
      <div className="flex min-w-0 flex-col">
        <span className="truncate font-semibold text-fg-1">{member ? member.name : 'momoshtrem'}</span>
        <span className="t-small truncate text-fg-3">{member ? memberTraktLine(member) : 'Signed in'}</span>
      </div>
    </div>
  );
}

/** "Account menu, Teymur: switch, settings, log out"; "Account menu: settings, log out" without a member. */
export function accountMenuLabel(member: Member | null, canSwitch: boolean): string {
  const actions = `${canSwitch ? 'switch, ' : ''}settings, log out`;
  return member ? `Account menu, ${member.name}: ${actions}` : `Account menu: ${actions}`;
}

/** Settings › Household (#247). */
const HOUSEHOLD_HREF = '/settings?section=household';

/**
 * The account menu (HANDOFF §3 Menu): who's watching, "Switch to …" for
 * everyone else and Manage household (#239), then Settings, Infuse setup,
 * Command palette and Log out. A popover on desktop; a sheet on phone.
 * Switching goes to `/` as the other person, with every cached query dropped.
 */
export function UserMenu({ className }: { className?: string }) {
  const desktop = useMediaQuery(DESKTOP_QUERY);
  const signOut = useSignOut();
  const switchTo = useSwitchMember();
  const { openPalette } = useShell();
  const [sheetOpen, setSheetOpen] = useState(false);
  const [switching, setSwitching] = useState<number | null>(null);
  // The household comes from a client query: render it only after hydration, so the first render matches the server's (#148)
  const hydrated = useHydrated();
  const members = useMembers();
  const member = hydrated ? members.current : null;
  const others = hydrated && member ? members.others : [];

  const switchMember = (id: number) => {
    if (switching !== null) return;
    setSwitching(id);
    switchTo(id);
  };

  const triggerProps = {
    type: 'button' as const,
    className: `avatar-btn ${member ? 'has-member' : ''} ${className ?? ''}`,
    'aria-label': accountMenuLabel(member, others.length > 0),
  };
  const face = member ? <MemberAvatar name={member.name} color={member.color} size="md" /> : <MarkMono size={20} fill="var(--text-2)" />;

  if (desktop) {
    return (
      <Menu>
        <MenuTrigger asChild>
          <button {...triggerProps}>{face}</button>
        </MenuTrigger>
        <MenuContent aria-label="Account" className="min-w-[260px]">
          <AccountMenuHeader member={member} />
          <MenuSeparator />
          {member && (
            <>
              {others.map((other) => (
                <MenuItem key={other.id} icon={<MemberAvatar name={other.name} color={other.color} size="xs" />} onSelect={() => switchMember(other.id)}>
                  Switch to {other.name}
                </MenuItem>
              ))}
              <MenuItem href={HOUSEHOLD_HREF} icon={<Users size={18} aria-hidden="true" />}>
                Manage household
              </MenuItem>
              <MenuSeparator />
            </>
          )}
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
          <MenuItem danger icon={<LogOut size={18} aria-hidden="true" />} onSelect={() => signOut()}>
            Log out
          </MenuItem>
        </MenuContent>
      </Menu>
    );
  }

  return (
    <>
      <button {...triggerProps} aria-haspopup="dialog" onClick={() => setSheetOpen(true)}>
        {face}
      </button>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen} title="Account" size="narrow">
        <AccountMenuHeader member={member} />
        <nav aria-label="Account" className="mt-3 flex flex-col gap-1">
          {others.map((other) => (
            <button
              key={other.id}
              type="button"
              className="list-row rounded-md text-left"
              aria-busy={switching === other.id || undefined}
              disabled={switching !== null && switching !== other.id}
              onClick={() => switchMember(other.id)}
            >
              <MemberAvatar name={other.name} color={other.color} size="sm" />
              <span className="grow font-medium">Switch to {other.name}</span>
              {switching === other.id && <Spinner size={18} className="text-fg-3" />}
            </button>
          ))}
          {member && (
            <>
              <Link href={HOUSEHOLD_HREF} className="list-row rounded-md" onClick={() => setSheetOpen(false)}>
                <Users size={20} className="text-fg-3" aria-hidden="true" />
                <span className="grow font-medium">Manage household</span>
              </Link>
              <hr className="my-1 border-line-1" />
            </>
          )}
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
        <Button variant="danger-quiet" block className="mt-4" onClick={() => signOut()}>
          <LogOut size={18} aria-hidden="true" /> Log out
        </Button>
      </Sheet>
    </>
  );
}
