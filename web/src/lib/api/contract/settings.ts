import type { Timestamp } from './common';
import type { RunProgress, RunSummary } from './recommendations';

/** AI settings and usage (PLAN §6 "Recommendations (R1–R3)"; #9), and storage (#77, below). The quality profile lives in search.ts. */

export type RecModel = 'claude-opus-5-5' | 'claude-sonnet-5-5';
export type ClassifierModel = 'claude-haiku-5-5' | 'claude-sonnet-5-5' | 'claude-opus-5-5';
/** At 03:00 server-local time; weekly runs on Sunday. */
export type RecSchedule = 'nightly' | 'weekly' | 'manual';
export type AiFeature = 'recs' | 'ask' | 'classifier';

/** GET/PUT /settings/ai: app_settings key `ai`. Readable and writable without an API key. */
export interface AiSettings {
  /** The master switch for picks: shelves, hero picks, runs and title reasons. Ask doesn't depend on it. */
  rec_enabled: boolean;
  /** Picks and Ask; a thread keeps the model it started on. */
  rec_model: RecModel;
  rec_schedule: RecSchedule;
  /** Shared by every AI feature, per server-local calendar month; null is no cap. 15 on a fresh install. */
  monthly_budget_usd: number | null;
  classifier_enabled: boolean;
  classifier_model: ClassifierModel;
  auto_apply_high_confidence: boolean;
}

/** PUT /settings/ai is partial: fields left out keep their value, unknown ones are ignored, a bad one is 422 naming it. */
export type AiSettingsPatch = Partial<AiSettings>;

/** GET /settings/ai/usage: this server-local month. */
export interface AiUsage {
  /** ANTHROPIC_API_KEY is set. */
  configured: boolean;
  /** "2026-10" */
  month: string;
  /** The 1st of next month, 00:00 server-local. */
  resets_at: Timestamp;
  total_usd: number;
  budget_usd: number | null;
  /** total_usd ≥ budget_usd: runs are skipped, Ask answers 429 and mapping runs on rules. */
  budget_reached: boolean;
  /** Always all three, in this order; `requests` counts API calls. */
  by_feature: Array<{ feature: AiFeature; cost_usd: number; requests: number }>;
  recs: {
    /** The newest finished run: succeeded, failed or skipped. */
    last_run: RunSummary | null;
    active_run: RunProgress | null;
    /** The schedule's next run; null when manual, off or not configured. */
    next_run_at: Timestamp | null;
  };
  estimates: {
    /** The mean cost of the last 5 succeeded runs on each model; null before any. */
    run_usd: Record<RecModel, number | null>;
    /** The mean cost of the last 20 done Ask turns; null before any. */
    ask_usd: number | null;
  };
  /** Ask turns started today (server-local), cancelled and failed ones included. */
  asks_today: number;
  ask_daily_limit: number;
}

// ---- Storage (PLAN §6 "Storage (#77)"): the piece cache's limit and how it's split. Sizes are bytes.

/** Where the limit comes from: the stored setting, config.yaml's `global_cache_size` (only when the file sets it), or auto (75% of the disk). */
export type StorageLimitSource = 'setting' | 'config' | 'auto';
/** A Keep's default maximum in days (null: no maximum). */
export type KeepDays = 7 | 30 | 90;

/** A download window, `HH:MM` in the server's TZ. It may cross midnight ("23:00"–"06:00"); start ≠ end. */
export interface DownloadWindow {
  start: string;
  end: string;
}

export interface StorageKeptSettings {
  /** Null: 50% of the limit. An explicit cap is at most the limit less the reserve (else 422). */
  max_bytes: number | null;
  /**
   * The cap applied: kept + smart leave the streaming reserve free. A default (null) cap shrinks, smart
   * first; a PUT never shrinks an explicit one (422), but when the limit falls on its own (auto following
   * the disk, or config.yaml's) explicit caps are clamped too.
   */
  effective_max_bytes: number;
  /** With `default_days: 30`, the defaults: "until watched, at most 30 days". */
  default_until_watched: boolean;
  default_days: KeepDays | null;
}

