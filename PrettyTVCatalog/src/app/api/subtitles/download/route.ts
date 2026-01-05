import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import type { DownloadSubtitleRequest, Subtitle } from '@/types/subtitle';

const VALID_ITEM_TYPES = ['movie', 'episode'] as const;

function isValidItemType(type: unknown): type is 'movie' | 'episode' {
  return typeof type === 'string' && VALID_ITEM_TYPES.includes(type as 'movie' | 'episode');
}

/**
 * POST /api/subtitles/download
 * Download and save a subtitle from OpenSubtitles.
 */
export async function POST(
  request: NextRequest
): Promise<NextResponse<{ subtitle: Subtitle } | { error: string }>> {
  try {
    const body = await request.json();

    // Validate item_type
    if (!isValidItemType(body.item_type)) {
      return NextResponse.json(
        { error: 'Invalid item_type: must be "movie" or "episode"' },
        { status: 400 }
      );
    }

    // Validate item_id
    if (typeof body.item_id !== 'number' || body.item_id <= 0 || !Number.isInteger(body.item_id)) {
      return NextResponse.json(
        { error: 'Invalid item_id: must be a positive integer' },
        { status: 400 }
      );
    }

    // Validate file_id
    if (typeof body.file_id !== 'number' || body.file_id <= 0 || !Number.isInteger(body.file_id)) {
      return NextResponse.json(
        { error: 'Invalid file_id: must be a positive integer' },
        { status: 400 }
      );
    }

    // Validate language_code
    if (typeof body.language_code !== 'string' || body.language_code.length === 0) {
      return NextResponse.json(
        { error: 'Invalid language_code: must be a non-empty string' },
        { status: 400 }
      );
    }

    const downloadRequest: DownloadSubtitleRequest = {
      item_type: body.item_type,
      item_id: body.item_id,
      file_id: body.file_id,
      language_code: body.language_code,
      language_name: typeof body.language_name === 'string' ? body.language_name : body.language_code,
    };

    const subtitle = await momoshtremClient.downloadSubtitle(downloadRequest);

    return NextResponse.json({ subtitle });
  } catch (error) {
    console.error('Subtitle download error:', error);

    if (isAppError(error)) {
      return NextResponse.json({ error: error.message }, { status: error.statusCode });
    }

    return NextResponse.json({ error: 'Failed to download subtitle' }, { status: 500 });
  }
}
