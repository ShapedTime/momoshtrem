import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'momoshtrem',
  description: 'Self-hosted library and torrent streaming for Infuse on Apple TV',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="min-h-screen">{children}</body>
    </html>
  );
}
