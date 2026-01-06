import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { TVShowDetails } from '@/types/tmdb';

interface RouteParams {
  params: Promise<{ id: string }>;
}

export async function GET(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<TVShowDetails | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const showId = parseInt(id, 10);

    if (isNaN(showId) || showId <= 0) {
      throw new ValidationError('Invalid TV show ID');
    }

    const show = await tmdbClient.getTVShow(showId);
    return NextResponse.json(show);
  } catch (error) {
    console.error('TMDB TV show error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch TV show details' },
      { status: 500 }
    );
  }
}
