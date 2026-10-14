import type { MetadataRoute } from 'next';

// PWA manifest: installable on the iOS home screen and desktop Chrome.
// Icons are rendered from the design's logo by scripts/render-icons.mjs.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: 'momoshtrem',
    short_name: 'momoshtrem',
    description: 'Self-hosted library and torrent streaming for Infuse on Apple TV',
    start_url: '/',
    display: 'standalone',
    background_color: '#0B0D12',
    theme_color: '#0B0D12',
    icons: [
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      { src: '/icons/maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  };
}
