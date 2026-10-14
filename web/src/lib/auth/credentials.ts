import { createHash, timingSafeEqual } from 'crypto';

/** Username used when APP_USERNAME isn't set (a startup warning says so). */
export const DEFAULT_USERNAME = 'admin';

export function getConfiguredUsername(): string {
  return process.env.APP_USERNAME?.trim() || DEFAULT_USERNAME;
}

function digest(value: string): Buffer {
  return createHash('sha256').update(value, 'utf8').digest();
}

/** Constant-time comparison: hashing first makes the lengths equal. */
function safeEqual(input: string, expected: string): boolean {
  return timingSafeEqual(digest(input), digest(expected));
}

/**
 * Check a username and password against APP_USERNAME / APP_PASSWORD.
 * Both are always compared, so timing doesn't reveal which one was wrong.
 */
export function verifyCredentials(username: string, password: string): boolean {
  const expectedPassword = process.env.APP_PASSWORD;
  if (!expectedPassword) {
    console.error('[auth] APP_PASSWORD is not set; refusing every sign-in');
    return false;
  }
  const userOk = safeEqual(username.trim(), getConfiguredUsername());
  const passwordOk = safeEqual(password, expectedPassword);
  return userOk && passwordOk;
}
