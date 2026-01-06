import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { RecentlyAiredResponse } from '@/types/momoshtrem';

/**
 * GET /api/library/recently-aired
 * Get recently aired episodes from library shows.
 * Query params:
 *   - lookback_days: Number of days to look back (default: 30, max: 90)
 */
export async function GET(
  request: NextRequest
): Promise<NextResponse<RecentlyAiredResponse | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { searchParams } = new URL(request.url);
    const lookbackDays = parseInt(searchParams.get('lookback_days') || '30', 10);

    // Validate lookback_days
    const validLookbackDays = Math.min(Math.max(lookbackDays, 1), 90);

    const response = await momoshtremClient.getRecentlyAiredEpisodes(validLookbackDays);
    return NextResponse.json(response);
  } catch (error) {
    console.error('Recently aired fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch recently aired episodes' },
      { status: 500 }
    );
  }
}
