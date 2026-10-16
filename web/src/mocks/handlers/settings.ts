import { http, HttpResponse } from 'msw';
import type {
  AiSettings,
  AiSettingsPatch,
  ApiErrorBody,
  ClassifierModel,
  DownloadWindow,
  KeepDays,
  QualityProfile,
  RecModel,
  RecSchedule,
} from '@/lib/api/contract';
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

// ---- Storage (PLAN §6 "Storage (#77)", checked as internal/storagecap checks it) ----

type FieldError = { field: string; message: string };

/** A PUT's fields once they've passed the type checks: undefined is "not sent", null "null". */
interface StoragePatch {
  limit?: number | null;
  kept: { max?: number | null; untilWatched?: boolean; days?: KeepDays | null };
  smart: { enabled?: boolean; ahead?: number; max?: number | null };
  background: { enabled?: boolean; pause?: boolean; windows?: DownloadWindow[] };
}

const KEEP_DAYS: readonly KeepDays[] = [7, 30, 90];
const HHMM = /^([01]\d|2[0-3]):[0-5]\d$/;
const gb = (bytes: number) => `${(bytes / 1e9).toFixed(1)} GB`;
const isBytes = (value: unknown): value is number | null => value === null || (typeof value === 'number' && Number.isInteger(value) && value >= 0);

/** At most 4 windows of two different HH:MM times; null is refused. */
function windowsError(value: unknown): string | null {
  if (!Array.isArray(value)) return 'background.windows must be a list of {"start": "HH:MM", "end": "HH:MM"}, or [] for any time';
  if (value.length > 4) return 'background.windows takes at most 4 windows';
  const bad = value.findIndex((w) => !isRecord(w) || typeof w.start !== 'string' || typeof w.end !== 'string' || !HHMM.test(w.start) || !HHMM.test(w.end));
  if (bad >= 0) return `background.windows: window ${bad + 1} needs a start and an end from 00:00 to 23:59`;
  const same = value.findIndex((w) => w.start === w.end);
  return same >= 0 ? `background.windows: window ${same + 1} starts when it ends` : null;
}

/** The nested object at `key`: not sent (null), its fields, or a 422 naming it. */
function nested(body: Record<string, unknown>, key: string): { fields: Record<string, unknown> } | { error: FieldError } | null {
  if (!(key in body)) return null;
  const value = body[key];
  return isRecord(value) ? { fields: value } : { error: { field: key, message: `${key} must be an object of the fields to change` } };
}

/** The first pass: every field sent, its type and its allowed values. The ranges that depend on the disk come second. */
function parseStorage(body: Record<string, unknown>): { patch: StoragePatch } | { error: FieldError } {
  const patch: StoragePatch = { kept: {}, smart: {}, background: {} };
  const fail = (field: string, message: string) => ({ error: { field, message } });
  const bool = (field: string, value: unknown) => (typeof value === 'boolean' ? null : fail(field, `${field} must be true or false`));

  if ('limit_bytes' in body) {
    if (!isBytes(body.limit_bytes)) return fail('limit_bytes', 'limit_bytes must be a whole number of bytes, or null for auto');
    patch.limit = body.limit_bytes;
  }

  const keptPart = nested(body, 'kept');
  if (keptPart && 'error' in keptPart) return keptPart;
  const kept = keptPart?.fields;
  if (kept) {
    if ('max_bytes' in kept) {
      if (!isBytes(kept.max_bytes)) return fail('kept.max_bytes', 'kept.max_bytes must be a whole number of bytes, or null for half the limit');
      patch.kept.max = kept.max_bytes;
    }
    if ('default_until_watched' in kept) {
      const bad = bool('kept.default_until_watched', kept.default_until_watched);
      if (bad) return bad;
      patch.kept.untilWatched = kept.default_until_watched as boolean;
    }
    if ('default_days' in kept) {
      const days = KEEP_DAYS.find((d) => d === kept.default_days);
      if (kept.default_days !== null && days === undefined) return fail('kept.default_days', 'kept.default_days must be 7, 30 or 90, or null for no maximum');
      patch.kept.days = days ?? null;
    }
  }

  const smartPart = nested(body, 'smart');
  if (smartPart && 'error' in smartPart) return smartPart;
  const smart = smartPart?.fields;
  if (smart) {
    if ('enabled' in smart) {
      const bad = bool('smart.enabled', smart.enabled);
      if (bad) return bad;
      patch.smart.enabled = smart.enabled as boolean;
    }
    if ('episodes_ahead' in smart) {
      const ahead = smart.episodes_ahead;
      if (!(typeof ahead === 'number' && Number.isInteger(ahead) && ahead >= 1 && ahead <= 5)) {
        return fail('smart.episodes_ahead', 'smart.episodes_ahead must be a whole number from 1 to 5');
      }
      patch.smart.ahead = ahead;
    }
    if ('max_bytes' in smart) {
      if (!isBytes(smart.max_bytes)) return fail('smart.max_bytes', 'smart.max_bytes must be a whole number of bytes, or null for 15% of the limit');
      patch.smart.max = smart.max_bytes;
    }
  }

  const backgroundPart = nested(body, 'background');
  if (backgroundPart && 'error' in backgroundPart) return backgroundPart;
  const background = backgroundPart?.fields;
  if (background) {
    for (const [key, field] of [['enabled', 'enabled'], ['pause_while_streaming', 'pause']] as const) {
      if (!(key in background)) continue;
      const bad = bool(`background.${key}`, background[key]);
      if (bad) return bad;
      patch.background[field] = background[key] as boolean;
    }
    if ('windows' in background) {
      const message = windowsError(background.windows);
      if (message) return fail('background.windows', message);
      patch.background.windows = (background.windows as DownloadWindow[]).map(({ start, end }) => ({ start, end }));
    }
  }
  return { patch };
}

