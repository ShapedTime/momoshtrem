/**
 * Torrent parsing utilities using parse-torrent library.
 * Handles conversion of .torrent file URLs to magnet URIs.
 */

import parseTorrent, { toMagnetURI } from 'parse-torrent';

// ============================================
// Configuration
// ============================================

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
