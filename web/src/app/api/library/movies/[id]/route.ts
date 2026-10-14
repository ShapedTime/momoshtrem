import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { ApiErrorBody } from '@/lib/api/contract';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * DELETE /api/library/movies/[id]
 * Remove a movie from the library.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string } | ApiErrorBody>> {
  const authError = await requireAuth();
  if (authError) return authError;

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
