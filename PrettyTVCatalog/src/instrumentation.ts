/**
 * Next.js instrumentation - runs once at server startup.
 * Use for environment validation, telemetry setup, etc.
 */
export async function register() {
  // Only validate on server (not during build)
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    const { validateEnvironment } = await import('@/lib/env');
    validateEnvironment();
    console.log('[STARTUP] Environment validation passed');
  }
}
