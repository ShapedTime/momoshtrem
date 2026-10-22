'use client';

import { type FormEvent, useId, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { UserMinus, UserPlus } from 'lucide-react';
import { Banner, Button, Input, MemberAvatar, Sheet, useConfirm, useToast } from '@/components/ui';
import type { Member, MembersList } from '@/lib/api/contract';
import { memberKeys, membersApi } from '@/lib/api/endpoints/members';
import { settingsKeys } from '@/lib/api/endpoints/settings';
import { memberTraktLine } from '@/lib/members/display';
import { leaveHousehold, useSwitchMember } from '@/lib/members/switch';
import {
  addedToast,
  alreadyRemoved,
  LAST_MEMBER,
  type MemberDraft,
  memberDraft,
  memberPatch,
  memberSheetTitle,
  NAME_ERRORS,
  nameError,
  removeConfirmCopy,
  removedToast,
  removeProblem,
  saveProblem,
  updatedToast,
} from '@/lib/settings/household';
import { ColourField } from './ColourField';

interface MemberSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Who's being edited; null adds someone. */
  member: Member | null;
  /** Everyone, by id. */
  members: Member[];
  /** Who's watching on this device: removing them leaves the household. */
  currentId: number | null;
}

/**
 * Add someone, or edit one person (#247): a live preview, the name and the
 * colour, and (editing) Remove in a quiet zone. Remove is confirmed with
 * what goes and what stays, and has no Undo: the server deletes their data
 * at once. Removing yourself forgets who's watching and goes to `/`.
 */
export function MemberSheet({ open, onOpenChange, member, members, currentId }: MemberSheetProps) {
  const formId = useId();
  const lastId = useId();
  const nameInput = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();
  const switchTo = useSwitchMember();
  const [draft, setDraft] = useState<MemberDraft>(() => memberDraft(member, members));
  // An empty name says so once Add or Save was pressed, not while the sheet opens
  const [submitted, setSubmitted] = useState(false);
  const last = members.length <= 1;

  /** The list and the usage's names (by_member) follow every change; the gate and the account menu read the list. */
  const refresh = (items: (list: Member[]) => Member[]) => {
    queryClient.setQueryData<MembersList>(memberKeys.all, (list) => list && { items: items(list.items) });
    void queryClient.invalidateQueries({ queryKey: memberKeys.all });
    void queryClient.invalidateQueries({ queryKey: settingsKeys.aiUsage });
  };

  const save = useMutation({
    mutationFn: (next: MemberDraft) =>
      member ? membersApi.update(member.id, memberPatch(member, next)) : membersApi.create({ name: next.name.trim(), color: next.color }),
    onSuccess: (saved) => {
      refresh((items) => (items.some((m) => m.id === saved.id) ? items.map((m) => (m.id === saved.id ? saved : m)) : [...items, saved]));
      onOpenChange(false);
      if (member) {
        toast({ tone: 'ready', ...updatedToast(saved.name) });
      } else {
        const words = addedToast(saved.name);
        toast({ tone: 'ready', icon: UserPlus, title: words.title, action: { label: words.action, onClick: () => switchTo(saved.id) } });
      }
    },
    onError: (error) => {
      if (saveProblem(error, draft.name, members, member).field === 'name') nameInput.current?.focus();
    },
  });

  const remove = useMutation({ mutationFn: (id: number) => membersApi.remove(id) });

  const problem = save.error ? saveProblem(save.error, draft.name, members, member) : null;
  const removeError = remove.error && !alreadyRemoved(remove.error) && member ? removeProblem(remove.error, member.name) : null;
  const typed = nameError(draft.name);
  // Too long shows as it's typed; empty once Add or Save was pressed; the server's answer over both
  const nameMessage = problem?.field === 'name' ? problem.message : typed === NAME_ERRORS.tooLong || submitted ? typed : null;
  const banner = removeError ?? (problem?.field === null ? problem.message : null);

  const change = (next: Partial<MemberDraft>) => {
    setDraft((previous) => ({ ...previous, ...next }));
    if (save.error) save.reset();
    if (remove.error) remove.reset();
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setSubmitted(true);
    if (typed) {
      nameInput.current?.focus();
      return;
    }
    if (member && Object.keys(memberPatch(member, draft)).length === 0) {
      onOpenChange(false);
      return;
    }
    save.mutate(draft);
  };

  const removeMember = async () => {
    if (!member) return;
    const self = member.id === currentId;
    if (!(await confirm({ ...removeConfirmCopy(member, self, members), tone: 'danger' }))) return;
    const removed = () => {
      if (self) {
        // Nobody is chosen on this device any more: the picker (or the only person left) follows
        leaveHousehold(() => queryClient.clear());
        return;
      }
      refresh((items) => items.filter((m) => m.id !== member.id));
      onOpenChange(false);
      toast({ tone: 'info', icon: UserMinus, ...removedToast(member.name) });
    };
    remove.mutate(member.id, {
      onSuccess: removed,
      onError: (error) => {
        if (alreadyRemoved(error)) removed();
      },
    });
  };

  const name = draft.name.trim();

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={memberSheetTitle(member)}
      description={member ? undefined : 'Once they’re added, switch to them to connect their Trakt.'}
      size="narrow"
      footer={
        <>
          <Button className="grow" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" className="grow" type="submit" form={formId} loading={save.isPending} disabled={remove.isPending}>
            {member ? 'Save' : 'Add'}
          </Button>
        </>
      }
    >
      <form id={formId} className="flex flex-col gap-5" noValidate onSubmit={submit}>
        {/* What the list will show: it repeats the fields, so screen readers skip it */}
        <div className="set-member-preview" aria-hidden="true">
          <MemberAvatar name={name} color={draft.color} size="lg" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className={name ? 't-title-3 wrap-any' : 't-title-3 text-fg-3'}>{name || 'New person'}</span>
            <span className="t-small text-fg-3">{member ? memberTraktLine(member) : 'No Trakt yet'}</span>
          </div>
        </div>

        <Input
          ref={nameInput}
          label="Name"
          value={draft.name}
          autoCapitalize="words"
          autoComplete="off"
          spellCheck={false}
          enterKeyHint="done"
          error={nameMessage ?? undefined}
          onChange={(event) => change({ name: event.target.value })}
        />

        <ColourField value={draft.color} onChange={(color) => change({ color })} />

        {banner && <Banner tone="error" role="alert" title={banner} />}

        {member && (
          <div className="set-member-remove">
            <Button
              variant="danger-quiet"
              className="-ml-3"
              loading={remove.isPending}
              disabled={last || save.isPending}
              aria-describedby={last ? lastId : undefined}
              onClick={() => void removeMember()}
            >
              <UserMinus size={17} aria-hidden="true" /> Remove {member.name}
            </Button>
            {last && (
              <p id={lastId} className="t-small text-fg-3">
                {LAST_MEMBER}
              </p>
            )}
          </div>
        )}
      </form>
    </Sheet>
  );
}
