import { isApiError } from '@/lib/api/client';
import { MAX_MEMBERS, type Member, type MemberColor, MEMBER_NAME_MAX, type MemberPatch } from '@/lib/api/contract';
import { nextFreeColor, possessive } from '@/lib/members/display';

/**
 * Settings › Household (#247; PLAN §6 "Household members"): the name's
 * checks, what each error says and where it shows, the remove confirm, the
 * toasts and the nav's line. Pure, so Jest covers it;
 * components/settings/HouseholdSection renders it.
 */

export const HOUSEHOLD_NOTE =
  'Everyone shares the library, downloads, Activity and settings. Each person has their own Trakt, For You, Ask threads and picks feedback.';
/** Add is off at six (`member_limit`). */
export const HOUSEHOLD_FULL = 'A household can have up to six people.';
/** Remove is off for the last one (`last_member`). */
export const LAST_MEMBER = 'The household needs at least one person.';
/** GET /members is 404: a backend from before household members. */
export const HOUSEHOLD_ABSENT = {
  title: 'Households need a newer momoshtrem',
  body: 'Update the server to add people with their own Trakt and For You.',
} as const;

export const NAME_ERRORS = {
  empty: 'Give them a name.',
  tooLong: `Keep it to ${MEMBER_NAME_MAX} characters.`,
} as const;

const SAVE_FAILED = 'Couldn’t save. Check your connection and try again.';

export const canAddMember = (members: readonly unknown[]): boolean => members.length < MAX_MEMBERS;

/** The name as the server keeps it: trimmed, and counted in code points (an emoji is one). */
export function nameError(name: string): string | null {
  const length = Array.from(name.trim()).length;
  if (length === 0) return NAME_ERRORS.empty;
  if (length > MEMBER_NAME_MAX) return NAME_ERRORS.tooLong;
  return null;
}

// ---- The sheet's draft ----------------------------------------------------------------------

export interface MemberDraft {
  name: string;
  color: MemberColor;
}

/** Edit starts from the member; Add from an empty name and the first colour nobody uses (the server's default too). */
export function memberDraft(member: Pick<Member, 'name' | 'color'> | null, members: ReadonlyArray<Pick<Member, 'color'>>): MemberDraft {
  return member ? { name: member.name, color: member.color } : { name: '', color: nextFreeColor(members) };
}

/** PATCH /members/{id}: only what changed (fields left out keep their value), the name trimmed. */
export function memberPatch(member: Pick<Member, 'name' | 'color'>, draft: MemberDraft): MemberPatch {
  const patch: MemberPatch = {};
  const name = draft.name.trim();
  if (name !== member.name) patch.name = name;
  if (draft.color !== member.color) patch.color = draft.color;
  return patch;
}

export const memberSheetTitle = (member: Pick<Member, 'name'> | null): string => (member ? `Edit ${member.name}` : 'Add someone');

// ---- What a failure says -------------------------------------------------------------------

/** Where a failed save shows: on the name field, or in the sheet's banner. */
export interface SaveProblem {
  field: 'name' | null;
  message: string;
}

/**
 * A failed POST or PATCH, in words. `name` is what was typed; `editing` the
 * member being edited, if any. "Someone here is already called Sam." names
 * the person who has it, as the list spells it.
 */
export function saveProblem(error: unknown, name: string, members: ReadonlyArray<Pick<Member, 'id' | 'name'>>, editing: Pick<Member, 'name'> | null): SaveProblem {
  if (isApiError(error)) {
    const typed = name.trim();
    if (error.code === 'member_name_taken') {
      const holder = members.find((m) => m.name.toLocaleLowerCase() === typed.toLocaleLowerCase());
      return { field: 'name', message: `Someone here is already called ${holder?.name ?? typed}.` };
    }
    if (error.code === 'member_limit') return { field: null, message: HOUSEHOLD_FULL };
    // The checks here match the server's, so this is a name it counts differently
    if (error.status === 422 && error.field === 'name') return { field: 'name', message: nameError(name) ?? NAME_ERRORS.tooLong };
    if (error.status === 404 && editing) return { field: null, message: `${editing.name} isn’t in the household any more.` };
  }
  return { field: null, message: SAVE_FAILED };
}

/** DELETE answered 404: they were removed already (on another device), which is what was asked. */
export const alreadyRemoved = (error: unknown): boolean => isApiError(error) && error.status === 404;

/** A failed DELETE, in words. */
export function removeProblem(error: unknown, name: string): string {
  if (isApiError(error) && error.code === 'last_member') return LAST_MEMBER;
  return `Couldn’t remove ${name}. Check your connection and try again.`;
}

// ---- Remove ------------------------------------------------------------------------------

/**
 * The remove confirm: what goes and what stays. There's no Undo: the server
 * deletes their data at once. Removing yourself says who's watching next:
 * the picker, or the only person left.
 */
export function removeConfirmCopy(
  member: Pick<Member, 'id' | 'name'>,
  self: boolean,
  members: ReadonlyArray<Pick<Member, 'id' | 'name'>>
): { title: string; description: string; confirmLabel: string } {
  const remaining = members.filter((m) => m.id !== member.id);
  const next = !self ? '' : remaining.length === 1 ? ` You’ll continue as ${remaining[0].name}.` : ' You’ll pick who’s watching next.';
  return {
    title: `Remove ${member.name} from the household?`,
    description: `${possessive(member.name)} Trakt connection and imported history, For You picks, Ask threads and picks feedback are deleted from momoshtrem. The library, downloads and settings stay. Nothing changes on Trakt.${next}`,
    confirmLabel: `Remove ${member.name}`,
  };
}

// ---- Toasts and the nav ----------------------------------------------------------------------

export const addedToast = (name: string) => ({ title: `${name} was added`, action: `Switch to ${name}` });
export const updatedToast = (name: string) => ({ title: `${name} was updated` });
export const removedToast = (name: string) => ({ title: `${name} was removed from the household` });

/** The settings nav's line: "Teymur · Sam", up to three names; "4 people" from four. */
export function householdNavSummary(members: ReadonlyArray<Pick<Member, 'name'>> | undefined): { text: string } {
  if (!members || members.length === 0) return { text: '' };
  if (members.length >= 4) return { text: `${members.length} people` };
  return { text: members.map((m) => m.name).join(' · ') };
}
