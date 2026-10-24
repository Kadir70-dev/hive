import 'server-only';

/**
 * Server-side forwarding from Next.js (Netlify) to the Go API.
 *
 * Trust boundary: the Go API trusts X-Hive-Client-IP only when the request also
 * carries HIVE_PROXY_SHARED_SECRET, which exists solely in server-side env on
 * Netlify and the VPS. The browser's address comes from Netlify's edge header
 * `x-nf-client-connection-ip`, which Netlify sets itself; any value a client
 * sends in that header is overwritten. Client-supplied forwarding headers are
 * never passed through.
 */
const passThroughResponseHeaders = ['content-type', 'retry-after'];

export function clientIP(req: Request): string | null {
  const ip = req.headers.get('x-nf-client-connection-ip')?.trim();
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
