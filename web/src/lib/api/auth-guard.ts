/**
 * Authentication guard for API routes.
 * Provides a consistent way to protect API endpoints.
 */

import { NextResponse } from 'next/server';
import { getSession } from '@/lib/auth/session';

/**
 * Check if the request is authenticated.
 * Returns a 401 response if not authenticated, or null if authenticated.
 *
 * Usage in API routes:
 * ```typescript
 * export async function GET() {
 *   const authError = await requireAuth();
 *   if (authError) return authError;
 *   // ... rest of handler
 * }
 * ```
 *
 * @returns NextResponse with 401 status if not authenticated, null if authenticated
 */
export async function requireAuth(): Promise<NextResponse<{ error: string }> | null> {
  const session = await getSession();

  if (!session || !session.authenticated) {
    return NextResponse.json(
      { error: 'Unauthorized' },
      { status: 401 }
    );
  }

  return null;
}
