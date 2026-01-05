import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import type { Subtitle } from '@/types/subtitle';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * GET /api/movies/:id/subtitles
 * Get all subtitles for a movie.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ subtitles: Subtitle[] } | { error: string }>> {
  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId)) {
      return NextResponse.json({ error: 'Invalid movie ID' }, { status: 400 });
    }

    const subtitles = await momoshtremClient.getMovieSubtitles(movieId);

    return NextResponse.json({ subtitles });
  } catch (error) {
    console.error('Get movie subtitles error:', error);

    if (isAppError(error)) {
      return NextResponse.json({ error: error.message }, { status: error.statusCode });
    }

    return NextResponse.json({ error: 'Failed to fetch movie subtitles' }, { status: 500 });
  }
}
