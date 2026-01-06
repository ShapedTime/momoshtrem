import { NextRequest, NextResponse } from 'next/server';
import {
  verifyPassword,
  createSessionToken,
  setSessionCookie,
  clearSession,
} from '@/lib/auth';
import {
  checkRateLimit,
  recordFailedAttempt,
  clearRateLimit,
  getClientIp,
} from '@/lib/auth/rate-limit';
import type { LoginRequest, LoginResponse } from '@/types/auth';

// POST /api/auth - Login
export async function POST(
  request: NextRequest
): Promise<NextResponse<LoginResponse>> {
  const clientIp = getClientIp(request);

  // Check rate limit before processing
  const rateLimitResult = checkRateLimit(clientIp);
  if (!rateLimitResult.allowed) {
    const retryAfterSeconds = rateLimitResult.blockedUntil
      ? Math.ceil((rateLimitResult.blockedUntil - Date.now()) / 1000)
      : 900; // 15 minutes default

    return NextResponse.json(
      { success: false, error: 'Too many login attempts. Please try again later.' },
      {
        status: 429,
        headers: {
          'Retry-After': String(retryAfterSeconds),
        },
      }
    );
  }

  try {
    const body = (await request.json()) as LoginRequest;
    const { password } = body;

    // Validate input
    if (!password || typeof password !== 'string') {
      return NextResponse.json(
        { success: false, error: 'Password is required' },
        { status: 400 }
      );
    }

    // Verify password
    if (!verifyPassword(password)) {
      // Record failed attempt for rate limiting
      recordFailedAttempt(clientIp);
      return NextResponse.json(
        { success: false, error: 'Invalid password' },
        { status: 401 }
      );
    }

    // Successful login - clear rate limit
    clearRateLimit(clientIp);

    // Create and set session
    const token = await createSessionToken();
    await setSessionCookie(token);

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Auth error:', error);
    return NextResponse.json(
      { success: false, error: 'Authentication failed' },
      { status: 500 }
    );
  }
}

// DELETE /api/auth - Logout
export async function DELETE(): Promise<NextResponse<LoginResponse>> {
  try {
    await clearSession();
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Logout error:', error);
    return NextResponse.json(
      { success: false, error: 'Logout failed' },
      { status: 500 }
    );
  }
}
