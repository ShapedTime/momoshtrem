import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { RefreshShowResult } from '@/types/momoshtrem';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * POST /api/library/shows/[id]/refresh
 * Re-fetch seasons and episodes from TMDB, upserting any missing records.
 */
export async function POST(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<RefreshShowResult | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const showId = parseInt(id, 10);

    if (isNaN(showId)) {
      throw new ValidationError('Invalid show ID');
    }

    const result = await momoshtremClient.refreshShow(showId);
    return NextResponse.json(result);
  } catch (error) {
    console.error('Show refresh error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to refresh show from TMDB' },
      { status: 500 }
    );
  }
}
