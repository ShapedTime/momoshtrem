/**
 * Best-effort client address for rate limiting.
 *
 * Route handlers can't see the socket. Next.js copies the socket address into
 * `X-Forwarded-For` only when the header is missing, and every trusted proxy
 * appends the address it received the request from. So the client sits
 * `TRUSTED_PROXY_HOPS` entries from the right (or is the rightmost entry when
 * there's no proxy). Entries further left are client-supplied and ignored.
 *
 * With no proxy (hops = 0) a client can still forge the header; the global
 * failure ceiling in rate-limit.ts is what stops guessing in that case.
 */
export function trustedProxyHops(): number {
  const parsed = Number.parseInt(process.env.TRUSTED_PROXY_HOPS ?? '0', 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

export function getClientIp(headers: Headers, hops: number = trustedProxyHops()): string {
  const chain = (headers.get('x-forwarded-for') ?? '')
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
  if (chain.length === 0) return 'unknown';
  return chain[Math.max(0, chain.length - Math.max(1, hops))];
}
