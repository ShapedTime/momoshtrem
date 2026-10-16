import type {
  AiFeature,
  AiSettings,
  AiSettingsPatch,
  QualityProfile,
  StorageBackgroundSettings,
  StorageKeptSettings,
  StorageSettings,
  StorageSmartSettings,
} from '@/lib/api/contract';
import { type MockScenario, scenario } from '../scenario';

/** A fresh install's AI settings (PLAN §6): picks on, Opus 5.5 nightly, a $15 budget every feature shares. */
export const AI_SETTINGS: AiSettings = {
  rec_enabled: true,
  rec_model: 'claude-opus-5-5',
  rec_schedule: 'nightly',
  monthly_budget_usd: 15,
  classifier_enabled: true,
  classifier_model: 'claude-haiku-5-5',
  auto_apply_high_confidence: true,
};

/**
 * The page's AI settings, kept here (not in handlers/settings.ts) so the
 * recommendations, mapping and status mocks read them without msw. A new
 * scenario starts over: `recs-off` with picks off, `ask-budget` with the
 * design's $5 budget (all spent).
 */
let aiSettings: { from: MockScenario | null; value: AiSettings } | undefined;

function startingAiSettings(from: MockScenario | null): AiSettings {
  if (from === 'recs-off') return { ...AI_SETTINGS, rec_enabled: false };
  if (from === 'ask-budget') return { ...AI_SETTINGS, monthly_budget_usd: 5 };
  return { ...AI_SETTINGS };
}

export function mockAiSettings(): AiSettings {
  const from = scenario();
  if (!aiSettings || aiSettings.from !== from) aiSettings = { from, value: startingAiSettings(from) };
  return aiSettings.value;
}

/** PUT /settings/ai, once validated: fields left out keep their value. */
export function updateMockAiSettings(patch: AiSettingsPatch): AiSettings {
  const current = mockAiSettings();
  Object.assign(current, patch);
  return current;
}

/** Tests: back to a fresh install's AI settings (rebuilt for the scenario on the next read). */
export function resetMockAiSettings(): void {
  aiSettings = undefined;
}

/** ANTHROPIC_API_KEY is set: everywhere but `ai-off`. */
export const aiConfigured = (): boolean => scenario() !== 'ai-off';

/**
 * This month's spend before the page did anything (the design's Settings ›
 * AI & picks: $3.12 in all). Runs and asks on the page add to it
 * (fixtures/recommendations.ts).
 */
export const AI_SPEND: Record<AiFeature, { cost_usd: number; requests: number }> = {
  recs: { cost_usd: 2.18, requests: 41 },
  ask: { cost_usd: 0.53, requests: 23 },
  classifier: { cost_usd: 0.41, requests: 96 },
};

/** The prototype's profile (screens/settings/quality-*), not the backend's defaults. */
export const QUALITY_PROFILE: QualityProfile = {
  preferred_resolution: '2160p',
  max_resolution: '2160p',
  max_gb_per_hour: 8,
  prefer_x265: true,
  prefer_hdr: true,
  prefer_dolby_vision: true,
  avoid_terms: ['CAM', 'TS', 'HDCAM', 'TELESYNC', 'HC', 'KORSUB'],
  preferred_groups: ['FLUX', 'NTb', 'SubsPlease', 'VARYG'],
};

// ---- Storage (PLAN §6 "Storage (#77)") ---------------------------------------

const GiB = 1024 ** 3;

/**
 * The mock's data disk, as /torrents/summary reports it: 1 TB with 812 GB free.
 * The piece cache holds 62.4 GB of it: the downloads' pinned bytes
 * (fixtures/downloads.ts) and streaming cache.
 */
export const MOCK_DISK = { total_bytes: 1_000_000_000_000, free_bytes: 812_000_000_000 } as const;
export const CACHE_FILLED_BYTES = 62_400_000_000;

/** The server's constants: the 5 GiB floor, the 4 GiB minimum, and the streaming reserve's 20 GiB. */
export const STORAGE_FLOOR_BYTES = 5 * GiB;
export const STORAGE_MIN_BYTES = 4 * GiB;
const RESERVE_CAP_BYTES = 20 * GiB;

/** What app_settings key `storage` holds: what PUT writes. */
export interface StorageDocument {
  limit_bytes: number | null;
  kept: Pick<StorageKeptSettings, 'max_bytes' | 'default_until_watched' | 'default_days'>;
  smart: Pick<StorageSmartSettings, 'enabled' | 'episodes_ahead' | 'max_bytes'>;
  background: StorageBackgroundSettings;
}

/** A fresh install's (PLAN §6): auto limit, until watched for 30 days at most, smart downloads off, paused while streaming, any time. */
export const STORAGE_DEFAULTS: StorageDocument = {
  limit_bytes: null,
  kept: { max_bytes: null, default_until_watched: true, default_days: 30 },
  smart: { enabled: false, episodes_ahead: 2, max_bytes: null },
  background: { enabled: true, pause_while_streaming: true, windows: [] },
};

