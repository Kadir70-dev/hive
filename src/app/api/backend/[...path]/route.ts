import { NextRequest } from 'next/server';
import { forwardToApi } from '@/lib/backend-proxy';
export const runtime = 'nodejs';
async function proxy(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  return forwardToApi(req, `/v1/${path.map(encodeURIComponent).join('/')}${req.nextUrl.search}`);
}
export { proxy as GET, proxy as POST, proxy as PATCH, proxy as PUT, proxy as DELETE };
