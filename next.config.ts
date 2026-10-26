import type { NextConfig } from 'next';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { apiCSP, baseSecurityHeaders, staticHomepageCSP } from './src/lib/security-headers';

const production = process.env.NODE_ENV === 'production';
// The homepage is a static file; its CSP pins the exact inline script by hash,
// so any edit to public/hive.html is picked up at build time.
const homepage = readFileSync(path.join(process.cwd(), 'public/hive.html'), 'utf8');

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // Only the Hostinger Docker build emits a self-contained server.
  // The existing Netlify/static build stays unchanged.
  output: process.env.HIVE_STANDALONE === '1' ? 'standalone' : undefined,
  poweredByHeader: false,
  async headers() {
    return [
      { source: '/:path*', headers: baseSecurityHeaders(production) },
      {
        source: '/hive.html',
        headers: [
          { key: 'Content-Security-Policy', value: staticHomepageCSP(homepage) },
          { key: 'X-Frame-Options', value: 'SAMEORIGIN' },
        ],
      },
      { source: '/api/:path*', headers: [{ key: 'Content-Security-Policy', value: apiCSP }, { key: 'X-Frame-Options', value: 'DENY' }] },
    ];
  },
};
export default nextConfig;
