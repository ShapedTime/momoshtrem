import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { MovieDetails } from '@/types/tmdb';

interface RouteParams {
  params: Promise<{ id: string }>;
}

export async function GET(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<MovieDetails | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId) || movieId <= 0) {
      throw new ValidationError('Invalid movie ID');
    }

    const movie = await tmdbClient.getMovie(movieId);
    return NextResponse.json(movie);
  } catch (error) {
    console.error('TMDB movie error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch movie details' },
      { status: 500 }
    );
  }
}