/** The stored document with the patch applied: nested objects merge, `windows` is replaced whole. */
function mergeStorage(current: StorageDocument, patch: StoragePatch): StorageDocument {
  const next = cloneStorage(current);
  if (patch.limit !== undefined) Object.assign(next, { limit_bytes: patch.limit, limit_set: true });
  if (patch.kept.max !== undefined) next.kept.max_bytes = patch.kept.max;
  if (patch.kept.untilWatched !== undefined) next.kept.default_until_watched = patch.kept.untilWatched;
  if (patch.kept.days !== undefined) next.kept.default_days = patch.kept.days;
  if (patch.smart.enabled !== undefined) next.smart.enabled = patch.smart.enabled;
  if (patch.smart.ahead !== undefined) next.smart.episodes_ahead = patch.smart.ahead;
  if (patch.smart.max !== undefined) next.smart.max_bytes = patch.smart.max;
  if (patch.background.enabled !== undefined) next.background.enabled = patch.background.enabled;
  if (patch.background.pause !== undefined) next.background.pause_while_streaming = patch.background.pause;
  if (patch.background.windows !== undefined) next.background.windows = patch.background.windows;
  return next;
}

/**
 * The second pass (storagecap.checkLimits), on the merged settings: the limit's range,
 * then explicit caps. A PUT that sends the limit or a cap must leave every explicit cap
 * fitting; the 422 names a field it sent: the cap that doesn't fit, else the one that
 * squeezed it. Default caps always fit (they shrink).
 */
function checkLimits(patch: StoragePatch, merged: StorageDocument): FieldError | null {
  const next = storageCaps(merged);
  if (patch.limit !== undefined && patch.limit !== null && (patch.limit < STORAGE_MIN_BYTES || patch.limit > next.max)) {
    return { field: 'limit_bytes', message: `limit_bytes must be from ${gb(STORAGE_MIN_BYTES)} to ${gb(next.max)} (90% of the disk), or null for auto` };
  }
  const [sentLimit, sentKept, sentSmart] = [patch.limit !== undefined, patch.kept.max !== undefined, patch.smart.max !== undefined];
  if (!sentLimit && !sentKept && !sentSmart) return null;
  const kept = merged.kept.max_bytes;
  if (kept !== null && kept > next.room) {
    if (sentKept) return { field: 'kept.max_bytes', message: `kept.max_bytes can be at most ${gb(next.room)}: the limit less the streaming reserve` };
    if (sentLimit) return { field: 'limit_bytes', message: `This limit leaves kept downloads ${gb(next.room)}, below their cap of ${gb(kept)}: lower kept.max_bytes too` };
  }
  const left = next.room - next.kept;
  const smart = merged.smart.max_bytes;
  if (smart !== null && smart > left) {
    const field = sentSmart ? 'smart.max_bytes' : sentKept ? 'kept.max_bytes' : 'limit_bytes';
    return { field, message: `smart.max_bytes can be at most ${gb(left)}: what kept downloads and the streaming reserve leave of the limit` };
  }
  return null;
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
    const parsed = parseStorage(body);
    if ('error' in parsed) return apiError(422, 'validation_error', parsed.error.message, { field: parsed.error.field });
    const current = mockStorageDocument();
    const next = mergeStorage(current, parsed.patch);
    const tooBig = checkLimits(parsed.patch, next);
    if (tooBig) return apiError(422, 'validation_error', tooBig.message, { field: tooBig.field });
    // Only a change that lowers the kept cap below what's committed is refused: a raise that's still short helps
    const { committed_kept_bytes } = downloadsUsage();
    const [before, after] = [storageCaps(current).kept, storageCaps(next).kept];
    if (after < committed_kept_bytes && after < before) {
      return apiError(
        409,
        'limit_below_committed',
        `Kept downloads need ${gb(committed_kept_bytes)}, but this leaves them ${gb(after)}. Free some kept downloads first.`,
        { committed_kept_bytes }
      );
    }
    updateMockStorage(next);
    // Usage after the change: switching smart downloads off releases the smart picks
    return HttpResponse.json(storageView(next, downloadsUsage()));
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
