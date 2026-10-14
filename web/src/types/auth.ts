export interface SessionPayload {
  authenticated: boolean;
  expiresAt: number;
}

export type LoginErrorCode = 'invalid_credentials' | 'missing_credentials' | 'rate_limited' | 'server_error';

/** JSON answer of POST /api/auth (when the client sends Accept: application/json). */
export type LoginResponse =
  | { ok: true; redirect: string }
  | {
      ok: false;
      error: { code: LoginErrorCode; message: string };
      /** Failures left before a short pause (wrong credentials only). */
      remaining_attempts?: number;
      /** Seconds until sign-in works again (rate limited only). */
      retry_after?: number;
    };
