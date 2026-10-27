import { createHash } from 'node:crypto';

/** Headers sent on every response (pages, API routes, static files). */
export function baseSecurityHeaders(production: boolean): { key: string; value: string }[] {
  const headers = [
    { key: 'X-Content-Type-Options', value: 'nosniff' },
    { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
    { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()' },
    { key: 'Cross-Origin-Opener-Policy', value: 'same-origin' },
  ];
  // HSTS without includeSubDomains/preload so other subdomains (e.g. Titan mail) are unaffected.
  if (production) headers.push({ key: 'Strict-Transport-Security', value: 'max-age=31536000' });
  return headers;
}

const fonts = { style: 'https://fonts.googleapis.com', font: 'https://fonts.gstatic.com' };

/**
 * CSP for Next.js pages. Scripts need the per-request nonce ('strict-dynamic'
 * lets Next's nonce'd loader pull in its own chunks). Inline style attributes
 * are used across the existing UI, so styles keep 'unsafe-inline'.
 */
export function pageCSP(nonce: string, dev: boolean, opts: { httpsImages?: boolean } = {}): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${dev ? " 'unsafe-eval'" : ''}`,
    `style-src 'self' 'unsafe-inline' ${fonts.style}`,
    `font-src 'self' ${fonts.font}`,
    `img-src 'self' data: blob:${opts.httpsImages ? ' https:' : ''}`,
    `connect-src 'self'${dev ? ' ws:' : ''}`,
    "frame-src 'self'",
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
    "manifest-src 'self'",
    ...(dev ? [] : ['upgrade-insecure-requests']),
  ].join('; ');
}

/** SHA-256 CSP sources for every inline <script> (no src attribute) in an HTML document. */
export function inlineScriptHashes(html: string): string[] {
  const out: string[] = [];
  for (const m of html.matchAll(/<script>([\s\S]*?)<\/script>/g)) {
    out.push(`'sha256-${createHash('sha256').update(m[1] ?? '', 'utf8').digest('base64')}'`);
  }
  return out;
}

/** CSP for the static homepage (public/hive.html), which is framed by "/". */
export function staticHomepageCSP(html: string, opts: { httpsImages?: boolean } = {}): string {
  return [
    "default-src 'self'",
    `script-src 'self' ${inlineScriptHashes(html).join(' ')}`,
    `style-src 'self' 'unsafe-inline' ${fonts.style}`,
    `font-src 'self' ${fonts.font}`,
    `img-src 'self' data:${opts.httpsImages ? ' https:' : ''}`,
    "connect-src 'self'",
    "frame-ancestors 'self'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join('; ');
}

/** API responses are JSON only. */
export const apiCSP = "default-src 'none'; frame-ancestors 'none'";
