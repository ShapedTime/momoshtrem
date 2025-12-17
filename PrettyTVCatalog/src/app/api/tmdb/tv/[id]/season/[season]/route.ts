import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError, ValidationError } from '@/lib/errors';
import type { SeasonDetails } from '@/types/tmdb';

interface RouteParams {
  params: Promise<{ id: string; season: string }>;
}

export async function GET(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<SeasonDetails | { error: string }>> {
  try {
    const { id, season } = await params;
    const showId = parseInt(id, 10);
    const seasonNumber = parseInt(season, 10);

    if (isNaN(showId) || showId <= 0) {
      throw new ValidationError('Invalid TV show ID');
    }

    if (isNaN(seasonNumber) || seasonNumber < 0) {
      throw new ValidationError('Invalid season number');
    }

    const seasonDetails = await tmdbClient.getSeason(showId, seasonNumber);
    return NextResponse.json(seasonDetails);
  } catch (error) {
    console.error('TMDB season error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch season details' },
      { status: 500 }
    );
  }
}
