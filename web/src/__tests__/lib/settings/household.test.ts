import { ApiError } from '@/lib/api/client';
import type { Member } from '@/lib/api/contract';
import {
  alreadyRemoved,
  canAddMember,
  HOUSEHOLD_FULL,
  householdNavSummary,
  LAST_MEMBER,
  memberDraft,
  memberPatch,
  memberSheetTitle,
  NAME_ERRORS,
  nameError,
  removeConfirmCopy,
  removeProblem,
  saveProblem,
} from '@/lib/settings/household';

/** Settings › Household (#247): the name's checks, errors in words, the remove confirm and the nav's line. */

const person = (id: number, name: string, color: Member['color'] = 'teal'): Member => ({
  id,
  name,
  color,
  created_at: '2026-01-01T00:00:00Z',
  trakt: { state: 'off', username: null },
});
const TEYMUR = person(1, 'Teymur', 'teal');
const SAM = person(2, 'Sam', 'rose');
const ALEX = person(3, 'Alex', 'periwinkle');

describe('nameError', () => {
  it('wants a name once trimmed, of at most 32 code points', () => {
    expect(nameError('')).toBe(NAME_ERRORS.empty);
    expect(nameError('   ')).toBe('Give them a name.');
    expect(nameError('Sam')).toBeNull();
    expect(nameError(`  ${'x'.repeat(32)}  `)).toBeNull();
    expect(nameError('x'.repeat(33))).toBe('Keep it to 32 characters.');
    // An emoji is one character to the server, two UTF-16 units to JS
    expect(nameError('🦊'.repeat(32))).toBeNull();
    expect(nameError('🦊'.repeat(33))).toBe(NAME_ERRORS.tooLong);
  });
});

describe('the sheet', () => {
  it('Add starts empty with the first colour nobody uses; Edit from the member', () => {
    expect(memberDraft(null, [TEYMUR, SAM])).toEqual({ name: '', color: 'orchid' });
    expect(memberDraft(SAM, [TEYMUR, SAM])).toEqual({ name: 'Sam', color: 'rose' });
    expect(memberSheetTitle(null)).toBe('Add someone');
    expect(memberSheetTitle(SAM)).toBe('Edit Sam');
  });

  it('PATCH sends only what changed, the name trimmed', () => {
    expect(memberPatch(SAM, { name: 'Sam', color: 'rose' })).toEqual({});
    expect(memberPatch(SAM, { name: '  Sam  ', color: 'rose' })).toEqual({});
    expect(memberPatch(SAM, { name: ' Samira ', color: 'rose' })).toEqual({ name: 'Samira' });
    expect(memberPatch(SAM, { name: 'Sam', color: 'lime' })).toEqual({ color: 'lime' });
  });

  it('Add is off at six', () => {
    expect(canAddMember([TEYMUR, SAM])).toBe(true);
    expect(canAddMember(Array.from({ length: 5 }, (_, i) => person(i + 1, `P${i}`)))).toBe(true);
    expect(canAddMember(Array.from({ length: 6 }, (_, i) => person(i + 1, `P${i}`)))).toBe(false);
  });
});

describe('saveProblem', () => {
  const members = [TEYMUR, SAM];

  it('a taken name shows on the field, named as the list spells it', () => {
    expect(saveProblem(new ApiError(409, 'member_name_taken', 'taken'), ' sam ', members, null)).toEqual({
      field: 'name',
      message: 'Someone here is already called Sam.',
    });
    // Someone this page's list doesn't have yet (added on another device): the name as typed
    expect(saveProblem(new ApiError(409, 'member_name_taken', 'taken'), ' Jo ', members, null).message).toBe('Someone here is already called Jo.');
  });

  it('the cap, a 422 on the name and someone gone show where they belong', () => {
    expect(saveProblem(new ApiError(409, 'member_limit', 'full'), 'Jo', members, null)).toEqual({ field: null, message: HOUSEHOLD_FULL });
    expect(saveProblem(new ApiError(422, 'validation_error', 'bad', 'name'), 'x'.repeat(40), members, null)).toEqual({
      field: 'name',
      message: NAME_ERRORS.tooLong,
    });
    expect(saveProblem(new ApiError(404, 'not_found', 'gone'), 'Samira', members, SAM)).toEqual({ field: null, message: 'Sam isn’t in the household any more.' });
  });

  it('anything else is a connection problem, in the sheet', () => {
    const network = { field: null, message: 'Couldn’t save. Check your connection and try again.' };
    expect(saveProblem(new TypeError('Failed to fetch'), 'Jo', members, null)).toEqual(network);
    expect(saveProblem(new ApiError(502, 'bad_gateway', 'down'), 'Jo', members, null)).toEqual(network);
  });
});

describe('remove', () => {
  it('says what goes and what stays', () => {
    expect(removeConfirmCopy(SAM, false, [TEYMUR, SAM])).toEqual({
      title: 'Remove Sam from the household?',
      description:
        'Sam’s Trakt connection and imported history, For You picks, Ask threads and picks feedback are deleted from momoshtrem. The library, downloads and settings stay. Nothing changes on Trakt.',
      confirmLabel: 'Remove Sam',
    });
  });

  it('removing yourself says who’s watching next', () => {
    expect(removeConfirmCopy(SAM, true, [TEYMUR, SAM, ALEX]).description).toMatch(/Nothing changes on Trakt\. You’ll pick who’s watching next\.$/);
    expect(removeConfirmCopy(SAM, true, [TEYMUR, SAM]).description).toMatch(/Nothing changes on Trakt\. You’ll continue as Teymur\.$/);
  });

  it('the last person stays; a 404 means it’s done already', () => {
    expect(removeProblem(new ApiError(409, 'last_member', 'last'), 'Sam')).toBe(LAST_MEMBER);
    expect(removeProblem(new TypeError('Failed to fetch'), 'Sam')).toBe('Couldn’t remove Sam. Check your connection and try again.');
    expect(alreadyRemoved(new ApiError(404, 'not_found', 'gone'))).toBe(true);
    expect(alreadyRemoved(new ApiError(409, 'last_member', 'last'))).toBe(false);
  });
});

describe('householdNavSummary', () => {
  it('names up to three people, then counts', () => {
    expect(householdNavSummary(undefined)).toEqual({ text: '' });
    expect(householdNavSummary([TEYMUR])).toEqual({ text: 'Teymur' });
    expect(householdNavSummary([TEYMUR, SAM])).toEqual({ text: 'Teymur · Sam' });
    expect(householdNavSummary([TEYMUR, SAM, ALEX])).toEqual({ text: 'Teymur · Sam · Alex' });
    expect(householdNavSummary([TEYMUR, SAM, ALEX, person(4, 'Mina')])).toEqual({ text: '4 people' });
  });
});
