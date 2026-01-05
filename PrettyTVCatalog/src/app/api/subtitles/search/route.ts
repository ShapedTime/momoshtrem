import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';

/**
 * GET /api/subtitles/search
 * Search for subtitles on OpenSubtitles.
 */
export async function GET(
  request: NextRequest
): Promise<NextResponse<{ results: unknown[] } | { error: string }>> {
  try {
    const searchParams = request.nextUrl.searchParams;
    const tmdbId = searchParams.get('tmdb_id');
    const type = searchParams.get('type') as 'movie' | 'episode';
    const languages = searchParams.get('languages');
    const season = searchParams.get('season');
    const episode = searchParams.get('episode');

    if (!tmdbId || !type || !languages) {
      return NextResponse.json(
        { error: 'Missing required parameters: tmdb_id, type, languages' },
        { status: 400 }
      );
    }

    const languageList = languages.split(',').filter(Boolean);
    const results = await momoshtremClient.searchSubtitles(
      parseInt(tmdbId, 10),
      type,
      languageList,
      season ? parseInt(season, 10) : undefined,
      episode ? parseInt(episode, 10) : undefined
    );

    return NextResponse.json({ results });
  } catch (error) {
    console.error('Subtitle search error:', error);

    if (isAppError(error)) {
      return NextResponse.json({ error: error.message }, { status: error.statusCode });
    }

    return NextResponse.json({ error: 'Failed to search subtitles' }, { status: 500 });
  }
}
