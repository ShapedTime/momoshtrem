import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import type { TorrentStatus } from '@/types/torrent';

interface RouteParams {
  params: Promise<{ hash: string }>;
}

/**
 * GET /api/torrents/:hash
 * Get detailed status for a specific torrent.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<TorrentStatus | { error: string }>> {
  try {
    const { hash } = await params;

    if (!hash) {
      throw new ValidationError('Torrent hash is required');
    }

    const status = await momoshtremClient.getTorrentStatus(hash);
    return NextResponse.json(status);
  } catch (error) {
    console.error('Torrent status fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch torrent status' },
      { status: 500 }
    );
  }
}

/**
 * DELETE /api/torrents/:hash
 * Remove a torrent.
 * Query params:
 *   - delete_data: If 'true', also deletes downloaded data
 */
export async function DELETE(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  try {
    const { hash } = await params;

    if (!hash) {
      throw new ValidationError('Torrent hash is required');
    }

    const deleteData = request.nextUrl.searchParams.get('delete_data') === 'true';

    await momoshtremClient.removeTorrent(hash, deleteData);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Torrent remove error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to remove torrent' },
      { status: 500 }
    );
  }
}
