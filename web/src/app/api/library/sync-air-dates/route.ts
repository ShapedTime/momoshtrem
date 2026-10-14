import { NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';

/**
 * POST /api/library/sync-air-dates
 * Trigger a manual air date sync from TMDB.
 * Returns immediately; sync runs in background.
 */
export async function POST(): Promise<NextResponse<{ message: string } | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    await momoshtremClient.triggerAirDateSync();
    return NextResponse.json(
      { message: 'Air date sync started' },
      { status: 202 }
    );
  } catch (error) {
    console.error('Air date sync trigger error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to trigger air date sync' },
      { status: 500 }
    );
  }
}
