import { cookies } from 'next/headers';
import type { NextResponse } from 'next/server';
import { SignJWT, jwtVerify } from 'jose';
import type { SessionPayload } from '@/types/auth';
import {
  SESSION_COOKIE_NAME,
  SESSION_DURATION_MS,
  SESSION_DURATION_SECONDS,
} from '@/config/auth';

// Get secret key as Uint8Array for jose
function getSecretKey(): Uint8Array {
  const secret = process.env.SESSION_SECRET;
  if (!secret || secret.length < 32) {
    throw new Error('SESSION_SECRET must be at least 32 characters');
  }
  return new TextEncoder().encode(secret);
}

// Create encrypted session token
export async function createSessionToken(): Promise<string> {
  const payload: SessionPayload = {
    authenticated: true,
    expiresAt: Date.now() + SESSION_DURATION_MS,
  };

  return new SignJWT(payload as unknown as Record<string, unknown>)
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime(`${SESSION_DURATION_SECONDS}s`)
    .sign(getSecretKey());
}

// Verify and decode session token
export async function verifySessionToken(
  token: string
): Promise<SessionPayload | null> {
  try {
    const { payload } = await jwtVerify(token, getSecretKey());
    const session = payload as unknown as SessionPayload;

    // Check if session has expired
    if (session.expiresAt < Date.now()) {
      return null;
    }

    return session;
  } catch {
    return null;
  }
}

/** Cookie attributes shared by sign-in and sign-out. */
function sessionCookieOptions() {
  // SECURE_COOKIES=false allows signing in over plain HTTP, such as http://localhost:3000
  const secure = process.env.SECURE_COOKIES !== 'false' && process.env.NODE_ENV === 'production';
  return { httpOnly: true, secure, sameSite: 'lax' as const, path: '/' };
}

/** Set the session cookie on a response (route handlers build their own response). */
export function applySessionCookie(response: NextResponse, token: string): NextResponse {
  response.cookies.set(SESSION_COOKIE_NAME, token, { ...sessionCookieOptions(), maxAge: SESSION_DURATION_SECONDS });
  return response;
}

/** Expire the session cookie on a response. */
export function clearSessionCookie(response: NextResponse): NextResponse {
  response.cookies.set(SESSION_COOKIE_NAME, '', { ...sessionCookieOptions(), maxAge: 0 });
  return response;
}

// Get session from cookie
export async function getSession(): Promise<SessionPayload | null> {
  const cookieStore = await cookies();
  const token = cookieStore.get(SESSION_COOKIE_NAME)?.value;

  if (!token) {
    return null;
  }

  return verifySessionToken(token);
}
