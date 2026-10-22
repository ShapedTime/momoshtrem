'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { RefreshCw, UserPlus } from 'lucide-react';
import { useMembers } from '@/components/members/MembersGate';
import { Banner, Button, LoadingRegion, Skeleton } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import type { Member } from '@/lib/api/contract';
import { membersQuery } from '@/lib/api/endpoints/members';
import { canAddMember, HOUSEHOLD_ABSENT, HOUSEHOLD_FULL, HOUSEHOLD_NOTE } from '@/lib/settings/household';
import { MemberRow } from './household/MemberRow';
import { MemberSheet } from './household/MemberSheet';
import { SettingsCard } from './SettingsCard';

function HouseholdSkeleton() {
  return (
    <LoadingRegion label="Loading the household" className="flex flex-col gap-2">
      <Skeleton height={72} className="rounded-lg" />
      <Skeleton height={72} className="rounded-lg" />
    </LoadingRegion>
  );
}

/**
 * Settings › Household (#247; PLAN §6 "Household members"): the people who
 * share this momoshtrem, up to six. Add someone, rename or recolour them, or
 * remove them, in a sheet. Each has their own Trakt, For You, Ask and picks
 * feedback; the rest is everyone's, as the note under the list says.
 */
export function HouseholdSection() {
  // The gate's list, so a change here reaches the account menu at once
  const query = useQuery(membersQuery());
  const { current } = useMembers();
  // The sheet stays mounted while it closes; each open starts a fresh form (a new key)
  const [sheet, setSheet] = useState<{ member: Member | null; key: number } | null>(null);
  const [open, setOpen] = useState(false);

  if (query.isPending) return <HouseholdSkeleton />;

  if (!query.data) {
    if (isApiError(query.error) && query.error.status === 404) {
      return <Banner title={HOUSEHOLD_ABSENT.title}>{HOUSEHOLD_ABSENT.body}</Banner>;
    }
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load the household"
        action={
          <Button size="sm" loading={query.isFetching} onClick={() => void query.refetch()}>
            <RefreshCw size={15} aria-hidden="true" /> Try again
          </Button>
        }
      >
        {query.error?.message}
      </Banner>
    );
  }

  const members = query.data.items;
  const full = !canAddMember(members);
  const openSheet = (member: Member | null) => {
    setSheet((previous) => ({ member, key: (previous?.key ?? 0) + 1 }));
    setOpen(true);
  };

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <SettingsCard
        id="household-people"
        title="People"
        description={full ? HOUSEHOLD_FULL : undefined}
        aside={
          <Button size="sm" disabled={full} onClick={() => openSheet(null)}>
            <UserPlus size={15} aria-hidden="true" /> Add someone
          </Button>
        }
      >
        <ul className="set-members">
          {members.map((member) => (
            <MemberRow key={member.id} member={member} current={member.id === current?.id} onEdit={() => openSheet(member)} />
          ))}
        </ul>
      </SettingsCard>

      <p className="t-small px-1 text-fg-3">{HOUSEHOLD_NOTE}</p>

      {sheet && <MemberSheet key={sheet.key} open={open} onOpenChange={setOpen} member={sheet.member} members={members} currentId={current?.id ?? null} />}
    </div>
  );
}
