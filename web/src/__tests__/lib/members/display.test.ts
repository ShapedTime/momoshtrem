import type { Member, MemberColor } from '@/lib/api/contract';
import { MEMBER_COLOR_LABELS, memberInitial, memberTraktLine, nextFreeColor, possessive } from '@/lib/members/display';

/** How a household member reads in the UI (#239). */

describe('memberInitial', () => {
  it.each([
    ['Teymur', 'T'],
    ['sam', 'S'],
    ['  alex ', 'A'],
    ['émile', 'É'],
    ['Øystein', 'Ø'],
    ['🦊 Fox', '🦊'],
    ['👩‍👩‍👧 Family', '👩‍👩‍👧'],
    ['ßarah', 'ß'],
    ['   ', '?'],
  ])('%p → %p', (name, want) => {
    expect(memberInitial(name)).toBe(want);
  });
});

describe('nextFreeColor', () => {
  const colors = (...keys: MemberColor[]) => keys.map((color) => ({ color }));

  it('is the first colour nobody uses', () => {
    expect(nextFreeColor([])).toBe('rose');
    expect(nextFreeColor(colors('teal', 'rose'))).toBe('orchid');
    expect(nextFreeColor(colors('rose', 'orchid', 'periwinkle', 'teal', 'lime'))).toBe('sand');
  });

  it('falls back to the first key when all six are taken', () => {
    expect(nextFreeColor(colors('rose', 'orchid', 'periwinkle', 'teal', 'lime', 'sand'))).toBe('rose');
  });

  it('names every colour', () => {
    expect(Object.values(MEMBER_COLOR_LABELS)).toEqual(['Rose', 'Orchid', 'Periwinkle', 'Teal', 'Lime', 'Sand']);
  });
});

describe('memberTraktLine', () => {
  const trakt = (state: Member['trakt']['state'], username: string | null = null) => ({ trakt: { state, username } });

  it.each([
    [trakt('connected', 'teymur'), 'Trakt · teymur'],
    [trakt('connected'), 'Trakt · connected'],
    [trakt('reauth', 'teymur'), 'Trakt · sign in again'],
    [trakt('pending'), 'Trakt · waiting for approval'],
    [trakt('off'), 'No Trakt yet'],
  ])('%p → %p', (member, want) => {
    expect(memberTraktLine(member)).toBe(want);
  });
});

it('possessive', () => {
  expect(possessive('Sam')).toBe('Sam’s');
});
