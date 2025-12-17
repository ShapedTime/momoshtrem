import { timingSafeEqual } from 'crypto';

// Timing-safe password comparison to prevent timing attacks
export function verifyPassword(input: string): boolean {
  const appPassword = process.env.APP_PASSWORD;

  if (!appPassword) {
    console.error('APP_PASSWORD environment variable is not set');
    return false;
  }

  // Convert strings to buffers for timing-safe comparison
  const inputBuffer = Buffer.from(input);
  const passwordBuffer = Buffer.from(appPassword);

  // If lengths differ, compare anyway to maintain constant time
  // but always return false
  if (inputBuffer.length !== passwordBuffer.length) {
    // Create same-length buffer to maintain timing consistency
    const paddedInput = Buffer.alloc(passwordBuffer.length);
    inputBuffer.copy(paddedInput);
    timingSafeEqual(paddedInput, passwordBuffer);
    return false;
  }

  return timingSafeEqual(inputBuffer, passwordBuffer);
}
