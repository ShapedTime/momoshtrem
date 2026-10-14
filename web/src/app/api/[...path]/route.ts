import type { NextRequest } from 'next/server';
import { proxyToBackend } from '@/lib/api/backend-proxy';

/**
 * Thin proxy for the PLAN §6 contract: /api/<path> → momoshtrem /api/<path>.
 * Explicit routes under app/api/ take precedence, so legacy routes keep
 * working; new backend endpoints need no per-route code here.
 */
export const dynamic = 'force-dynamic';

type Context = { params: Promise<{ path: string[] }> };

async function handler(request: NextRequest, { params }: Context): Promise<Response> {
  const { path } = await params;
  return proxyToBackend(request, path);
}

export { handler as GET, handler as POST, handler as PUT, handler as PATCH, handler as DELETE };