/**
 * The mock install's: smart downloads on (Frieren's next episodes are picked), and
 * pause-while-streaming off, so Activity's downloads keep going while Severance
 * streams. Turn it on in Settings › Storage and they wait for the stream.
 */
export const MOCK_STORAGE: StorageDocument = {
  ...STORAGE_DEFAULTS,
  smart: { ...STORAGE_DEFAULTS.smart, enabled: true },
  background: { ...STORAGE_DEFAULTS.background, pause_while_streaming: false },
};

/** The seeded kept downloads commit 86.4 GB (fixtures/downloads.ts); `downloads-full` leaves 1.2 GB of the kept cap. */
export const FULL_KEPT_MAX_BYTES = 87_600_000_000;

/** A copy to change (jsdom, where component tests build titles, has no structuredClone). */
export const cloneStorage = (doc: StorageDocument): StorageDocument => ({
  limit_bytes: doc.limit_bytes,
  kept: { ...doc.kept },
  smart: { ...doc.smart },
  background: { ...doc.background, windows: doc.background.windows.map((w) => ({ ...w })) },
});

function startingStorage(from: MockScenario | null): StorageDocument {
  if (from === 'downloads-empty') return cloneStorage(STORAGE_DEFAULTS);
  const doc = cloneStorage(MOCK_STORAGE);
  if (from === 'downloads-full') doc.kept.max_bytes = FULL_KEPT_MAX_BYTES;
  return doc;
}

/** The page's storage document, rebuilt when the scenario changes (as the AI settings are). */
let storage: { from: MockScenario | null; value: StorageDocument } | undefined;

export function mockStorageDocument(): StorageDocument {
  const from = scenario();
  if (!storage || storage.from !== from) storage = { from, value: startingStorage(from) };
  return storage.value;
}

/** PUT /settings/storage, once validated and checked: nested objects merge, `windows` is replaced whole. */
export function updateMockStorage(next: StorageDocument): StorageDocument {
  mockStorageDocument();
  if (storage) storage.value = next;
  return next;
}

/** Tests: back to the mock install's storage settings. */
export function resetMockStorage(): void {
  storage = undefined;
}

/** What the downloads have on disk and commit (fixtures/downloads.ts measures it). */
export interface StorageUsage {
  kept_bytes: number;
  smart_bytes: number;
  committed_kept_bytes: number;
}

/** `percent`% of `bytes`, in whole bytes (integer maths: 0.15 and 0.9 aren't exact in binary). */
const share = (bytes: number, percent: number) => Math.floor((bytes * percent) / 100);

/** The limit and the caps a document gives (PLAN §6 "Storage (#77)"), without the usage. */
export function storageCaps(doc: StorageDocument) {
  const auto = share(MOCK_DISK.total_bytes, 75);
  const max = share(MOCK_DISK.total_bytes, 90);
  const limit = Math.min(max, Math.max(STORAGE_MIN_BYTES, doc.limit_bytes ?? auto));
  const reserve = Math.min(RESERVE_CAP_BYTES, share(limit, 25));
  const room = limit - reserve;
  // Kept first: when the two don't fit beside the reserve, smart shrinks
  const kept = Math.min(doc.kept.max_bytes ?? share(limit, 50), room);
  const smart = Math.min(doc.smart.max_bytes ?? share(limit, 15), room - kept);
  return { auto, max, limit, reserve, room, kept, smart };
}

/** GET /settings/storage for a document and what the downloads hold. */
export function storageView(doc: StorageDocument, usage: StorageUsage): StorageSettings {
  const caps = storageCaps(doc);
  const pinned = usage.kept_bytes + usage.smart_bytes;
  const effective = Math.max(STORAGE_MIN_BYTES, Math.min(caps.limit, CACHE_FILLED_BYTES + MOCK_DISK.free_bytes - STORAGE_FLOOR_BYTES));
  return {
    limit_bytes: doc.limit_bytes,
    source: doc.limit_bytes === null ? 'auto' : 'setting',
    auto_bytes: caps.auto,
    config_bytes: null,
    limit_effective_bytes: caps.limit,
    effective_bytes: effective,
    floored: effective < caps.limit,
    floor_free_bytes: STORAGE_FLOOR_BYTES,
    min_bytes: STORAGE_MIN_BYTES,
    max_bytes: caps.max,
    reserve_bytes: caps.reserve,
    disk: { ...MOCK_DISK },
    usage: { kept_bytes: usage.kept_bytes, smart_bytes: usage.smart_bytes, cache_bytes: Math.max(0, CACHE_FILLED_BYTES - pinned) },
    committed_kept_bytes: usage.committed_kept_bytes,
    kept: { ...doc.kept, effective_max_bytes: caps.kept },
    smart: { ...doc.smart, effective_max_bytes: caps.smart },
    background: cloneStorage(doc).background,
  };
}
