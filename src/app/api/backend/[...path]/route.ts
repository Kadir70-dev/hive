import { NextRequest, NextResponse } from 'next/server';
import { forwardToApi } from '@/lib/backend-proxy';
export const runtime = 'nodejs';
async function proxy(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  // Only /v1 API paths may be reached; dot segments would let fetch() normalise
  // the URL to other backend routes.
  if (path.length === 0 || path.some((p) => p === '' || p === '.' || p === '..')) {
    return NextResponse.json({ error: 'not found' }, { status: 404 });
  }
  return forwardToApi(req, `/v1/${path.map(encodeURIComponent).join('/')}${req.nextUrl.search}`);
}
export { proxy as GET, proxy as POST, proxy as PATCH, proxy as PUT, proxy as DELETE };
