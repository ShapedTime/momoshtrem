import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * DELETE /api/episodes/[id]/assign
 * Unassign torrent from a specific episode.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  try {
    const { id } = await params;
    const episodeId = parseInt(id, 10);

    if (isNaN(episodeId)) {
      throw new ValidationError('Invalid episode ID');
    }

    await momoshtremClient.unassignEpisodeTorrent(episodeId);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Episode unassign error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to unassign episode torrent' },
      { status: 500 }
    );
  }
}