export interface StorageSmartSettings {
  /** The next episodes of the shows being watched. False on a fresh install. */
  enabled: boolean;
  /** 1–5; 2 by default. */
  episodes_ahead: number;
  /** Null: 15% of the limit. An explicit cap is at most the limit less the reserve and `kept.effective_max_bytes` (else 422, never shrunk). */
  max_bytes: number | null;
  effective_max_bytes: number;
}

export interface StorageBackgroundSettings {
  /** The kill switch for every background download, kept and smart. */
  enabled: boolean;
  /** While anything streams, downloads of the other torrents wait, until a minute after the last stream ends. */
  pause_while_streaming: boolean;
  /** At most 4; empty: any time (null is 422). */
  windows: DownloadWindow[];
}

/**
 * GET/PUT /settings/storage: app_settings key `storage`, plus what the server measures. A disk that
 * can't be read is 500 `internal_error`; without the storage governor both answer 503 `unavailable`.
 */
export interface StorageSettings {
  /**
   * The stored limit. Null is either not set (config.yaml's or auto then: `source` says which) or auto.
   * config.yaml gives way only once a PUT has sent `limit_bytes`, so send it only when the limit control
   * changes ("Use auto" is `limit_bytes: null`), never by echoing GET back.
   */
  limit_bytes: number | null;
  source: StorageLimitSource;
  /** 75% of the disk's total. */
  auto_bytes: number;
  /** Null unless config.yaml sets `global_cache_size`. */
  config_bytes: number | null;
  /** After precedence (setting, then config, then auto), clamped to [min_bytes, max_bytes]. */
  limit_effective_bytes: number;
  /** The cap applied to the cache now: min(limit, filled + free − floor), never below min_bytes. */
  effective_bytes: number;
  /** The free-space floor holds `effective_bytes` below the limit. */
  floored: boolean;
  /** The disk keeps at least this much free: 5 GiB. */
  floor_free_bytes: number;
  /** 4 GiB. */
  min_bytes: number;
  /** 90% of the disk's total. */
  max_bytes: number;
  /** The streaming reserve, min(20 GiB, 25% of the limit): pins never take it. */
  reserve_bytes: number;
  /** statfs(metadata_folder). */
  disk: { total_bytes: number; free_bytes: number };
  /** On disk now, from the piece cache; cache = filled − pinned. */
  usage: { kept_bytes: number; smart_bytes: number; cache_bytes: number };
  /** The wanted size of every kept download's admitted files, on disk or not; a shared file counts once. */
  committed_kept_bytes: number;
  kept: StorageKeptSettings;
  smart: StorageSmartSettings;
  background: StorageBackgroundSettings;
}

/**
 * PUT /settings/storage is partial: fields left out keep their value, nested objects merge and
 * `windows` is replaced whole. A bad value is 422 naming a field the PUT sent: types first, then the
 * ranges that depend on the disk, checked on the stored settings with the patch applied. A change that
 * lowers the kept cap below what kept downloads commit (and below the cap before) is 409
 * `limit_below_committed`; a raise that's still short, or a PUT that doesn't touch the limit or the
 * caps, saves. Answers every setting.
 */
export interface StorageSettingsPatch {
  limit_bytes?: number | null;
  kept?: Partial<Pick<StorageKeptSettings, 'max_bytes' | 'default_until_watched' | 'default_days'>>;
  smart?: Partial<Pick<StorageSmartSettings, 'enabled' | 'episodes_ahead' | 'max_bytes'>>;
  background?: Partial<StorageBackgroundSettings>;
}

/** The extra field of PUT /settings/storage's `409 limit_below_committed`. */
export interface LimitBelowCommittedDetails {
  committed_kept_bytes: number;
}
