import { NextRequest, NextResponse } from 'next/server';
export const runtime = 'nodejs';
async function proxy(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const base = process.env.HIVE_API_INTERNAL_URL;
  if (!base) return NextResponse.json({ error: 'API not configured' }, { status: 503 });
  const url = `${base.replace(/\/$/, '')}/v1/${path.map(encodeURIComponent).join('/')}${req.nextUrl.search}`;
  const headers = new Headers();
  const cookie = req.headers.get('cookie');
  if (cookie) headers.set('cookie', cookie);
  const contentType = req.headers.get('content-type');
  if (contentType) headers.set('content-type', contentType);
  const origin = req.headers.get('origin');
  if (origin) headers.set('origin', origin);
  try {
    const resp = await fetch(url, { method: req.method, headers, body: ['GET','HEAD'].includes(req.method) ? undefined : await req.arrayBuffer(), cache: 'no-store', redirect: 'manual' });
    const outgoing = new NextResponse(resp.body, { status: resp.status });
    outgoing.headers.set('Cache-Control', 'no-store');
    const setCookie = resp.headers.get('set-cookie');
    if (setCookie) outgoing.headers.set('set-cookie', setCookie);
    outgoing.headers.set('content-type', resp.headers.get('content-type') ?? 'application/json');
    return outgoing;
  } catch { return NextResponse.json({ error: 'API unavailable' }, { status: 503 }); }
}
export { proxy as GET, proxy as POST, proxy as PATCH, proxy as PUT, proxy as DELETE };
