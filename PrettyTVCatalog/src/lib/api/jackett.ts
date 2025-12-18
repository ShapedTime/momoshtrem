import { XMLParser } from 'fast-xml-parser';
import { JACKETT_CONFIG, TORZNAB_CATEGORIES, TorznabCategory } from '@/config/jackett';
import { APIError, ValidationError } from '@/lib/errors';
import { batchConvertTorrentUrls } from '@/lib/utils/torrent';
import type { TorrentResult, VideoQuality } from '@/types/jackett';
import { parseQualityFromTitle } from '@/types/jackett';

// ============================================
// Torznab XML Types
// ============================================

interface TorznabAttr {
  '@_name': string;
  '@_value': string;
}

interface TorznabItem {
  title?: string;
  guid?: string | { '#text'?: string };
  pubDate?: string;
  enclosure?: {
    '@_url'?: string;
    '@_length'?: string;
  };
  'torznab:attr'?: TorznabAttr | TorznabAttr[];
  jackettindexer?: string | { '@_id'?: string; '#text'?: string };
}

/**
 * Internal type for items that may need magnet conversion.
 * Some items have magnetUri directly, others have torrentUrl that needs conversion.
 */
interface PendingTorrentResult {
  guid: string;
  title: string;
  size: number;
  seeders: number;
  leechers: number;
  magnetUri: string | null;
  torrentUrl: string | null;
  indexer: string;
  publishDate: string | null;
  quality: VideoQuality;
}

// ============================================
// Jackett API Client
// ============================================

class JackettClient {
  private parser: XMLParser;

  constructor() {
    // Configure XML parser for Torznab format
    this.parser = new XMLParser({
      ignoreAttributes: false,
      attributeNamePrefix: '@_',
      textNodeName: '#text',
    });
  }

  // ----------------------------------------
  // Private Helpers
  // ----------------------------------------

  /**
   * Get API key from environment.
   */
  private getApiKey(): string {
    const apiKey = process.env.JACKETT_API_KEY;
    if (!apiKey) {
      throw new ValidationError('JACKETT_API_KEY environment variable is not set');
    }
    return apiKey;
  }

  /**
   * Build search URL with query parameters.
   */
  private buildSearchUrl(query: string, category?: string): string {
    const url = new URL(JACKETT_CONFIG.searchPath, JACKETT_CONFIG.baseUrl);
    url.searchParams.set('apikey', this.getApiKey());
    url.searchParams.set('t', 'search');
    url.searchParams.set('q', query);
    url.searchParams.set('limit', String(JACKETT_CONFIG.maxResults));

    if (category) {
      url.searchParams.set('cat', category);
    }

    return url.toString();
  }

  /**
   * Get Torznab attributes as array (handles single attr or array).
   */
  private getAttrArray(attrs: TorznabAttr | TorznabAttr[] | undefined): TorznabAttr[] {
    if (!attrs) return [];
    return Array.isArray(attrs) ? attrs : [attrs];
  }

  /**
   * Find Torznab attribute value by name.
   */
  private getTorznabAttr(
    attrs: TorznabAttr | TorznabAttr[] | undefined,
    name: string
  ): string | null {
    const attrArray = this.getAttrArray(attrs);
    const attr = attrArray.find((a) => a['@_name'] === name);
    return attr ? attr['@_value'] : null;
  }

  /**
   * Parse size from Torznab (may be string with units or number).
   */
  private parseSize(sizeValue: string | undefined): number {
    if (!sizeValue) return 0;

    // Try parsing as plain number first
    const numeric = parseInt(sizeValue, 10);
    if (!isNaN(numeric)) return numeric;

    // Parse string with units (e.g., "1.5 GB")
    const match = sizeValue.match(/^([\d.]+)\s*(TB|GB|MB|KB|B)?$/i);
    if (!match) return 0;

    const value = parseFloat(match[1]);
    const unit = (match[2] || 'B').toUpperCase();

    const multipliers: Record<string, number> = {
      B: 1,
      KB: 1024,
      MB: 1024 ** 2,
      GB: 1024 ** 3,
      TB: 1024 ** 4,
    };

    return Math.round(value * (multipliers[unit] || 1));
  }

  /**
   * Extract GUID from item (handles string or object format).
   */
  private extractGuid(item: TorznabItem): string {
    if (!item.guid) return '';
    if (typeof item.guid === 'string') return item.guid;
    return item.guid['#text'] || '';
  }

  /**
   * Extract indexer name from jackettindexer element.
   * Jackett returns indexer info in a dedicated element, not in torznab:attr.
   */
  private extractIndexer(item: TorznabItem): string | null {
    const indexer = item.jackettindexer;
    if (!indexer) return null;
    if (typeof indexer === 'string') return indexer;
    return indexer['#text'] || null;
  }

  /**
   * Build a magnet URI from an info hash and title.
   */
  private buildMagnetUri(infoHash: string, title: string): string {
    const encodedTitle = encodeURIComponent(title);
    return `magnet:?xt=urn:btih:${infoHash}&dn=${encodedTitle}`;
  }

