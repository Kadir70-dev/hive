import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { forwardToApi } from '@/lib/backend-proxy';

type Captured = { url: string; init: RequestInit };
let captured: Captured[] = [];

function upstream(status: number, headers: [string, string][] = [], body = '{"ok":true}') {
  return vi.fn(async (url: string, init: RequestInit) => {
    captured.push({ url, init });
    const h = new Headers();
    for (const [k, v] of headers) h.append(k, v);
    return new Response(body, { status, headers: h });
  });
}

function sent(i = 0) {
  return new Headers(captured[i]!.init.headers);
}

beforeEach(() => {
  captured = [];
  vi.stubEnv('HIVE_API_INTERNAL_URL', 'https://api.example.test/');
  vi.stubEnv('HIVE_PROXY_SHARED_SECRET', 'proxy-secret-0123456789abcdef0123456789');
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe('forwardToApi', () => {
  it('forwards the Netlify client IP with the shared secret', async () => {
    vi.stubGlobal('fetch', upstream(200));
    const req = new Request('https://thehivesociety.ae/api/backend/login', {
      method: 'POST',
      headers: { 'x-nf-client-connection-ip': '203.0.113.9', origin: 'https://thehivesociety.ae', cookie: 'hive_session=abc', 'content-type': 'application/json' },
      body: '{}',
    });
    await forwardToApi(req, '/v1/login');
    expect(captured[0]!.url).toBe('https://api.example.test/v1/login');
    const h = sent();
    expect(h.get('x-hive-client-ip')).toBe('203.0.113.9');
    expect(h.get('x-hive-proxy-secret')).toBe('proxy-secret-0123456789abcdef0123456789');
    expect(h.get('cookie')).toBe('hive_session=abc');
    expect(h.get('origin')).toBe('https://thehivesociety.ae');
  });

  it('never passes client-supplied forwarding headers through', async () => {
    vi.stubGlobal('fetch', upstream(200));
    const req = new Request('https://thehivesociety.ae/api/backend/login', {
      method: 'POST',
      headers: { 'x-hive-client-ip': '1.1.1.1', 'x-hive-proxy-secret': 'guess', 'x-forwarded-for': '2.2.2.2', 'x-real-ip': '3.3.3.3' },
      body: '{}',
    });
    await forwardToApi(req, '/v1/login');
    const h = sent();
    expect(h.get('x-hive-client-ip')).toBeNull();
    expect(h.get('x-hive-proxy-secret')).toBeNull();
    expect(h.get('x-forwarded-for')).toBeNull();
    expect(h.get('x-real-ip')).toBeNull();
  });

  it('omits the client IP when no secret is configured', async () => {
    vi.stubEnv('HIVE_PROXY_SHARED_SECRET', '');
    vi.stubGlobal('fetch', upstream(200));
    await forwardToApi(new Request('https://x.test/', { headers: { 'x-nf-client-connection-ip': '203.0.113.9' } }), '/v1/me');
    expect(sent().get('x-hive-client-ip')).toBeNull();
  });

  it('ignores a malformed client IP header', async () => {
    vi.stubGlobal('fetch', upstream(200));
    await forwardToApi(new Request('https://x.test/', { headers: { 'x-nf-client-connection-ip': '<script>' } }), '/v1/me');
    expect(sent().get('x-hive-client-ip')).toBeNull();
  });

  it('passes Retry-After and every Set-Cookie back to the browser', async () => {
    vi.stubGlobal('fetch', upstream(429, [['retry-after', '120'], ['set-cookie', 'a=1; Path=/'], ['set-cookie', 'b=2; Path=/']], '{"error":"too many attempts"}'));
    const res = await forwardToApi(new Request('https://x.test/', { method: 'POST', body: '{}' }), '/v1/login');
    expect(res.status).toBe(429);
    expect(res.headers.get('retry-after')).toBe('120');
    expect(res.headers.getSetCookie()).toEqual(['a=1; Path=/', 'b=2; Path=/']);
    expect(res.headers.get('cache-control')).toBe('no-store');
  });

  it('returns 503 when the API is not configured or unreachable', async () => {
    vi.stubEnv('HIVE_API_INTERNAL_URL', '');
    expect((await forwardToApi(new Request('https://x.test/'), '/v1/me')).status).toBe(503);
    vi.stubEnv('HIVE_API_INTERNAL_URL', 'https://api.example.test');
    vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('down'); }));
    expect((await forwardToApi(new Request('https://x.test/'), '/v1/me')).status).toBe(503);
  });
});
