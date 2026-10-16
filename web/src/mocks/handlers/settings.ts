import { http, HttpResponse } from 'msw';
import type { AiSettings, AiSettingsPatch, ApiErrorBody, ClassifierModel, KeepDays, QualityProfile, RecModel, RecSchedule } from '@/lib/api/contract';
import { type QualityFieldError, validateQualityProfile } from '@/lib/settings/quality';
import { downloadsUsage } from '../fixtures/downloads';
import { aiUsage } from '../fixtures/recommendations';
import {
  cloneStorage,
  mockAiSettings,
  mockStorageDocument,
  QUALITY_PROFILE,
  resetMockAiSettings,
  resetMockStorage,
  type StorageDocument,
  storageCaps,
  STORAGE_MIN_BYTES,
  storageView,
  updateMockAiSettings,
  updateMockStorage,
} from '../fixtures/settings';
import { scenario } from '../scenario';
import { apiError, isRecord, jsonBody, latency } from './util';

let profile: QualityProfile = { ...QUALITY_PROFILE };

const tidy = (list: string[]) =>
  list.map((item) => item.trim()).filter((item, i, all) => all.findIndex((other) => other.toLowerCase() === item.toLowerCase()) === i);

/** This page's AI settings (mapping jobs read `classifier_enabled` and `auto_apply_high_confidence`; picks read the rest). */
export { mockAiSettings };

/** This page's quality profile as Settings › Quality saved it (torrent search ranks with it). */
export const getMockQualityProfile = () => profile;

/** Tests: back to the fixture's AI settings, quality profile and storage settings. */
export function resetMockSettings(): void {
  resetMockAiSettings();
  resetMockStorage();
  profile = { ...QUALITY_PROFILE };
}

const REC_MODELS: readonly RecModel[] = ['claude-opus-5-5', 'claude-sonnet-5-5'];
const CLASSIFIER_MODELS: readonly ClassifierModel[] = ['claude-haiku-5-5', 'claude-sonnet-5-5', 'claude-opus-5-5'];
const SCHEDULES: readonly RecSchedule[] = ['nightly', 'weekly', 'manual'];

type Check = (value: unknown) => string | null;
const oneOf =
  (field: string, allowed: readonly string[]): Check =>
  (value) =>
    typeof value === 'string' && allowed.includes(value) ? null : `${field} must be ${allowed.slice(0, -1).join(', ')} or ${allowed.at(-1)}`;
const boolean =
  (field: string): Check =>
  (value) =>
    typeof value === 'boolean' ? null : `${field} must be true or false`;

/** PUT /settings/ai's checks, as the backend's: each known field that's sent; unknown fields are ignored. */
const CHECKS: Record<keyof AiSettings, Check> = {
  rec_enabled: boolean('rec_enabled'),
  rec_model: oneOf('rec_model', REC_MODELS),
  rec_schedule: oneOf('rec_schedule', SCHEDULES),
  monthly_budget_usd: (value) =>
    value === null || (typeof value === 'number' && Number.isFinite(value) && value >= 0) ? null : 'monthly_budget_usd must be 0 or more, or null for no cap',
  classifier_enabled: boolean('classifier_enabled'),
  classifier_model: oneOf('classifier_model', CLASSIFIER_MODELS),
  auto_apply_high_confidence: boolean('auto_apply_high_confidence'),
};

// ---- Storage (PLAN §6 "Storage (#77)") -----------------------------------------

type FieldError = { field: string; message: string };

const KEEP_DAYS: readonly KeepDays[] = [7, 30, 90];
const HHMM = /^([01]\d|2[0-3]):[0-5]\d$/;
const isBytes = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value >= 0;
const gb = (bytes: number) => `${(bytes / 1e9).toFixed(1)} GB`;

/** A window list as the backend takes it: at most 4, `HH:MM`, start ≠ end (crossing midnight is fine). */
function windowsError(value: unknown): string | null {
  if (!Array.isArray(value) || value.length > 4) return 'background.windows must be a list of at most 4 windows';
  const bad = value.find((w) => !isRecord(w) || typeof w.start !== 'string' || typeof w.end !== 'string' || !HHMM.test(w.start) || !HHMM.test(w.end) || w.start === w.end);
  return bad === undefined ? null : 'background.windows: each window is {start, end} as HH:MM, and they differ';
}

/**
 * PUT /settings/storage's checks, as the backend's: in the contract's order, the first bad value
 * is a 422 naming it. Fields left out keep their value; nested objects merge; `windows` is replaced.
 */
