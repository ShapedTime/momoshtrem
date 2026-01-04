import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { LibraryMovie } from '@/types/momoshtrem';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * GET /api/library/movies/[id]
 * Get a single movie from the library.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<LibraryMovie | { error: string }>> {
  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId)) {
      throw new ValidationError('Invalid movie ID');
    }

    const movie = await momoshtremClient.getMovie(movieId);
    return NextResponse.json(movie);
  } catch (error) {
    console.error('Library movie fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch movie' },
      { status: 500 }
    );
  }
}

/**
 * DELETE /api/library/movies/[id]
 * Remove a movie from the library.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId)) {
      throw new ValidationError('Invalid movie ID');
    }

    await momoshtremClient.deleteMovie(movieId);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Library movie delete error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to delete movie' },
      { status: 500 }
    );
  }
}
