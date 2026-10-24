import { NextResponse, type NextRequest } from 'next/server';
import { pageCSP } from '@/lib/security-headers';

/**
 * Adds a per-request nonce Content-Security-Policy to every page. Next.js reads
 * the CSP from the request headers and stamps the nonce onto its own scripts.
 */
export function proxy(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString('base64');
  const csp = pageCSP(nonce, process.env.NODE_ENV === 'development');
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set('x-nonce', nonce);
  requestHeaders.set('Content-Security-Policy', csp);
  const response = NextResponse.next({ request: { headers: requestHeaders } });
  response.headers.set('Content-Security-Policy', csp);
  response.headers.set('X-Frame-Options', 'DENY');
  return response;
}

export const config = {
  matcher: [
    {
      // Pages only: API routes, Next assets and files in public/ get static headers from next.config.ts.
      source: '/((?!api|_next/static|_next/image|vendor|hive\\.html|favicon\\.ico|icon-|apple-touch-icon|manifest\\.webmanifest|robots\\.txt|sitemap\\.xml).*)',
      missing: [
        { type: 'header', key: 'next-router-prefetch' },
        { type: 'header', key: 'purpose', value: 'prefetch' },
      ],
    },
  ],
};
