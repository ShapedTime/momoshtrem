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
  JACKETT_URL: string | undefined;
  JACKETT_API_KEY: string | undefined;
  MOMOSHTREM_URL: string | undefined;
}

const REQUIRED_VARS = ['APP_PASSWORD', 'TMDB_API_KEY', 'SESSION_SECRET'] as const;

const OPTIONAL_VARS = [
  'JACKETT_URL',
  'JACKETT_API_KEY',
  'MOMOSHTREM_URL',
] as const;

/**
 * Validate all environment variables at startup.
 * @throws ConfigurationError if required variables are missing
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

  // SESSION_SECRET validation - must be at least 32 chars
  if (process.env.SESSION_SECRET && process.env.SESSION_SECRET.length < 32) {
    console.warn(
      '[CONFIG] SESSION_SECRET should be at least 32 characters for security.'
    );
  }

  return {
    APP_PASSWORD: process.env.APP_PASSWORD!,
    TMDB_API_KEY: process.env.TMDB_API_KEY!,
    SESSION_SECRET: process.env.SESSION_SECRET!,
    JACKETT_URL: process.env.JACKETT_URL,
    JACKETT_API_KEY: process.env.JACKETT_API_KEY,
    MOMOSHTREM_URL: process.env.MOMOSHTREM_URL,
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
