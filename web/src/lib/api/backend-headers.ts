import type { NextRequest } from 'next/server';
import { readMemberCookie } from '@/lib/members/cookie';

/** Header carrying the shared secret (BACKEND_API_TOKEN) to momoshtrem. */
export const TOKEN_HEADER = 'X-Momoshtrem-Token';

/** Header naming the household member a request is for (PLAN §6 "Household members"). */
export const MEMBER_HEADER = 'X-Momoshtrem-Member';

/**
 * Headers for a server-side call to momoshtrem: `base`, the shared secret
 * when configured, and the member when there is one. Without a member the
 * backend answers as the lowest-id member (or for the household).
 */
export function backendHeaders(memberId: number | null, base: Record<string, string> = {}): Record<string, string> {
  const headers = { ...base };
  // Read at call time (not module load) so runtime env changes apply
  const token = process.env.BACKEND_API_TOKEN;
  if (token) headers[TOKEN_HEADER] = token;
  if (memberId !== null) headers[MEMBER_HEADER] = String(memberId);
  return headers;
}

/**
 * The request's member, from its cookie; a malformed cookie is no member (never a malformed header).
 * Parsed as the browser's own code reads it (the first of duplicates, not decoded), never with
 * `request.cookies`, which keeps the last: two `momoshtrem_member` cookies (one at a nested Path) would
 * otherwise name different people on the server and in the page.
 */
export const requestMemberId = (request: NextRequest): number | null => readMemberCookie(request.headers.get('cookie') ?? '');
