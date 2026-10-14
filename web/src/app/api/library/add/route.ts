import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { LibraryMovie, LibraryShow } from '@/types/momoshtrem';

interface AddToLibraryBody {
  media_type: 'movie' | 'tv';
  tmdb_id: number;
}

interface AddToLibraryResponse {
  success: boolean;
  media_type: 'movie' | 'tv';
  library_id: number;
  title: string;
  year: number;
}

/**
 * POST /api/library/add
 * Add an item to the library without a torrent (for curating "want to watch" list).
 */
export async function POST(
  request: NextRequest
): Promise<NextResponse<AddToLibraryResponse | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const body = (await request.json()) as AddToLibraryBody;
    const { media_type, tmdb_id } = body;

    if (!media_type || !['movie', 'tv'].includes(media_type)) {
      throw new ValidationError('Media type must be "movie" or "tv"');
    }

    if (!tmdb_id || typeof tmdb_id !== 'number') {
      throw new ValidationError('TMDB ID is required and must be a number');
    }

    let result: LibraryMovie | LibraryShow;

    if (media_type === 'movie') {
      // Check if already exists
      const existing = await momoshtremClient.findMovieByTmdbId(tmdb_id);
      if (existing) {
        return NextResponse.json({
          success: true,
          media_type: 'movie',
          library_id: existing.id,
          title: existing.title,
          year: existing.year,
        });
      }

      result = await momoshtremClient.addMovie(tmdb_id);
    } else {
      // Check if already exists
      const existing = await momoshtremClient.findShowByTmdbId(tmdb_id);
      if (existing) {
        return NextResponse.json({
          success: true,
          media_type: 'tv',
          library_id: existing.id,
          title: existing.title,
          year: existing.year,
        });
      }

      result = await momoshtremClient.addShow(tmdb_id);
    }

    return NextResponse.json(
      {
        success: true,
        media_type,
        library_id: result.id,
        title: result.title,
        year: result.year,
      },
      { status: 201 }
    );
  } catch (error) {
    console.error('Add to library error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to add to library' },
      { status: 500 }
    );
  }
}
