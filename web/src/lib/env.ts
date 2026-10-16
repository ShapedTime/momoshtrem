/**
 * Environment configuration validation.
 * Called at application startup to fail fast on missing critical vars.
 */

export class ConfigurationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ConfigurationError';
  }
}

export interface EnvConfig {
  // Required - app will fail to start without these
  APP_PASSWORD: string;
  TMDB_API_KEY: string;
  SESSION_SECRET: string;

  // Optional - warnings logged but app continues
  MOMOSHTREM_URL: string | undefined;
  BACKEND_API_TOKEN: string | undefined;
}

const REQUIRED_VARS = ['APP_PASSWORD', 'TMDB_API_KEY', 'SESSION_SECRET'] as const;

const OPTIONAL_VARS = ['MOMOSHTREM_URL'] as const;

/**
 * Validate all environment variables at startup.
 * @throws ConfigurationError if required variables are missing or SESSION_SECRET is too short
 */
export function validateEnvironment(): EnvConfig {
  const missing: string[] = [];
  const warnings: string[] = [];

  // Check required variables
  for (const varName of REQUIRED_VARS) {
    if (!process.env[varName]) {
      missing.push(varName);
    }
  }

  // Fail fast if required vars missing
  if (missing.length > 0) {
    throw new ConfigurationError(
      `Missing required environment variables: ${missing.join(', ')}\n` +
        `Please set these in your .env file or environment.`
    );
  }

  // SESSION_SECRET must be at least 32 chars: sign-in refuses a shorter one
  if (process.env.SESSION_SECRET!.length < 32) {
    throw new ConfigurationError(
      'SESSION_SECRET must be at least 32 characters.\n' +
        'Generate one with: openssl rand -base64 32'
    );
  }

  // Check optional variables and log warnings
  for (const varName of OPTIONAL_VARS) {
    if (!process.env[varName]) {
      warnings.push(varName);
    }
  }

  if (warnings.length > 0) {
    console.warn(
      `[CONFIG] Optional environment variables not set: ${warnings.join(', ')}\n` +
        `Some features may be unavailable.`
    );
  }

  if (!process.env.BACKEND_API_TOKEN) {
    console.warn(
      '[CONFIG] BACKEND_API_TOKEN is not set. momoshtrem rejects requests without ' +
        'the X-Momoshtrem-Token header when its own BACKEND_API_TOKEN is set; ' +
        'set the same value on both services.'
    );
  }

  if (!process.env.APP_USERNAME) {
    console.warn('[CONFIG] APP_USERNAME is not set; the sign-in username is "admin".');
  }

  const hops = process.env.TRUSTED_PROXY_HOPS;
  if (hops !== undefined && !/^\d+$/.test(hops)) {
    console.warn(`[CONFIG] TRUSTED_PROXY_HOPS must be a whole number (got "${hops}"); using 0.`);
  }

  return {
    APP_PASSWORD: process.env.APP_PASSWORD!,
    TMDB_API_KEY: process.env.TMDB_API_KEY!,
    SESSION_SECRET: process.env.SESSION_SECRET!,
    MOMOSHTREM_URL: process.env.MOMOSHTREM_URL,
    BACKEND_API_TOKEN: process.env.BACKEND_API_TOKEN,
  };
}

// Singleton for validated config
let _validatedEnv: EnvConfig | null = null;

/**
 * Get validated environment config.
 * Validates on first call, returns cached result thereafter.
 */
export function getEnv(): EnvConfig {
  if (!_validatedEnv) {
    _validatedEnv = validateEnvironment();
  }
  return _validatedEnv;
}
