import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { ShowAssignmentResponse } from '@/types/momoshtrem';

interface RouteParams {
  params: Promise<{ id: string }>;
}

interface AssignBody {
  magnet_uri: string;
}

/**
 * POST /api/library/shows/[id]/assign
 * Assign a torrent to a show (auto-matches episodes by filename).
 * Returns detailed results of which episodes were matched.
 */
export async function POST(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<ShowAssignmentResponse | { error: string }>> {
  try {
    const { id } = await params;
    const showId = parseInt(id, 10);

    if (isNaN(showId)) {
      throw new ValidationError('Invalid show ID');
    }

    const body = (await request.json()) as AssignBody;
    const { magnet_uri } = body;

    if (!magnet_uri) {
      throw new ValidationError('Magnet URI is required');
    }

    const result = await momoshtremClient.assignShowTorrent(showId, magnet_uri);
    return NextResponse.json(result, { status: 201 });
  } catch (error) {
    console.error('Show torrent assign error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to assign torrent to show' },
      { status: 500 }
    );
  }
}
