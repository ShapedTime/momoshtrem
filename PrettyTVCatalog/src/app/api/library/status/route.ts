import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { LibraryStatus } from '@/types/momoshtrem';

interface StatusResponse {
  status: LibraryStatus;
  library_id?: number;
  has_assignment: boolean;
}

/**
 * GET /api/library/status?media_type=movie&tmdb_id=550
 * Check if an item is in the library and its assignment status.
 */
export async function GET(
  request: NextRequest
): Promise<NextResponse<StatusResponse | { error: string }>> {
  try {
    const { searchParams } = new URL(request.url);
    const mediaType = searchParams.get('media_type');
    const tmdbIdParam = searchParams.get('tmdb_id');

    if (!mediaType || !['movie', 'tv'].includes(mediaType)) {
      throw new ValidationError('media_type query param must be "movie" or "tv"');
    }

    if (!tmdbIdParam) {
      throw new ValidationError('tmdb_id query param is required');
    }

    const tmdbId = parseInt(tmdbIdParam, 10);
    if (isNaN(tmdbId)) {
      throw new ValidationError('tmdb_id must be a number');
    }

    if (mediaType === 'movie') {
      const movie = await momoshtremClient.findMovieByTmdbId(tmdbId);

      if (!movie) {
        return NextResponse.json({
          status: 'not_in_library',
          has_assignment: false,
        });
      }

      return NextResponse.json({
        status: movie.has_assignment ? 'has_assignment' : 'in_library',
        library_id: movie.id,
        has_assignment: movie.has_assignment,
      });
    } else {
      const show = await momoshtremClient.findShowByTmdbId(tmdbId);

      if (!show) {
        return NextResponse.json({
          status: 'not_in_library',
          has_assignment: false,
        });
      }

      // Check if any episode has assignment
      const hasAnyAssignment = show.seasons?.some((season) =>
        season.episodes?.some((episode) => episode.has_assignment)
      ) ?? false;

      return NextResponse.json({
        status: hasAnyAssignment ? 'has_assignment' : 'in_library',
        library_id: show.id,
        has_assignment: hasAnyAssignment,
      });
    }
  } catch (error) {
    console.error('Library status check error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to check library status' },
      { status: 500 }
    );
  }
}
