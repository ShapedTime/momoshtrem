import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { LibraryShow } from '@/types/momoshtrem';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * GET /api/library/shows/[id]
 * Get a single show from the library with all seasons and episodes.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<LibraryShow | { error: string }>> {
  try {
    const { id } = await params;
    const showId = parseInt(id, 10);

    if (isNaN(showId)) {
      throw new ValidationError('Invalid show ID');
    }

    const show = await momoshtremClient.getShow(showId);
    return NextResponse.json(show);
  } catch (error) {
    console.error('Library show fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch show' },
      { status: 500 }
    );
  }
}

/**
 * DELETE /api/library/shows/[id]
 * Remove a show from the library.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  try {
    const { id } = await params;
    const showId = parseInt(id, 10);

    if (isNaN(showId)) {
      throw new ValidationError('Invalid show ID');
    }

    await momoshtremClient.deleteShow(showId);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Library show delete error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to delete show' },
      { status: 500 }
    );
  }
}
