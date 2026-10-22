import { http, HttpResponse } from 'msw';
import { type Member, MEMBER_COLORS, MEMBER_NAME_MAX, type MemberColor, MAX_MEMBERS, UNKNOWN_MEMBER_CODE } from '@/lib/api/contract';
import { BACKEND_PREFIXES } from '@/lib/api/backend-prefixes';
import { nextFreeColor } from '@/lib/members/display';
import { type MockMember, mockMembers, mockMemberStale, resetMockMembers, setMockMember, updateMockMembers } from '../fixtures/members';
import { forgetMockRecs } from '../fixtures/recommendations';
import { forgetMockTrakt, traktSummary } from '../fixtures/trakt';
import { scenario } from '../scenario';
import { apiError, isRecord, jsonBody, latency, noContent, notFound } from './util';

/** Tests: back to the seeded household, and nobody named. */
export { resetMockMembers, setMockMember };

const view = (member: MockMember): Member => ({ ...member, trakt: traktSummary(member.id) });

const isColor = (value: unknown): value is MemberColor => typeof value === 'string' && (MEMBER_COLORS as readonly string[]).includes(value);

/** `members-absent`: a backend before household members (404); `members-error`: it fails (500). */
function unavailable() {
  if (scenario() === 'members-absent') return notFound('No such API route.');
  if (scenario() === 'members-error') return apiError(500, 'internal_error', 'Couldn’t read the household.');
  return null;
}

/**
 * A name or a colour from a body, checked in the contract's order: 400 for a body that isn't an object or a
 * field of the wrong JSON type (`null` included), then 422 naming the field for a bad value.
 */
function validate(body: unknown, creating: boolean): { name?: string; color?: MemberColor } | Response {
  if (!isRecord(body)) return apiError(400, 'bad_request', 'The body must be a JSON object.');
  for (const field of ['name', 'color'] as const) {
    if (field in body && typeof body[field] !== 'string') return apiError(400, 'bad_request', `${field} must be a string.`);
  }
  const out: { name?: string; color?: MemberColor } = {};
  if (creating || body.name !== undefined) {
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    if (!name || Array.from(name).length > MEMBER_NAME_MAX) {
      return apiError(422, 'validation_error', `A name is 1–${MEMBER_NAME_MAX} characters.`, { field: 'name' });
    }
    out.name = name;
  }
  if (body.color !== undefined) {
    if (!isColor(body.color)) return apiError(422, 'validation_error', `color is one of ${MEMBER_COLORS.join(', ')}.`, { field: 'color' });
    out.color = body.color;
  }
  return out;
}

const taken = (name: string, except?: number) => mockMembers().some((m) => m.id !== except && m.name.toLocaleLowerCase() === name.toLocaleLowerCase());

/**
 * The household (PLAN §6 "Household members"): CRUD with the contract's
 * rules. `/members` ignores the member header, so the picker always loads.
 */
export const membersHandlers = [
  http.get('*/api/members', async () => {
    await latency();
    return unavailable() ?? HttpResponse.json({ items: mockMembers().map(view) });
  }),
  http.post('*/api/members', async ({ request }) => {
    await latency();
    const failed = unavailable();
    if (failed) return failed;
    const fields = validate(await jsonBody(request), true);
    if (fields instanceof Response) return fields;
    const name = fields.name ?? '';
    if (mockMembers().length >= MAX_MEMBERS) return apiError(409, 'member_limit', `A household can have up to ${MAX_MEMBERS} people.`);
    if (taken(name)) return apiError(409, 'member_name_taken', `Someone here is already called ${name}.`);
    const created = updateMockMembers((household) => {
      const member: MockMember = { id: household.nextId++, name, color: fields.color ?? nextFreeColor(household.items), created_at: new Date().toISOString() };
      household.items.push(member);
      return member;
    });
    return HttpResponse.json(view(created), { status: 201 });
  }),
  http.patch('*/api/members/:id', async ({ params, request }) => {
    await latency();
    const failed = unavailable();
    if (failed) return failed;
    // The contract's order: 400, 422, 404, 409
    const fields = validate(await jsonBody(request), false);
    if (fields instanceof Response) return fields;
    const id = Number(params.id);
    if (!mockMembers().some((m) => m.id === id)) return notFound('No such member.');
    if (fields.name !== undefined && taken(fields.name, id)) return apiError(409, 'member_name_taken', `Someone here is already called ${fields.name}.`);
    const updated = updateMockMembers((household) => {
      const member = household.items.find((m) => m.id === id);
      if (member) Object.assign(member, fields);
      return member && { ...member };
    });
    return updated ? HttpResponse.json(view(updated)) : notFound('No such member.');
  }),
  http.delete('*/api/members/:id', async ({ params }) => {
    await latency();
    const failed = unavailable();
    if (failed) return failed;
    const id = Number(params.id);
    if (!mockMembers().some((m) => m.id === id)) return notFound('No such member.');
    if (mockMembers().length === 1) return apiError(409, 'last_member', 'The household needs at least one person.');
    updateMockMembers((household) => {
      household.items = household.items.filter((m) => m.id !== id);
    });
    // Their Trakt and picks go; what they spent stays in the month's total
    forgetMockTrakt(id);
    forgetMockRecs(id);
    return noContent();
  }),
];

/** Routes that ignore a member nobody has: the picker's list and health. */
const IGNORES_MEMBER = new Set(['members', 'status']);

/**
 * Placed first: a personal route asked for a member nobody has answers 400
 * `unknown_member` (the web forgets the choice and reloads). Otherwise it
 * returns nothing and the request falls through to its handler.
 */
export const unknownMemberGuard = http.all('*/api/*', ({ request }) => {
  const segment = new URL(request.url).pathname.split('/')[2] ?? '';
  if (!BACKEND_PREFIXES.has(segment) || IGNORES_MEMBER.has(segment) || !mockMemberStale()) return undefined;
  return apiError(400, UNKNOWN_MEMBER_CODE, 'No member has that id.');
});
