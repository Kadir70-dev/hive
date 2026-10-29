import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import { connection } from 'next/server';
import { Providers } from '@/components/hive/Providers';
import '../styles.css';

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL || 'https://thehivesociety.ae'),
  title: { default: 'The Hive Society', template: '%s · The Hive Society' },
  description: 'An Abu Dhabi community for men and women of all nationalities. Thoughtfully curated gatherings inspired by creativity, cultural heritage and sustainability, with dedicated experiences for women.',
  openGraph: { title: 'The Hive Society — The art of getting together', type: 'website', description: 'Thoughtfully curated gatherings for everyone in Abu Dhabi, with dedicated experiences for women.' },
  twitter: { card: 'summary_large_image' },
  applicationName: 'The Hive Society',
  manifest: '/manifest.webmanifest?v=2',
  // ?v= changes the URL so browsers drop the old cached (Lovable) tab icon; bump it with the icon files.
  icons: {
    icon: [{ url: '/favicon.ico?v=2', sizes: '16x16 32x32 48x48 64x64 256x256' }, { url: '/icon-mark.svg?v=2', type: 'image/svg+xml' }],
    apple: '/apple-touch-icon.png?v=2',
  },
};
export const viewport: Viewport = { width: 'device-width', initialScale: 1, themeColor: '#3F6E5E' };
// Pages render per request so each response carries a fresh CSP nonce (see src/proxy.ts).
export default async function RootLayout({children}: {children:ReactNode}) {
  await connection();
  return <html lang="en"><head>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400;1,500&family=Manrope:wght@400;500;600;700&family=Noto+Naskh+Arabic:wght@500;600;700&family=IBM+Plex+Sans+Arabic:wght@400;500;600&display=swap" />
  </head><body><Providers>{children}</Providers></body></html>;
}
