/**
 * Torrent parsing utilities using parse-torrent library.
 * Handles conversion of .torrent file URLs to magnet URIs.
 */

import parseTorrent, { toMagnetURI } from 'parse-torrent';

// ============================================
// Configuration
// ============================================

/** Regex pattern to detect private/internal IP addresses */
const PRIVATE_IP_PATTERN =
  /^(localhost|127\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|0\.0\.0\.0|\[::1\]|\[::ffff:127\.\d{1,3}\.\d{1,3}\.\d{1,3}\])$/i;

/**
 * Validate that a URL is safe to fetch (SSRF protection).
 * Blocks internal/private network addresses and non-HTTP protocols.
 *
 * @param url - URL to validate
 * @returns Object with valid status and error message if invalid
 */
function validateExternalUrl(url: string): { valid: boolean; error?: string } {
  try {
    const parsed = new URL(url);

    // Only allow HTTP and HTTPS protocols
    if (!['http:', 'https:'].includes(parsed.protocol)) {
      return { valid: false, error: `Blocked protocol: ${parsed.protocol}` };
    }

    // Block private/internal IP addresses to prevent SSRF
    if (PRIVATE_IP_PATTERN.test(parsed.hostname)) {
      return { valid: false, error: 'Blocked internal address' };
    }

    return { valid: true };
  } catch {
    return { valid: false, error: 'Invalid URL format' };
  }
}

export const TORRENT_CONVERSION_CONFIG = {
  /** Timeout for fetching individual .torrent files (ms) */
  fetchTimeout: 5000,
  /** Maximum concurrent .torrent file fetches */
  maxConcurrency: 5,
  /** Total timeout for all conversions in a batch (ms) */
  batchTimeout: 10000,
} as const;

// ============================================
// Types
// ============================================

export interface ConversionResult {
  success: boolean;
  magnetUri: string | null;
  error?: string;
}

// ============================================
// Conversion Functions
// ============================================

/**
 * Fetch a .torrent file and convert it to a magnet URI.
 *
 * @param torrentUrl - URL to a .torrent file
 * @returns Promise resolving to conversion result
 */
export async function convertTorrentUrlToMagnet(
  torrentUrl: string
): Promise<ConversionResult> {
  // SSRF protection: validate URL before fetching
  const validation = validateExternalUrl(torrentUrl);
  if (!validation.valid) {
    return {
      success: false,
      magnetUri: null,
      error: validation.error,
    };
  }

  try {
    // Fetch the .torrent file with timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      TORRENT_CONVERSION_CONFIG.fetchTimeout
    );

    const response = await fetch(torrentUrl, {
      signal: controller.signal,
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; PrettyTVCatalog/1.0)',
      },
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      return {
        success: false,
        magnetUri: null,
        error: `HTTP ${response.status}: ${response.statusText}`,
      };
    }

    // Get the torrent file as ArrayBuffer then convert to Buffer
    const arrayBuffer = await response.arrayBuffer();
    const buffer = Buffer.from(arrayBuffer);

    // Parse the torrent file
    const parsed = await parseTorrent(buffer);

    if (!parsed || !parsed.infoHash) {
      return {
        success: false,
        magnetUri: null,
        error: 'Failed to parse torrent: no infoHash',
      };
    }

    // Convert to magnet URI (includes trackers from the .torrent file)
    const magnetUri = toMagnetURI(parsed);

    return { success: true, magnetUri };
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') {
      return {
        success: false,
        magnetUri: null,
        error: 'Fetch timeout',
      };
    }

    return {
      success: false,
      magnetUri: null,
      error: error instanceof Error ? error.message : 'Unknown error',
    };
  }
}

/**
 * Convert multiple .torrent URLs to magnet URIs in parallel with concurrency control.
 *
 * @param urls - Array of .torrent file URLs
 * @returns Promise resolving to Map of URL -> ConversionResult
 */
export async function batchConvertTorrentUrls(
  urls: string[]
): Promise<Map<string, ConversionResult>> {
  const results = new Map<string, ConversionResult>();

  if (urls.length === 0) {
    return results;
  }

  // Track whether batch has timed out
  let batchTimedOut = false;
  const batchTimeoutId = setTimeout(() => {
    batchTimedOut = true;
  }, TORRENT_CONVERSION_CONFIG.batchTimeout);

  // Process in chunks to limit concurrency
  const chunks: string[][] = [];
  for (let i = 0; i < urls.length; i += TORRENT_CONVERSION_CONFIG.maxConcurrency) {
    chunks.push(urls.slice(i, i + TORRENT_CONVERSION_CONFIG.maxConcurrency));
  }

  for (const chunk of chunks) {
    // Check batch timeout before processing each chunk
    if (batchTimedOut) {
      break;
    }

    const chunkResults = await Promise.all(
      chunk.map(async (url) => {
        if (batchTimedOut) {
          return {
            url,
            result: { success: false, magnetUri: null, error: 'Batch timeout' } as ConversionResult,
          };
        }
        const result = await convertTorrentUrlToMagnet(url);
        return { url, result };
      })
    );

    for (const { url, result } of chunkResults) {
      results.set(url, result);
    }
  }

  clearTimeout(batchTimeoutId);

  // Mark any unprocessed URLs as timed out
  for (const url of urls) {
    if (!results.has(url)) {
      results.set(url, {
        success: false,
        magnetUri: null,
        error: 'Batch timeout',
      });
    }
  }

  return results;
}
