import { NextRequest, NextResponse } from 'next/server';
import { tmdbClient } from '@/lib/api/tmdb';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type {
  DiscoverResults,
  MovieSearchResult,
  TVSearchResult,
  SortOption,
} from '@/types/tmdb';

type DiscoverResponse = DiscoverResults<MovieSearchResult | TVSearchResult>;

export async function GET(
  request: NextRequest
): Promise<NextResponse<DiscoverResponse | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const searchParams = request.nextUrl.searchParams;
    const type = searchParams.get('type');
    const genreId = searchParams.get('genre');
    const sortBy = searchParams.get('sort') as SortOption | null;
    const page = searchParams.get('page');

    if (!type || (type !== 'movie' && type !== 'tv')) {
      return NextResponse.json(
        { error: 'Invalid type parameter. Must be "movie" or "tv"' },
        { status: 400 }
      );
    }

    const options = {
      genreId: genreId ? parseInt(genreId, 10) : undefined,
      sortBy: sortBy || 'popularity.desc',
      page: page ? parseInt(page, 10) : 1,
    };

    const results =
      type === 'movie'
        ? await tmdbClient.discoverMovies(options)
        : await tmdbClient.discoverTVShows(options);

    return NextResponse.json(results);
  } catch (error) {
    console.error('TMDB discover error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to discover content' },
      { status: 500 }
    );
  }
}
