import { NextResponse } from 'next/server';
import { TMDB_BASE_URL } from '@/config/tmdb';

export interface HealthResponse {
  status: 'healthy' | 'degraded';
  timestamp: string;
  services?: {
    tmdb?: 'up' | 'down';
  };
}

/**
 * GET /api/health - Health check endpoint for Docker/k8s probes.
 * Returns 200 if application is healthy or degraded.
 */
export async function GET(): Promise<NextResponse<HealthResponse>> {
  const timestamp = new Date().toISOString();

  const response: HealthResponse = {
    status: 'healthy',
    timestamp,
  };

  // Check TMDB connectivity if API key is configured
  if (process.env.TMDB_API_KEY) {
    try {
      const tmdbResponse = await fetch(
        `${TMDB_BASE_URL}/configuration?api_key=${process.env.TMDB_API_KEY}`,
        { next: { revalidate: 60 } }
      );
      response.services = {
        tmdb: tmdbResponse.ok ? 'up' : 'down',
      };
      if (!tmdbResponse.ok) {
        response.status = 'degraded';
      }
    } catch {
      response.services = { tmdb: 'down' };
      response.status = 'degraded';
    }
  }

  return NextResponse.json(response);
}
