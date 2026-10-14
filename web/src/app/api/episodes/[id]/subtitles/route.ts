import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { Subtitle } from '@/types/subtitle';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * GET /api/episodes/:id/subtitles
 * Get all subtitles for an episode.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ subtitles: Subtitle[] } | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const episodeId = parseInt(id, 10);

    if (isNaN(episodeId)) {
      return NextResponse.json({ error: 'Invalid episode ID' }, { status: 400 });
    }

    const subtitles = await momoshtremClient.getEpisodeSubtitles(episodeId);

    return NextResponse.json({ subtitles });
  } catch (error) {
    console.error('Get episode subtitles error:', error);

    if (isAppError(error)) {
      return NextResponse.json({ error: error.message }, { status: error.statusCode });
    }

    return NextResponse.json({ error: 'Failed to fetch episode subtitles' }, { status: 500 });
  }
}