function patchStorage(body: Record<string, unknown>, current: StorageDocument): { next: StorageDocument } | { error: FieldError } {
  const next = cloneStorage(current);
  const fail = (field: string, message: string) => ({ error: { field, message } });

  if ('limit_bytes' in body) {
    const value = body.limit_bytes;
    const { max } = storageCaps(current);
    if (value !== null && !(isBytes(value) && value >= STORAGE_MIN_BYTES && value <= max)) {
      return fail('limit_bytes', `limit_bytes must be null (auto) or ${gb(STORAGE_MIN_BYTES)}–${gb(max)}`);
    }
    next.limit_bytes = value as number | null;
  }
  // A cap fits beside the streaming reserve of the limit being saved
  const { room } = storageCaps(next);
  const capError = (field: string, value: unknown) =>
    value === null || (isBytes(value) && value <= room) ? null : `${field} must be null (the default share) or 0–${gb(room)}`;

  if ('kept' in body) {
    const kept = body.kept;
    if (!isRecord(kept)) return fail('kept', 'kept must be an object');
    if ('max_bytes' in kept) {
      const message = capError('kept.max_bytes', kept.max_bytes);
      if (message) return fail('kept.max_bytes', message);
      next.kept.max_bytes = kept.max_bytes as number | null;
    }
    if ('default_until_watched' in kept) {
      if (typeof kept.default_until_watched !== 'boolean') return fail('kept.default_until_watched', 'kept.default_until_watched must be true or false');
      next.kept.default_until_watched = kept.default_until_watched;
    }
    if ('default_days' in kept) {
      const days = KEEP_DAYS.find((d) => d === kept.default_days);
      if (kept.default_days !== null && days === undefined) return fail('kept.default_days', 'kept.default_days must be 7, 30, 90 or null');
      next.kept.default_days = days ?? null;
    }
  }

  if ('smart' in body) {
    const smart = body.smart;
    if (!isRecord(smart)) return fail('smart', 'smart must be an object');
    if ('enabled' in smart) {
      if (typeof smart.enabled !== 'boolean') return fail('smart.enabled', 'smart.enabled must be true or false');
      next.smart.enabled = smart.enabled;
    }
    if ('episodes_ahead' in smart) {
      const ahead = smart.episodes_ahead;
      if (!(typeof ahead === 'number' && Number.isInteger(ahead) && ahead >= 1 && ahead <= 5)) return fail('smart.episodes_ahead', 'smart.episodes_ahead must be 1–5');
      next.smart.episodes_ahead = ahead;
    }
    if ('max_bytes' in smart) {
      const message = capError('smart.max_bytes', smart.max_bytes);
      if (message) return fail('smart.max_bytes', message);
      next.smart.max_bytes = smart.max_bytes as number | null;
    }
  }

  if ('background' in body) {
    const background = body.background;
    if (!isRecord(background)) return fail('background', 'background must be an object');
    for (const key of ['enabled', 'pause_while_streaming'] as const) {
      if (!(key in background)) continue;
      const value = background[key];
      if (typeof value !== 'boolean') return fail(`background.${key}`, `background.${key} must be true or false`);
      next.background[key] = value;
    }
    if ('windows' in background) {
      const message = windowsError(background.windows);
      if (message) return fail('background.windows', message);
      next.background.windows = (background.windows as Array<{ start: string; end: string }>).map(({ start, end }) => ({ start, end }));
    }
  }
  return { next };
}

export const settingsHandlers = [
  http.get('*/api/settings/storage', async () => {
    await latency();
    return HttpResponse.json(storageView(mockStorageDocument(), downloadsUsage()));
  }),
  http.put('*/api/settings/storage', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return apiError(400, 'bad_request', 'Send the settings to change as a JSON object.');
    const patched = patchStorage(body, mockStorageDocument());
    if ('error' in patched) return apiError(422, 'validation_error', patched.error.message, { field: patched.error.field });
    const { committed_kept_bytes } = downloadsUsage();
    if (storageCaps(patched.next).kept < committed_kept_bytes) {
      return apiError(409, 'limit_below_committed', `Kept downloads need ${gb(committed_kept_bytes)}: stop keeping some first.`, { committed_kept_bytes });
    }
    updateMockStorage(patched.next);
    // Usage after the change: switching smart downloads off releases the smart picks
    return HttpResponse.json(storageView(patched.next, downloadsUsage()));
  }),
  http.get('*/api/settings/ai', async () => {
    await latency();
    return HttpResponse.json(mockAiSettings());
  }),
  http.put('*/api/settings/ai', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return apiError(400, 'bad_request', 'Send the settings to change as a JSON object.');
    if (scenario() === 'ai-budget-invalid' && 'monthly_budget_usd' in body) {
      return apiError(422, 'validation_error', CHECKS.monthly_budget_usd(-1) ?? 'monthly_budget_usd is invalid', { field: 'monthly_budget_usd' });
    }
    const patch: Record<string, unknown> = {};
    for (const [field, check] of Object.entries(CHECKS)) {
      if (!(field in body)) continue;
      const message = check(body[field]);
      if (message) return apiError(422, 'validation_error', message, { field });
      patch[field] = body[field];
    }
    return HttpResponse.json(updateMockAiSettings(patch as AiSettingsPatch));
  }),
  http.get('*/api/settings/ai/usage', async () => {
    await latency();
    return HttpResponse.json(aiUsage());
  }),
  http.get('*/api/settings/quality-profile', () => HttpResponse.json(profile)),
  http.put('*/api/settings/quality-profile', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return HttpResponse.json<ApiErrorBody>({ error: { code: 'bad_request', message: 'Send the whole profile as JSON.' } }, { status: 400 });
    const sent = { ...QUALITY_PROFILE, ...body } as QualityProfile;
    // Like the backend: lists trimmed and deduped (the first spelling stays) before the checks
    const next = { ...sent, avoid_terms: tidy(sent.avoid_terms), preferred_groups: tidy(sent.preferred_groups) };
    const invalid: QualityFieldError | null =
      scenario() === 'quality-invalid' ? { field: 'avoid_terms', message: 'avoid_terms must be 1–30 characters each' } : validateQualityProfile(next);
    if (invalid) return HttpResponse.json<ApiErrorBody>({ error: { code: 'validation_error', ...invalid } }, { status: 422 });
    profile = next;
    return HttpResponse.json(profile);
  }),
];
