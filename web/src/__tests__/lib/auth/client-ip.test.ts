/** @jest-environment node */
import { getClientIp } from '@/lib/auth/client-ip';

const headers = (xff?: string) => new Headers(xff ? { 'x-forwarded-for': xff } : {});

describe('getClientIp', () => {
  it('uses the socket address Next.js fills in when there is no proxy', () => {
    expect(getClientIp(headers('192.168.1.20'), 0)).toBe('192.168.1.20');
  });

  it('never trusts the left-most (client-supplied) entries', () => {
    // A client sends a forged header; Caddy (1 hop) appends the real address
    expect(getClientIp(headers('1.2.3.4, 203.0.113.9'), 1)).toBe('203.0.113.9');
  });

  it('counts trusted hops from the right', () => {
    // client → CDN → Caddy → web: Caddy appends the CDN's address
    expect(getClientIp(headers('6.6.6.6, 198.51.100.7, 10.0.0.2'), 2)).toBe('198.51.100.7');
  });

  it('falls back to the first entry when the chain is shorter than the hop count', () => {
    expect(getClientIp(headers('198.51.100.7'), 3)).toBe('198.51.100.7');
  });

  it('returns "unknown" without the header', () => {
    expect(getClientIp(headers(), 0)).toBe('unknown');
  });
});
