import { forwardToApi } from '@/lib/backend-proxy';
export const runtime = 'nodejs';
export async function POST(request: Request) {
  return forwardToApi(request, '/v1/founding-signups');
}
