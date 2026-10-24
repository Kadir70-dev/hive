import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { NextRequest } from 'next/server';
import { baseSecurityHeaders, inlineScriptHashes, pageCSP, staticHomepageCSP } from '@/lib/security-headers';
import { proxy } from '@/proxy';
import pkg from '../../package.json';

const homepage = readFileSync(path.join(process.cwd(), 'public/hive.html'), 'utf8');

describe('security headers', () => {
  it('page CSP uses a nonce and never allows inline or eval scripts in production', () => {
    const csp = pageCSP('abc123', false);
    expect(csp).toContain("script-src 'self' 'nonce-abc123' 'strict-dynamic'");
    expect(csp).not.toMatch(/script-src[^;]*'unsafe-(inline|eval)'/);
    for (const d of ["object-src 'none'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'"]) expect(csp).toContain(d);
    expect(pageCSP('x', true)).toContain("'unsafe-eval'"); // dev-only for React refresh
  });

  it('homepage CSP pins exactly its inline script by hash and allows same-origin framing', () => {
    const hashes = inlineScriptHashes(homepage);
    expect(hashes).toHaveLength(1);
    const csp = staticHomepageCSP(homepage);
    expect(csp).toContain(hashes[0]!);
    expect(csp).toContain("frame-ancestors 'self'");
    expect(csp).not.toMatch(/script-src[^;]*'unsafe-inline'/);
  });

  it('HSTS is production-only and does not cover other subdomains', () => {
    expect(baseSecurityHeaders(false).find(h => h.key === 'Strict-Transport-Security')).toBeUndefined();
    const hsts = baseSecurityHeaders(true).find(h => h.key === 'Strict-Transport-Security');
    expect(hsts?.value).toBe('max-age=31536000');
  });

  it('proxy issues a fresh nonce per request and forwards it to Next', () => {
    const a = proxy(new NextRequest('https://thehivesociety.ae/auth'));
    const b = proxy(new NextRequest('https://thehivesociety.ae/auth'));
    const nonceOf = (csp: string | null) => /'nonce-([^']+)'/.exec(csp ?? '')?.[1];
    expect(nonceOf(a.headers.get('content-security-policy'))).toBeTruthy();
    expect(nonceOf(a.headers.get('content-security-policy'))).not.toBe(nonceOf(b.headers.get('content-security-policy')));
    expect(a.headers.get('x-frame-options')).toBe('DENY');
    expect(a.headers.get('x-middleware-request-x-nonce')).toBe(nonceOf(a.headers.get('content-security-policy')));
  });

  it('no Lovable or AI SDK dependencies remain', () => {
    const deps = Object.keys({ ...pkg.dependencies, ...pkg.devDependencies });
    expect(deps.filter(d => /lovable|^ai$|@ai-sdk|react-email/.test(d))).toEqual([]);
  });
});
