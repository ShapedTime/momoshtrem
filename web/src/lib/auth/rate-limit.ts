/**
 * In-memory sign-in rate limiting (single instance; resets on restart).
 *
 * Two limits:
 * - per client: AUTH_MAX_ATTEMPTS failures within the window block that
 *   client for AUTH_BLOCK_MINUTES;
 * - global: AUTH_GLOBAL_MAX_FAILURES failures from anyone within the window
 *   block every sign-in for the same time. That bounds guessing even when the
 *   client address can be forged (see client-ip.ts).
 */

interface Limits {
  maxAttempts: number;
  globalMaxFailures: number;
  windowMs: number;
  blockMs: number;
}

function envInt(name: string, fallback: number): number {
  const parsed = Number.parseInt(process.env[name] ?? '', 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function limits(): Limits {
  return {
    maxAttempts: envInt('AUTH_MAX_ATTEMPTS', 5),
    globalMaxFailures: envInt('AUTH_GLOBAL_MAX_FAILURES', 30),
    windowMs: envInt('AUTH_WINDOW_MINUTES', 15) * 60_000,
    blockMs: envInt('AUTH_BLOCK_MINUTES', 15) * 60_000,
  };
}

interface Bucket {
  failures: number[];
  blockedUntil: number;
}

const clients = new Map<string, Bucket>();
const global: Bucket = { failures: [], blockedUntil: 0 };

function prune(bucket: Bucket, now: number, windowMs: number): void {
  bucket.failures = bucket.failures.filter((at) => now - at < windowMs);
}

export interface RateLimitResult {
  allowed: boolean;
  /** Failures left before this client is blocked. */
  remaining: number;
  /** Seconds until sign-in is possible again (when not allowed). */
  retryAfterSeconds: number;
}

export function checkRateLimit(key: string, now: number = Date.now()): RateLimitResult {
  const { maxAttempts, windowMs } = limits();
  const bucket = clients.get(key);
  if (bucket) {
    prune(bucket, now, windowMs);
    if (bucket.failures.length === 0 && bucket.blockedUntil <= now) clients.delete(key);
  }
  const blockedUntil = Math.max(bucket?.blockedUntil ?? 0, global.blockedUntil);
  if (blockedUntil > now) {
    return { allowed: false, remaining: 0, retryAfterSeconds: Math.ceil((blockedUntil - now) / 1000) };
  }
  return { allowed: true, remaining: maxAttempts - (bucket?.failures.length ?? 0), retryAfterSeconds: 0 };
}

/** Record a failed sign-in; the result says whether this failure triggered a block. */
export function recordFailedAttempt(key: string, now: number = Date.now()): RateLimitResult {
  const { maxAttempts, globalMaxFailures, windowMs, blockMs } = limits();
  const bucket = clients.get(key) ?? { failures: [], blockedUntil: 0 };
  prune(bucket, now, windowMs);
  prune(global, now, windowMs);
  bucket.failures.push(now);
  global.failures.push(now);
  if (bucket.failures.length >= maxAttempts) bucket.blockedUntil = now + blockMs;
  if (global.failures.length >= globalMaxFailures) global.blockedUntil = now + blockMs;
  clients.set(key, bucket);
  return checkRateLimit(key, now);
}

/** A successful sign-in clears that client's failures (not the global count). */
export function clearRateLimit(key: string): void {
  clients.delete(key);
}

/** Tests only. */
export function resetRateLimits(): void {
  clients.clear();
  global.failures = [];
  global.blockedUntil = 0;
}
