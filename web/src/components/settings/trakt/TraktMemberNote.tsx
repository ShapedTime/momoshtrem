'use client';

import { useState } from 'react';
import { useMembers } from '@/components/members/MembersGate';
import { Button, MemberAvatar } from '@/components/ui';
import { useHydrated } from '@/hooks/useHydrated';
import { possessive } from '@/lib/members/display';
import { currentUrl, useSwitchMember } from '@/lib/members/switch';

/**
 * Settings › Trakt with two or more people (#239): whose Trakt this is, and
 * (with exactly one other person) a way to switch to theirs. Switching stays
 * here, since connecting someone's account means being them.
 */
export function TraktMemberNote() {
  const hydrated = useHydrated();
  const { current, members, others } = useMembers();
  const switchTo = useSwitchMember();
  const [switching, setSwitching] = useState(false);
  if (!hydrated || !current || members.length < 2) return null;
  const other = others.length === 1 ? others[0] : null;

  return (
    <div className="flex items-center gap-3 rounded-lg border border-line-1 bg-surface-1 px-4 py-3">
      <MemberAvatar name={current.name} color={current.color} size="sm" />
      {/* At 390 the button wraps under the text */}
      <div className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <p className="min-w-0 basis-56 grow text-fg-2">
          <strong className="font-semibold text-fg-1">This is {possessive(current.name)} Trakt.</strong> Everyone in the household connects their own.
        </p>
        {other && (
          <Button
            variant="ghost"
            size="sm"
            className="-ml-3 sm:ml-0"
            loading={switching}
            onClick={() => {
              setSwitching(true);
              switchTo(other.id, currentUrl());
            }}
          >
            Switch to {other.name}
          </Button>
        )}
      </div>
    </div>
  );
}
