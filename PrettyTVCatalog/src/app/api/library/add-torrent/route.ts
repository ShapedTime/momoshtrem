import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { AddTorrentResponse } from '@/types/momoshtrem';

interface AddTorrentBody {
  magnet_uri: string;
  media_type: 'movie' | 'tv';
  tmdb_id: number;
}

/**
 * POST /api/library/add-torrent
 * Combined endpoint for adding a torrent with auto-library management.
 *
 * Flow:
 * 1. Check if item exists in library (by TMDB ID)
 * 2. If not, add to library
 * 3. Assign the torrent
 * 4. Return assignment result
 */
export async function POST(
  request: NextRequest
): Promise<NextResponse<AddTorrentResponse | { error: string }>> {
  try {
    const body = (await request.json()) as AddTorrentBody;
    const { magnet_uri, media_type, tmdb_id } = body;

    // Validate required fields
    if (!magnet_uri) {
      throw new ValidationError('Magnet URI is required');
    }

    if (!media_type || !['movie', 'tv'].includes(media_type)) {
      throw new ValidationError('Media type must be "movie" or "tv"');
    }

    if (!tmdb_id || typeof tmdb_id !== 'number') {
      throw new ValidationError('TMDB ID is required and must be a number');
    }

    if (media_type === 'movie') {
      const result = await momoshtremClient.addMovieTorrent(tmdb_id, magnet_uri);

      return NextResponse.json(
        {
          success: true,
          added_to_library: result.addedToLibrary,
          library_id: result.libraryId,
          media_type: 'movie',
          assignment: result.assignment.assignment,
        },
        { status: 201 }
      );
    } else {
      const result = await momoshtremClient.addShowTorrent(tmdb_id, magnet_uri);

      return NextResponse.json(
        {
          success: true,
          added_to_library: result.addedToLibrary,
          library_id: result.libraryId,
          media_type: 'tv',
          summary: result.assignment.summary,
          matched: result.assignment.matched,
          unmatched: result.assignment.unmatched,
        },
        { status: 201 }
      );
    }
  } catch (error) {
    console.error('Add torrent error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to add torrent' },
      { status: 500 }
    );
  }
}
