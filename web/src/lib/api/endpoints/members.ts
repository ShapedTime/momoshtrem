import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { Member, MemberCreate, MemberPatch, MembersList } from '../contract';

/** Household members (PLAN §6 "Household members"; #239). */
export const memberKeys = {
  all: ['members'] as const,
};

export const membersApi = {
  list: () => apiJson<MembersList>('/api/members'),
  /** 201. 409 `member_limit` or `member_name_taken`; 422 names `name` or `color`. */
  create: (body: MemberCreate) => apiJson<Member>('/api/members', { method: 'POST', json: body }),
  /** 404, 409 `member_name_taken`, 422 `validation_error`. */
  update: (id: number, patch: MemberPatch) => apiJson<Member>(`/api/members/${id}`, { method: 'PATCH', json: patch }),
  /** 204. 404; 409 `last_member`. The member's Trakt, picks, feedback and Ask threads go with them. */
  remove: (id: number) => apiJson<void>(`/api/members/${id}`, { method: 'DELETE' }),
};

/** The defaults stay on (refetch on focus), so a removal made on another device is caught. */
export const membersQuery = () => queryOptions({ queryKey: memberKeys.all, queryFn: membersApi.list });
