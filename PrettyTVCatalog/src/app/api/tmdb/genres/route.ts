import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError } from '@/lib/errors';
import type { Genre } from '@/types/tmdb';

interface GenresResponse {
  genres: Genre[];
}

export async function GET(
  request: NextRequest
): Promise<NextResponse<GenresResponse | { error: string }>> {
  try {
    const searchParams = request.nextUrl.searchParams;
    const type = searchParams.get('type');

    if (!type || (type !== 'movie' && type !== 'tv')) {
      return NextResponse.json(
        { error: 'Invalid type parameter. Must be "movie" or "tv"' },
        { status: 400 }
      );
    }

    const genres =
      type === 'movie'
        ? await tmdbClient.getMovieGenres()
        : await tmdbClient.getTVGenres();

    return NextResponse.json({ genres });
  } catch (error) {
    console.error('TMDB genres error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch genres' },
      { status: 500 }
    );
  }
}
