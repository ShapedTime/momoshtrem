/**
 * Who's watching on this device (#239): the `momoshtrem_member` cookie holds
 * the chosen member's id. It's plain (not httpOnly): the browser writes it
 * when someone picks, and the web proxy turns it into `X-Momoshtrem-Member`.
 * No server imports, so the proxy, the client and the mocks share it.
 * Signing out leaves it alone: the next sign-in on this device continues as
 * the same person.
 */

export const MEMBER_COOKIE = 'momoshtrem_member';

/** A year: the choice is the device's, like a TV's profile. */
const MAX_AGE_SECONDS = 31_536_000;

/** A positive safe integer written plainly ("1", never "01", "1.0" or "+1"), else null. */
export function parseMemberId(value: string | null | undefined): number | null {
  if (!value || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/** The cookie's member from a `Cookie` header (the browser's own by default); null when absent or malformed. */
export function readMemberCookie(header: string = typeof document === 'undefined' ? '' : document.cookie): number | null {
  for (const part of header.split(';')) {
    const [name, ...rest] = part.trim().split('=');
    if (name === MEMBER_COOKIE) return parseMemberId(rest.join('='));
  }
  return null;
}

function secure(): string {
  return typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
}

export function writeMemberCookie(id: number): void {
  if (typeof document === 'undefined') return;
  document.cookie = `${MEMBER_COOKIE}=${id}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure()}`;
}

export function clearMemberCookie(): void {
  if (typeof document === 'undefined') return;
  document.cookie = `${MEMBER_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax${secure()}`;
}