  /**
   * Transform raw XML item to PendingTorrentResult.
   * At this stage, some items may have torrentUrl instead of magnetUri.
   */
  private transformItem(item: TorznabItem): PendingTorrentResult | null {
    const title = item.title;
    if (!title) return null;

    const attrs = item['torznab:attr'];

    // Extract magnet URI from multiple sources (in order of preference):
    // 1. magneturl attribute
    // 2. Construct from infohash attribute
    // 3. Enclosure URL if it's a magnet
    // 4. Store torrentUrl for later conversion
    let magnetUri: string | null = this.getTorznabAttr(attrs, 'magneturl');
    let torrentUrl: string | null = null;

    if (!magnetUri) {
      // Try to construct from infohash
      const infoHash = this.getTorznabAttr(attrs, 'infohash');
      if (infoHash) {
        magnetUri = this.buildMagnetUri(infoHash, title);
      }
    }

    if (!magnetUri) {
      const enclosureUrl = item.enclosure?.['@_url'];
      if (enclosureUrl?.startsWith('magnet:')) {
        magnetUri = enclosureUrl;
      } else if (enclosureUrl) {
        // This is a .torrent URL that needs conversion
        torrentUrl = enclosureUrl;
      }
    }

    // Skip results without any download option
    if (!magnetUri && !torrentUrl) return null;

    // Parse numeric values
    const seedersStr = this.getTorznabAttr(attrs, 'seeders');
    const peersStr = this.getTorznabAttr(attrs, 'peers');
    const sizeAttr = this.getTorznabAttr(attrs, 'size');

    const seeders = seedersStr ? parseInt(seedersStr, 10) : 0;
    const peers = peersStr ? parseInt(peersStr, 10) : 0;
    const leechers = Math.max(0, peers - seeders);

    // Size from Torznab attr or enclosure
    let size = sizeAttr ? this.parseSize(sizeAttr) : 0;
    if (!size && item.enclosure?.['@_length']) {
      size = parseInt(item.enclosure['@_length'], 10) || 0;
    }

    return {
      guid: this.extractGuid(item) || magnetUri || torrentUrl || '',
      title,
      size,
      seeders: isNaN(seeders) ? 0 : seeders,
      leechers: isNaN(leechers) ? 0 : leechers,
      magnetUri,
      torrentUrl,
      indexer: this.extractIndexer(item) || 'Unknown',
      publishDate: item.pubDate || null,
      quality: parseQualityFromTitle(title),
    };
  }

  /**
   * Convert pending results with torrent URLs to final results with magnet URIs.
   * Items that fail conversion are filtered out.
   */
  private async convertPendingResults(
    pending: PendingTorrentResult[]
  ): Promise<TorrentResult[]> {
    // Separate items that already have magnets from those needing conversion
    const withMagnets: TorrentResult[] = [];
    const needsConversion: PendingTorrentResult[] = [];

    for (const item of pending) {
      if (item.magnetUri) {
        withMagnets.push({
          guid: item.guid,
          title: item.title,
          size: item.size,
          seeders: item.seeders,
          leechers: item.leechers,
          magnetUri: item.magnetUri,
          indexer: item.indexer,
          publishDate: item.publishDate,
          quality: item.quality,
        });
      } else if (item.torrentUrl) {
        needsConversion.push(item);
      }
    }

    // If no conversions needed, return early
    if (needsConversion.length === 0) {
      return withMagnets;
    }

    // Batch convert .torrent URLs
    const urlsToConvert = needsConversion
      .map((item) => item.torrentUrl)
      .filter((url): url is string => url !== null);

    const conversionResults = await batchConvertTorrentUrls(urlsToConvert);

    // Add successfully converted items
    for (const item of needsConversion) {
      if (!item.torrentUrl) continue;

      const conversion = conversionResults.get(item.torrentUrl);
      if (conversion?.success && conversion.magnetUri) {
        withMagnets.push({
          guid: item.guid,
          title: item.title,
          size: item.size,
          seeders: item.seeders,
          leechers: item.leechers,
          magnetUri: conversion.magnetUri,
          indexer: item.indexer,
          publishDate: item.publishDate,
          quality: item.quality,
        });
      }
      // Failed conversions are silently dropped
    }

    return withMagnets;
  }

  // ----------------------------------------
  // Public API Methods
  // ----------------------------------------

  /**
   * Search for torrents using Jackett's Torznab API.
   * All returned results will have valid magnet URIs.
   */
  async search(query: string, category?: TorznabCategory): Promise<TorrentResult[]> {
    if (!query.trim()) {
      return [];
    }

    const categoryId = category ? TORZNAB_CATEGORIES[category] : undefined;
    const url = this.buildSearchUrl(query.trim(), categoryId);

    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), JACKETT_CONFIG.timeout);

      const response = await fetch(url, {
        signal: controller.signal,
        headers: {
          Accept: 'application/rss+xml, application/xml, text/xml',
        },
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        throw new APIError(
          `Jackett search failed: ${response.statusText}`,
          response.status
        );
      }

      const xmlText = await response.text();
      const parsed = this.parser.parse(xmlText);

      // Navigate to items in RSS structure
      const channel = parsed?.rss?.channel;
      if (!channel) {
        return [];
      }

      // Handle both single item and array of items
      let items = channel.item;
      if (!items) return [];
      if (!Array.isArray(items)) items = [items];

      // Transform to pending results
      const pendingResults = items
        .map((item: TorznabItem) => this.transformItem(item))
        .filter(
          (result: PendingTorrentResult | null): result is PendingTorrentResult =>
            result !== null
        );

      // Convert .torrent URLs to magnet URIs
      return await this.convertPendingResults(pendingResults);
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        throw new APIError('Jackett search timed out', 504);
      }
      if (error instanceof APIError) throw error;

      throw new APIError(
        `Jackett search failed: ${error instanceof Error ? error.message : 'Unknown error'}`,
        500
      );
    }
  }
}

// Export singleton instance
export const jackettClient = new JackettClient();

// Export class for testing
export { JackettClient };
