/** @jest-environment node */
import { clearMemberCookie, MEMBER_COOKIE, parseMemberId, readMemberCookie, writeMemberCookie } from '@/lib/members/cookie';

/** Who's watching on this device (#239): the cookie the proxy turns into X-Momoshtrem-Member. */

const written: string[] = [];

function stubBrowser(protocol: 'http:' | 'https:', cookie = '') {
  const doc = {};
  Object.defineProperty(doc, 'cookie', {
    get: () => cookie,
    set: (value: string) => written.push(value),
    configurable: true,
  });
  Object.assign(globalThis, { document: doc, location: { protocol } });
}

afterEach(() => {
  written.length = 0;
  delete (globalThis as { document?: unknown }).document;
  delete (globalThis as { location?: unknown }).location;
});

describe('parseMemberId', () => {
  it.each<[string | null | undefined, number | null]>([
    ['1', 1],
    ['42', 42],
    ['0', null],
    ['-1', null],
    ['1.5', null],
    ['01', null],
    ['+1', null],
    ['1e3', null],
    ['abc', null],
    ['1;x', null],
    [' 1', null],
    ['', null],
    [undefined, null],
    [null, null],
    ['9007199254740991', 9007199254740991],
    ['9007199254740992', null],
    ['1'.repeat(40), null],
  ])('%p → %p', (raw, want) => {
    expect(parseMemberId(raw)).toBe(want);
  });
});

describe('readMemberCookie', () => {
  it('finds the member among other cookies', () => {
    expect(readMemberCookie(`momoshtrem_session=abc; ${MEMBER_COOKIE}=2; theme=dark`)).toBe(2);
    expect(readMemberCookie(`${MEMBER_COOKIE}=7`)).toBe(7);
  });

  it('is null when absent or malformed', () => {
    expect(readMemberCookie('momoshtrem_session=abc')).toBeNull();
    expect(readMemberCookie(`${MEMBER_COOKIE}=abc`)).toBeNull();
    expect(readMemberCookie(`${MEMBER_COOKIE}=`)).toBeNull();
    expect(readMemberCookie('')).toBeNull();
  });

  it('reads the browser’s own cookie by default, and nothing without a document', () => {
    expect(readMemberCookie()).toBeNull();
    stubBrowser('http:', `${MEMBER_COOKIE}=3`);
    expect(readMemberCookie()).toBe(3);
  });
});

describe('writing', () => {
  it('writes a plain, year-long, same-site cookie for the whole app', () => {
    stubBrowser('http:');
    writeMemberCookie(2);
    expect(written).toEqual(['momoshtrem_member=2; Path=/; Max-Age=31536000; SameSite=Lax']);
  });

  it('adds Secure on https', () => {
    stubBrowser('https:');
    writeMemberCookie(1);
    expect(written).toEqual(['momoshtrem_member=1; Path=/; Max-Age=31536000; SameSite=Lax; Secure']);
  });

  it('clears it', () => {
    stubBrowser('https:');
    clearMemberCookie();
    expect(written).toEqual(['momoshtrem_member=; Path=/; Max-Age=0; SameSite=Lax; Secure']);
  });

  it('does nothing without a document (the server)', () => {
    expect(() => writeMemberCookie(1)).not.toThrow();
    expect(() => clearMemberCookie()).not.toThrow();
  });
});
