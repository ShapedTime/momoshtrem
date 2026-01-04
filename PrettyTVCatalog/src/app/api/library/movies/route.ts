import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { LibraryMovie } from '@/types/momoshtrem';

/**
 * GET /api/library/movies
 * List all movies in the library.
 */
export async function GET(): Promise<NextResponse<{ movies: LibraryMovie[] } | { error: string }>> {
  try {
    const movies = await momoshtremClient.getMovies();
    return NextResponse.json({ movies });
  } catch (error) {
    console.error('Library movies fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch movies' },
      { status: 500 }
    );
  }
}

interface AddMovieBody {
  tmdb_id: number;
}

/**
 * POST /api/library/movies
 * Add a movie to the library by TMDB ID.
 */
export async function POST(
  request: NextRequest
): Promise<NextResponse<LibraryMovie | { error: string }>> {
  try {
    const body = (await request.json()) as AddMovieBody;
    const { tmdb_id } = body;

    if (!tmdb_id || typeof tmdb_id !== 'number') {
      throw new ValidationError('TMDB ID is required and must be a number');
    }

    const movie = await momoshtremClient.addMovie(tmdb_id);
    return NextResponse.json(movie, { status: 201 });
  } catch (error) {
    console.error('Library movie add error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to add movie to library' },
      { status: 500 }
    );
  }
}
