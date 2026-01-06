/**
 * In-memory rate limiting for authentication endpoints.
 * Protects against brute force password attacks.
 *
 * Note: This implementation resets on server restart.
 * For distributed deployments, consider using Redis or similar.
 */

import { NextRequest } from 'next/server';

// ============================================
// Configuration
// ============================================

const RATE_LIMIT_CONFIG = {
  /** Maximum failed attempts before blocking */
  maxAttempts: 5,
  /** Time window in milliseconds (15 minutes) */
  windowMs: 15 * 60 * 1000,
  /** How long to block after exceeding limit (15 minutes) */
  blockDurationMs: 15 * 60 * 1000,
} as const;

// ============================================
// Types
// ============================================

interface RateLimitEntry {
  attempts: number;
  firstAttemptAt: number;
  blockedUntil: number | null;
}

export interface RateLimitResult {
  allowed: boolean;
  remaining: number;
  resetAt: number | null;
  blockedUntil: number | null;
}

// ============================================
// In-Memory Store
// ============================================

const rateLimitStore = new Map<string, RateLimitEntry>();

// Clean up old entries every 5 minutes
setInterval(() => {
  const now = Date.now();
  for (const [key, entry] of rateLimitStore.entries()) {
    // Remove entries that are past their block period and window
    if (
      entry.blockedUntil &&
      entry.blockedUntil < now &&
      entry.firstAttemptAt + RATE_LIMIT_CONFIG.windowMs < now
    ) {
      rateLimitStore.delete(key);
    } else if (
      !entry.blockedUntil &&
      entry.firstAttemptAt + RATE_LIMIT_CONFIG.windowMs < now
    ) {
      rateLimitStore.delete(key);
    }
  }
}, 5 * 60 * 1000);

// ============================================
// Helper Functions
// ============================================

/**
 * Extract client IP address from request.
 * Handles X-Forwarded-For header for reverse proxy setups.
 */
export function getClientIp(request: NextRequest): string {
  // Check X-Forwarded-For header (set by reverse proxies like Caddy)
  const forwardedFor = request.headers.get('x-forwarded-for');
  if (forwardedFor) {
    // Take the first IP in the chain (original client)
    const ips = forwardedFor.split(',').map((ip) => ip.trim());
    if (ips[0]) {
      return ips[0];
    }
  }

  // Check X-Real-IP header (alternative proxy header)
  const realIp = request.headers.get('x-real-ip');
  if (realIp) {
    return realIp;
  }

  // Fallback to a generic identifier
  return 'unknown';
}

// ============================================
// Rate Limiting Functions
// ============================================

/**
 * Check if a request is allowed under rate limiting rules.
 * Does NOT increment the attempt counter - use recordFailedAttempt for that.
 *
 * @param identifier - Unique identifier for the client (usually IP)
 * @returns Rate limit check result
 */
export function checkRateLimit(identifier: string): RateLimitResult {
  const now = Date.now();
  const entry = rateLimitStore.get(identifier);

  // No previous attempts - allowed
  if (!entry) {
    return {
      allowed: true,
      remaining: RATE_LIMIT_CONFIG.maxAttempts,
      resetAt: null,
      blockedUntil: null,
    };
  }

  // Currently blocked
  if (entry.blockedUntil && entry.blockedUntil > now) {
    return {
      allowed: false,
      remaining: 0,
      resetAt: entry.blockedUntil,
      blockedUntil: entry.blockedUntil,
    };
  }

  // Block period expired - reset the entry
  if (entry.blockedUntil && entry.blockedUntil <= now) {
    rateLimitStore.delete(identifier);
    return {
      allowed: true,
      remaining: RATE_LIMIT_CONFIG.maxAttempts,
      resetAt: null,
      blockedUntil: null,
    };
  }

  // Check if window has expired
  const windowExpiry = entry.firstAttemptAt + RATE_LIMIT_CONFIG.windowMs;
  if (windowExpiry < now) {
    // Window expired - reset
    rateLimitStore.delete(identifier);
    return {
      allowed: true,
      remaining: RATE_LIMIT_CONFIG.maxAttempts,
      resetAt: null,
      blockedUntil: null,
    };
  }

  // Within window - check remaining attempts
  const remaining = Math.max(0, RATE_LIMIT_CONFIG.maxAttempts - entry.attempts);
  return {
    allowed: remaining > 0,
    remaining,
    resetAt: windowExpiry,
    blockedUntil: null,
  };
}

/**
 * Record a failed authentication attempt.
 * Should be called after a failed login.
 *
 * @param identifier - Unique identifier for the client (usually IP)
 */
export function recordFailedAttempt(identifier: string): void {
  const now = Date.now();
  const entry = rateLimitStore.get(identifier);

  if (!entry) {
    // First failed attempt
    rateLimitStore.set(identifier, {
      attempts: 1,
      firstAttemptAt: now,
      blockedUntil: null,
    });
    return;
  }

  // Check if window has expired
  const windowExpiry = entry.firstAttemptAt + RATE_LIMIT_CONFIG.windowMs;
  if (windowExpiry < now) {
    // Start new window
    rateLimitStore.set(identifier, {
      attempts: 1,
      firstAttemptAt: now,
      blockedUntil: null,
    });
    return;
  }

  // Increment attempts
  entry.attempts++;

  // Check if we should block
  if (entry.attempts >= RATE_LIMIT_CONFIG.maxAttempts) {
    entry.blockedUntil = now + RATE_LIMIT_CONFIG.blockDurationMs;
  }

  rateLimitStore.set(identifier, entry);
}

/**
 * Clear rate limit entry for an identifier.
 * Should be called after a successful login.
 *
 * @param identifier - Unique identifier for the client (usually IP)
 */
export function clearRateLimit(identifier: string): void {
  rateLimitStore.delete(identifier);
}
