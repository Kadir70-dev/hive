import 'server-only';

/**
 * Same-origin Next.js -> Go API forwarding.
 *
 * Netlify: uses Netlify's overwritten x-nf-client-connection-ip.
 * Hostinger: HIVE_SITE_EDGE=caddy trusts only x-hive-verified-client-ip,
 * which the Hostinger Caddy config overwrites with the TCP peer address.
 * In that mode Next.js MUST remain private (no published port 3000).
 *
 * Only Next.js adds the authenticated proxy secret to Go requests.
 * Never forward browser-provided proxy secret or client IP headers directly.
 */
const passThroughResponseHeaders = ['content-type', 'retry-after'];

export function clientIP(req: Request): string | null {
  const header = process.env.HIVE_SITE_EDGE === 'caddy'
    ? 'x-hive-verified-client-ip'
    : 'x-nf-client-connection-ip';
  const ip = req.headers.get(header)?.trim();
  return ip && /^[0-9a-fA-F:.]{2,45}$/.test(ip) ? ip : null;
}

export async function forwardToApi(req: Request, apiPath: string): Promise<Response> {
  const base = process.env.HIVE_API_INTERNAL_URL;
  if (!base) return Response.json({ error: 'API not configured' }, { status: 503 });
  const headers = new Headers();
  for (const name of ['cookie', 'content-type', 'origin']) {
    const value = req.headers.get(name);
    if (value) headers.set(name, value);
  }
  const secret = process.env.HIVE_PROXY_SHARED_SECRET;
  const ip = clientIP(req);
  if (secret && ip) {
    headers.set('x-hive-proxy-secret', secret);
    headers.set('x-hive-client-ip', ip);
  }
  const hasBody = !['GET', 'HEAD'].includes(req.method);
  let upstream: Response;
  try {
    upstream = await fetch(`${base.replace(/\/$/, '')}${apiPath}`, {
      method: req.method,
      headers,
      body: hasBody ? await req.arrayBuffer() : undefined,
      cache: 'no-store',
      redirect: 'manual',
    });
  } catch {
    return Response.json({ error: 'API unavailable' }, { status: 503 });
  }
  const out = new Headers({ 'cache-control': 'no-store' });
  for (const name of passThroughResponseHeaders) {
    const value = upstream.headers.get(name);
    if (value) out.set(name, value);
  }
  if (!out.has('content-type')) out.set('content-type', 'application/json');
  for (const cookie of upstream.headers.getSetCookie()) out.append('set-cookie', cookie);
  return new Response(upstream.body, { status: upstream.status, headers: out });
}
