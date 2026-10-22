import type { Timestamp } from './common';
import type { TraktState } from './trakt';

/**
 * Household members (PLAN §6 "Household members"; #239). One shared login, up
 * to six people. The web names the viewer with `X-Momoshtrem-Member` (the
 * proxy adds it from the `momoshtrem_member` cookie); without it the backend
 * answers as the lowest-id member.
 */

/** Identity colours, never status: the web maps each key to a `--member-*` token. */
export const MEMBER_COLORS = ['rose', 'orchid', 'periwinkle', 'teal', 'lime', 'sand'] as const;
export type MemberColor = (typeof MEMBER_COLORS)[number];
export const MAX_MEMBERS = 6;
/** Code points, once trimmed. */
export const MEMBER_NAME_MAX = 32;
/** 400 on a personal route for a member id nobody has (`/members*` and `/status` ignore it). */
export const UNKNOWN_MEMBER_CODE = 'unknown_member';

export interface Member {
  id: number;
  /** Trimmed, 1–32 characters, unique case-insensitively. */
  name: string;
  color: MemberColor;
  created_at: Timestamp;
  /** This member's Trakt in brief; GET /integrations/trakt with their header has the rest. */
  trakt: { state: TraktState; username: string | null };
}

/** GET /members: by id, never empty; the first is the default member (no header). */
export interface MembersList {
  items: Member[];
}

/**
 * POST /members. `color` is optional: the server picks the first unused key. The web always sends one.
 * 409 `member_limit` (6), 409 `member_name_taken`, 422 `validation_error` with `field` `name` or `color`.
 */
export interface MemberCreate {
  name: string;
  color?: MemberColor;
}

/** PATCH /members/{id}: any of the two. 404, 409 `member_name_taken`, 422 `validation_error`. */
export type MemberPatch = Partial<{ name: string; color: MemberColor }>;
