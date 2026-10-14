import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { MovieAssignmentResponse } from '@/types/momoshtrem';

interface RouteParams {
  params: Promise<{ id: string }>;
}

interface AssignBody {
  magnet_uri: string;
}

/**
 * POST /api/library/movies/[id]/assign
 * Assign a torrent to a movie (auto-detects best file).
 */
export async function POST(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<MovieAssignmentResponse | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId)) {
      throw new ValidationError('Invalid movie ID');
    }

    const body = (await request.json()) as AssignBody;
    const { magnet_uri } = body;

    if (!magnet_uri) {
      throw new ValidationError('Magnet URI is required');
    }

    const result = await momoshtremClient.assignMovieTorrent(movieId, magnet_uri);
    return NextResponse.json(result, { status: 201 });
  } catch (error) {
    console.error('Movie torrent assign error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to assign torrent to movie' },
      { status: 500 }
    );
  }
}

/**
 * DELETE /api/library/movies/[id]/assign
 * Unassign torrent from a movie.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const movieId = parseInt(id, 10);

    if (isNaN(movieId)) {
      throw new ValidationError('Invalid movie ID');
    }

    await momoshtremClient.unassignMovie(movieId);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Movie torrent unassign error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to unassign torrent from movie' },
      { status: 500 }
    );
  }
}
