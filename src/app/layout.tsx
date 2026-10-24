import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import { Providers } from '@/components/hive/Providers';
import '../styles.css';

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL || 'https://thehivesociety.ae'),
  title: { default: 'The Hive Society', template: '%s · The Hive Society' },
  description: 'The Hive Society — a community gathering in a warm, welcoming space.',
  openGraph: { title: 'The Hive Society', type: 'website', description: 'A warm community to gather and belong.' },
  twitter: { card: 'summary_large_image' },
  applicationName: 'The Hive Society',
  manifest: '/manifest.webmanifest',
  icons: { icon: '/favicon.ico', apple: '/apple-touch-icon.png' },
};
export const viewport: Viewport = { width: 'device-width', initialScale: 1, themeColor: '#3F6E5E' };
export default function RootLayout({children}: {children:ReactNode}) {
  return <html lang="en"><head>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,400;0,500;0,600;1,400;1,500&family=Manrope:wght@400;500;600;700&family=Noto+Naskh+Arabic:wght@500;600;700&family=IBM+Plex+Sans+Arabic:wght@400;500;600&display=swap" />
  </head><body><Providers>{children}</Providers></body></html>;
}
