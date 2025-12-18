import { NextRequest, NextResponse } from 'next/server';
import { distribytedClient } from '@/lib/api/distribyted';
import { isAppError, ValidationError } from '@/lib/errors';

interface AddTorrentBody {
  magnetUri: string;
  route?: string;
}

export async function POST(
  request: NextRequest
): Promise<NextResponse<{ success: boolean } | { error: string }>> {
  try {
    const body = (await request.json()) as AddTorrentBody;
    const { magnetUri, route } = body;

    if (!magnetUri) {
      throw new ValidationError('Magnet URI is required');
    }

    await distribytedClient.addTorrent(magnetUri, route);

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Distribyted add error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to add torrent' },
      { status: 500 }
    );
  }
}
