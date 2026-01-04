import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { LibraryShow } from '@/types/momoshtrem';

/**
 * GET /api/library/shows
 * List all shows in the library.
 */
export async function GET(): Promise<NextResponse<{ shows: LibraryShow[] } | { error: string }>> {
  try {
    const shows = await momoshtremClient.getShows();
    return NextResponse.json({ shows });
  } catch (error) {
    console.error('Library shows fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch shows' },
      { status: 500 }
    );
  }
}

interface AddShowBody {
  tmdb_id: number;
}

/**
 * POST /api/library/shows
 * Add a show to the library by TMDB ID.
 * Automatically fetches all seasons and episodes from TMDB.
 */
export async function POST(
  request: NextRequest
): Promise<NextResponse<LibraryShow | { error: string }>> {
  try {
    const body = (await request.json()) as AddShowBody;
    const { tmdb_id } = body;

    if (!tmdb_id || typeof tmdb_id !== 'number') {
      throw new ValidationError('TMDB ID is required and must be a number');
    }

    const show = await momoshtremClient.addShow(tmdb_id);
    return NextResponse.json(show, { status: 201 });
  } catch (error) {
    console.error('Library show add error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to add show to library' },
      { status: 500 }
    );
  }
}
