import { type Member, MEMBER_COLORS, type MemberColor } from '@/lib/api/contract';

/** How a household member reads in the UI (#239): initials, colour names and their Trakt in a line. */

/**
 * The first grapheme of the trimmed name, upper-cased for the locale ("émile" → "É", "🦊 Fox" → "🦊").
 * One that upper-cases to more characters ("ß" → "SS") stays as it is: an avatar shows one.
 */
export function memberInitial(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return '?';
  const first =
    typeof Intl !== 'undefined' && 'Segmenter' in Intl
      ? (new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(trimmed)[Symbol.iterator]().next().value?.segment ?? '')
      : (Array.from(trimmed)[0] ?? '');
  const upper = first.toLocaleUpperCase();
  return Array.from(upper).length === Array.from(first).length ? upper : first;
}

export const MEMBER_COLOR_LABELS: Record<MemberColor, string> = {
  rose: 'Rose',
  orchid: 'Orchid',
  periwinkle: 'Periwinkle',
  teal: 'Teal',
  lime: 'Lime',
  sand: 'Sand',
};

/** The first colour nobody uses (the server's default for POST without one); the first key when all are taken. */
export function nextFreeColor(members: ReadonlyArray<Pick<Member, 'color'>>): MemberColor {
  const used = new Set(members.map((m) => m.color));
  return MEMBER_COLORS.find((color) => !used.has(color)) ?? MEMBER_COLORS[0];
}

/** Under a member's name: "Trakt · teymur", "Trakt · sign in again", "Trakt · waiting for approval", "No Trakt yet". */
export function memberTraktLine(member: Pick<Member, 'trakt'>): string {
  switch (member.trakt.state) {
    case 'connected':
      return `Trakt · ${member.trakt.username ?? 'connected'}`;
    case 'reauth':
      return 'Trakt · sign in again';
    case 'pending':
      return 'Trakt · waiting for approval';
    default:
      return 'No Trakt yet';
  }
}

/** "Teymur’s", for "This is Teymur’s Trakt." */
export const possessive = (name: string): string => `${name}’s`;
