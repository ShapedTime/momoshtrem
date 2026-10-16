import type { Metadata, Viewport } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import { NuqsAdapter } from 'nuqs/adapters/next/app';
import './globals.css';

const geistSans = Geist({ subsets: ['latin'], variable: '--font-geist-sans' });
const geistMono = Geist_Mono({ subsets: ['latin'], variable: '--font-geist-mono' });

export const metadata: Metadata = {
  title: {
    default: 'momoshtrem',
    template: '%s · momoshtrem',
  },
  description: 'Self-hosted library and torrent streaming for Infuse on Apple TV',
  applicationName: 'momoshtrem',
  appleWebApp: {
    capable: true,
    title: 'momoshtrem',
    statusBarStyle: 'black-translucent',
  },
};

export const viewport: Viewport = {
  themeColor: '#0B0D12',
  colorScheme: 'dark',
  viewportFit: 'cover',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    // The font variables live on <html> because tokens.css reads them in :root
    <html lang="en" className={`dark ${geistSans.variable} ${geistMono.variable}`}>
      <body className="min-h-screen">
        {/* URL state for nuqs (Discover's filters, #100) */}
        <NuqsAdapter>{children}</NuqsAdapter>
      </body>
    </html>
  );
}
