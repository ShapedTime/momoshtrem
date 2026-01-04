import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';

interface RouteParams {
  params: Promise<{ hash: string }>;
}

/**
 * POST /api/torrents/:hash/resume
 * Resume a paused torrent.
 */
export async function POST(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean; message: string } | { error: string }>> {
  try {
    const { hash } = await params;

    if (!hash) {
      throw new ValidationError('Torrent hash is required');
    }

    await momoshtremClient.resumeTorrent(hash);
    return NextResponse.json({ success: true, message: 'Torrent resumed' });
  } catch (error) {
    console.error('Torrent resume error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to resume torrent' },
      { status: 500 }
    );
  }
}
