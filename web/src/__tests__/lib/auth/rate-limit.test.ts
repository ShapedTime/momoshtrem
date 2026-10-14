/** @jest-environment node */
import { checkRateLimit, clearRateLimit, recordFailedAttempt, resetRateLimits } from '@/lib/auth/rate-limit';

const MINUTE = 60_000;

describe('rate limiting', () => {
  const env = process.env;
  beforeEach(() => {
    process.env = { ...env, AUTH_MAX_ATTEMPTS: '3', AUTH_GLOBAL_MAX_FAILURES: '5', AUTH_WINDOW_MINUTES: '15', AUTH_BLOCK_MINUTES: '15' };
    resetRateLimits();
  });
  afterAll(() => {
    process.env = env;
  });

  it('counts down remaining attempts and blocks at the limit', () => {
    const t = 1_000_000;
    expect(checkRateLimit('a', t).remaining).toBe(3);
    expect(recordFailedAttempt('a', t).remaining).toBe(2);
    expect(recordFailedAttempt('a', t + 1).remaining).toBe(1);
    const blocked = recordFailedAttempt('a', t + 2);
    expect(blocked.allowed).toBe(false);
    expect(blocked.retryAfterSeconds).toBe(15 * 60);
    expect(checkRateLimit('a', t + 10 * MINUTE).allowed).toBe(false);
    expect(checkRateLimit('a', t + 16 * MINUTE).allowed).toBe(true);
  });

  it('forgets failures outside the window', () => {
    const t = 1_000_000;
    recordFailedAttempt('a', t);
    recordFailedAttempt('a', t + 1);
    expect(checkRateLimit('a', t + 16 * MINUTE).remaining).toBe(3);
  });

  it('clears a client after a successful sign-in', () => {
    recordFailedAttempt('a');
    clearRateLimit('a');
    expect(checkRateLimit('a').remaining).toBe(3);
  });

  it('blocks everyone once the global ceiling is hit, so forged addresses cannot keep guessing', () => {
    const t = 1_000_000;
    for (let i = 0; i < 5; i += 1) recordFailedAttempt(`forged-${i}`, t + i);
    expect(checkRateLimit('someone-new', t + 10).allowed).toBe(false);
    expect(checkRateLimit('someone-new', t + 16 * MINUTE).allowed).toBe(true);
  });
});
